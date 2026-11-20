/*
 * Copyright (c) 2022  The AdvantEDGE Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package server

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	apps "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-applications"
	dkm "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-data-key-mgr"
	dataModel "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-data-model"
	mfed "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-federation-client"
	log "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-logger"
	mod "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-model"
	mq "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-mq"
	redis "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-redis"
	uuid "github.com/google/uuid"
	"github.com/gorilla/mux"
)

type AppCtrl struct {
	sandboxName string
	appStore    *apps.ApplicationStore
	mqLocal     *mq.MsgQueue
	handlerId   int
	rc          *redis.Connector
}

// MQ payload fields
const (
	mqFieldAppId       = "id"
	mqFieldPersist     = "persist"
	mqFieldGracePeriod = "gracePeriod"
)

const defaultGracePeriod int = 10

// App Controller
var appCtrl *AppCtrl

// MEC Fed
const MEC_FED_DISCOVERY_TIMEOUT = 15
const MAX_MEC_FED_DISCOVERY_ATTEMPT = 30

var (
	fed             *mfed.APIClient
	fedResources    mfed.SystemInfo
	fed_timer       *time.Ticker
	fed_timer_count int
	fed_list        map[string]bool
)

// Initialize App Controller
func appCtrlInit(sandboxName string, mqLocal *mq.MsgQueue) error {
	var err error

	// Create new App Controller
	appCtrl = new(AppCtrl)
	appCtrl.sandboxName = sandboxName
	appCtrl.mqLocal = mqLocal

	// Connect to Redis DB
	appCtrl.rc, err = redis.NewConnector(redisDBAddr, redisDBTable)
	if err != nil {
		log.Error("Failed connection to Redis DB. Error: ", err)
		return err
	}
	log.Info("Connected to Redis DB")

	// Create Application Store
	cfg := &apps.ApplicationStoreCfg{
		Name:      moduleName,
		Namespace: sandboxName,
		UpdateCb:  appStoreUpdateCb,
		RedisAddr: redisDBAddr,
	}
	appCtrl.appStore, err = apps.NewApplicationStore(cfg)
	if err != nil {
		log.Error("Failed to connect to Application Store. Error: ", err)
		return err
	}
	log.Info("Connected to Application Store")

	fed_list = make(map[string]bool)

	return nil
}

// Start App Controller
func appCtrlRun() error {
	var err error

	// Register Message Queue handler
	handler := mq.MsgHandler{Handler: msgHandler, UserData: nil}
	appCtrl.handlerId, err = appCtrl.mqLocal.RegisterHandler(handler)
	if err != nil {
		log.Error("Failed to listen for sandbox updates: ", err.Error())
		return err
	}
	return nil
}

// Stop App Controller
func appCtrlStop() error {

	// Unregister handler
	if appCtrl.mqLocal != nil {
		appCtrl.mqLocal.UnregisterHandler(appCtrl.handlerId)
	}

	stopMECFederationDiscovery()

	if fed != nil {
		// Send registration
		_, err := fed.SystemInfoApi.SysteminfoByIdDELETE(context.TODO(), fedResources.SystemId)
		if err != nil {
			log.Error(err.Error())
			// Do not raise error, MEC Federation is not mandatory
		} else {
			log.Info("Successfuly de-registered from MEC Federation")
		}
	}

	return nil
}

// Message Queue handler
func msgHandler(msg *mq.Msg, userData interface{}) {
	switch msg.Message {
	case mq.MsgAppRemoveCnf:
		log.Debug("RX MSG: ", mq.PrintMsg(msg))
		appId := msg.Payload[mqFieldAppId]
		removeNodeConfirm(appId)
	case mq.MecFedConfig:
		log.Debug("RX MSG: ", mq.PrintMsg(msg))
		startMECFederationDiscovery(msg.Payload["URL"])
	default:
	}
}

func createAppInstance(proc *dataModel.Process, ctx *mod.NodeContext) (*apps.Application, error) {
	// Determine app type
	appType := apps.TypeUser
	if proc.Type_ == mod.NodeTypeEdgeApp {
		appType = apps.TypeSystem
	}

	// If group name is present, use it as application name
	// Otherwise, use process name
	appName := proc.Name
	if proc.ServiceConfig != nil && proc.ServiceConfig.MeSvcName != "" {
		appName = proc.ServiceConfig.MeSvcName
	}

	// Create & app instance
	app := &apps.Application{
		Id:      proc.Id,
		Name:    appName,
		Node:    ctx.Parents[mod.PhyLoc],
		Type:    appType,
		Persist: false,
	}
	return app, nil
}

func setAppInstance(name string, activeModel *mod.Model) error {
	// Get scenario Process & context
	proc, ctx, err := getScenarioProcess(name, activeModel)
	if err != nil {
		log.Error(err.Error())
		return err
	}

	// Create app instance
	app, err := createAppInstance(proc, ctx)
	if err != nil {
		log.Error(err.Error())
		return err
	}

	// Set app instance
	err = appCtrl.appStore.Set(app, nil)
	if err != nil {
		log.Error(err.Error())
		return err
	}
	return nil
}

func delAppInstance(id string, gracePeriod int) error {
	// Validate ID
	if id == "" {
		return errors.New("Invalid app instance ID")
	}

	// Delete app instance
	err := appCtrl.appStore.Del(id, &gracePeriod)
	if err != nil {
		log.Warn(err.Error())
		return err
	}
	return nil
}

func resetAppInstances(activeModel *mod.Model) error {
	// Flush non-persistent app instances
	appCtrl.appStore.FlushNonPersistent(nil)

	// Get active scenario app list
	scenarioAppList, err := getScenarioAppInstanceList(activeModel)
	if err != nil {
		log.Error(err.Error())
		return err
	}

	// Create app instances for scenario apps
	for _, app := range scenarioAppList {
		err := appCtrl.appStore.Set(app, nil)
		if err != nil {
			log.Error(err.Error())
		}
	}
	return nil
}

func getScenarioAppInstanceList(activeModel *mod.Model) ([]*apps.Application, error) {
	var appList []*apps.Application

	if activeModel != nil {
		// Get all processes
		processes := activeModel.GetProcesses(nil)
		for _, proc := range processes.Processes {
			// Only handle EDGE-APP for duplicates
			if proc.Type_ != mod.NodeTypeEdgeApp {
				continue
			}

			// Get context by ID
			ctx := activeModel.GetNodeContextById(proc.Id)
			if ctx == nil {
				log.Error("Failed to get context for process ID: " + proc.Id)
				continue
			}

			// Create app instance
			app, err := createAppInstance(&proc, ctx)
			if err != nil {
				log.Error(err.Error())
				continue
			}

			// Add app instance to list
			appList = append(appList, app)
		}
	}
	return appList, nil
}

func getScenarioProcess(name string, activeModel *mod.Model) (*dataModel.Process, *mod.NodeContext, error) {
	// Get app node
	node := activeModel.GetNode(name)
	if node == nil {
		return nil, nil, errors.New("Failed to get app node")
	}
	// Get App Process & context
	proc, ok := node.(*dataModel.Process)
	if !ok {
		return nil, nil, errors.New("Failed to cast node as Process")
	}
	ctx := activeModel.GetNodeContext(proc.Name)
	if ctx == nil {
		return nil, nil, errors.New("Missing node context for " + proc.Name)
	}
	return proc, ctx, nil
}

func applicationsPOST(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	log.Info("applicationsPOST")

	// Retrieve & validate Application info from request body
	var appInfo dataModel.ApplicationInfo
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&appInfo)
	if err != nil {
		log.Error(err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	err = validateAppInfo(&appInfo, "")
	if err != nil {
		log.Error(err.Error())
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Obtain a new App Instance ID if none provided
	if appInfo.Id == "" {
		appInstanceId, err := getNewInstanceId()
		if err != nil {
			log.Error(err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		appInfo.Id = appInstanceId
	} else {
		// Make sure App instance does not exist
		if appInstanceExists(appInfo.Id) {
			errStr := "AppInstanceId already exists"
			log.Error(errStr)
			http.Error(w, errStr, http.StatusBadRequest)
			return
		}
	}

	// Create new App instance
	err = appCtrl.appStore.Set(convertApplicationInfoToApp(&appInfo), nil)
	if err != nil {
		log.Error(err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Send response
	jsonResponse := convertApplicationInfoToJson(&appInfo)
	w.WriteHeader(http.StatusCreated)
	fmt.Fprint(w, string(jsonResponse))
}

func applicationsAppInstanceIdPUT(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	log.Info("applicationsAppInstanceIdPUT")
	vars := mux.Vars(r)
	appInstanceId := vars["appInstanceId"]

	// Retrieve & Validate Application info from request body
	var appInfo dataModel.ApplicationInfo
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&appInfo)
	if err != nil {
		log.Error(err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	err = validateAppInfo(&appInfo, appInstanceId)
	if err != nil {
		log.Error(err.Error())
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	// Make sure App instance exists
	if !appInstanceExists(appInfo.Id) {
		errStr := "AppInstanceId does not exist"
		log.Error(errStr)
		http.Error(w, errStr, http.StatusNotFound)
		return
	}

	// Override entry in DB
	err = appCtrl.appStore.Set(convertApplicationInfoToApp(&appInfo), nil)
	if err != nil {
		log.Error(err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Send response
	jsonResponse := convertApplicationInfoToJson(&appInfo)
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(jsonResponse))
}

func applicationsAppInstanceIdGET(w http.ResponseWriter, r *http.Request) {
	log.Info("applicationsAppInstanceIdGET")
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	vars := mux.Vars(r)
	appInstanceId := vars["appInstanceId"]

	// Get App info for requested App instance ID
	app, err := appCtrl.appStore.Get(appInstanceId)
	if err != nil {
		log.Error(err.Error())
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	appInfo := convertAppToApplicationInfo(app)

	// Send response
	jsonResponse := convertApplicationInfoToJson(appInfo)
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(jsonResponse))
}

func applicationsAppInstanceIdDELETE(w http.ResponseWriter, r *http.Request) {
	log.Info("applicationsAppInstanceIdDELETE")
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	vars := mux.Vars(r)
	appId := vars["appInstanceId"]

	// Flush App instance data
	gracePeriod := defaultGracePeriod
	err := appCtrl.appStore.Del(appId, &gracePeriod)
	if err != nil {
		log.Error(err.Error())
	}

	// Send response
	w.WriteHeader(http.StatusNoContent)
}

func applicationsGET(w http.ResponseWriter, r *http.Request) {
	log.Info("applicationsGET")
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	// Validate & retrieve query parameters
	u, _ := url.Parse(r.URL.String())
	q := u.Query()
	validParams := []string{"app", "nodeName", "type"}
	err := validateQueryParams(q, validParams)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	appName := q.Get("app")
	nodeName := q.Get("nodeName")
	appType := q.Get("type")

	// Get application list
	appList, err := appCtrl.appStore.GetAll()
	if err != nil {
		log.Error(err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Prepare AppInfo list
	appInfoList := make([]dataModel.ApplicationInfo, 0)
	for _, app := range appList {
		// Filter using query params
		if (appName != "" && app.Name != appName) ||
			(nodeName != "" && app.Node != nodeName) ||
			(appType != "" && app.Type != appType) {
			continue
		}
		// Append appInfo
		appInfo := convertAppToApplicationInfo(app)
		appInfoList = append(appInfoList, *appInfo)
	}

	// Send response
	jsonResponse, err := json.Marshal(appInfoList)
	if err != nil {
		log.Error(err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(jsonResponse))
}

func servicesGET(w http.ResponseWriter, r *http.Request) {
	log.Info("servicesGET")
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	// Validate & retrieve query parameters
	u, _ := url.Parse(r.URL.String())
	q := u.Query()
	validParams := []string{"appInstanceId"}
	err := validateQueryParams(q, validParams)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	appId := q.Get("appInstanceId")

	// Check if app instance exists
	if appId != "" {
		_, err = appCtrl.appStore.Get(appId)
		if err != nil {
			http.Error(w, "Invalid App instance ID", http.StatusBadRequest)
			return
		}
	}

	// Retrieve service info list if scenario is active
	serviceInfoList := []*dataModel.ServiceInfo{}
	activeModel := getActiveModel()
	if activeModel != nil {
		// Create key match string
		baseKey := dkm.GetKeyRoot(appCtrl.sandboxName) + "app-enablement:mep:*:"
		var keyMatchStr string
		if appId == "" {
			keyMatchStr = baseKey + "app:*:svc:*"
		} else {
			keyMatchStr = baseKey + "app:" + appId + ":svc:*"
		}

		// Get list of services registered to app enablement service
		err = appCtrl.rc.ForEachJSONEntry(keyMatchStr, populateServiceInfoList, &serviceInfoList)
		if err != nil {
			log.Error(err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Send response
	jsonResponse, err := json.Marshal(serviceInfoList)
	if err != nil {
		log.Error(err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(jsonResponse))
}

func populateServiceInfoList(key string, jsonInfo string, sInfoList interface{}) error {
	// Get query params & userlist from user data
	serviceInfoList := sInfoList.(*[]*dataModel.ServiceInfo)
	if serviceInfoList == nil {
		return errors.New("ServiceInfoList not found")
	}

	// Use entry key only to avoid dependency on MEC011 packages
	// Obtain app instance id & service instance id from key
	fields := strings.Split(strings.TrimPrefix(key, dkm.GetKeyRoot(appCtrl.sandboxName)+"app-enablement:mep:"), ":")
	if len(fields) != 5 {
		return nil
	}

	// Add service info to list
	serviceInfo := &dataModel.ServiceInfo{
		AppId: fields[2],
		Id:    fields[4],
	}
	*serviceInfoList = append(*serviceInfoList, serviceInfo)
	return nil
}

// Get a new unique app instance
func getNewInstanceId() (string, error) {
	//allow 3 tries, if not return an error
	maxNbRetries := 3
	for try := maxNbRetries; try > 0; try-- {
		appInstanceId := uuid.New().String()
		if !appInstanceExists(appInstanceId) {
			return appInstanceId, nil
		}
		try--
	}
	return "", errors.New("Can't allocate a unique instance Id")
}

// Validate that App Instance exists
func appInstanceExists(appInstanceId string) bool {
	_, err := appCtrl.appStore.Get(appInstanceId)
	return err == nil
}

// Validate query params
func validateQueryParams(params url.Values, validParams []string) error {
	for param := range params {
		found := false
		for _, validParam := range validParams {
			if param == validParam {
				found = true
				break
			}
		}
		if !found {
			err := errors.New("Invalid query param: " + param)
			log.Error(err.Error())
			return err
		}
	}
	return nil
}

// Validate App Info params
func validateAppInfo(appInfo *dataModel.ApplicationInfo, appInstanceId string) error {
	// Validate content
	if appInstanceId != "" && appInfo.Id != appInstanceId {
		return errors.New("Mandatory Application Instance Id parameter and body content not matching")
	}
	if appInfo.Name == "" {
		return errors.New("Mandatory Name not present")
	}
	if appInfo.NodeName == "" {
		return errors.New("Mandatory Node Name not present")
	}

	// Set default App type if missing
	if appInfo.Type_ == "" {
		appInfo.Type_ = apps.TypeUser
	}
	return nil
}

func convertApplicationInfoToJson(appInfo *dataModel.ApplicationInfo) string {
	jsonInfo, err := json.Marshal(*appInfo)
	if err != nil {
		log.Error(err.Error())
		return ""
	}
	return string(jsonInfo)
}

func convertAppToApplicationInfo(app *apps.Application) *dataModel.ApplicationInfo {
	appInfo := &dataModel.ApplicationInfo{
		Id:       app.Id,
		Name:     app.Name,
		NodeName: app.Node,
		Type_:    app.Type,
		Persist:  app.Persist,
	}
	return appInfo
}

func convertApplicationInfoToApp(appInfo *dataModel.ApplicationInfo) *apps.Application {
	app := &apps.Application{
		Id:      appInfo.Id,
		Name:    appInfo.Name,
		Node:    appInfo.NodeName,
		Type:    appInfo.Type_,
		Persist: appInfo.Persist,
	}
	return app
}

func appStoreUpdateCb(eventType string, eventData interface{}, userData interface{}) {
	var msg *mq.Msg

	// Create message to send on MQ
	switch eventType {
	case apps.EventAdd:
		msg = appCtrl.mqLocal.CreateMsg(mq.MsgAppUpdate, mq.TargetAll, appCtrl.sandboxName)
		msg.Payload[mqFieldAppId] = eventData.(string)
	case apps.EventRemove:
		msg = appCtrl.mqLocal.CreateMsg(mq.MsgAppRemove, mq.TargetAll, appCtrl.sandboxName)
		msg.Payload[mqFieldAppId] = eventData.(string)
		gracePeriodStr := "0"
		if gracePeriod, ok := userData.(*int); ok && gracePeriod != nil {
			gracePeriodStr = strconv.Itoa(*gracePeriod)
		}
		msg.Payload[mqFieldGracePeriod] = gracePeriodStr
	case apps.EventFlush:
		msg = appCtrl.mqLocal.CreateMsg(mq.MsgAppFlush, mq.TargetAll, appCtrl.sandboxName)
		msg.Payload[mqFieldPersist] = strconv.FormatBool(eventData.(bool))
	default:
		return
	}

	// Send message to inform other modules of app store changes
	log.Debug("TX MSG: ", mq.PrintMsg(msg))
	err := appCtrl.mqLocal.SendMsg(msg)
	if err != nil {
		log.Error("Failed to send message. Error: ", err.Error())
		return
	}
}

func startMECFederationDiscovery(hostUrl string) {
	log.Debug(">>> startMECFederationDiscovery: ", hostUrl)

	// Add MEC platform to the map if it not already present
	if _, ok := fed_list[hostUrl]; !ok {
		fed_list[hostUrl] = false
	}
	log.Debug("startMECFederationDiscovery: fed_list=", fed_list)

	// Make sure ticker is not running
	if fed_timer != nil {
		log.Debug("startMECFederationDiscovery: MEC Federation ticker already running")
		return
	}

	// Start registration ticker
	fed_timer = time.NewTicker(MEC_FED_DISCOVERY_TIMEOUT * time.Second)
	go func() {
		for range fed_timer.C {
			if fed_timer_count == MAX_MEC_FED_DISCOVERY_ATTEMPT {
				log.Info("startMECFederationDiscovery: Stop MEC federation discovery timer")
				stopMECFederationDiscovery()
				return
			}

			for k, v := range fed_list {
				if !v { // Not register yet
					log.Debug("startMECFederationDiscovery: Try to connect to MEC federation " + k)
					cfg := mfed.NewConfiguration()
					u, err := url.Parse(k)
					if err != nil {
						log.Error("startMECFederationDiscovery: Failed to parse " + k)
						stopMECFederationDiscovery()
						return
					}
					log.Debug("startMECFederationDiscovery: u:", u)
					s := strings.Split(u.Host, "-") // Extract MEP identifier
					if len(s) == 0 {
						log.Error("startMECFederationDiscovery: Failed to extract MEP identifier")
						stopMECFederationDiscovery()
						return
					}
					log.Debug("startMECFederationDiscovery: s: ", s)
					cfg.BasePath = strings.TrimSuffix(k, "/")
					log.Debug("startMECFederationDiscovery: cfg.BasePath: ", cfg.BasePath)
					tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
					cfg.HTTPClient = &http.Client{Transport: tr}
					log.Debug("startMECFederationDiscovery: cfg.HTTPClient: ", cfg.HTTPClient)
					fed = mfed.NewAPIClient(cfg)
					if fed != nil {
						// Send registration
						var body = mfed.SystemInfo{
							SystemId:       "", // Shall be empty in the request
							SystemName:     appCtrl.sandboxName + "-" + s[0],
							SystemProvider: "ETSI",
						}
						fedResources, response, err := fed.SystemInfoApi.SysteminfoPOST(context.TODO(), body)
						log.Debug("startMECFederationDiscovery: response: ", response)
						if err != nil {
							log.Error(err.Error())
						} else {
							log.Debug("startMECFederationDiscovery: ", fedResources)
							delete(fed_list, k)
							fed_list[k] = true
						}
					} else {
						log.Warn("startMECFederationDiscovery: Failed to connect to MEC federation")
					}
				}
			} // End of 'for' statement
			fed_timer_count += 1
			var stop bool = true
			for _, v := range fed_list {
				if !v {
					stop = false
					break
				}
			} // End of 'for' statement
			//log.Debug("startMECFederationDiscovery: Stop MEC federation discovery timer: ", stop)
			if stop {
				log.Info("startMECFederationDiscovery: Terminate MEC federation discovery timer, all registration done")
				stopMECFederationDiscovery()
				return
			}
		} // End of 'for' statement
	}()
}

func stopMECFederationDiscovery() {
	log.Debug(">>> stopMECFederationDiscovery")
	if fed_timer != nil {
		log.Debug("stopMECFederationDiscovery: Stopping MEC Federation ticker")
		fed_timer.Stop()
		fed_timer = nil
		fed_timer_count = 0
		fed_list = make(map[string]bool)
		log.Debug("stopMECFederationDiscovery: len(fed_list)=", len(fed_list))
	}
}
