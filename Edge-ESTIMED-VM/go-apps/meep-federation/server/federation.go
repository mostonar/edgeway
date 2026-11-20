/*
 * Copyright (c) 2024-2025  The AdvantEDGE Authors
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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	sbi "github.com/InterDigitalInc/AdvantEDGE/go-apps/meep-federation/sbi"
	asc "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-app-support-client"
	dkm "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-data-key-mgr"
	httpLog "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-http-logger"
	log "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-logger"
	met "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-metrics"
	redis "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-redis"
	scc "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-sandbox-ctrl-client"
	smc "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-service-mgmt-client"
	sm "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-subscriptions"

	uuid "github.com/google/uuid"

	"github.com/gorilla/mux"
)

const moduleName = "meep-federation"
const fedBasePath = "fed_enablement/v1/"
const fedKey = "federation"

const serviceName = "Federation Service"
const serviceCategory = "FED"
const defaultLocationName = "global"
const defaultScopeOfLocality = "MEC_SYSTEM"
const defaultConsumedLocalOnly = true
const appTerminationPath = "notifications/mec011/appTermination"

var redisAddr string = "meep-redis-master.default.svc.cluster.local:6379"
var influxAddr string = "http://meep-influxdb.default.svc.cluster.local:8086"
var sbxCtrlUrl string = "http://meep-sandbox-ctrl"

var currentStoreName = ""

var FED_DB = 0

var rc *redis.Connector
var hostUrl *url.URL
var instanceId string
var instanceName string
var sandboxName string
var mepName string = defaultLocationName
var scopeOfLocality string = defaultScopeOfLocality
var consumedLocalOnly bool = defaultConsumedLocalOnly
var locality []string
var fed_broker string
var basePath string
var baseKey string

const serviceAppVersion = "3.3.1"

var serviceAppInstanceId string

var appEnablementUrl string
var appEnablementEnabled bool
var sendAppTerminationWhenDone bool = false
var appTermSubId string
var appEnablementServiceId string
var appSupportClient *asc.APIClient
var svcMgmtClient *smc.APIClient
var sbxCtrlClient *scc.APIClient

var registrationTicker *time.Ticker
var subMgr *sm.SubscriptionMgr = nil

var SystemUpdateNotificationSubscriptionMap = map[int]*SystemUpdateNotificationSubscription{}
var subscriptionExpiryMap = map[int64][]int{}    // List of subscriptions
var systemInfopMap = map[string]sbi.SystemInfo{} // List of discovered MEC Plateform
var localSystemInfo sbi.SystemInfo               // Local registration from MEO

var mutex sync.Mutex
var expiryTicker *time.Ticker
var nextSubscriptionIdAvailable int

var metricStore *met.MetricStore

func getAppInstanceId() (id string, err error) {
	var appInfo scc.ApplicationInfo
	appInfo.Id = instanceId
	appInfo.Name = serviceCategory
	appInfo.Type_ = "SYSTEM"
	appInfo.NodeName = mepName
	if mepName == defaultLocationName {
		appInfo.Persist = true
	} else {
		appInfo.Persist = false
	}
	response, _, err := sbxCtrlClient.ApplicationsApi.ApplicationsPOST(context.TODO(), appInfo)
	if err != nil {
		log.Error("Failed to get App Instance ID with error: ", err)
		return "", err
	}
	return response.Id, nil
}

func deregisterService(appInstanceId string, serviceId string) error {
	_, err := svcMgmtClient.MecServiceMgmtApi.AppServicesServiceIdDELETE(context.TODO(), appInstanceId, serviceId)
	if err != nil {
		log.Error("Failed to unregister the service to app enablement registry: ", err)
		return err
	}
	return nil
}

func registerService(appInstanceId string) error {
	// Build Service Info
	state := smc.ACTIVE_ServiceState
	serializer := smc.JSON_SerializerType
	transportType := smc.REST_HTTP_TransportType
	localityType := smc.LocalityType(scopeOfLocality)
	srvInfo := smc.ServiceInfo{
		SerName:           instanceName,
		Version:           serviceAppVersion,
		State:             &state,
		Serializer:        &serializer,
		ScopeOfLocality:   &localityType,
		ConsumedLocalOnly: consumedLocalOnly,
		TransportInfo: &smc.TransportInfo{
			Id:       "sandboxTransport",
			Name:     "REST",
			Type_:    &transportType,
			Protocol: "HTTP",
			Version:  "2.0",
			Endpoint: &smc.OneOfTransportInfoEndpoint{},
		},
		SerCategory: &smc.CategoryRef{
			Href:    "catalogueHref",
			Id:      "fedId",
			Name:    serviceCategory,
			Version: "v1",
		},
	}
	srvInfo.TransportInfo.Endpoint.Uris = append(srvInfo.TransportInfo.Endpoint.Uris, hostUrl.String()+basePath)

	appServicesPostResponse, _, err := svcMgmtClient.MecServiceMgmtApi.AppServicesPOST(context.TODO(), srvInfo, appInstanceId)
	if err != nil {
		log.Error("Failed to register the service to app enablement registry: ", err)
		return err
	}
	log.Info("Application Enablement Service instance Id: ", appServicesPostResponse.SerInstanceId)
	appEnablementServiceId = appServicesPostResponse.SerInstanceId
	return nil
}

func getMecServices(host string) ([]smc.ServiceInfo, error) {
	log.Info(">>> getMecServices: ", host)

	if svcMgmtClient == nil {
		err := errors.New("App Enablement Service Management REST API client not set")
		log.Error(err.Error())
		return nil, err
	}
	svcs, _, err := svcMgmtClient.MecServiceMgmtApi.ServicesGET(context.TODO(), nil)
	if err != nil {
		log.Error("Failed to retrieve mec services on platform ", err)
		return nil, err
	}
	// Extract the services from the specified host
	var l []smc.ServiceInfo = []smc.ServiceInfo{}
	for _, v := range svcs {
		//log.Debug("GetMecServices: processing: ", v)
		if v.Links != nil && v.Links.Self != nil {
			//log.Debug("GetMecServices: processing: v._links=", *v.Links.Self)
			if strings.Contains(v.Links.Self.Href, host) {
				l = append(l, v)
			}
		}
	} // End of 'for' statement
	//log.Debug("GetMecServices: processing: l=", l)

	return l, nil
}

func sendReadyConfirmation(appInstanceId string) error {
	var appReady asc.AppReadyConfirmation
	appReady.Indication = "READY"
	_, err := appSupportClient.MecAppSupportApi.ApplicationsConfirmReadyPOST(context.TODO(), appReady, appInstanceId)
	if err != nil {
		log.Error("Failed to send a ready confirm acknowlegement: ", err)
		return err
	}
	return nil
}

func sendTerminationConfirmation(appInstanceId string) error {
	var appTermination asc.AppTerminationConfirmation
	operationAction := asc.TERMINATING_OperationActionType
	appTermination.OperationAction = &operationAction
	_, err := appSupportClient.MecAppSupportApi.ApplicationsConfirmTerminationPOST(context.TODO(), appTermination, appInstanceId)
	if err != nil {
		log.Error("Failed to send a confirm termination acknowlegement: ", err)
		return err
	}
	return nil
}

func subscribeAppTermination(appInstanceId string) error {
	var sub asc.AppTerminationNotificationSubscription
	sub.SubscriptionType = "AppTerminationNotificationSubscription"
	sub.AppInstanceId = appInstanceId
	if mepName == defaultLocationName {
		sub.CallbackReference = "http://" + moduleName + "/" + fedBasePath + appTerminationPath
	} else {
		sub.CallbackReference = "http://" + mepName + "-" + moduleName + "/" + fedBasePath + appTerminationPath
	}
	subscription, _, err := appSupportClient.MecAppSupportApi.ApplicationsSubscriptionsPOST(context.TODO(), sub, appInstanceId)
	if err != nil {
		log.Error("Failed to register to App Support subscription: ", err)
		return err
	}
	appTermSubLink := subscription.Links.Self.Href
	appTermSubId = appTermSubLink[strings.LastIndex(appTermSubLink, "/")+1:]
	return nil
}

func unsubscribeAppTermination(appInstanceId string, subId string) error {
	//only subscribe to one subscription, so we force number to be one, couldn't be anything else
	_, err := appSupportClient.MecAppSupportApi.ApplicationsSubscriptionDELETE(context.TODO(), appInstanceId, subId)
	if err != nil {
		log.Error("Failed to unregister to App Support subscription: ", err)
		return err
	}
	return nil
}

func mec011AppTerminationPOST(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	var notification AppTerminationNotification
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&notification)
	if err != nil {
		log.Error(err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if !appEnablementEnabled {
		//just ignore the message
		w.WriteHeader(http.StatusNoContent)
		return
	}

	go func() {
		// Wait to allow app termination response to be sent
		time.Sleep(20 * time.Millisecond)

		// Deregister service
		_ = deregisterService(serviceAppInstanceId, appEnablementServiceId)

		// Delete subscriptions
		_ = unsubscribeAppTermination(serviceAppInstanceId, appTermSubId)

		// Confirm App termination if necessary
		if sendAppTerminationWhenDone {
			_ = sendTerminationConfirmation(serviceAppInstanceId)
		}
	}()

	w.WriteHeader(http.StatusNoContent)
}

// Init - FED Service initialization
func Init() (err error) {

	// Retrieve Instance ID from environment variable if present
	instanceIdEnv := strings.TrimSpace(os.Getenv("MEEP_INSTANCE_ID"))
	if instanceIdEnv != "" {
		instanceId = instanceIdEnv
	}
	log.Info("MEEP_INSTANCE_ID: ", instanceId)

	// Retrieve Instance Name from environment variable
	instanceName = moduleName
	instanceNameEnv := strings.TrimSpace(os.Getenv("MEEP_POD_NAME"))
	if instanceNameEnv != "" {
		instanceName = instanceNameEnv
	}
	log.Info("MEEP_POD_NAME: ", instanceName)

	// Retrieve Sandbox name from environment variable
	sandboxNameEnv := strings.TrimSpace(os.Getenv("MEEP_SANDBOX_NAME"))
	if sandboxNameEnv != "" {
		sandboxName = sandboxNameEnv
	}
	if sandboxName == "" {
		err = errors.New("MEEP_SANDBOX_NAME env variable not set")
		log.Error(err.Error())
		return err
	}
	log.Info("MEEP_SANDBOX_NAME: ", sandboxName)

	// hostUrl is the url of the node serving the resourceURL
	// Retrieve public url address where service is reachable, if not present, use Host URL environment variable
	hostUrl, err = url.Parse(strings.TrimSpace(os.Getenv("MEEP_PUBLIC_URL")))
	if err != nil || hostUrl == nil || hostUrl.String() == "" {
		hostUrl, err = url.Parse(strings.TrimSpace(os.Getenv("MEEP_HOST_URL")))
		if err != nil {
			hostUrl = new(url.URL)
		}
	}
	log.Info("MEEP_HOST_URL: ", hostUrl)

	// Get MEP name
	mepNameEnv := strings.TrimSpace(os.Getenv("MEEP_LOCATION_NAME"))
	if mepNameEnv != "" {
		mepName = mepNameEnv
	}
	log.Info("MEEP_LOCATION_NAME: ", mepName)

	// Get App Enablement URL
	appEnablementEnabled = false
	appEnablementEnv := strings.TrimSpace(os.Getenv("MEEP_APP_ENABLEMENT"))
	if appEnablementEnv != "" {
		appEnablementUrl = "http://" + appEnablementEnv
		appEnablementEnabled = true
	}
	log.Info("MEEP_APP_ENABLEMENT: ", appEnablementUrl)

	// Get scope of locality
	scopeOfLocalityEnv := strings.TrimSpace(os.Getenv("MEEP_SCOPE_OF_LOCALITY"))
	if scopeOfLocalityEnv != "" {
		scopeOfLocality = scopeOfLocalityEnv
	}
	log.Info("MEEP_SCOPE_OF_LOCALITY: ", scopeOfLocality)

	// Get local consumption
	consumedLocalOnlyEnv := strings.TrimSpace(os.Getenv("MEEP_CONSUMED_LOCAL_ONLY"))
	if consumedLocalOnlyEnv != "" {
		value, err := strconv.ParseBool(consumedLocalOnlyEnv)
		if err == nil {
			consumedLocalOnly = value
		}
	}
	log.Info("MEEP_CONSUMED_LOCAL_ONLY: ", consumedLocalOnly)

	// Get locality
	localityEnv := strings.TrimSpace(os.Getenv("MEEP_LOCALITY"))
	if localityEnv != "" {
		locality = strings.Split(localityEnv, ":")
	}
	log.Info("MEEP_LOCALITY: ", locality)

	// Get FED brokers. E.g. mqtt://broker.emqx.io:1883
	fed_broker := strings.TrimSpace(os.Getenv("MEEP_BROKER"))
	log.Info("MEEP_BROKER: ", fed_broker)

	// Get FED topic. E.g. ETSI/MEC/Federation
	fed_topic := strings.TrimSpace(os.Getenv("MEEP_TOPIC"))
	log.Info("MEEP_TOPIC: ", fed_topic)

	// Set base path
	if mepName == defaultLocationName {
		basePath = "/" + sandboxName + "/" + fedBasePath
	} else {
		basePath = "/" + sandboxName + "/" + mepName + "/" + fedBasePath
	}

	// Set base storage key
	baseKey = dkm.GetKeyRoot(sandboxName) + fedKey + ":mep:" + mepName + ":"
	log.Error("FED baseKey: ", baseKey)

	// Connect to Redis DB (FED_DB)
	rc, err = redis.NewConnector(redisAddr, FED_DB)
	if err != nil {
		log.Error("Failed connection to Redis DB (FED_DB). Error: ", err)
		return err
	}
	_ = rc.DBFlush(baseKey)
	log.Info("Connected to Redis DB, FED service table")

	reInit()

	expiryTicker = time.NewTicker(time.Second)
	go func() {
		for range expiryTicker.C {
			checkForExpiredSubscriptions()
		}
	}()

	// Initialize SBI
	sbiCfg := sbi.SbiCfg{
		ModuleName:     moduleName,
		SandboxName:    sandboxName,
		FedBasePath:    fedBasePath,
		HostUrl:        hostUrl.String() + basePath,
		RedisAddr:      redisAddr,
		Locality:       locality,
		ScenarioNameCb: updateStoreName,
		FedBroker:      fed_broker,
		FedTopic:       fed_topic,
		FedNotify:      fedNotify,
		CleanUpCb:      cleanUp,
	}
	if mepName != defaultLocationName {
		sbiCfg.LocationName = mepName
	}
	err = sbi.Init(sbiCfg)
	if err != nil {
		log.Error("Failed initialize SBI. Error: ", err)
		return err
	}
	log.Info("SBI Initialized")

	// Create App Enablement REST clients
	if appEnablementEnabled {
		// Create Sandbox Controller client
		sbxCtrlClientCfg := scc.NewConfiguration()
		sbxCtrlClientCfg.BasePath = sbxCtrlUrl + "/sandbox-ctrl/v1"
		sbxCtrlClient = scc.NewAPIClient(sbxCtrlClientCfg)
		if sbxCtrlClient == nil {
			return errors.New("Failed to create Sandbox Controller REST API client")
		}
		log.Info("Create Sandbox Controller REST API client")

		// Create App Support client
		appSupportClientCfg := asc.NewConfiguration()
		appSupportClientCfg.BasePath = appEnablementUrl + "/mec_app_support/v2"
		appSupportClient = asc.NewAPIClient(appSupportClientCfg)
		if appSupportClient == nil {
			return errors.New("Failed to create App Enablement App Support REST API client")
		}
		log.Info("Create App Enablement App Support REST API client")

		// Create App Info client
		srvMgmtClientCfg := smc.NewConfiguration()
		srvMgmtClientCfg.BasePath = appEnablementUrl + "/mec_service_mgmt/v1"
		svcMgmtClient = smc.NewAPIClient(srvMgmtClientCfg)
		if svcMgmtClient == nil {
			return errors.New("Failed to create App Enablement Service Management REST API client")
		}
		log.Info("Create App Enablement Service Management REST API client")
	}

	log.Info("FED successfully initialized")
	return nil
}

// reInit - finds the value already in the DB to repopulate local stored info
func reInit() {
	log.Debug(">>> reInit")

	//next available subsId will be overrriden if subscriptions already existed
	nextSubscriptionIdAvailable = 1

	keyName := baseKey + "subscriptions:" + "*"
	log.Info("reInit: keyName: ", keyName)
	err := rc.ForEachJSONEntry(keyName, repopulateSubscriptionMap, nil)
	if err != nil {
		log.Error(err.Error())
	}
}

// Run - Start FED
func Run() (err error) {
	// Start MEC Service registration ticker
	if appEnablementEnabled {
		startRegistrationTicker()
	}
	return sbi.Run()
}

// Stop - Stop FED
func Stop() (err error) {
	// Stop MEC Service registration ticker
	if appEnablementEnabled {
		stopRegistrationTicker()
	}
	return sbi.Stop()
}

func startRegistrationTicker() {
	// Make sure ticker is not running
	if registrationTicker != nil {
		log.Warn("Registration ticker already running")
		return
	}

	// Wait a few seconds to allow App Enablement Service to start.
	// This is done to avoid the default 20 second TCP socket connect timeout
	// if the App Enablement Service is not yet running.
	log.Info("Waiting for App Enablement Service to start")
	time.Sleep(5 * time.Second)

	// Start registration ticker
	registrationTicker = time.NewTicker(5 * time.Second)
	go func() {
		mecAppReadySent := false
		registrationSent := false
		subscriptionSent := false
		for range registrationTicker.C {
			// Get Application instance ID
			if serviceAppInstanceId == "" {
				// If a sandbox service, request an app instance ID from Sandbox Controller
				// Otherwise use the scenario-profedioned instance ID
				if mepName == defaultLocationName {
					var err error
					serviceAppInstanceId, err = getAppInstanceId()
					if err != nil || serviceAppInstanceId == "" {
						continue
					}
				} else {
					serviceAppInstanceId = instanceId
				}
			}

			// Send App Ready message
			if !mecAppReadySent {
				err := sendReadyConfirmation(serviceAppInstanceId)
				if err != nil {
					log.Error("Failure when sending the MecAppReady message. Error: ", err)
					continue
				}
				mecAppReadySent = true
			}

			// Register service instance
			if !registrationSent {
				err := registerService(serviceAppInstanceId)
				if err != nil {
					log.Error("Failed to register to appEnablement DB, keep trying. Error: ", err)
					continue
				}
				registrationSent = true
			}

			// Register for graceful termination
			if !subscriptionSent {
				err := subscribeAppTermination(serviceAppInstanceId)
				if err != nil {
					log.Error("Failed to subscribe to graceful termination. Error: ", err)
					continue
				}
				sendAppTerminationWhenDone = true
				subscriptionSent = true
			}

			if mecAppReadySent && registrationSent && subscriptionSent {

				// Registration complete
				log.Info("Successfully registered with App Enablement Service")
				stopRegistrationTicker()
				return
			}
		}
	}()
}

func stopRegistrationTicker() {
	if registrationTicker != nil {
		log.Info("Stopping App Enablement registration ticker")
		registrationTicker.Stop()
		registrationTicker = nil
	}
}

func cleanUp() {
	log.Info("Terminate all")

	// Flush subscriptions
	if subMgr != nil {
		_ = subMgr.DeleteAllSubscriptions()
	}

	// Flush all service data
	rc.DBFlush(baseKey)

	subscriptionExpiryMap = map[int64][]int{}
	SystemUpdateNotificationSubscriptionMap = map[int]*SystemUpdateNotificationSubscription{}

	// Reset metrics store name
	updateStoreName("")
}

func updateStoreName(storeName string) {
	log.Debug(">>> updateStoreName: ", storeName)

	if currentStoreName != storeName {
		currentStoreName = storeName

		logComponent := moduleName
		if mepName != defaultLocationName {
			logComponent = moduleName + "-" + mepName
		}
		err := httpLog.ReInit(logComponent, sandboxName, storeName, redisAddr, influxAddr)
		if err != nil {
			log.Error("Failed to initialise httpLog: ", err)
			return
		}
	}

	// Connect to Metric Store
	var err error
	metricStore, err = met.NewMetricStore(storeName, sandboxName, influxAddr, redisAddr)
	if err != nil {
		log.Error("Failed connection to metric-store: ", err)
		return
	}

}

/*
 * errHandlerProblemDetails sends an error message
 * @param {struct} HTTP write reference
 * @param {string} error contains the error message
 * @param {int} code contains the error code
 */
func errHandlerProblemDetails(w http.ResponseWriter, error string, code int) {
	var pb ProblemDetails
	pb.Detail = error
	pb.Status = int32(code)

	jsonResponse := convertProblemDetailstoJson(&pb)

	w.WriteHeader(code)
	fmt.Fprint(w, jsonResponse)
}

// FIXME To be rmoved if not used at the end
func repopulateSubscriptionMap(key string, jsonInfo string, userData interface{}) error {
	log.Debug(">>> repopulateSubscriptionMap: key: ", key)
	log.Debug(">>> repopulateSubscriptionMap: jsonInfo: ", jsonInfo)

	// 	var systemUpdateNotificationSubscription SystemUpdateNotificationSubscriptionMap

	// 	// Format response
	// 	err := json.Unmarshal([]byte(jsonInfo), &systemUpdateNotificationSubscription)
	// 	if err != nil {
	// 		return err
	// 	}

	// 	selfUrl := strings.Split(systemUpdateNotificationSubscription.Links.Self.Href, "/")
	// 	subsIdStr := selfUrl[len(selfUrl)-1]
	// 	subsId, _ := strconv.Atoi(subsIdStr)

	// 	mutex.Lock()
	// 	defer mutex.Unlock()

	// 	fedMsgSubscriptionMap[subsId] = &systemUpdateNotificationSubscription
	// 	if systemUpdateNotificationSubscription.ExpiryDeadline != nil {
	// 		intList := subscriptionExpiryMap[int(systemUpdateNotificationSubscription.ExpiryDeadline.Seconds)]
	// 		intList = append(intList, subsId)
	// 		subscriptionExpiryMap[int(systemUpdateNotificationSubscription.ExpiryDeadline.Seconds)] = intList
	// 	}

	// 	//reinitialisation of next available Id for future subscription request
	// 	if subsId >= nextSubscriptionIdAvailable {
	// 		nextSubscriptionIdAvailable = subsId + 1
	// 	}

	return nil
}

func systeminfoByIdDELETE(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> systeminfoByIdDELETE: ", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	vars := mux.Vars(r)
	log.Debug("systeminfoByIdDELETE: vars: ", vars)

	systemIdParamStr := vars["systemId"]
	log.Debug("systeminfoByIdDELETE: systemIdParamStr: ", systemIdParamStr)

	if _, ok := systemInfopMap[systemIdParamStr]; !ok {
		err := errors.New("Unknown systemId: " + systemIdParamStr)
		errHandlerProblemDetails(w, err.Error(), http.StatusNotFound)
		return
	}

	err := sbi.DeregisterSystemInfo(systemIdParamStr)
	if err != nil {
		log.Error("Failed to de-register SystemInfo: ", err)
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}
	log.Info("De-registration succeed")

	delete(systemInfopMap, systemIdParamStr)

	w.WriteHeader(http.StatusNoContent)
}

func systeminfoByIdGET(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> systeminfoByIdGET: ", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	vars := mux.Vars(r)
	log.Debug("systeminfoByIdGET: vars: ", vars)

	systemIdParamStr := vars["systemId"]
	log.Debug("systeminfoByIdGET: systemIdParamStr: ", systemIdParamStr)

	val, ok := systemInfopMap[systemIdParamStr]
	if !ok {
		err := errors.New("Unknown SystemId: " + systemIdParamStr)
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	} else {
		var l = SystemInfo{
			SystemId:       val.SystemId,
			SystemName:     val.SystemName,
			SystemProvider: val.SystemProvider,
		}
		jsonResponse, err := json.Marshal(l)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(w, string(jsonResponse))
		w.WriteHeader(http.StatusOK)
	}
}

func systeminfoByIdPATCH(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> systeminfoByIdPATCH: ", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	vars := mux.Vars(r)
	log.Debug("systeminfoByIdPATCH: vars: ", vars)

	if _, ok := vars["systemId"]; !ok {
		err := errors.New("Wrong parameters")
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	systemIdParamStr := vars["systemId"]
	log.Debug("systeminfoByIdPATCH: systemIdParamStr: ", systemIdParamStr)

	val, ok := systemInfopMap[systemIdParamStr]
	if !ok {
		err := errors.New("Unknown SystemId: " + systemIdParamStr)
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	} else {
		var requestData SystemInfoUpdate
		decoder := json.NewDecoder(r.Body)
		err := decoder.Decode(&requestData)
		if err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
			return
		}
		log.Debug("systeminfoByIdPATCH: requestData: ", requestData)

		// Validating mandatory parameters in request
		if requestData.SystemName == "" && requestData.Endpoint == nil {
			log.Error("At least one attribute shall be set")
			errHandlerProblemDetails(w, "At least one attribute shall be set.", http.StatusBadRequest)
			return
		}

		val.SystemName = requestData.SystemName

		if requestData.Endpoint != nil {
			v := sbi.SystemInfoUpdate{
				SystemId:       val.SystemId,
				SystemName:     val.SystemName,
				SystemProvider: val.SystemProvider,
				Endpoint: sbi.EndPointInfo{
					Uris:        requestData.Endpoint.Uris,
					Fqdn:        requestData.Endpoint.Fqdn,
					Alternative: requestData.Endpoint.Alternative,
				},
			}
			if len(requestData.Endpoint.Addresses) != 0 {
				v.Endpoint.Addresses = make([]sbi.Addresses, len(requestData.Endpoint.Addresses))
				for k, d := range requestData.Endpoint.Addresses {
					v.Endpoint.Addresses[k] = sbi.Addresses{Host: d.Host, Port: d.Port}
				} // End of 'for' statement
			}
			err = sbi.UpdateRegisteredSystemInfo(v)
			if err != nil {
				log.Error(err.Error())
				errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		val.SystemName = requestData.SystemName
		systemInfopMap[systemIdParamStr] = val
	}

	// Prepare & send response
	var l = SystemInfo{
		SystemId:       val.SystemId,
		SystemName:     val.SystemName,
		SystemProvider: val.SystemProvider,
	}
	jsonResponse := convertSystemInfotoJson(&l)
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, jsonResponse)
}

func systeminfoGET(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> systeminfoGET: ", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	mutex.Lock()
	defer mutex.Unlock()

	// Validate query parameters
	u, _ := url.Parse(r.URL.String())
	q := u.Query()
	log.Debug("systeminfoGET: q: ", q)
	// If no query parameters are provided, return all systemInfo
	if len(q) == 0 {
		log.Debug("systeminfoGET: No query parameters provided, returning all systemInfo")

		if len(systemInfopMap) == 0 {
			errHandlerProblemDetails(w, "No systemInfo registered", http.StatusNotFound)
			return
		}

		var systemInfos = []SystemInfo{}
		for _, val := range systemInfopMap {
			systemInfos = append(systemInfos, SystemInfo{
				SystemId:       val.SystemId,
				SystemName:     val.SystemName,
				SystemProvider: val.SystemProvider,
			})
		}

		jsonResponse, err := json.Marshal(systemInfos)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, string(jsonResponse))
		return
	}
	// Validate query parameters
	validParams := []string{"systemId", "systemName", "systemProvider"}
	err := validateQueryParams(q, validParams)
	if err != nil {
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}
	// Check if systemInfopMap is empty
	if len(systemInfopMap) == 0 {
		errHandlerProblemDetails(w, "No systemInfo registered", http.StatusNotFound)
		return
	}
	// Extract query parameter values
	systemId := q["systemId"]
	systemName := q["systemName"]
	systemProvider := q["systemProvider"]
	filter := len(systemId) != 0 || len(systemName) != 0 || len(systemProvider) != 0
	log.Debug("systeminfoGET: systemId: ", systemId)
	log.Debug("systeminfoGET: systemName: ", systemName)
	log.Debug("systeminfoGET: systemProvider: ", systemProvider)
	log.Debug("systeminfoGET: filter: ", filter)

	log.Debug("systeminfoGET: systemInfopMap: ", systemInfopMap)
	if len(systemInfopMap) == 0 {
		err = errors.New("No systemInfo registered")
		errHandlerProblemDetails(w, err.Error(), http.StatusNotFound)
		return
	} else {
		l := []sbi.SystemInfo{}
		for _, val := range systemInfopMap {
			log.Debug("systeminfoGET: processing ", val)
			if !filter {
				l = append(l, val)
			} else {
				if len(systemId) != 0 {
					for _, v := range systemId {
						if v == val.SystemId {
							log.Debug("systeminfoGET: Apply filter on systemId")
							l = append(l, val)
							break
						}
					}
				}
				if len(systemName) != 0 {
					for _, v := range systemName {
						if v == val.SystemName {
							log.Debug("systeminfoGET: Apply filter on systemName")
							l = append(l, val)
							break
						}
					}
				}
				if len(systemProvider) != 0 {
					for _, v := range systemProvider {
						if v == val.SystemProvider {
							log.Debug("systeminfoGET: Apply filter on systemProvider")
							l = append(l, val)
							break
						}
					}
				}
			}
		}
		if len(l) == 0 {
			err = errors.New("No systemInfo registered with applied filter")
			errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
			return
		}
		// Convert to response format
		var systemInfos = []SystemInfo{}
		for _, val := range l {
			systemInfos = append(systemInfos, SystemInfo{
				SystemId:       val.SystemId,
				SystemName:     val.SystemName,
				SystemProvider: val.SystemProvider,
			})
		}
		// Marshal and send response
		jsonResponse, err := json.Marshal(systemInfos)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, string(jsonResponse))
	}
}

func systeminfoPOST(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> systeminfoPOST: ", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	var requestData SystemInfo
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&requestData)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}
	log.Debug("systeminfoPOST: requestData: ", requestData)

	// Validating mandatory parameters in request
	if requestData.SystemId != "" {
		log.Error("Mandatory systemId parameter shall be absent")
		errHandlerProblemDetails(w, "Mandatory attribute systemId shall be absent in the request body.", http.StatusBadRequest)
		return
	}

	if requestData.SystemName == "" {
		log.Error("Mandatory systemName parameter not present")
		errHandlerProblemDetails(w, "Mandatory attribute systemName is missing in the request body.", http.StatusBadRequest)
		return
	}

	if requestData.SystemProvider == "" {
		log.Error("Mandatory systemProvider parameter not present")
		errHandlerProblemDetails(w, "Mandatory attribute systemProvider is missing in the request body.", http.StatusBadRequest)
		return
	}

	var s = sbi.SystemInfo{
		SystemId:       uuid.New().String(),
		SystemName:     requestData.SystemName,
		SystemProvider: requestData.SystemProvider,
		TransportTypes: "HTTP",
		Protocol:       "REST_HTTP",
		SerializerType: "JSON",
		Host:           hostUrl.String() + "/" + sandboxName + "/" + mepName,
	}
	responseData, err := sbi.RegisterSystemInfo(s)
	if err != nil {
		log.Error("Failed to register SystemInfo: ", err)
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Populate systemInfopMap
	systemInfopMap[responseData.SystemId] = responseData
	log.Debug("systeminfoPOST: new systemInfopMap: ", systemInfopMap)

	// Populate localSystemInfo
	localSystemInfo = responseData
	log.Debug("systeminfoPOST: new localSystemInfo: ", localSystemInfo)

	// Prepare & send response
	var l = SystemInfo{
		SystemId:       responseData.SystemId,
		SystemName:     responseData.SystemName,
		SystemProvider: responseData.SystemProvider,
	}
	jsonResponse := convertSystemInfotoJson(&l)
	w.WriteHeader(http.StatusCreated)
	fmt.Fprint(w, jsonResponse)
}

func subscriptionDELETE(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> subscriptionDELETE: ", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	// Retrieve variable parameters
	vars := mux.Vars(r)
	log.Info("vars: ", vars)
	subscriptionId := vars["subscriptionId"]
	if subscriptionId == "" {
		err := errors.New("Wrong parameters")
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}
	log.Info("subscriptionId: ", subscriptionId)

	err := delSubscription(subscriptionId)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Prepare & send response
	w.WriteHeader(http.StatusNoContent)
}

func subscriptionGET(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> subscriptionGET: ", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	// Retrieve variable parameters
	vars := mux.Vars(r)
	log.Info("vars: ", vars)
	subscriptionId := vars["subscriptionId"]
	if subscriptionId == "" {
		err := errors.New("Wrong parameters")
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}
	log.Info("subscriptionId: ", subscriptionId)

	keyName := baseKey + "subscriptions:" + subscriptionId
	log.Info("subscriptionGET: keyName: ", keyName)
	jsonRespDB, err := rc.JSONGetEntry(keyName, ".")
	if err != nil { // subscriptionId does not exist
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusNotFound)
		return
	}

	if jsonRespDB == "" {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var subscription SystemUpdateNotificationSubscription
	err = json.Unmarshal([]byte(jsonRespDB), &subscription)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fmt.Fprint(w, string(jsonRespDB))
	w.WriteHeader(http.StatusOK)
}

func subscriptionPOST(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> subscriptionPOST")

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	// Read JSON input stream provided in the Request, and stores it in the bodyBytes as bytes
	var subscription SystemUpdateNotificationSubscription
	bodyBytes, _ := ioutil.ReadAll(r.Body)
	log.Info("subscriptionsPost: bodyBytes: ", string(bodyBytes))
	// Unmarshal function to converts a JSON-formatted string into a SubscriptionCommon struct and store it in extractSubType
	err := json.Unmarshal(bodyBytes, &subscription)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Info("subscriptionsPost: subscription: ", subscription)

	// Validating mandatory parameters provided in the request body
	if subscription.SubscriptionType == "" {
		log.Error("Mandatory SubscriptionType parameter should be present")
		errHandlerProblemDetails(w, "Mandatory attribute SubscriptionType is missing in the request body.", http.StatusBadRequest)
		return
	}

	if subscription.SubscriptionType != "SystemUpdateNotificationSubscription" {
		log.Error("Invalid SubscriptionType")
		errHandlerProblemDetails(w, "Invalid SubscriptionType", http.StatusBadRequest)
		return
	}

	if subscription.CallbackReference == "" {
		log.Error("CallbackReference shall be present.")
		errHandlerProblemDetails(w, "CallbackReference shall be present.", http.StatusBadRequest)
		return
	}

	// Validating mandatory parameters provided in the request body
	if subscription.Links != nil {
		log.Error("Links attribute should not be present in request body")
		errHandlerProblemDetails(w, "Links attribute should not be present in request body", http.StatusBadRequest)
		return
	}

	// subscriptionId will be generated sequentially
	newSubsId := nextSubscriptionIdAvailable
	nextSubscriptionIdAvailable++
	subsIdStr := strconv.Itoa(newSubsId)

	// create a unique link for every subscription and concatenate subscription to it
	link := new(Links)
	self := new(LinkType)
	self.Href = hostUrl.String() + basePath + "subscriptions/" + subsIdStr
	link.Self = self
	subscription.Links = link

	registerSubscription(subsIdStr, &subscription)
	w.Header().Set("Location", subscription.Links.Self.Href)

	// Prepare response
	jsonResponse := convertSystemUpdateNotificationSubscriptionToJson(&subscription)
	if jsonResponse == "" {
		log.Error("Marshalling failure")
		errHandlerProblemDetails(w, "Marshalling failure", http.StatusInternalServerError)
		return
	}

	// Store subscription key in redis
	keyName := baseKey + "subscriptions:" + subsIdStr
	log.Info("subscriptionPOST: keyName: ", keyName)
	log.Info("subscriptionPOST: subscription: ", subscription)
	err = rc.JSONSetEntry(keyName, ".", jsonResponse)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Info("subscriptionPOST: jsonResponse: ", jsonResponse)

	// Send response
	w.WriteHeader(http.StatusCreated)
	fmt.Fprint(w, jsonResponse)
}

func subscriptionPUT(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> subscriptionPUT: ", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	// Retrieve variable parameters
	vars := mux.Vars(r)
	log.Info("vars: ", vars)
	subscriptionId := vars["subscriptionId"]
	if subscriptionId == "" {
		err := errors.New("Wrong parameters")
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}
	log.Info("subscriptionId: ", subscriptionId)

	// Read JSON input stream provided in the Request
	var subscription SystemUpdateNotificationSubscription
	bodyBytes, _ := ioutil.ReadAll(r.Body)
	log.Info("subscriptionsPut: bodyBytes: ", string(bodyBytes))

	err := json.Unmarshal(bodyBytes, &subscription)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Info("subscriptionsPut: subscription: ", subscription)

	// Validating mandatory parameters
	if subscription.SubscriptionType == "" {
		log.Error("Mandatory SubscriptionType parameter should be present")
		errHandlerProblemDetails(w, "Mandatory attribute SubscriptionType is missing in the request body.", http.StatusBadRequest)
		return
	}

	if subscription.SubscriptionType != "SystemUpdateNotificationSubscription" {
		log.Error("Invalid SubscriptionType")
		errHandlerProblemDetails(w, "Invalid SubscriptionType", http.StatusBadRequest)
		return
	}

	if subscription.CallbackReference == "" {
		log.Error("CallbackReference shall be present.")
		errHandlerProblemDetails(w, "CallbackReference shall be present.", http.StatusBadRequest)
		return
	}

	if subscription.Links != nil {
		log.Error("Links attribute should not be present in request body")
		errHandlerProblemDetails(w, "Links attribute should not be present in request body", http.StatusBadRequest)
		return
	}

	// Create a self-link for the subscription
	link := new(Links)
	self := new(LinkType)
	self.Href = hostUrl.String() + basePath + "subscriptions/" + subscriptionId
	link.Self = self
	subscription.Links = link

	// Set response header Location
	w.Header().Set("Location", subscription.Links.Self.Href)

	// Convert subscription to JSON
	jsonResponse := convertSystemUpdateNotificationSubscriptionToJson(&subscription)
	if jsonResponse == "" {
		log.Error("Marshalling failure")
		errHandlerProblemDetails(w, "Marshalling failure", http.StatusInternalServerError)
		return
	}

	// update in Redis
	keyName := baseKey + "subscriptions:" + subscriptionId
	log.Info("subscriptionPUT: keyName: ", keyName)
	log.Info("subscriptionPUT: subscription: ", subscription)

	err = rc.JSONSetEntry(keyName, ".", jsonResponse)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Info("subscriptionPUT: jsonResponse: ", jsonResponse)

	// Send response
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, jsonResponse)
}

func subscriptionsGET(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> subscriptionsGET: ", r)
	log.Debug(">>> subscriptionsGET: SystemUpdateNotificationSubscriptionMap: ", SystemUpdateNotificationSubscriptionMap)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	u, _ := url.Parse(r.URL.String())
	log.Info("url: ", u.RequestURI())
	q := u.Query()
	subscriptionType := q.Get("subscriptionType")
	systemId := q.Get("systemId")

	noFilter := true
	if subscriptionType != "" || systemId != "" {
		noFilter = false
	}

	subscriptionLinkList := new(SubscriptionLinkList)
	link := new(Links)
	self := new(LinkType)
	self.Href = hostUrl.String() + basePath + "subscriptions"
	link.Self = self
	subscriptionLinkList.Links = link
	for _, v := range SystemUpdateNotificationSubscriptionMap {
		if noFilter {
			subscriptionLinkList.Subscription = append(subscriptionLinkList.Subscription, SubscriptionLinkListSubscription{Href: v.Links.Self.Href, SubscriptionType: v.SubscriptionType})
		} else {
			if subscriptionType == "system_update_notification_subscription" {
				subscriptionLinkList.Subscription = append(subscriptionLinkList.Subscription, SubscriptionLinkListSubscription{Href: v.Links.Self.Href, SubscriptionType: v.SubscriptionType})
			}
			if systemId != "" {
				for _, s := range v.SystemId {
					if s == systemId {
						subscriptionLinkList.Subscription = append(subscriptionLinkList.Subscription, SubscriptionLinkListSubscription{Href: v.Links.Self.Href, SubscriptionType: v.SubscriptionType})
						break
					}
				}
			}
		}
	}
	log.Debug("subscriptionsGET: subscriptionLinkList: ", subscriptionLinkList)

	// Prepare and send the response
	jsonResponse, err := json.Marshal(subscriptionLinkList)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Debug("subscriptionsGET: jsonResponse: ", jsonResponse)
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(jsonResponse))
}

func serviceGET(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> serviceGET: ", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	mutex.Lock()
	defer mutex.Unlock()

	vars := mux.Vars(r)
	log.Debug("serviceGET: vars: ", vars)

	systemIdParamStr := vars["systemId"]
	log.Debug("serviceGET: systemIdParamStr: ", systemIdParamStr)
	serviceIdParamStr := vars["serviceId"]
	log.Debug("serviceGET: serviceIdParamStr: ", serviceIdParamStr)

	val, ok := systemInfopMap[systemIdParamStr]
	var services []smc.ServiceInfo
	var err error
	if len(systemInfopMap) == 0 || !ok {
		err := errors.New("servicesGET: Unknown system ID: " + val.SystemId)
		log.Debug(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusNotFound)
		return
	} else if localSystemInfo.SystemId == systemIdParamStr { // Local request
		log.Debug("servicesGET: Local request: ", systemIdParamStr)
		services, err = getMecServices(localSystemInfo.Host)
		if err != nil {
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else { // Forward to peer MEF
		services, err = sbi.GetMecServices(val.Host)
		if err != nil {
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	log.Debug("servicesGET: ", services)

	// Prepare & send response
	var fedServiceInfo FedServiceInfo
	for _, v := range services {
		if serviceIdParamStr == v.SerName {
			fedServiceInfo = FedServiceInfo{
				SystemId: val.SystemId,
				MecHostInformation: &MecHostInformation{
					HostName: val.Host,
					HostId:   "",
				},
				ServiceInfo: &ServiceInfo{
					SerName: v.SerName,
					Version: v.Version,
				},
			}
			fedServiceInfo.ServiceInfo = &ServiceInfo{
				SerName: v.SerName,
				Version: v.Version,
			}
			if v.SerCategory != nil {
				fedServiceInfo.ServiceInfo.SerCategory = &CategoryRef{
					Href:    v.SerCategory.Href,
					Id:      v.SerCategory.Id,
					Name:    v.SerCategory.Name,
					Version: v.SerCategory.Version,
				}
			}
			if v.TransportInfo != nil {
				fedServiceInfo.ServiceInfo.TransportsSupported = []TransportsSupported{}
				var ts = TransportsSupported{}
				// Transport field
				ts.Transport = &TransportDescriptor{
					Name:        v.TransportInfo.Name,
					Description: v.TransportInfo.Description,
					Protocol:    v.TransportInfo.Protocol,
					//Security *SecurityInfo
					//Type_ *TransportTypes `json:"type"`
					Version:          v.TransportInfo.Version,
					ImplSpecificInfo: v.TransportInfo.ImplSpecificInfo,
				}
				if v.TransportInfo.Security != nil {
					ts.Transport.Security = &SecurityInfo{
						// 		OAuth2Info: v.TransportInfo.Security.OAuth2Info,
					}
				}
				if v.TransportInfo.Type_ != nil && *v.TransportInfo.Type_ == "REST_HTTP" {
					ts.Transport.Type_ = new(TransportTypes)
					*ts.Transport.Type_ = REST_HTTP
				}

				// // Serializers field
				ts.Serializers = []SerializerType{}
				ts.Serializers = append(ts.Serializers, JSON)

				// Add it to the list
				fedServiceInfo.ServiceInfo.TransportsSupported = append(fedServiceInfo.ServiceInfo.TransportsSupported, ts)
			}
		}
	} // End of 'for' statement

	jsonResponse, err := json.Marshal(fedServiceInfo)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprint(w, string(jsonResponse))
	w.WriteHeader(http.StatusOK)
}

func servicesGET(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> servicesGET: ", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	vars := mux.Vars(r)
	log.Debug("servicesGET: vars: ", vars)

	systemIdParamStr := vars["systemId"]
	log.Debug("servicesGET: systemIdParamStr: ", systemIdParamStr)

	log.Debug("servicesGET: systemInfopMap: ", systemInfopMap)
	log.Debug("servicesGET: localSystemInfo: ", localSystemInfo)

	val, ok := systemInfopMap[systemIdParamStr]
	var services []smc.ServiceInfo
	var err error
	if len(systemInfopMap) == 0 || !ok {
		err := errors.New("servicesGET: Unknown system ID: " + val.SystemId)
		log.Debug(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusNotFound)
		return
	} else if localSystemInfo.SystemId == systemIdParamStr { // Local request
		log.Debug("servicesGET: Local request: ", systemIdParamStr)
		services, err = getMecServices(localSystemInfo.Host)
		if err != nil {
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else { // Forward to peer MEF
		services, err = sbi.GetMecServices(val.Host)
		if err != nil {
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	log.Debug("servicesGET: ", services)

	// Prepare & send response
	var fedServiceInfos = []FedServiceInfo{}
	for _, v := range services {
		var f = FedServiceInfo{
			SystemId: val.SystemId,
			MecHostInformation: &MecHostInformation{
				HostName: val.Host,
				HostId:   "",
			},
			ServiceInfo: &ServiceInfo{
				SerName: v.SerName,
				Version: v.Version,
			},
		}
		f.ServiceInfo = &ServiceInfo{
			SerName: v.SerName,
			Version: v.Version,
		}
		if v.SerCategory != nil {
			f.ServiceInfo.SerCategory = &CategoryRef{
				Href:    v.SerCategory.Href,
				Id:      v.SerCategory.Id,
				Name:    v.SerCategory.Name,
				Version: v.SerCategory.Version,
			}
		}
		if v.TransportInfo != nil {
			f.ServiceInfo.TransportsSupported = []TransportsSupported{}
			var ts = TransportsSupported{}
			// Transport field
			ts.Transport = &TransportDescriptor{
				Name:        v.TransportInfo.Name,
				Description: v.TransportInfo.Description,
				Protocol:    v.TransportInfo.Protocol,
				//Security *SecurityInfo
				//Type_ *TransportTypes `json:"type"`
				Version:          v.TransportInfo.Version,
				ImplSpecificInfo: v.TransportInfo.ImplSpecificInfo,
			}
			if v.TransportInfo.Security != nil {
				ts.Transport.Security = &SecurityInfo{
					// 		OAuth2Info: v.TransportInfo.Security.OAuth2Info,
				}
			}
			if v.TransportInfo.Type_ != nil && *v.TransportInfo.Type_ == "REST_HTTP" {
				ts.Transport.Type_ = new(TransportTypes)
				*ts.Transport.Type_ = REST_HTTP
			}

			// // Serializers field
			ts.Serializers = []SerializerType{}
			ts.Serializers = append(ts.Serializers, JSON)

			// Add it to the list
			f.ServiceInfo.TransportsSupported = append(f.ServiceInfo.TransportsSupported, ts)
		}
		fedServiceInfos = append(fedServiceInfos, f)
	} // End of 'for' statement

	jsonResponse, err := json.Marshal(fedServiceInfos)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprint(w, string(jsonResponse))
	w.WriteHeader(http.StatusOK)
}

func fedNotify(msg string, systemId string) {
	log.Debug(">>> fedNotify: msg: ", msg)
	log.Debug(">>> fedNotify: systemId: ", systemId)

	if msg == "" { // A MEC system de-registered
		// Skip it
		return
	}

	// A MEC system registered or updated its registration
	var systemInfo sbi.SystemInfo
	err := json.Unmarshal([]byte(msg), &systemInfo)
	if err != nil {
		log.Error(err.Error())
		return
	}
	log.Debug("fedNotify: systemInfo: ", systemInfo)
	if systemInfo.SystemId == "" {
		log.Error("fedNotify: Invalid SystemId value")
		return
	}

	if systemInfo.SystemId != localSystemInfo.SystemId { // Do not notify on our registration
		// Populate systemInfopMap
		systemInfopMap[systemInfo.SystemId] = systemInfo
		log.Debug("fedNotify: new systemInfopMap: ", systemInfopMap)

		// Send notification if any subscription
		for _, v := range SystemUpdateNotificationSubscriptionMap {
			if v.SystemId == nil || len(v.SystemId) == 0 {
				// Send notification to v.CallbackReference
				log.Info("Sending notification ", v.CallbackReference)
				var notif = SystemUpdateNotification{
					NotificationType: v.SubscriptionType,
					UpdatedSystemInfo: []SystemInfo{
						SystemInfo{
							SystemId:       systemInfo.SystemId,
							SystemName:     systemInfo.SystemName,
							SystemProvider: systemInfo.SystemProvider,
						},
					},
					Links: &Links1{
						Subscription: v.Links.Self,
					},
				}
				log.Debug("fedNotify: Send notification: ", notif)
				go sendNotification(v.CallbackReference, notif)
			}
		}
	}
}

func sendNotification(notifyUrl string, notification SystemUpdateNotification) {
	log.Info(">>> sendNotification: ", notifyUrl)

	startTime := time.Now()
	jsonNotif, err := json.Marshal(notification)
	if err != nil {
		log.Error(err.Error())
	}

	resp, err := http.Post(notifyUrl, "application/json", bytes.NewBuffer(jsonNotif))
	duration := float64(time.Since(startTime).Microseconds()) / 1000.0
	_ = httpLog.LogNotification(notifyUrl, "POST", "", "", string(jsonNotif), resp, startTime)
	if err != nil {
		log.Error(err)
		met.ObserveNotification(sandboxName, serviceName, notification.NotificationType, notifyUrl, nil, duration)
		return
	}
	met.ObserveNotification(sandboxName, serviceName, notification.NotificationType, notifyUrl, resp, duration)
	defer resp.Body.Close()
}

func registerSubscription(subsIdStr string, eventSub *SystemUpdateNotificationSubscription) {
	log.Debug(">>> registerSubscription")

	subsId, err := strconv.Atoi(subsIdStr)
	if err != nil {
		log.Error(err)
	}

	mutex.Lock()
	defer mutex.Unlock()

	SystemUpdateNotificationSubscriptionMap[subsId] = eventSub

	// Calculate expiry time by adding seconds to the current time
	if eventSub != nil && eventSub.ExpiryDeadline != nil {
		expiryTime := time.Now().Unix() + int64(eventSub.ExpiryDeadline.Seconds)
		// 1.1 Remove the existing one
		intList := subscriptionExpiryMap[expiryTime]
		// TODO FSCOM Common code with delSubscription, create a function to remove an expiruDeadline from the subscriptionExpiryMap
		for i, subsIndex := range intList {
			log.Info("registerSubscription: i: ", i)
			log.Info("registerSubscription: subsIndex: ", subsIndex)
			if subsIndex == subsId {
				log.Info("registerSubscription: found index, delete entry")
				// Remove item and update subscriptionExpiryMap
				subscriptionExpiryMap[expiryTime] = append(intList[:i], intList[i+1:]...)
				break
			}
		} // End of 'for' statement
		// 1.2 And add the new one
		subscriptionExpiryMap[expiryTime] = append(subscriptionExpiryMap[expiryTime], subsId)
	}

	log.Debug("registerSubscription: SystemUpdateNotificationSubscriptionMap: ", SystemUpdateNotificationSubscriptionMap)
	log.Debug("registerSubscription: subscriptionExpiryMap: ", subscriptionExpiryMap)
}

func delSubscription(subsIdStr string) error {
	log.Debug(">>> delSubscription: ", subsIdStr)

	subsId, err := strconv.Atoi(subsIdStr)
	if err != nil {
		log.Error(err)
	}

	mutex.Lock()
	defer mutex.Unlock()

	keyName := baseKey + "subscriptions:" + subsIdStr
	log.Info("delSubscription: keyName: ", keyName)
	_, err = rc.JSONGetEntry(keyName, ".")
	if err != nil { // subscriptionId does not exist
		log.Error(err.Error())
		return err
	}
	err = rc.JSONDelEntry(keyName, ".")
	if err != nil {
		log.Error(err.Error())
		return err
	}

	log.Info("delSubscription: Before removal: subscriptionExpiryMap: ", subscriptionExpiryMap)
	for i, subsIndexList := range subscriptionExpiryMap {
		log.Info("delSubscription: subsIndexList: ", subsIndexList)
		for j, subsIndex := range subsIndexList {
			log.Info("delSubscription: j: ", j)
			log.Info("delSubscription: subsIndex: ", subsIndex)
			if subsIndex == subsId {
				// FIXME FSCOM How to manage it subscriptionExpiryMap
				log.Info("delSubscription: found index, delete entry")
				subscriptionExpiryMap[i] = append(subscriptionExpiryMap[i][:j], subscriptionExpiryMap[i][j+1:]...)
				break
			}
		} // End of 'for' statement
	} // End of 'for' statement
	log.Info("delSubscription: After removal: subscriptionExpiryMap: ", subscriptionExpiryMap)

	log.Info("delSubscription: Befor SystemUpdateNotificationSubscriptionMap", SystemUpdateNotificationSubscriptionMap)
	delete(SystemUpdateNotificationSubscriptionMap, subsId)
	log.Info("delSubscription: After SystemUpdateNotificationSubscriptionMap", SystemUpdateNotificationSubscriptionMap)

	return nil
}

/*
 * checkForExpiredSubscriptions delete those subscriptions whose expiryTime is reached
 */
func checkForExpiredSubscriptions() {
	//log.Debug(">>> checkForExpiredSubscriptions")
	// log.Info("checkForExpiredSubscriptions: fedMsgSubscriptionMap: ", fedMsgSubscriptionMap)

	nowTime := time.Now().Unix()
	mutex.Lock()
	defer mutex.Unlock()
	for expiryTime, subsIndexList := range subscriptionExpiryMap {
		if expiryTime <= nowTime {
			subscriptionExpiryMap[expiryTime] = nil
			for _, subsId := range subsIndexList {
				subsIdStr := strconv.Itoa(subsId)
				keyName := baseKey + "subscriptions:" + subsIdStr
				log.Info("checkForExpiredSubscriptions: keyName: ", keyName)
				// FIXME FSCOM To be done
				subscription, err := rc.JSONGetEntry(keyName, ".")
				if err != nil {
					log.Error(err.Error())
					continue
				}
				log.Info("checkForExpiredSubscriptions: subscription: ", subscription)
				cbRef := SystemUpdateNotificationSubscriptionMap[subsId].CallbackReference
				log.Info("checkForExpiredSubscriptions: cbRef: ", cbRef)

				// var notif ExpiryNotification
				// var expiryTimeStamp TimeStamp
				// expiryTimeStamp.Seconds = int32(expiryTime)

				// link := new(ExpiryNotificationLinks)
				// link.Subscription.Href = cbRef
				// notif.Links = link

				// notif.ExpiryDeadline = &expiryTimeStamp
				// sendExpiryNotification(link.Subscription.Href, notif)

				// Delete subscription
				err = delSubscription(subsIdStr)
				if err != nil {
					log.Error(err.Error())
				}
			} // End of 'for' statement
		}
	} // End of 'for' statement
}

// func computeElapseTime(expiryDeadline TimeStamp) *TimeStamp {
// 	return &TimeStamp{NanoSeconds: 0, Seconds: int32(expiryDeadline.Seconds) - int32(time.Now().Unix())}
// }

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
