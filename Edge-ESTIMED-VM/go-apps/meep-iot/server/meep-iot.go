/*
 * Copyright (c) 2025  The AdvantEDGE Authors
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
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	sbi "github.com/InterDigitalInc/AdvantEDGE/go-apps/meep-iot/sbi"
	asc "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-app-support-client"
	dkm "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-data-key-mgr"
	httpLog "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-http-logger"
	log "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-logger"
	redis "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-redis"
	scc "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-sandbox-ctrl-client"
	smc "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-service-mgmt-client"
	"github.com/gorilla/mux"
	//uuid "github.com/google/uuid"
)

const moduleName = "meep-iot"
const iotBasePath = "iots/v1/"
const iotKey = "iot"

const serviceName = "IOT Service"
const serviceCategory = "IOT"
const defaultLocationName = "global"
const defaultScopeOfLocality = "MEC_SYSTEM"
const defaultConsumedLocalOnly = true
const appTerminationPath = "notifications/mec011/appTermination"

var redisAddr string = "meep-redis-master.default.svc.cluster.local:6379"
var influxAddr string = "http://meep-influxdb.default.svc.cluster.local:8086"
var sbxCtrlUrl string = "http://meep-sandbox-ctrl"

var currentStoreName = ""

var IOT_DB = 0

var rc *redis.Connector
var hostUrl *url.URL
var instanceId string
var instanceName string
var sandboxName string
var mepName string = defaultLocationName
var scopeOfLocality string = defaultScopeOfLocality
var consumedLocalOnly bool = defaultConsumedLocalOnly
var locality []string
var basePath string
var baseKey string

const serviceAppVersion = "3.1.1"

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

var registeredIotPlatformsMap = map[string]sbi.IotPlatformInfo{} // List of discovered IOT Plateform

var validParams = []string{"deviceMetadata", "gpsi", "msisdn", "deviceId", "requestedMecTrafficRule", "requestedIotPlatformId", "requestedUserTransportId"}
var visitedFilter = map[string]bool{ // ETSI GS MEC 033 V3.1.1 (2022-12) Clause 7.3.3.1 GET
	"deviceMetadata":           false,
	"gpsi":                     false,
	"msisdn":                   false,
	"deviceId":                 false,
	"requestedMecTrafficRule":  false,
	"requestedIotPlatformId":   false,
	"requestedUserTransportId": false,
}
var visitedOp = map[string]bool{ // ETSI GS MEC 009 V3.3.1 (2024-02) Clause 6.19.2 Resource definition(s) and HTTP methods
	"eq":    false,
	"neq":   false,
	"gt":    false,
	"lt":    false,
	"gte":   false,
	"lte":   false,
	"in":    false,
	"nin":   false,
	"cont":  false,
	"ncont": false,
}

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
			Id:      "iotId",
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
		sub.CallbackReference = "http://" + moduleName + "/" + iotBasePath + appTerminationPath
	} else {
		sub.CallbackReference = "http://" + mepName + "-" + moduleName + "/" + iotBasePath + appTerminationPath
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

func mec011AppTerminationPost(w http.ResponseWriter, r *http.Request) {
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

// Init - IOT Service initialization
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

	// Set base path
	if mepName == defaultLocationName {
		basePath = "/" + sandboxName + "/" + iotBasePath
	} else {
		basePath = "/" + sandboxName + "/" + mepName + "/" + iotBasePath
	}

	// Set base storage key
	baseKey = dkm.GetKeyRoot(sandboxName) + iotKey + ":mep:" + mepName + ":"

	// Connect to Redis DB (IOT_DB)
	rc, err = redis.NewConnector(redisAddr, IOT_DB)
	if err != nil {
		log.Error("Failed connection to Redis DB (IOT_DB). Error: ", err)
		return err
	}
	_ = rc.DBFlush(baseKey)
	log.Info("Connected to Redis DB, IOT service table")

	originator := os.Getenv("MEEP_IOT_ORIGINATOR")
	if originator == "" {
		originator = "Admin"
	}
	log.Info("MEEP_IOT_ORIGINATOR: ", originator)

	// Initialize SBI
	sbiCfg := sbi.SbiCfg{
		ModuleName:     moduleName,
		SandboxName:    sandboxName,
		Originator:     originator,
		RedisAddr:      redisAddr,
		Locality:       locality,
		ScenarioNameCb: updateStoreName,
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

	log.Info("IOT successfully initialized")
	return nil
}

// Run - Start IOT
func Run() (err error) {
	// Start MEC Service registration ticker
	if appEnablementEnabled {
		startRegistrationTicker()
	}
	return sbi.Run()
}

// Stop - Stop IOT
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
				// Otherwise use the scenario-proiotioned instance ID
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

	// Flush all service data
	rc.DBFlush(baseKey)

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

func registerediotplatformsByIdDELETE(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> registerediotplatformsByIdDELETE: ", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	vars := mux.Vars(r)
	log.Debug("registerediotplatformsByIdDELETE: vars: ", vars)

	iotPlatformIdParamStr := vars["registeredIotPlatformId"]
	log.Debug("registerediotplatformsByIdDELETE: iotPlatformIdParamStr: ", iotPlatformIdParamStr)

	if _, ok := registeredIotPlatformsMap[iotPlatformIdParamStr]; !ok {
		err := errors.New("Unknown iotPlatformId: " + iotPlatformIdParamStr)
		errHandlerProblemDetails(w, err.Error(), http.StatusNotFound)
		return
	}

	err := sbi.DeregisterIotPlatformInfo(iotPlatformIdParamStr)
	if err != nil {
		log.Error("Failed to de-register IotPlatformInfo: ", err)
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}
	log.Info("De-registration succeed")

	delete(registeredIotPlatformsMap, iotPlatformIdParamStr)

	w.WriteHeader(http.StatusNoContent)
}

func registerediotplatformsByIdGET(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> registerediotplatformsByIdGET: ", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	vars := mux.Vars(r)
	log.Debug("registerediotplatformsByIdGET: vars: ", vars)
	iotPlatformIdParamStr := vars["registeredIotPlatformId"]
	log.Debug("systeminfoByIdGET: registerediotplatformsByIdGET: ", iotPlatformIdParamStr)

	// Validate query parameters
	u, _ := url.Parse(r.URL.String())
	q := u.Query()
	err := validateQueryParams(q, validParams)
	if err != nil {
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}

	userTransportInfo := q["userTransportInfo"]
	log.Debug("systeminfoByIdGET: userTransportInfo: ", userTransportInfo)
	customServiceTransportInfo := q["customServiceTransportInfo"]
	log.Debug("systeminfoByIdGET: customServiceTransportInfo: ", customServiceTransportInfo)
	noFilter := len(userTransportInfo) == 0 && len(customServiceTransportInfo) == 0
	log.Debug("systeminfoByIdGET: noFilter: ", noFilter)

	val, ok := registeredIotPlatformsMap[iotPlatformIdParamStr]
	if len(registeredIotPlatformsMap) == 0 || !ok {
		w.WriteHeader(http.StatusNotFound)
	} else {
		var iotPlatformInfo IotPlatformInfo
		if noFilter {
			iotPlatformInfo = convertIotPlatformInfoFromSbi(val)
		} else {
			// FIXME FSCOM To be done
			http.Error(w, "Filtering not implemented yet", http.StatusNotImplemented)
			return
		}
		jsonResponse, err := json.Marshal(iotPlatformInfo)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		fmt.Fprint(w, string(jsonResponse))
		w.WriteHeader(http.StatusOK)
	}
}

func registerediotplatformsByIdPUT(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> registerediotplatformsByIdPUT: ", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	vars := mux.Vars(r)
	iotPlatformIdParamStr := vars["registeredIotPlatformId"]
	log.Debug("registerediotplatformsByIdPUT: iotPlatformIdParamStr: ", iotPlatformIdParamStr)

	var iotPlatformInfo IotPlatformInfo
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&iotPlatformInfo)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}

	if iotPlatformInfo.IotPlatformId != iotPlatformIdParamStr {
		errHandlerProblemDetails(w, "IotPlatformId in body does not match path parameter", http.StatusBadRequest)
		return
	}

	sbiIotPlatformInfo := convertIotPlatformInfoToSbi(iotPlatformInfo)
	updatedPlatform, err := sbi.UpdateIotPlatform(sbiIotPlatformInfo)
	if err != nil {
		log.Error(err.Error())
		if err.Error() == "IoT platform not found" {
			errHandlerProblemDetails(w, err.Error(), http.StatusNotFound)
		} else {
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	apiUpdatedPlatform := convertIotPlatformInfoFromSbi(updatedPlatform)

	jsonResponse, err := json.Marshal(apiUpdatedPlatform)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprint(w, string(jsonResponse))
	w.WriteHeader(http.StatusOK)
}

func registerediotplatformsGET(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> registerediotplatformsGET: ", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	// Validate query parameters
	u, _ := url.Parse(r.URL.String())
	q := u.Query()
	validParams := []string{"iotPlatformId", "enabled"}
	err := validateQueryParams(q, validParams)
	if err != nil {
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}

	iotPlatformId := q.Get("iotPlatformId")
	enabled := q.Get("enabled")
	noFilter := iotPlatformId == "" && enabled == ""
	log.Debug("registerediotplatformsGET: noFilter: ", noFilter)

	log.Debug("registerediotplatformsGET: len(registeredIotPlatformsMap): ", len(registeredIotPlatformsMap))
	if len(registeredIotPlatformsMap) == 0 {
		w.WriteHeader(http.StatusNotFound)
	} else {
		l := []sbi.IotPlatformInfo{}
		for _, val := range registeredIotPlatformsMap {
			if noFilter {
				l = append(l, val)
			} else if iotPlatformId != "" && val.IotPlatformId == iotPlatformId || enabled != "" && strconv.FormatBool(val.Enabled) == enabled {
				l = append(l, val)
			} // else skip this item
		}
		log.Debug("registerediotplatformsGET: len(l): ", len(l))
		if len(l) == 0 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var iotPlatformInfos = []IotPlatformInfo{}
		for _, val := range l {
			item := convertIotPlatformInfoFromSbi(val)
			iotPlatformInfos = append(iotPlatformInfos, item)
		}
		jsonResponse, err := json.Marshal(iotPlatformInfos)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		fmt.Fprint(w, string(jsonResponse))
		w.WriteHeader(http.StatusOK)
	}
}

func registerediotplatformsPOST(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> registerediotplatformsPOST: ", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	var requestData IotPlatformInfo
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&requestData)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}
	log.Debug("registerediotplatformsPOST: requestData: ", requestData)

	// Validating mandatory parameters in request
	if requestData.IotPlatformId == "" {
		log.Error("Mandatory iotPlatformId parameter shall be absent")
		errHandlerProblemDetails(w, "Mandatory attribute iotPlatformId shall be present in the request body.", http.StatusBadRequest)
		return
	}

	if len(requestData.UserTransportInfo) == 0 {
		log.Error("Mandatory UserTransportInfo parameter not present")
		errHandlerProblemDetails(w, "Mandatory attribute UserTransportInfo is missing in the request body.", http.StatusBadRequest)
		return
	} else {
		for _, v := range requestData.UserTransportInfo {
			if v.Id == "" {
				log.Error("Mandatory Id parameter shall be absent")
				errHandlerProblemDetails(w, "Mandatory attribute Id shall be absent in the request body.", http.StatusBadRequest)
				return
			}
			if v.Name == "" {
				log.Error("Mandatory Name parameter shall be absent")
				errHandlerProblemDetails(w, "Mandatory attribute Name shall be absent in the request body.", http.StatusBadRequest)
				return
			}
			if v.Type_ == nil {
				log.Error("Mandatory Type_ parameter shall be present")
				errHandlerProblemDetails(w, "Mandatory attribute Type_ shall be present.", http.StatusBadRequest)
				return
			}
			if *v.Type_ != "MB_TOPIC_BASED" {
				log.Error("Mandatory Type_ parameter shall be set to MB_TOPIC_BASED")
				errHandlerProblemDetails(w, "Mandatory attribute Type_ shall be set to MB_TOPIC_BASED in the request body.", http.StatusBadRequest)
				return
			}
			if v.Protocol != "MQTT" && v.Protocol != "MQTT+WSS" && v.Protocol != "AMQP" {
				log.Error("Mandatory Protocol parameter shall be set to MQTT, MQTT+WSS or AMQP")
				errHandlerProblemDetails(w, "Mandatory attribute Protocol shall be set to MQTT, MQTT+WSS or AMQP in the request body.", http.StatusBadRequest)
				return
			}
			if v.Version == "" {
				log.Error("Mandatory Version parameter shall be present")
				errHandlerProblemDetails(w, "Mandatory attribute Version shall be present in the request body.", http.StatusBadRequest)
				return
			}
			if v.Endpoint == nil {
				log.Error("Mandatory Endpoint parameter shall be present")
				errHandlerProblemDetails(w, "Mandatory attribute Endpoint shall be present in the request body.", http.StatusBadRequest)
				return
			}
			if v.Security == nil {
				log.Error("Mandatory Security parameter shall be present")
				errHandlerProblemDetails(w, "Mandatory attribute Security shall be present in the request body.", http.StatusBadRequest)
				return
			}
			if v.ImplSpecificInfo == nil {
				log.Error("Mandatory ImplSpecificInfo parameter shall be present")
				errHandlerProblemDetails(w, "Mandatory attribute ImplSpecificInfo shall be present in the request body.", http.StatusBadRequest)
				return
			}
		}
	}

	log.Debug("registerediotplatformsPOST: registeredIotPlatformsMap (before): ", registeredIotPlatformsMap)
	if _, ok := registeredIotPlatformsMap[requestData.IotPlatformId]; ok {
		log.Error("IoT platform already created")
		errHandlerProblemDetails(w, "IoT platform already created.", http.StatusBadRequest)
		return
	}

	// Populate registeredIotPlatformsMap
	s := convertIotPlatformInfoToSbi(requestData)
	responseData, err := sbi.RegisterIotPlatformInfo(s)
	if err != nil {
		log.Error("Failed to register IotPlatformInfo: ", err)
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}
	registeredIotPlatformsMap[responseData.IotPlatformId] = responseData
	log.Debug("registerediotplatformsPOST: registeredIotPlatformsMap (after): ", registeredIotPlatformsMap)

	// Prepare & send response
	c := convertIotPlatformInfoFromSbi(responseData)
	jsonResponse := convertIoTPlatformInfotoJson(&c)
	w.Header().Set("Location", hostUrl.String()+basePath+"registered_iot_platforms/"+responseData.IotPlatformId)
	w.WriteHeader(http.StatusCreated)
	fmt.Fprint(w, jsonResponse)
}

func registereddevicesByIdDELETE(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> registereddevicesByIdDELETE: ", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	vars := mux.Vars(r)
	log.Debug("registereddevicesByIdDELETE: vars: ", vars)

	registeredDeviceIdParamStr := vars["registeredDeviceId"]
	log.Debug("registereddevicesByIdDELETE: registeredDeviceIdParamStr: ", registeredDeviceIdParamStr)

	err := sbi.DeleteDevice(registeredDeviceIdParamStr)
	if err != nil {
		errHandlerProblemDetails(w, err.Error(), http.StatusNotFound)
		return
	}

	// FIXME FSCOM Remove entry into redist

	w.WriteHeader(http.StatusNoContent)
}

func registereddevicesByIdGET(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> registereddevicesByIdGET: ", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	vars := mux.Vars(r)
	log.Debug("registereddevicesByIdGET: vars: ", vars)

	registeredDeviceIdParamStr := vars["registeredDeviceId"]
	log.Debug("registereddevicesByIdGET: registeredDeviceIdParamStr: ", registeredDeviceIdParamStr)

	device, err := sbi.GetDevice(registeredDeviceIdParamStr)
	if err != nil {
		errHandlerProblemDetails(w, err.Error(), http.StatusNotFound)
		return
	}
	d := convertDeviceInfoFromSbi(device)
	log.Debug("registereddevicesByIdGET: d=", d)

	// Prepare & send the response
	jsonResponse, err := json.Marshal(d)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Debug("registereddevicesByIdGET: jsonResponse=", string(jsonResponse))
	fmt.Fprint(w, string(jsonResponse))
	w.WriteHeader(http.StatusOK)
}

func registereddevicesByIdPUT(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> registereddevicesByIdPUT: ", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	vars := mux.Vars(r)
	registeredDeviceIdParamStr := vars["registeredDeviceId"]
	log.Debug("registereddevicesByIdPUT: registeredDeviceIdParamStr: ", registeredDeviceIdParamStr)

	var deviceInfo DeviceInfo
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&deviceInfo)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}

	if deviceInfo.DeviceId != registeredDeviceIdParamStr {
		errHandlerProblemDetails(w, "DeviceId in body does not match path parameter", http.StatusBadRequest)
		return
	}

	sbiDeviceInfo := convertDeviceInfoToSbi(deviceInfo)
	updatedDevice, err := sbi.UpdateDevice(sbiDeviceInfo)
	if err != nil {
		log.Error(err.Error())
		if err.Error() == "Device not found" {
			errHandlerProblemDetails(w, err.Error(), http.StatusNotFound)
		} else {
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	apiUpdatedDevice := convertDeviceInfoFromSbi(updatedDevice)

	jsonResponse, err := json.Marshal(apiUpdatedDevice)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprint(w, string(jsonResponse))
	w.WriteHeader(http.StatusOK)
}

func registereddevicesGET(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> registereddevicesGET: ", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	// Validate query parameters
	u, _ := url.Parse(r.URL.String())
	q := u.Query()
	log.Debug("registereddevicesGET: q=", q)
	validParams := []string{"filter"}
	err := validateQueryParams(q, validParams)
	if err != nil {
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}
	filter, err := validateFilter(q["filter"], validParams)
	if err != nil {
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}

	devicesList, err := sbi.GetDevices()
	if err != nil {
		log.Error("registereddevicesGET: ", err)
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Debug("registereddevicesGET: devicesList=", devicesList)
	if len(devicesList) == 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	var devices []DeviceInfo
	if len(filter) == 0 {
		devices, err = convertDeviceInfosFromSbi(devicesList)
	} else {
		devices, err = applyFiltering(devicesList, filter)
	}
	if err != nil {
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(devices) == 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Prepare & send the response
	jsonResponse, err := json.Marshal(devices)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Debug("registereddevicesGET: jsonResponse=", string(jsonResponse))
	fmt.Fprint(w, string(jsonResponse))
	w.WriteHeader(http.StatusOK)
}

func applyFiltering(devicesList []sbi.DeviceInfo, filter []string) (devices []DeviceInfo, err error) {
	log.Debug(">>> applyFiltering")
	devices, err = convertDeviceInfosFromSbi_with_filter(devicesList, filter)
	log.Debug("After applyFiltering: devices: ", devices)
	return devices, err
}

func registereddevicesPOST(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> registereddevicesPOST: ", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	// Read JSON input stream provided in the Request, and stores it in the bodyBytes as bytes
	var deviceInfo DeviceInfo
	bodyBytes, _ := ioutil.ReadAll(r.Body)
	// Unmarshal function to converts a JSON-formatted string into a SubscriptionCommon struct and store it in extractSubType
	err := json.Unmarshal(bodyBytes, &deviceInfo)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Info("registereddevicesPOST: ", deviceInfo)

	// Sanity checks
	if deviceInfo.DeviceId == "" {
		err = errors.New("DeviceId field shall be present")
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(deviceInfo.RequestedMecTrafficRule) == 0 {
		if deviceInfo.RequestedIotPlatformId == "" && deviceInfo.RequestedUserTransportId == "" {
			err = errors.New("Invalid traffic rule provided")
			errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
			return
		}
		// Traffic rule provided by either RequestedIotPlatformId or RequestedUserTransportId
	} // else Traffic rule provided by RequestedMecTrafficRule

	// Process the request
	dev := convertDeviceInfoToSbi(deviceInfo)
	deviceInfoResp, err := sbi.CreateDevice(dev)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}
	devInfoResp := convertDeviceInfoFromSbi(deviceInfoResp)
	log.Info("registereddevicesPOST: devInfoResp: ", devInfoResp)

	// Prepare & send the reponse
	w.Header().Set("Location", hostUrl.String()+basePath+"registered_devices/"+devInfoResp.DeviceId)
	jsonResponse, err := json.Marshal(devInfoResp)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Info("registereddevicesPOST: jsonResponse: ", string(jsonResponse))
	w.WriteHeader(http.StatusCreated)
	fmt.Fprint(w, string(jsonResponse))
}

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

func validateFilter(filter []string, validParams []string) (f []string, err error) {
	log.Debug(">>> validateFilter: ", filter)

	for _, val := range filter {
		log.Debug("validateFilter: Processing ", val)
		val := strings.Trim(val, "()")
		f = strings.Split(val, ",")
		if len(f) != 3 {
			return nil, errors.New("validateFilter: Invalid filter structure: " + val)
		}
		if _, ok := visitedOp[f[0]]; !ok {
			return nil, errors.New("validateFilter: Invalid filter operator value: " + f[0])
		}
		if _, ok := visitedFilter[f[1]]; !ok {
			return nil, errors.New("validateFilter: Invalid filter field value: " + f[1])
		}
	} // End of 'for' statement

	return f, nil
}

func convertIotPlatformInfoFromSbi(val sbi.IotPlatformInfo) (item IotPlatformInfo) {
	item.IotPlatformId = val.IotPlatformId
	item.Enabled = val.Enabled

	for _, userTransportInfo := range val.UserTransportInfo {
		v := MbTransportInfo{
			Id:          userTransportInfo.Id,
			Name:        userTransportInfo.Name,
			Description: userTransportInfo.Description,
			Protocol:    userTransportInfo.Protocol,
			Version:     userTransportInfo.Version,
		}
		if userTransportInfo.Type_ != nil {
			s := TransportType(*userTransportInfo.Type_)
			v.Type_ = &s
		}
		if userTransportInfo.Endpoint != nil {
			e := EndPointInfo{
				Uris:        userTransportInfo.Endpoint.Uris,
				Fqdn:        userTransportInfo.Endpoint.Fqdn,
				Alternative: userTransportInfo.Endpoint.Alternative,
			}
			if len(userTransportInfo.Endpoint.Addresses) != 0 {
				for _, a := range userTransportInfo.Endpoint.Addresses {
					e.Addresses = append(e.Addresses, Addresses{Host: a.Host, Port: a.Port})
				}
			}
			v.Endpoint = &e
		}
		if userTransportInfo.Security != nil {
			e := SecurityInfo{}
			if userTransportInfo.Security.OAuth2Info != nil {
				e.OAuth2Info = &OAuth2Info{
					GrantTypes:    userTransportInfo.Security.OAuth2Info.GrantTypes,
					TokenEndpoint: userTransportInfo.Security.OAuth2Info.TokenEndpoint,
				}
			}
			e.Extensions = userTransportInfo.Security.Extensions
			v.Security = &e
		}
		if userTransportInfo.ImplSpecificInfo != nil {
			v.ImplSpecificInfo = &ImplSpecificInfo{
				EventTopics:    userTransportInfo.ImplSpecificInfo.EventTopics,
				UplinkTopics:   userTransportInfo.ImplSpecificInfo.UplinkTopics,
				DownlinkTopics: userTransportInfo.ImplSpecificInfo.DownlinkTopics,
			}
		}
		item.UserTransportInfo = append(item.UserTransportInfo, v)
	}
	if val.CustomServicesTransportInfo != nil && len(val.CustomServicesTransportInfo) != 0 {
		item.CustomServicesTransportInfo = make([]TransportInfo, 0)
		for _, customServicesTransportInfo := range val.CustomServicesTransportInfo {
			v := TransportInfo{
				Id:          customServicesTransportInfo.Id,
				Name:        customServicesTransportInfo.Name,
				Description: customServicesTransportInfo.Description,
				Protocol:    customServicesTransportInfo.Protocol,
				Version:     customServicesTransportInfo.Version,
			}
			if customServicesTransportInfo.Type_ != nil {
				s := TransportType(*customServicesTransportInfo.Type_)
				v.Type_ = &s
			}
			if customServicesTransportInfo.Endpoint != nil {
				e := EndPointInfo{
					Uris:        customServicesTransportInfo.Endpoint.Uris,
					Fqdn:        customServicesTransportInfo.Endpoint.Fqdn,
					Alternative: customServicesTransportInfo.Endpoint.Alternative,
				}
				if len(customServicesTransportInfo.Endpoint.Addresses) != 0 {
					for _, a := range customServicesTransportInfo.Endpoint.Addresses {
						e.Addresses = append(e.Addresses, Addresses{Host: a.Host, Port: a.Port})
					}
				}
				v.Endpoint = &e
			}
			item.CustomServicesTransportInfo = append(item.CustomServicesTransportInfo, v)
		}
	}

	return item
}

func convertIotPlatformInfoToSbi(val IotPlatformInfo) (item sbi.IotPlatformInfo) {
	item.IotPlatformId = val.IotPlatformId
	item.Enabled = val.Enabled
	for _, userTransportInfo := range val.UserTransportInfo {
		v := sbi.MbTransportInfo{
			Id:          userTransportInfo.Id,
			Name:        userTransportInfo.Name,
			Description: userTransportInfo.Description,
			Protocol:    userTransportInfo.Protocol,
			Version:     userTransportInfo.Version,
		}
		if userTransportInfo.Type_ != nil {
			s := string(*userTransportInfo.Type_)
			v.Type_ = &s
		}
		if userTransportInfo.Endpoint != nil {
			e := sbi.EndPointInfo{
				Uris:        userTransportInfo.Endpoint.Uris,
				Fqdn:        userTransportInfo.Endpoint.Fqdn,
				Alternative: userTransportInfo.Endpoint.Alternative,
			}
			if len(userTransportInfo.Endpoint.Addresses) != 0 {
				for _, a := range userTransportInfo.Endpoint.Addresses {
					e.Addresses = append(e.Addresses, sbi.Addresses{Host: a.Host, Port: a.Port})
				}
			}
			v.Endpoint = &e
		}
		if userTransportInfo.Security != nil {
			e := sbi.SecurityInfo{}
			if userTransportInfo.Security.OAuth2Info != nil {
				e.OAuth2Info = &sbi.OAuth2Info{
					GrantTypes:    userTransportInfo.Security.OAuth2Info.GrantTypes,
					TokenEndpoint: userTransportInfo.Security.OAuth2Info.TokenEndpoint,
				}
			}
			e.Extensions = userTransportInfo.Security.Extensions
			v.Security = &e
		}
		if userTransportInfo.ImplSpecificInfo != nil {
			v.ImplSpecificInfo = &sbi.ImplSpecificInfo{
				EventTopics:    userTransportInfo.ImplSpecificInfo.EventTopics,
				UplinkTopics:   userTransportInfo.ImplSpecificInfo.UplinkTopics,
				DownlinkTopics: userTransportInfo.ImplSpecificInfo.DownlinkTopics,
			}
		}
		item.UserTransportInfo = append(item.UserTransportInfo, v)
	} // End of 'for' statement
	if val.CustomServicesTransportInfo != nil && len(val.CustomServicesTransportInfo) != 0 {
		item.CustomServicesTransportInfo = make([]sbi.TransportInfo, 0)
		for _, customServicesTransportInfo := range val.CustomServicesTransportInfo {
			v := sbi.TransportInfo{
				Id:          customServicesTransportInfo.Id,
				Name:        customServicesTransportInfo.Name,
				Description: customServicesTransportInfo.Description,
				Protocol:    customServicesTransportInfo.Protocol,
				Version:     customServicesTransportInfo.Version,
			}
			if customServicesTransportInfo.Type_ != nil {
				s := string(*customServicesTransportInfo.Type_)
				v.Type_ = &s
			}
			if customServicesTransportInfo.Endpoint != nil {
				e := sbi.EndPointInfo{
					Uris:        customServicesTransportInfo.Endpoint.Uris,
					Fqdn:        customServicesTransportInfo.Endpoint.Fqdn,
					Alternative: customServicesTransportInfo.Endpoint.Alternative,
				}
				if len(customServicesTransportInfo.Endpoint.Addresses) != 0 {
					for _, a := range customServicesTransportInfo.Endpoint.Addresses {
						e.Addresses = append(e.Addresses, sbi.Addresses{Host: a.Host, Port: a.Port})
					}
				}
				v.Endpoint = &e
			}
			item.CustomServicesTransportInfo = append(item.CustomServicesTransportInfo, v)
		} // End of 'for' statement
	}

	return item
}

func convertDeviceInfoFromSbi(dev sbi.DeviceInfo) (device DeviceInfo) {
	device = DeviceInfo{
		DeviceAuthenticationInfo: dev.DeviceAuthenticationInfo,
		Gpsi:                     dev.Gpsi,
		Pei:                      dev.Pei,
		Supi:                     dev.Supi,
		Msisdn:                   dev.Msisdn,
		Imei:                     dev.Imei,
		Imsi:                     dev.Imsi,
		Iccid:                    dev.Iccid,
		DeviceId:                 dev.DeviceId,
		RequestedIotPlatformId:   dev.RequestedIotPlatformId,
		RequestedUserTransportId: dev.RequestedUserTransportId,
		ClientCertificate:        dev.ClientCertificate,
		Enabled:                  dev.Enabled,
	}
	if len(dev.DeviceMetadata) != 0 {
		device.DeviceMetadata = make([]KeyValuePair, len(dev.DeviceMetadata))
		for i, k := range dev.DeviceMetadata {
			device.DeviceMetadata[i] = KeyValuePair{Key: k.Key, Value: k.Value}
		} // End of 'for' statement
	}
	if len(dev.RequestedMecTrafficRule) != 0 {
		device.RequestedMecTrafficRule = make([]TrafficRuleDescriptor, len(dev.RequestedMecTrafficRule))
		for i, v := range dev.RequestedMecTrafficRule {
			device.RequestedMecTrafficRule[i] = TrafficRuleDescriptor{
				TrafficRuleId: v.TrafficRuleId,
				FilterType:    v.FilterType,
				Priority:      v.Priority,
				Action:        v.Action,
			}
			if len(dev.RequestedMecTrafficRule[i].TrafficFilter) != 0 {
				device.RequestedMecTrafficRule[i].TrafficFilter = make([]TrafficFilter, len(dev.RequestedMecTrafficRule[i].TrafficFilter))
				for j, u := range dev.RequestedMecTrafficRule[i].TrafficFilter {
					device.RequestedMecTrafficRule[i].TrafficFilter[j] = TrafficFilter{
						SrcAddress:       u.SrcAddress,
						DstAddress:       u.DstAddress,
						SrcPort:          u.SrcPort,
						DstPort:          u.DstPort,
						Protocol:         u.Protocol,
						Tag:              u.Tag,
						Uri:              u.Uri,
						PacketLabel:      u.PacketLabel,
						SrcTunnelAddress: u.SrcTunnelAddress,
						TgtTunnelAddress: u.TgtTunnelAddress,
						SrcTunnelPort:    u.SrcTunnelPort,
						DstTunnelPort:    u.DstTunnelPort,
						QCI:              u.QCI,
						DSCP:             u.DSCP,
						TC:               u.TC,
					}
				} // End of 'for' statement
			}
			if v.DstInterface != nil {
				device.RequestedMecTrafficRule[i].DstInterface = &InterfaceDescriptor{
					InterfaceType: v.DstInterface.InterfaceType,
					SrcMACAddress: v.DstInterface.SrcMACAddress,
					DstMACAddress: v.DstInterface.DstMACAddress,
					DstIPAddress:  v.DstInterface.DstIPAddress,
				}
				if v.DstInterface.TunnelInfo != nil {
					device.RequestedMecTrafficRule[i].DstInterface.TunnelInfo = &TunnelInfo{
						TunnelType:         v.DstInterface.TunnelInfo.TunnelType,
						TunnelDstAddress:   v.DstInterface.TunnelInfo.TunnelDstAddress,
						TunnelSrcAddress:   v.DstInterface.TunnelInfo.TunnelSrcAddress,
						TunnelSpecificData: v.DstInterface.TunnelInfo.TunnelSpecificData,
					}
				}
			}
		} // End of 'for' statement
	}
	if dev.DeviceSpecificMessageFormats != nil {
		var deviceSpecificMessageFormats = DeviceSpecificMessageFormats{}
		if dev.DeviceSpecificMessageFormats.EventMsgFormat != nil {
			deviceSpecificMessageFormats.EventMsgFormat = &EventMsg{
				EventTopic:            dev.DeviceSpecificMessageFormats.EventMsgFormat.EventTopic,
				IncludeDeviceAddr:     dev.DeviceSpecificMessageFormats.EventMsgFormat.IncludeDeviceAddr,
				IncludeDeviceMetadata: dev.DeviceSpecificMessageFormats.EventMsgFormat.IncludeDeviceMetadata,
				IncludePei:            dev.DeviceSpecificMessageFormats.EventMsgFormat.IncludePei,
				IncludeSupi:           dev.DeviceSpecificMessageFormats.EventMsgFormat.IncludeSupi,
				IncludeImei:           dev.DeviceSpecificMessageFormats.EventMsgFormat.IncludeImei,
				IncludeImsi:           dev.DeviceSpecificMessageFormats.EventMsgFormat.IncludeImsi,
				IncludeIccid:          dev.DeviceSpecificMessageFormats.EventMsgFormat.IncludeIccid,
				IncludeDeviceId:       dev.DeviceSpecificMessageFormats.EventMsgFormat.IncludeDeviceId,
			}
			if dev.DeviceSpecificMessageFormats.EventMsgFormat.SelectedSerializer != nil {
				var s SerializerType
				if *dev.DeviceSpecificMessageFormats.EventMsgFormat.SelectedSerializer == "JSON" {
					s = JSON
				} else if *dev.DeviceSpecificMessageFormats.EventMsgFormat.SelectedSerializer == "XML" {
					s = XML
				} else {
					s = PROTOBUF3
				}
				deviceSpecificMessageFormats.EventMsgFormat.SelectedSerializer = &s
			}
		}
		if dev.DeviceSpecificMessageFormats.UplinkMsgFormat != nil {
			deviceSpecificMessageFormats.UplinkMsgFormat = &UplinkMsg{
				UplinkTopic:           dev.DeviceSpecificMessageFormats.UplinkMsgFormat.UplinkTopic,
				IncludeDevicePort:     dev.DeviceSpecificMessageFormats.UplinkMsgFormat.IncludeDevicePort,
				IncludeDeviceAddr:     dev.DeviceSpecificMessageFormats.UplinkMsgFormat.IncludeDeviceAddr,
				IncludeDeviceMetadata: dev.DeviceSpecificMessageFormats.UplinkMsgFormat.IncludeDeviceMetadata,
				IncludePei:            dev.DeviceSpecificMessageFormats.UplinkMsgFormat.IncludePei,
				IncludeSupi:           dev.DeviceSpecificMessageFormats.UplinkMsgFormat.IncludeSupi,
				IncludeImei:           dev.DeviceSpecificMessageFormats.UplinkMsgFormat.IncludeImei,
				IncludeImsi:           dev.DeviceSpecificMessageFormats.UplinkMsgFormat.IncludeImsi,
				IncludeIccid:          dev.DeviceSpecificMessageFormats.UplinkMsgFormat.IncludeIccid,
				IncludeDeviceId:       dev.DeviceSpecificMessageFormats.UplinkMsgFormat.IncludeDeviceId,
			}
			if dev.DeviceSpecificMessageFormats.UplinkMsgFormat.SelectedSerializer != nil {
				var s SerializerType
				if *dev.DeviceSpecificMessageFormats.UplinkMsgFormat.SelectedSerializer == "JSON" {
					s = JSON
				} else if *dev.DeviceSpecificMessageFormats.UplinkMsgFormat.SelectedSerializer == "XML" {
					s = XML
				} else {
					s = PROTOBUF3
				}
				deviceSpecificMessageFormats.UplinkMsgFormat.SelectedSerializer = &s
			}
		}
		device.DeviceSpecificMessageFormats = &deviceSpecificMessageFormats
	}
	if dev.DownlinkInfo != nil {
		device.DownlinkInfo = &DownlinkInfo{
			DownlinkTopic: dev.DownlinkInfo.DownlinkTopic,
			DevicePort:    dev.DownlinkInfo.DevicePort,
		}
	}

	//log.Debug("convertDeviceInfosFromSbi: devices: ", devices)
	return device
}

func convertDeviceInfosFromSbi(devicesList []sbi.DeviceInfo) (devices []DeviceInfo, err error) {
	devices = make([]DeviceInfo, len(devicesList))
	for idx, item := range devicesList { // FIXME FSCOM Add Filter
		devices[idx] = convertDeviceInfoFromSbi(item)
	} // End of 'for' statement
	//log.Debug("convertDeviceInfosFromSbi: devices: ", devices)

	return devices, nil
}

func convertDeviceInfosFromSbi_with_filter(devicesList []sbi.DeviceInfo, filter []string) (devices []DeviceInfo, err error) {
	log.Debug(">>> convertDeviceInfosFromSbi_with_filter: ", filter)

	devices = make([]DeviceInfo, 0)
	for _, item := range devicesList { // FIXME FSCOM Add Filter
		process_it := false
		if filter[0] == "eq" { // E.g. [eq requestedUserTransportId 4ba6bf28-a748-4b35-aba6-d882a70c4337]
			if filter[1] == "gpsi" {
				process_it = item.Gpsi == filter[2]
			} else if filter[1] == "Msisdn" {
				process_it = item.Gpsi == filter[2]
			} else if filter[1] == "deviceId" {
				process_it = item.DeviceId == filter[2]
			} else if filter[1] == "requestedIotPlatformId" {
				process_it = item.RequestedIotPlatformId == filter[2]
			} else if filter[1] == "requestedUserTransportId" {
				process_it = item.RequestedUserTransportId == filter[2]
			} // FIXME FSCOM Add support of deviceMetadata & requestedMecTrafficRule
		} else if filter[0] == "neq" {
			if filter[1] == "gpsi" {
				process_it = item.Gpsi != filter[2]
			} else if filter[1] == "Msisdn" {
				process_it = item.Gpsi != filter[2]
			} else if filter[1] == "deviceId" {
				process_it = item.DeviceId != filter[2]
			} else if filter[1] == "requestedIotPlatformId" {
				process_it = item.RequestedIotPlatformId != filter[2]
			} else if filter[1] == "requestedUserTransportId" {
				process_it = item.RequestedUserTransportId != filter[2]
			} // FIXME FSCOM Add support of deviceMetadata & requestedMecTrafficRule
		}
		if process_it {
			devices = append(devices, convertDeviceInfoFromSbi(item))
		} else {
			log.Debug("convertDeviceInfosFromSbi_with_filter: skip ", item)
		}
	} // End of 'for' statement
	//log.Debug("convertDeviceInfosFromSbi_with_filter: devices: ", devices)

	return devices, nil
}

func convertDeviceInfoToSbi(dev DeviceInfo) (device sbi.DeviceInfo) {
	device = sbi.DeviceInfo{
		DeviceAuthenticationInfo: dev.DeviceAuthenticationInfo,
		Gpsi:                     dev.Gpsi,
		Pei:                      dev.Pei,
		Supi:                     dev.Supi,
		Msisdn:                   dev.Msisdn,
		Imei:                     dev.Imei,
		Imsi:                     dev.Imsi,
		Iccid:                    dev.Iccid,
		DeviceId:                 dev.DeviceId,
		RequestedIotPlatformId:   dev.RequestedIotPlatformId,
		RequestedUserTransportId: dev.RequestedUserTransportId,
		ClientCertificate:        dev.ClientCertificate,
		Enabled:                  dev.Enabled,
	}
	if len(dev.DeviceMetadata) != 0 {
		device.DeviceMetadata = make([]sbi.KeyValuePair, len(dev.DeviceMetadata))
		for i, k := range dev.DeviceMetadata {
			device.DeviceMetadata[i] = sbi.KeyValuePair{Key: k.Key, Value: k.Value}
		} // End of 'for' statement
	}
	if len(dev.RequestedMecTrafficRule) != 0 {
		device.RequestedMecTrafficRule = make([]sbi.TrafficRuleDescriptor, len(dev.RequestedMecTrafficRule))
		for i, v := range dev.RequestedMecTrafficRule {
			device.RequestedMecTrafficRule[i] = sbi.TrafficRuleDescriptor{
				TrafficRuleId: v.TrafficRuleId,
				FilterType:    v.FilterType,
				Priority:      v.Priority,
				Action:        v.Action,
			}
			if len(dev.RequestedMecTrafficRule[i].TrafficFilter) != 0 {
				device.RequestedMecTrafficRule[i].TrafficFilter = make([]sbi.TrafficFilter, len(dev.RequestedMecTrafficRule[i].TrafficFilter))
				for j, u := range dev.RequestedMecTrafficRule[i].TrafficFilter {
					device.RequestedMecTrafficRule[i].TrafficFilter[j] = sbi.TrafficFilter{
						SrcAddress:       u.SrcAddress,
						DstAddress:       u.DstAddress,
						SrcPort:          u.SrcPort,
						DstPort:          u.DstPort,
						Protocol:         u.Protocol,
						Tag:              u.Tag,
						Uri:              u.Uri,
						PacketLabel:      u.PacketLabel,
						SrcTunnelAddress: u.SrcTunnelAddress,
						TgtTunnelAddress: u.TgtTunnelAddress,
						SrcTunnelPort:    u.SrcTunnelPort,
						DstTunnelPort:    u.DstTunnelPort,
						QCI:              u.QCI,
						DSCP:             u.DSCP,
						TC:               u.TC,
					}
				} // End of 'for' statement
			}
			if v.DstInterface != nil {
				device.RequestedMecTrafficRule[i].DstInterface = &sbi.InterfaceDescriptor{
					InterfaceType: v.DstInterface.InterfaceType,
					SrcMACAddress: v.DstInterface.SrcMACAddress,
					DstMACAddress: v.DstInterface.DstMACAddress,
					DstIPAddress:  v.DstInterface.DstIPAddress,
				}
				if v.DstInterface.TunnelInfo != nil {
					device.RequestedMecTrafficRule[i].DstInterface.TunnelInfo = &sbi.TunnelInfo{
						TunnelType:         v.DstInterface.TunnelInfo.TunnelType,
						TunnelDstAddress:   v.DstInterface.TunnelInfo.TunnelDstAddress,
						TunnelSrcAddress:   v.DstInterface.TunnelInfo.TunnelSrcAddress,
						TunnelSpecificData: v.DstInterface.TunnelInfo.TunnelSpecificData,
					}
				}
			}
		} // End of 'for' statement
	}
	if dev.DeviceSpecificMessageFormats != nil {
		var deviceSpecificMessageFormats = sbi.DeviceSpecificMessageFormats{}
		if dev.DeviceSpecificMessageFormats.EventMsgFormat != nil {
			deviceSpecificMessageFormats.EventMsgFormat = &sbi.EventMsg{
				EventTopic:            dev.DeviceSpecificMessageFormats.EventMsgFormat.EventTopic,
				IncludeDeviceAddr:     dev.DeviceSpecificMessageFormats.EventMsgFormat.IncludeDeviceAddr,
				IncludeDeviceMetadata: dev.DeviceSpecificMessageFormats.EventMsgFormat.IncludeDeviceMetadata,
				IncludePei:            dev.DeviceSpecificMessageFormats.EventMsgFormat.IncludePei,
				IncludeSupi:           dev.DeviceSpecificMessageFormats.EventMsgFormat.IncludeSupi,
				IncludeImei:           dev.DeviceSpecificMessageFormats.EventMsgFormat.IncludeImei,
				IncludeImsi:           dev.DeviceSpecificMessageFormats.EventMsgFormat.IncludeImsi,
				IncludeIccid:          dev.DeviceSpecificMessageFormats.EventMsgFormat.IncludeIccid,
				IncludeDeviceId:       dev.DeviceSpecificMessageFormats.EventMsgFormat.IncludeDeviceId,
			}
			if dev.DeviceSpecificMessageFormats.EventMsgFormat.SelectedSerializer != nil {
				var s string = string(*dev.DeviceSpecificMessageFormats.EventMsgFormat.SelectedSerializer)
				deviceSpecificMessageFormats.EventMsgFormat.SelectedSerializer = &s
			}
		}
		if dev.DeviceSpecificMessageFormats.UplinkMsgFormat != nil {
			deviceSpecificMessageFormats.UplinkMsgFormat = &sbi.UplinkMsg{
				UplinkTopic:           dev.DeviceSpecificMessageFormats.UplinkMsgFormat.UplinkTopic,
				IncludeDevicePort:     dev.DeviceSpecificMessageFormats.UplinkMsgFormat.IncludeDevicePort,
				IncludeDeviceAddr:     dev.DeviceSpecificMessageFormats.UplinkMsgFormat.IncludeDeviceAddr,
				IncludeDeviceMetadata: dev.DeviceSpecificMessageFormats.UplinkMsgFormat.IncludeDeviceMetadata,
				IncludePei:            dev.DeviceSpecificMessageFormats.UplinkMsgFormat.IncludePei,
				IncludeSupi:           dev.DeviceSpecificMessageFormats.UplinkMsgFormat.IncludeSupi,
				IncludeImei:           dev.DeviceSpecificMessageFormats.UplinkMsgFormat.IncludeImei,
				IncludeImsi:           dev.DeviceSpecificMessageFormats.UplinkMsgFormat.IncludeImsi,
				IncludeIccid:          dev.DeviceSpecificMessageFormats.UplinkMsgFormat.IncludeIccid,
				IncludeDeviceId:       dev.DeviceSpecificMessageFormats.UplinkMsgFormat.IncludeDeviceId,
			}
			if dev.DeviceSpecificMessageFormats.UplinkMsgFormat.SelectedSerializer != nil {
				var s string = string(*dev.DeviceSpecificMessageFormats.UplinkMsgFormat.SelectedSerializer)
				deviceSpecificMessageFormats.UplinkMsgFormat.SelectedSerializer = &s
			}
		}
		device.DeviceSpecificMessageFormats = &deviceSpecificMessageFormats
	}
	if dev.DownlinkInfo != nil {
		device.DownlinkInfo = &sbi.DownlinkInfo{
			DownlinkTopic: dev.DownlinkInfo.DownlinkTopic,
			DevicePort:    dev.DownlinkInfo.DevicePort,
		}
	}

	//log.Debug("convertDeviceInfoToSbi: devices: ", devices)
	return device
}
