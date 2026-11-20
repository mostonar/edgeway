/*
 * Copyright (c) 2024  The AdvantEDGE Authors
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
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	sbi "github.com/InterDigitalInc/AdvantEDGE/go-apps/meep-vis/sbi"
	asc "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-app-support-client"
	dkm "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-data-key-mgr"
	gisClient "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-gis-engine-client"
	httpLog "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-http-logger"
	log "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-logger"
	met "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-metrics"
	redis "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-redis"
	scc "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-sandbox-ctrl-client"
	smc "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-service-mgmt-client"
	sm "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-subscriptions"
	//"github.com/gorilla/mux"
	//uuid "github.com/google/uuid"
)

const moduleName = "meep-vis"
const visBasePath = "vis/v2/"
const visKey = "vis"

const serviceName = "V2XI Service"
const serviceCategory = "V2XI"
const defaultLocationName = "global"
const defaultScopeOfLocality = "MEC_SYSTEM"
const defaultConsumedLocalOnly = true
const defaultPredictionModelSupported = false
const appTerminationPath = "notifications/mec011/appTermination"

var redisAddr string = "meep-redis-master.default.svc.cluster.local:6379"
var influxAddr string = "http://meep-influxdb.default.svc.cluster.local:8086"
var sbxCtrlUrl string = "http://meep-sandbox-ctrl"

var currentStoreName = ""

var VIS_DB = 0

var rc *redis.Connector
var hostUrl *url.URL
var instanceId string
var instanceName string
var sandboxName string
var mepName string = defaultLocationName
var scopeOfLocality string = defaultScopeOfLocality
var consumedLocalOnly bool = defaultConsumedLocalOnly
var predictionModelSupported bool = defaultPredictionModelSupported
var locality []string
var v2x_broker string
var v2x_poa_list []string
var basePath string
var baseKey string

var gisAppClient *gisClient.APIClient
var gisAppClientUrl string = "http://meep-gis-engine"
var postgisHost string = ""
var postgisPort string = ""

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
var subMgr *sm.SubscriptionMgr = nil

const PROV_CHG_UU_UNI = "ProvChgUuUniSubscription"
const PROV_CHG_UU_MBMS = "ProvChgUuMbmsSubscription"
const PROV_CHG_PC5 = "ProvChgPc5Subscription"
const V2X_MSG = "V2xMsgSubscription"
const PRED_QOS = "PredQosSubscription"

const PROV_CHG_UU_UNI_NOTIF = "ProvChgUuUniNotification"
const PROV_CHG_UU_MBMS_NOTIF = "ProvChgUuMbmsNotification"
const PROV_CHG_PC5_NOTIF = "ProvChgPc5Notification"
const V2X_MSG_NOTIF = "V2xMsgNotification"

//const PRED_QOS_NOTIF = "PredQosNotification" // FIXME FSCOM There is no PredQosNotification defined by the standard ETSI GS ETSI GS MEC 040 V3.2.1 (2024-03)
const TEST_NOTIF = "TestNotification"
const NOTIFY_EXPIRY = "ExpiryNotification"

var provChgUuUniSubscriptionMap = map[int]*ProvChgUuUniSubscription{}   // List of registered UU unicast subscription
var provChgUuMbmsSubscriptionMap = map[int]*ProvChgUuMbmsSubscription{} // List of registered UU MBMS subscription
var provChgPc5SubscriptionMap = map[int]*ProvChgPc5Subscription{}       // List of registered PC5 subscription
var v2xMsgSubscriptionMap = map[int]*V2xMsgSubscription{}               // List of registered V2X message subscription

//var predQosSubscriptionMap = map[int]*PredQosSubscription{} // FIXME FSCOM There is no PredQosNotification defined by the standard ETSI GS ETSI GS MEC 040 V3.2.1 (2024-03)

var subscriptionExpiryMap = map[int][]int{}

var mutex sync.Mutex
var expiryTicker *time.Ticker
var nextSubscriptionIdAvailable int

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
			Id:      "visId",
			Name:    serviceCategory,
			Version: "v2",
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
		sub.CallbackReference = "http://" + moduleName + "/" + visBasePath + appTerminationPath
	} else {
		sub.CallbackReference = "http://" + mepName + "-" + moduleName + "/" + visBasePath + appTerminationPath
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

// Init - V2XI Service initialization
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

	// Get V2X brokers. E.g. mqtt://172.29.10.56:1883, mqtt://broker.emqx.io:1883 or amqp://ofriqrpk:e_vS3dw1zs2gb8CVlyzGwQZ8gCRoyTt5@lrat-01.rmq2.cloudamqp.com:5672
	v2x_broker = strings.TrimSpace(os.Getenv("MEEP_BROKER"))
	log.Info("MEEP_BROKER: ", v2x_broker)

	// Get V2X topic. E.g. 3gpp/v2x/obu or AMQP queue name
	v2x_topic := strings.TrimSpace(os.Getenv("MEEP_TOPIC"))
	log.Info("MEEP_TOPIC: ", v2x_topic)

	// E.g. poa-5g1,poa-5g2
	poa_list := strings.TrimSpace(os.Getenv("MEEP_POA_LIST"))
	if poa_list != "" {
		v2x_poa_list = strings.Split(poa_list, ";")
		if len(v2x_poa_list) > 1 {
			sort.Strings(v2x_poa_list) // Sorting the PoA list to use search algorithms
		}
	}
	log.Info("MEEP_POA_LIST: ", v2x_poa_list)

	// Set base path
	if mepName == defaultLocationName {
		basePath = "/" + sandboxName + "/" + visBasePath
	} else {
		basePath = "/" + sandboxName + "/" + mepName + "/" + visBasePath
	}

	// Set base storage key
	baseKey = dkm.GetKeyRoot(sandboxName) + visKey + ":mep:" + mepName + ":"

	// Connect to Redis DB (VIS_DB)
	rc, err = redis.NewConnector(redisAddr, VIS_DB)
	if err != nil {
		log.Error("Failed connection to Redis DB (VIS_DB). Error: ", err)
		return err
	}
	_ = rc.DBFlush(baseKey)
	log.Info("Connected to Redis DB, V2XI service table")

	gisAppClientCfg := gisClient.NewConfiguration()
	gisAppClientCfg.BasePath = gisAppClientUrl + "/gis/v1"

	gisAppClient = gisClient.NewAPIClient(gisAppClientCfg)
	if gisAppClient == nil {
		log.Error("Failed to create GIS App REST API client: ", gisAppClientCfg.BasePath)
		err := errors.New("Failed to create GIS App REST API client")
		return err
	}

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
		V2xBroker:      v2x_broker,
		V2xTopic:       v2x_topic,
		PoaList:        v2x_poa_list,
		RedisAddr:      redisAddr,
		PostgisHost:    postgisHost,
		PostgisPort:    postgisPort,
		Locality:       locality,
		ScenarioNameCb: updateStoreName,
		V2xNotify:      v2xNotify,
		CleanUpCb:      cleanUp,
	}
	if mepName != defaultLocationName {
		sbiCfg.LocationName = mepName
	}
	predictionModelSupported, err = sbi.Init(sbiCfg)
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

	log.Info("VIS successfully initialized")
	return nil
}

// reInit - finds the value already in the DB to repopulate local stored info
func reInit() {
	log.Debug(">>> reInit")

	//next available subsId will be overrriden if subscriptions already existed
	nextSubscriptionIdAvailable = 1

	keyName := baseKey + "subscriptions:" + "*"
	log.Info("reInit: keyName: ", keyName)
	err := rc.ForEachJSONEntry(keyName, repopulateV2xMsgSubscriptionMap, nil)
	if err != nil {
		log.Error(err.Error())
	}

	// FSCOM For debug purpose
	// keyName = dkm.GetKeyRoot(sandboxName) + "vis:mep:" + mepName + ":" + "POA:*"
	// err := rc.ForEachJSONEntry(keyName, populatePoaInfo, nil)
	// if err != nil {
	// 	log.Error(err.Error())
	// }

}

// FSCOM For debug purpose
// func populatePoaInfo(key string, jsonInfo string, userData interface{}) error {
// 	log.Info("+++ key     : ", key)
// 	log.Info("+++ jsonInfo: ", jsonInfo)
// 	return nil
// }

// Run - Start VIS
func Run() (err error) {
	// Start MEC Service registration ticker
	if appEnablementEnabled {
		startRegistrationTicker()
	}
	return sbi.Run()
}

// Stop - Stop VIS
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
				// Otherwise use the scenario-provisioned instance ID
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

	subscriptionExpiryMap = map[int][]int{}
	provChgUuUniSubscriptionMap = map[int]*ProvChgUuUniSubscription{}
	provChgUuMbmsSubscriptionMap = map[int]*ProvChgUuMbmsSubscription{}
	provChgPc5SubscriptionMap = map[int]*ProvChgPc5Subscription{}
	v2xMsgSubscriptionMap = map[int]*V2xMsgSubscription{}
	//predQosSubscriptionMap = map[int]*PredQosSubscription{} // FIXME FSCOM There is no PredQosNotification defined by the standard ETSI GS ETSI GS MEC 040 V3.2.1 (2024-03)

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

/*
 * predictedQosPost process the PredictedQos POST request and sends the response
 * @param {struct} w HTTP write reference
 * @param {struct} r contains the HTTP request
 * @see ETSI GS ETSI GS MEC 040 V3.2.1 (2024-03) Clause 7.7.3.4 POST
 */
func predictedQosPost(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> predictedQosPost: ", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	// For debug purpose only
	// bytes, err := ioutil.ReadAll(r.Body)
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// log.Debug("predictedQosPost: bytes: ", string(bytes))

	var requestData PredictedQos
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&requestData)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Make sure scenario is running
	if currentStoreName == "" {
		log.Error("Scenario not deployed")
		errHandlerProblemDetails(w, "Scenario not deployed.", http.StatusBadRequest)
		return
	}

	// Validating mandatory parameters in request
	if requestData.LocationGranularity == "" {
		log.Error("Mandatory locationGranularity parameter not present")
		errHandlerProblemDetails(w, "Mandatory attribute locationGranularity is missing in the request body.", http.StatusBadRequest)
		return
	} else {
		_, err := strconv.Atoi(requestData.LocationGranularity)
		if err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	// Validating mandatory parameters in request
	if requestData.Qos != nil {
		log.Error("Qos field shall not be present in the request")
		errHandlerProblemDetails(w, "Qos field shall not be present in the request.", http.StatusBadRequest)
		return
	}

	if requestData.PredictionTarget == "SINGLE_UE_PREDICTION" {
		if requestData.PredictionArea != nil {
			log.Error("PredictionArea field shall not be present when PredictionTarget is SINGLE_UE_PREDICTION")
			errHandlerProblemDetails(w, "PredictionArea field shall not be present when PredictionTarget is SINGLE_UE_PREDICTION in the request.", http.StatusBadRequest)
			return
		}
		if requestData.Routes == nil || len(requestData.Routes) == 0 {
			log.Error("Mandatory Routes field is either empty or not present")
			errHandlerProblemDetails(w, "Mandatory Routes field is either empty or not present in the request.", http.StatusBadRequest)
			return
		}
	} else if requestData.PredictionTarget == "E2E_APPLICATION_INSTANCE_PREDICTION" {
		if requestData.Routes != nil || len(requestData.Routes) != 0 {
			log.Error("Routes field shall not present when PredictionTarget is E2E_APPLICATION_INSTANCE_PREDICTION")
			errHandlerProblemDetails(w, "Routes field shall not present when PredictionTarget is E2E_APPLICATION_INSTANCE_PREDICTION in the request.", http.StatusBadRequest)
			return
		}
		if requestData.PredictionArea == nil {
			log.Error("PredictionArea field shall be present when PredictionTarget is E2E_APPLICATION_INSTANCE_PREDICTION")
			errHandlerProblemDetails(w, "PredictionArea field shall be present when PredictionTarget is E2E_APPLICATION_INSTANCE_PREDICTION in the request.", http.StatusBadRequest)
			return
		}
		log.Error("E2E_APPLICATION_INSTANCE_PREDICTION is not supported")
		errHandlerProblemDetails(w, "E2E_APPLICATION_INSTANCE_PREDICTION is not supported.", http.StatusNotImplemented)
		return
	} else {
		log.Error("Invalid PredictionTarget field value")
		errHandlerProblemDetails(w, "Invalid PredictionTarget field value in the request.", http.StatusBadRequest)
		return
	}

	// Set maximum number of routes
	if len(requestData.Routes) > 10 {
		log.Error("A maximum of 10 routes are supported in the sandbox")
		errHandlerProblemDetails(w, "A maximum of 10 routes are supported in the sandbox", http.StatusBadRequest)
		return
	}

	responseData := requestData // Both request and response have same data model

	for i, route := range requestData.Routes {
		if route.RouteInfo == nil {
			log.Error("Mandatory routeInfo parameter not present in routes")
			errHandlerProblemDetails(w, "Mandatory attribute routes.routeInfo not present in the request.", http.StatusBadRequest)
			return
		}

		if len(route.RouteInfo) < 2 {
			log.Error("At least two location points required in routeInfo")
			errHandlerProblemDetails(w, "At least two location points required in routeInfo structure.", http.StatusBadRequest)
			return
		}

		// Set maximum number of geo-coordinates for each route
		if len(route.RouteInfo) > 10 {
			log.Error("A maximum of 10 geo-coordinates are supported for each route")
			errHandlerProblemDetails(w, "A maximum of 10 geo-coordinates are supported for each route", http.StatusBadRequest)
			return
		}

		var geocoordinates []gisClient.GeoCoordinate
		for _, routeInfo := range route.RouteInfo {
			// empty location attribute will cause a runtime error: invalid memory address or nil pointer dereference
			if routeInfo.Location == nil || routeInfo.Location.GeoArea == nil {
				log.Error("Mandatory attribute location is either empty or not present in routeInfo")
				errHandlerProblemDetails(w, "Mandatory attribute routes.routeInfo.location is either empty or not present in the request in at least one of the routeInfo structures.", http.StatusBadRequest)
				return
			}

			if routeInfo.Location.Ecgi != nil {
				log.Error("Ecgi is not supported in location for MEC Sandbox")
				errHandlerProblemDetails(w, "Ecgi is not supported inside routes.routeInfo.location attribute, only geoArea is supported.", http.StatusBadRequest)
				return
			}

			isValidGeoArea := routeInfo.Location.GeoArea != nil && (routeInfo.Location.GeoArea.Latitude == 0 || routeInfo.Location.GeoArea.Longitude == 0)
			if isValidGeoArea {
				log.Error("Mandatory latitude/longitude parameter(s) either not present in geoArea or have a zero value")
				errHandlerProblemDetails(w, "At least one of the routes.routeInfo structures either does not contain mandatory latitude / longitude parameter(s) in geoArea or have zero value(s).", http.StatusBadRequest)
				return
			}

			if routeInfo.Time != nil && !predictionModelSupported {
				log.Error("routes.routeInfo.time is not supported for this scenario")
				errHandlerProblemDetails(w, "routes.routeInfo.time is not supported for this scenario", http.StatusBadRequest)
				return
			}

			geocoordinates = append(geocoordinates, gisClient.GeoCoordinate{
				Latitude:  routeInfo.Location.GeoArea.Latitude,
				Longitude: routeInfo.Location.GeoArea.Longitude,
			})
		}

		var geocoordinatesList gisClient.GeoCoordinateList
		geocoordinatesList.GeoCoordinates = geocoordinates
		powerResp, _, err := gisAppClient.GeospatialDataApi.GetGeoDataPowerValues(context.TODO(), geocoordinatesList)
		log.Debug("predictedQosPost: powerResp: ", powerResp)
		if err != nil {
			log.Error("Failed to communicate with gis engine: ", err)
			errHandlerProblemDetails(w, "Failed to communicate with gis engine.", http.StatusInternalServerError)
			return
		}
		routeInfoList := responseData.Routes[i].RouteInfo
		var qos = Qos{}
		for j, routeInfo := range routeInfoList {
			currGeoCoordinate := powerResp.CoordinatesPower[j]
			log.Debug("predictedQosPost: currGeoCoordinate: ", currGeoCoordinate)
			log.Debug("predictedQosPost: predictionModelSupported: ", predictionModelSupported)
			log.Debug("predictedQosPost: routeInfo.Time: ", routeInfo.Time)
			if predictionModelSupported && routeInfo.Time != nil {
				rsrp := currGeoCoordinate.Rsrp
				rsrq := currGeoCoordinate.Rsrq
				poaName := currGeoCoordinate.PoaName
				estTimeHour := int32(time.Unix(int64(routeInfo.Time.Seconds), int64(routeInfo.Time.NanoSeconds)).Hour())
				log.Debug("predictedQosPost: rsrp: ", rsrp)
				log.Debug("predictedQosPost: rsrq: ", rsrq)
				currGeoCoordinate.Rsrp, currGeoCoordinate.Rsrq, err = sbi.GetPredictedPowerValues(estTimeHour, rsrp, rsrq, poaName)
				if err != nil {
					log.Error(err.Error())
					if err.Error() == "Missing POA Name" {
						errHandlerProblemDetails(w, err.Error(), http.StatusNotFound)
						return
					} else {
						errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
						return
					}
				}
			}
			log.Debug("predictedQosPost: new currGeoCoordinate: ", currGeoCoordinate)
			latCheck := routeInfo.Location.GeoArea.Latitude == currGeoCoordinate.Latitude
			longCheck := routeInfo.Location.GeoArea.Longitude == currGeoCoordinate.Longitude
			log.Debug("predictedQosPost: latCheck: ", latCheck)
			log.Debug("predictedQosPost: longCheck: ", longCheck)
			if latCheck && longCheck {
				qosKpis := make([]QosKpi, 2)
				qosKpis[0] = QosKpi{Confidence: "1", KpiName: "rsrp", KpiValue: strconv.Itoa(int(currGeoCoordinate.Rsrp))}
				qosKpis[1] = QosKpi{Confidence: "1", KpiName: "rsrq", KpiValue: strconv.Itoa(int(currGeoCoordinate.Rsrq))}
				qos.Stream = append(qos.Stream, Stream{StreamId: strconv.Itoa(j), QosKpi: qosKpis})
			}
			routeInfo.Location.Ecgi = nil
		} // End of 'for' statement
		responseData.Qos = &qos
	}

	jsonResponse := convertPredictedQostoJson(&responseData)
	log.Debug("predictedQosPost: jsonResponse: ", jsonResponse)
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, jsonResponse)
}

/*
 * provInfoUuUnicastGET process the uu_unicast_provisioning_info GET request and sends the response
 * @param {struct} w HTTP write reference
 * @param {struct} r contains the HTTP request
 * @see ETSI GS ETSI GS MEC 040 V3.2.1 (2024-03) Clause 7.3.3.1 GET
 */
func provInfoUuUnicastGET(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> provInfoUuUnicastGET", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	// Retrieve query parameters
	u, _ := url.Parse(r.URL.String())
	log.Info("url: ", u.RequestURI())
	q := u.Query()
	log.Info("infoUuUnicastGET: q= ", q)
	validQueryParams := []string{"location_info"}
	if !validateQueryParams(q, validQueryParams) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Get & validate query param values
	location_info := q.Get("location_info")
	log.Info("infoUuUnicastGET: location_info= ", location_info)
	// Extract parameters
	params := strings.Split(location_info, ",")
	log.Info("infoUuUnicastGET: args= ", params)

	if !validateQueryParamValue(params[0], []string{"ecgi", "latitude"}) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	// Extract list of items
	var i int
	for i = 1; i < len(params); i += 1 {
		if validateQueryParamValue(params[i], []string{"longitude"}) {
			break
		}
	} // End of 'for' statement
	i -= 1
	log.Info("infoUuUnicastGET: i= ", i)
	log.Info("infoUuUnicastGET: (len(params)-2)/2= ", (len(params)-2)/2)
	if i < 1 || ((params[0] == "latitude") && (i != (len(params)-2)/2)) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Process the request
	log.Info("infoUuUnicastGET: Process the request")
	resp, err := sbi.GetInfoUuUnicast(params, i)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}

	proInfoUuUnicast := make([]UuUnicastProvisioningInfoProInfoUuUnicast, len(resp))
	if len(resp) == 0 {
		err = errors.New("Cannot find UuUnicastProvisioningInfo")
		errHandlerProblemDetails(w, err.Error(), http.StatusNotFound)
		return
	}
	for i := range resp {
		if resp[i].LocationInfo != nil {
			proInfoUuUnicast[i].LocationInfo = new(LocationInfo)
			if resp[i].LocationInfo.Ecgi != nil {
				proInfoUuUnicast[i].LocationInfo.Ecgi = new(Ecgi)
				if resp[i].LocationInfo.Ecgi.CellId != nil {
					proInfoUuUnicast[i].LocationInfo.Ecgi.CellId = new(CellId)
					proInfoUuUnicast[i].LocationInfo.Ecgi.CellId.CellId = resp[i].LocationInfo.Ecgi.CellId.CellId
				}
				if resp[i].LocationInfo.Ecgi.Plmn != nil {
					proInfoUuUnicast[i].LocationInfo.Ecgi.Plmn = new(Plmn)
					proInfoUuUnicast[i].LocationInfo.Ecgi.Plmn.Mcc = resp[i].LocationInfo.Ecgi.Plmn.Mcc
					proInfoUuUnicast[i].LocationInfo.Ecgi.Plmn.Mnc = resp[i].LocationInfo.Ecgi.Plmn.Mnc
				}
			}
			if resp[i].LocationInfo.GeoArea != nil {
				proInfoUuUnicast[i].LocationInfo.GeoArea = new(LocationInfoGeoArea)
				proInfoUuUnicast[i].LocationInfo.GeoArea.Latitude = resp[i].LocationInfo.GeoArea.Latitude
				proInfoUuUnicast[i].LocationInfo.GeoArea.Longitude = resp[i].LocationInfo.GeoArea.Longitude
			}
		}

		if resp[i].NeighbourCellInfo != nil {
			proInfoUuUnicast[i].NeighbourCellInfo = make([]UuUniNeighbourCellInfo, len(resp[i].NeighbourCellInfo))
			for j := range resp[i].NeighbourCellInfo {

				if resp[i].NeighbourCellInfo[j].Ecgi != nil {
					proInfoUuUnicast[i].NeighbourCellInfo[j].Ecgi = new(Ecgi)
					if resp[i].NeighbourCellInfo[j].Ecgi.CellId != nil {
						proInfoUuUnicast[i].NeighbourCellInfo[j].Ecgi.CellId = new(CellId)
						proInfoUuUnicast[i].NeighbourCellInfo[j].Ecgi.CellId.CellId = resp[i].NeighbourCellInfo[j].Ecgi.CellId.CellId
					}
					if resp[i].NeighbourCellInfo[j].Ecgi.Plmn != nil {
						proInfoUuUnicast[i].NeighbourCellInfo[j].Ecgi.Plmn = new(Plmn)
						proInfoUuUnicast[i].NeighbourCellInfo[j].Ecgi.Plmn.Mcc = resp[i].NeighbourCellInfo[j].Ecgi.Plmn.Mcc
						proInfoUuUnicast[i].NeighbourCellInfo[j].Ecgi.Plmn.Mnc = resp[i].NeighbourCellInfo[j].Ecgi.Plmn.Mnc
					}
				}
				proInfoUuUnicast[i].NeighbourCellInfo[j].FddInfo = nil // FIXME Not supported yet
				proInfoUuUnicast[i].NeighbourCellInfo[j].Pci = resp[i].NeighbourCellInfo[j].Pci
				if resp[i].NeighbourCellInfo[j].Plmn != nil {
					proInfoUuUnicast[i].NeighbourCellInfo[j].Plmn = new(Plmn)
					proInfoUuUnicast[i].NeighbourCellInfo[j].Plmn.Mcc = resp[i].NeighbourCellInfo[j].Plmn.Mcc
					proInfoUuUnicast[i].NeighbourCellInfo[j].Plmn.Mnc = resp[i].NeighbourCellInfo[j].Plmn.Mnc
				}
				proInfoUuUnicast[i].NeighbourCellInfo[j].TddInfo = nil // FIXME Not supported yet
			} // End of 'for' statement
		}
		if resp[i].V2xApplicationServer != nil {
			proInfoUuUnicast[i].V2xApplicationServer = new(V2xApplicationServer)
			proInfoUuUnicast[i].V2xApplicationServer.IpAddress = resp[i].V2xApplicationServer.IpAddress
			proInfoUuUnicast[i].V2xApplicationServer.UdpPort = resp[i].V2xApplicationServer.UdpPort
		}
	} // End of 'for' statement
	uuUnicastProvisioningInfo := UuUnicastProvisioningInfo{
		ProInfoUuUnicast: proInfoUuUnicast,
		TimeStamp: &TimeStamp{
			Seconds: int32(time.Now().Unix()),
		},
	}
	log.Info("infoUuUnicastGET: uuUnicastProvisioningInfo: ", uuUnicastProvisioningInfo)

	// Send response
	jsonResponse, err := json.Marshal(uuUnicastProvisioningInfo)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Info("infoUuUnicastGET: Response: ", string(jsonResponse))
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(jsonResponse))
}

/*
 * provInfoUuMbmsGET process the uu_mbms_provisioning_info GET request and sends the response
 * @param {struct} w HTTP write reference
 * @param {struct} r contains the HTTP request
 * @see ETSI GS ETSI GS MEC 040 V3.2.1 (2024-03) Clause 7.4.3.1 GET
 */
func provInfoUuMbmsGET(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> provInfoUuMbmsGET", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	// Retrieve query parameters
	u, _ := url.Parse(r.URL.String())
	log.Info("url: ", u.RequestURI())
	q := u.Query()
	log.Info("infoUuMbmscastGET: q= ", q)
	validQueryParams := []string{"location_info"}
	if !validateQueryParams(q, validQueryParams) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Get & validate query param values
	location_info := q.Get("location_info")
	log.Info("infoUuMbmscastGET: location_info= ", location_info)
	// Extract parameters
	params := strings.Split(location_info, ",")
	log.Info("infoUuMbmscastGET: args= ", params)

	if !validateQueryParamValue(params[0], []string{"ecgi", "latitude"}) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	// Extract list of items
	var i int
	for i = 1; i < len(params); i += 1 {
		if validateQueryParamValue(params[i], []string{"longitude"}) {
			break
		}
	} // End of 'for' statement
	i -= 1
	log.Info("infoUuMbmscastGET: i= ", i)
	log.Info("infoUuMbmscastGET: (len(params)-2)/2= ", (len(params)-2)/2)
	if i < 1 || ((params[0] == "latitude") && (i != (len(params)-2)/2)) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Process the request
	log.Info("infoUuMbmscastGET: Process the request")
	resp, err := sbi.GetInfoUuMbmscast(params, i)
	if err != nil {
		log.Error(err.Error())
		if strings.Contains(err.Error(), "Cannot find") {
			errHandlerProblemDetails(w, err.Error(), http.StatusNotFound)
		} else {
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	log.Info("infoUuMbmscastGET: len(resp): ", len(resp))
	if len(resp) == 0 {
		err = errors.New("Cannot find UuUnicastProvisioningInfo")
		errHandlerProblemDetails(w, err.Error(), http.StatusNotFound)
		return
	}
	proInfoUuMbmscast := make([]UuMbmsProvisioningInfoProInfoUuMbms, len(resp))
	for i := range resp {
		if resp[i].LocationInfo != nil {
			proInfoUuMbmscast[i].LocationInfo = new(LocationInfo)
			if resp[i].LocationInfo.Ecgi != nil {
				proInfoUuMbmscast[i].LocationInfo.Ecgi = new(Ecgi)
				if resp[i].LocationInfo.Ecgi.CellId != nil {
					proInfoUuMbmscast[i].LocationInfo.Ecgi.CellId = new(CellId)
					proInfoUuMbmscast[i].LocationInfo.Ecgi.CellId.CellId = resp[i].LocationInfo.Ecgi.CellId.CellId
				}
				if resp[i].LocationInfo.Ecgi.Plmn != nil {
					proInfoUuMbmscast[i].LocationInfo.Ecgi.Plmn = new(Plmn)
					proInfoUuMbmscast[i].LocationInfo.Ecgi.Plmn.Mcc = resp[i].LocationInfo.Ecgi.Plmn.Mcc
					proInfoUuMbmscast[i].LocationInfo.Ecgi.Plmn.Mnc = resp[i].LocationInfo.Ecgi.Plmn.Mnc
				}
			}
			if resp[i].LocationInfo.GeoArea != nil {
				proInfoUuMbmscast[i].LocationInfo.GeoArea = new(LocationInfoGeoArea)
				proInfoUuMbmscast[i].LocationInfo.GeoArea.Latitude = resp[i].LocationInfo.GeoArea.Latitude
				proInfoUuMbmscast[i].LocationInfo.GeoArea.Longitude = resp[i].LocationInfo.GeoArea.Longitude
			}
		}

		if resp[i].NeighbourCellInfo != nil {
			proInfoUuMbmscast[i].NeighbourCellInfo = make([]UuMbmsNeighbourCellInfo, len(resp[i].NeighbourCellInfo))
			for j := range resp[i].NeighbourCellInfo {

				if resp[i].NeighbourCellInfo[j].Ecgi != nil {
					proInfoUuMbmscast[i].NeighbourCellInfo[j].Ecgi = new(Ecgi)
					if resp[i].NeighbourCellInfo[j].Ecgi.CellId != nil {
						proInfoUuMbmscast[i].NeighbourCellInfo[j].Ecgi.CellId = new(CellId)
						proInfoUuMbmscast[i].NeighbourCellInfo[j].Ecgi.CellId.CellId = resp[i].NeighbourCellInfo[j].Ecgi.CellId.CellId
					}
					if resp[i].NeighbourCellInfo[j].Ecgi.Plmn != nil {
						proInfoUuMbmscast[i].NeighbourCellInfo[j].Ecgi.Plmn = new(Plmn)
						proInfoUuMbmscast[i].NeighbourCellInfo[j].Ecgi.Plmn.Mcc = resp[i].NeighbourCellInfo[j].Ecgi.Plmn.Mcc
						proInfoUuMbmscast[i].NeighbourCellInfo[j].Ecgi.Plmn.Mnc = resp[i].NeighbourCellInfo[j].Ecgi.Plmn.Mnc
					}
				}
				proInfoUuMbmscast[i].NeighbourCellInfo[j].FddInfo = nil // FIXME Not supported yet
				proInfoUuMbmscast[i].NeighbourCellInfo[j].Pci = resp[i].NeighbourCellInfo[j].Pci
				if resp[i].NeighbourCellInfo[j].Plmn != nil {
					proInfoUuMbmscast[i].NeighbourCellInfo[j].Plmn = new(Plmn)
					proInfoUuMbmscast[i].NeighbourCellInfo[j].Plmn.Mcc = resp[i].NeighbourCellInfo[j].Plmn.Mcc
					proInfoUuMbmscast[i].NeighbourCellInfo[j].Plmn.Mnc = resp[i].NeighbourCellInfo[j].Plmn.Mnc
				}
				proInfoUuMbmscast[i].NeighbourCellInfo[j].TddInfo = nil // FIXME Not supported yet
			} // End of 'for' statement
		}
		if resp[i].V2xServerUsd != nil {
			proInfoUuMbmscast[i].V2xServerUsd = new(V2xServerUsd)
			if proInfoUuMbmscast[i].V2xServerUsd.SdpInfo != nil {
				proInfoUuMbmscast[i].V2xServerUsd.SdpInfo = new(V2xServerUsdSdpInfo)
				proInfoUuMbmscast[i].V2xServerUsd.SdpInfo.IpMulticastAddress = resp[i].V2xServerUsd.SdpInfo.IpMulticastAddress
				proInfoUuMbmscast[i].V2xServerUsd.SdpInfo.PortNumber = resp[i].V2xServerUsd.SdpInfo.PortNumber
			}
			proInfoUuMbmscast[i].V2xServerUsd.ServiceAreaIdentifier = resp[i].V2xServerUsd.ServiceAreaIdentifier
			if proInfoUuMbmscast[i].V2xServerUsd.Tmgi != nil {
				proInfoUuMbmscast[i].V2xServerUsd.Tmgi = new(V2xServerUsdTmgi)
				proInfoUuMbmscast[i].V2xServerUsd.Tmgi.MbmsServiceId = resp[i].V2xServerUsd.Tmgi.MbmsServiceId
				proInfoUuMbmscast[i].V2xServerUsd.Tmgi.Mcc = resp[i].V2xServerUsd.Tmgi.Mcc
				proInfoUuMbmscast[i].V2xServerUsd.Tmgi.Mnc = resp[i].V2xServerUsd.Tmgi.Mnc
			}
		} else { // ETSI GS MEC 030 V3.3.1 (2024-06) Clause 6.2.3 Type: UuMbmsProvisioningInfo: 2xServerUsd cardinality is 1
			proInfoUuMbmscast[i].V2xServerUsd = new(V2xServerUsd) // Keep it empty
			proInfoUuMbmscast[i].V2xServerUsd.SdpInfo = new(V2xServerUsdSdpInfo)
			proInfoUuMbmscast[i].V2xServerUsd.SdpInfo.IpMulticastAddress = ""
			proInfoUuMbmscast[i].V2xServerUsd.SdpInfo.PortNumber = ""
			proInfoUuMbmscast[i].V2xServerUsd.ServiceAreaIdentifier = []string{}
			proInfoUuMbmscast[i].V2xServerUsd.Tmgi = new(V2xServerUsdTmgi)
			proInfoUuMbmscast[i].V2xServerUsd.Tmgi.MbmsServiceId = ""
			proInfoUuMbmscast[i].V2xServerUsd.Tmgi.Mcc = ""
			proInfoUuMbmscast[i].V2xServerUsd.Tmgi.Mnc = ""
		}
	} // End of 'for' statement
	uuMbmsProvisioningInfo := UuMbmsProvisioningInfo{
		ProInfoUuMbms: proInfoUuMbmscast,
		TimeStamp: &TimeStamp{
			Seconds: int32(time.Now().Unix()),
		},
	}
	log.Info("infoUuMbmscastGET: uuMbmsProvisioningInfo: ", uuMbmsProvisioningInfo)

	// Send response
	jsonResponse, err := json.Marshal(uuMbmsProvisioningInfo)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Info("infoUuMbmscastGET: Response: ", string(jsonResponse))
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(jsonResponse))
}

/*
 * provInfoPc5GET process the pc5_provisioning_info GET request and sends the response
 * @param {struct} w HTTP write reference
 * @param {struct} r contains the HTTP request
 * @see ETSI GS ETSI GS MEC 040 V3.2.1 (2024-03) Clause 7.5.3.1 GET
 */
func provInfoPc5GET(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> provInfoPc5GET", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	// Retrieve query parameters
	u, _ := url.Parse(r.URL.String())
	log.Info("url: ", u.RequestURI())
	q := u.Query()
	log.Info("infoUuMbmscastGET: q= ", q)
	validQueryParams := []string{"location_info"}
	if !validateQueryParams(q, validQueryParams) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Get & validate query param values
	location_info := q.Get("location_info")
	log.Info("provInfoPc5GET: location_info= ", location_info)
	// Extract parameters
	params := strings.Split(location_info, ",")
	log.Info("provInfoPc5GET: args= ", params)

	if !validateQueryParamValue(params[0], []string{"ecgi", "latitude"}) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	// Extract list of items
	var i int
	for i = 1; i < len(params); i += 1 {
		if validateQueryParamValue(params[i], []string{"longitude"}) {
			break
		}
	} // End of 'for' statement
	i -= 1
	log.Info("provInfoPc5GET: i= ", i)
	log.Info("provInfoPc5GET: (len(params)-2)/2= ", (len(params)-2)/2)
	if i < 1 || ((params[0] == "latitude") && (i != (len(params)-2)/2)) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Process the request
	log.Info("provInfoPc5GET: Process the request")
	resp, err := sbi.GetInfoPc5(params, i)
	if err != nil {
		log.Error(err.Error())
		if strings.Contains(err.Error(), "Cannot find") {
			errHandlerProblemDetails(w, err.Error(), http.StatusNotFound)
		} else {
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	proInfoPc5 := make([]Pc5ProvisioningInfoProInfoPc5, len(resp))
	for i := range resp {
		proInfoPc5[i].DstLayer2Id = resp[i].DstLayer2Id

		if resp[i].LocationInfo != nil {
			proInfoPc5[i].LocationInfo = new(LocationInfo)
			if resp[i].LocationInfo.Ecgi != nil {
				proInfoPc5[i].LocationInfo.Ecgi = new(Ecgi)
				if resp[i].LocationInfo.Ecgi.CellId != nil {
					proInfoPc5[i].LocationInfo.Ecgi.CellId = new(CellId)
					proInfoPc5[i].LocationInfo.Ecgi.CellId.CellId = resp[i].LocationInfo.Ecgi.CellId.CellId
				}
				if resp[i].LocationInfo.Ecgi.Plmn != nil {
					proInfoPc5[i].LocationInfo.Ecgi.Plmn = new(Plmn)
					proInfoPc5[i].LocationInfo.Ecgi.Plmn.Mcc = resp[i].LocationInfo.Ecgi.Plmn.Mcc
					proInfoPc5[i].LocationInfo.Ecgi.Plmn.Mnc = resp[i].LocationInfo.Ecgi.Plmn.Mnc
				}
			}
			if resp[i].LocationInfo.GeoArea != nil {
				proInfoPc5[i].LocationInfo.GeoArea = new(LocationInfoGeoArea)
				proInfoPc5[i].LocationInfo.GeoArea.Latitude = resp[i].LocationInfo.GeoArea.Latitude
				proInfoPc5[i].LocationInfo.GeoArea.Longitude = resp[i].LocationInfo.GeoArea.Longitude
			}
		}

		if resp[i].NeighbourCellInfo != nil {
			proInfoPc5[i].NeighbourCellInfo = make([]Pc5NeighbourCellInfo, len(resp[i].NeighbourCellInfo))
			for j := range resp[i].NeighbourCellInfo {

				if resp[i].NeighbourCellInfo[j].Ecgi != nil {
					proInfoPc5[i].NeighbourCellInfo[j].Ecgi = new(Ecgi)
					if resp[i].NeighbourCellInfo[j].Ecgi.CellId != nil {
						proInfoPc5[i].NeighbourCellInfo[j].Ecgi.CellId = new(CellId)
						proInfoPc5[i].NeighbourCellInfo[j].Ecgi.CellId.CellId = resp[i].NeighbourCellInfo[j].Ecgi.CellId.CellId
					}
					if resp[i].NeighbourCellInfo[j].Ecgi.Plmn != nil {
						proInfoPc5[i].NeighbourCellInfo[j].Ecgi.Plmn = new(Plmn)
						proInfoPc5[i].NeighbourCellInfo[j].Ecgi.Plmn.Mcc = resp[i].NeighbourCellInfo[j].Ecgi.Plmn.Mcc
						proInfoPc5[i].NeighbourCellInfo[j].Ecgi.Plmn.Mnc = resp[i].NeighbourCellInfo[j].Ecgi.Plmn.Mnc
					}
				}
				if resp[i].NeighbourCellInfo[j].Plmn != nil {
					proInfoPc5[i].NeighbourCellInfo[j].Plmn = new(Plmn)
					proInfoPc5[i].NeighbourCellInfo[j].Plmn.Mcc = resp[i].NeighbourCellInfo[j].Plmn.Mcc
					proInfoPc5[i].NeighbourCellInfo[j].Plmn.Mnc = resp[i].NeighbourCellInfo[j].Plmn.Mnc
				}
				if resp[i].NeighbourCellInfo[j].SiV2xConfig != nil {
					proInfoPc5[i].NeighbourCellInfo[j].SiV2xConfig = new(SystemInformationBlockType21)
				}
			} // End of 'for' statement
		}
	} // End of 'for' statement
	pc5ProvisioningInfo := Pc5ProvisioningInfo{
		ProInfoPc5: proInfoPc5,
		TimeStamp: &TimeStamp{
			Seconds: int32(time.Now().Unix()),
		},
	}
	log.Info("provInfoPc5GET: pc5ProvisioningInfo: ", pc5ProvisioningInfo)

	// Send response
	jsonResponse, err := json.Marshal(pc5ProvisioningInfo)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Info("provInfoPc5GET: Response: ", string(jsonResponse))
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(jsonResponse))
}

/*
 * v2xMsgDistributionServerPost process the V2x message distribution server POST request and sends the response
 * @param {struct} w HTTP write reference
 * @param {struct} r contains the HTTP request
 * @see ETSI GS ETSI GS MEC 040 V3.2.1 (2024-03) Clause 7.6.3.1 GET
 */
func v2xMsgDistributionServerPost(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> v2xMsgDistributionServerPost: ", r)
	log.Debug(">>> v2xMsgDistributionServerPost: v2x_broker: ", v2x_broker)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	// Read JSON input stream provided in the Request, and stores it in the bodyBytes as bytes
	var v2xMsgDistributionServerInfo V2xMsgDistributionServerInfo
	bodyBytes, _ := ioutil.ReadAll(r.Body)
	// Unmarshal function to converts a JSON-formatted string into a SubscriptionCommon struct and store it in extractSubType
	err := json.Unmarshal(bodyBytes, &v2xMsgDistributionServerInfo)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Info("v2xMsgDistributionServerInfo: ", v2xMsgDistributionServerInfo)

	if len(v2xMsgDistributionServerInfo.V2xMsgDistributionServer) == 0 {
		log.Error("At least one V2xMsgDistributionServer parameters should be present")
		errHandlerProblemDetails(w, "V2xMsgDistributionServer parameters are missing in the request body.", http.StatusBadRequest)
		return
	} else {
		for _, v2xMsgDistributionServer := range v2xMsgDistributionServerInfo.V2xMsgDistributionServer {
			if v2xMsgDistributionServer.InfoConnection != nil {
				log.Error("InfoConnection parameters shall not be present")
				errHandlerProblemDetails(w, "InfoConnection parameters shall not be present", http.StatusBadRequest)
				return
			}
			if v2xMsgDistributionServer.InfoProtocol == nil {
				log.Error("At least one InfoProtocol parameters should be present")
				errHandlerProblemDetails(w, "InfoProtocol parameters are missing in the request body.", http.StatusBadRequest)
				return
			} else {
				if len(v2xMsgDistributionServer.InfoProtocol.MsgProtocol) == 0 {
					log.Error("At least one MsgProtocol parameters should be present")
					errHandlerProblemDetails(w, "MsgProtocol parameters are missing in the request body.", http.StatusBadRequest)
					return
				}
			}
		} // End of 'for'statement
	}

	u, err := url.ParseRequestURI(v2x_broker)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Info("v2xMsgDistributionServerPost: scheme:", u.Scheme, " - host:", u.Hostname(), " - Path:", u.Path, " - Port:", u.Port())
	portNumber, err := strconv.Atoi(u.Port())
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}

	v2xMsgDistributionServerInfoResp := v2xMsgDistributionServerInfo // Same format
	for i, v2xMsgDistributionServer := range v2xMsgDistributionServerInfo.V2xMsgDistributionServer {
		for _, msgProtocol := range v2xMsgDistributionServer.InfoProtocol.MsgProtocol {
			if msgProtocol == 0 || msgProtocol == 1 { // MQTT v3.1.0 or MQTT v3.1.1
				v2xMsgDistributionServerInfoResp.V2xMsgDistributionServer[i].InfoConnection = &InfoConnection{IpAddress: u.Hostname(), PortNumber: int32(portNumber)}
			} else {
				log.Warn("v2xMsgDistributionServerPost: Unsupported MsgProtocol: ", msgProtocol)
				err := errors.New("Unsupported MsgProtocol")
				errHandlerProblemDetails(w, err.Error(), http.StatusNotFound)
				return
			}
		} // End of 'for'statement
	} // End of 'for'statement
	log.Info("v2xMsgDistributionServerPost: v2xMsgDistributionServerInfoResp.V2xMsgDistributionServer: ", v2xMsgDistributionServerInfoResp.V2xMsgDistributionServer)

	jsonResponse, err := json.Marshal(v2xMsgDistributionServerInfoResp)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}
	log.Info("jsonResponse: ", jsonResponse)

	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(jsonResponse))
}

func v2xMsgPublicationPOST(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> V2xMsgPublicationPOST: ", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	// Read JSON input stream provided in the Request, and stores it in the bodyBytes as bytes
	bodyBytes, _ := ioutil.ReadAll(r.Body)
	// Unmarshal function to converts a JSON-formatted string into a V2xMsgPublication struct and store it in v2xMsgPubReq
	var v2xMsgPubReq V2xMsgPublication
	err := json.Unmarshal(bodyBytes, &v2xMsgPubReq)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Validating mandatory parameters provided in the request body
	if v2xMsgPubReq.MsgPropertiesValues == nil { // ETSI GS MEC 030 V3.1.1 Clause 6.2.7 Type: V2xMsgPublication
		log.Error("Mandatory MsgPropertiesValues parameter should be present")
		errHandlerProblemDetails(w, "Mandatory attribute MsgPropertiesValues is missing in the request body.", http.StatusBadRequest)
		return
	}

	var msgPropertiesValues V2xMsgPropertiesValues = *v2xMsgPubReq.MsgPropertiesValues
	if msgPropertiesValues.StdOrganization == "" {
		log.Error("Mandatory StdOrganization parameter should be present")
		errHandlerProblemDetails(w, "Mandatory attribute StdOrganization is missing in the request body.", http.StatusBadRequest)
		return
	}

	if msgPropertiesValues.MsgType == "" {
		log.Error("Mandatory MsgType parameter should be present")
		errHandlerProblemDetails(w, "Mandatory attribute MsgType is missing in the request body.", http.StatusBadRequest)
		return
	}
	var msgType int32 = parseMsgTypeToInt(msgPropertiesValues.MsgType)
	if msgType == -1 {
		log.Error("Mandatory MsgType parameter should be present")
		errHandlerProblemDetails(w, "Mandatory attribute MsgType is missing in the request body.", http.StatusBadRequest)
		return
	}

	if v2xMsgPubReq.MsgContent == "" {
		log.Error("Mandatory MsgContent parameter should be present")
		errHandlerProblemDetails(w, "Mandatory attribute MsgContent is missing in the request body.", http.StatusBadRequest)
		return
	}

	if len(v2xMsgSubscriptionMap) != 0 { // There are some subscription ongoing, we can publish it
		// Publish message on message broker
		err = sbi.PublishMessageOnMessageBroker(v2xMsgPubReq.MsgContent, v2xMsgPubReq.MsgRepresentationFormat, msgPropertiesValues.StdOrganization, &msgType)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}
		w.WriteHeader(http.StatusNoContent)
	} else { // No subscription ongoing, discard it
		log.Error("No subscription ongoing, discard it")
		errHandlerProblemDetails(w, "No subscription ongoing, discard it.", http.StatusBadRequest)
		return
	}
}

/*
 * sendV2xMsgNotification sends notification to the call reference address
 * @param {string} notifyUrl contains the call reference address
 * @param {struct} notification contains notification body of type V2xMsgNotification
 * @see ETSI GS ETSI GS MEC 040 V3.2.1 (2024-03) Clause 7.9.3.4 POST
 * @see ETSI GS ETSI GS MEC 040 V3.2.1 (2024-03) Clause 6.4.5 Type: V2xMsgNotification
 */
func sendV2xMsgNotification(notifyUrl string, notification V2xMsgNotification) {
	startTime := time.Now()
	jsonNotif, err := json.Marshal(notification)
	if err != nil {
		log.Error(err)
		return
	}
	log.Info("sendV2xMsgNotification: jsonNotif: ", string(jsonNotif))

	resp, err := http.Post(notifyUrl, "application/json", bytes.NewBuffer(jsonNotif))
	log.Info("sendV2xMsgNotification: resp: ", resp)
	duration := float64(time.Since(startTime).Microseconds()) / 1000.0
	_ = httpLog.LogNotification(notifyUrl, "POST", "", "", string(jsonNotif), resp, startTime)
	if err != nil {
		log.Error(err)
		met.ObserveNotification(sandboxName, serviceName, V2X_MSG_NOTIF, notifyUrl, nil, duration)
		return
	}
	met.ObserveNotification(sandboxName, serviceName, V2X_MSG_NOTIF, notifyUrl, resp, duration)
	defer resp.Body.Close()
}

/*
 * subscriptionsPost is to create subscription at /subscriptions endpoint
 * @param {struct} w HTTP write reference
 * @param {struct} r contains the HTTP request
 * @see ETSI GS ETSI GS MEC 040 V3.2.1 (2024-03) Clause 7.9.3.4 POST
 */
func subscriptionsPost(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> subscriptionsPost")

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	var subscriptionCommon SubscriptionCommon
	// Read JSON input stream provided in the Request, and stores it in the bodyBytes as bytes
	bodyBytes, _ := ioutil.ReadAll(r.Body)
	log.Info("subscriptionsPost: bodyBytes: ", string(bodyBytes))
	// Unmarshal function to converts a JSON-formatted string into a SubscriptionCommon struct and store it in extractSubType
	err := json.Unmarshal(bodyBytes, &subscriptionCommon)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Info("subscriptionsPost: subscriptionCommon: ", subscriptionCommon)

	// Validating mandatory parameters provided in the request body
	if subscriptionCommon.SubscriptionType == "" {
		log.Error("Mandatory SubscriptionType parameter should be present")
		errHandlerProblemDetails(w, "Mandatory attribute SubscriptionType is missing in the request body.", http.StatusBadRequest)
		return
	}

	if subscriptionCommon.SubscriptionType != "ProvChgUuUniSubscription" && subscriptionCommon.SubscriptionType != "ProvChgUuMbmsSubscription" && subscriptionCommon.SubscriptionType != "ProvChgPc5Subscription" && subscriptionCommon.SubscriptionType != "V2xMsgSubscription" && subscriptionCommon.SubscriptionType != "PredQosSubscription" {
		log.Error("Invalid SubscriptionType")
		errHandlerProblemDetails(w, "Invalid SubscriptionType", http.StatusBadRequest)
		return
	}

	if subscriptionCommon.CallbackReference == "" && subscriptionCommon.WebsockNotifConfig == nil {
		log.Error("At least one of CallbackReference and WebsockNotifConfig parameters should be present")
		errHandlerProblemDetails(w, "At least one of CallbackReference and WebsockNotifConfig parameters should be present.", http.StatusBadRequest)
		return
	}

	// extract subscription type
	subscriptionType := subscriptionCommon.SubscriptionType

	// subscriptionId will be generated sequentially
	newSubsId := nextSubscriptionIdAvailable
	nextSubscriptionIdAvailable++
	subsIdStr := strconv.Itoa(newSubsId)

	// create a unique link for every subscription and concatenate subscription to it
	link := new(Links)
	self := new(LinkType)
	self.Href = hostUrl.String() + basePath + "subscriptions/" + subsIdStr
	link.Self = self

	// switch statement is based on provided subscriptionType in the request body
	var jsonResponse string
	switch subscriptionType {
	case PROV_CHG_UU_UNI:
		var provChgUuUniSubscription ProvChgUuUniSubscription
		jsonResponse, err = processProvChgUuUniSubscription(bodyBytes, link, subsIdStr, &provChgUuUniSubscription)
		if err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Location", provChgUuUniSubscription.Links.Self.Href)

	case PROV_CHG_UU_MBMS:
		var provChgUuMbmsSubscription ProvChgUuMbmsSubscription
		jsonResponse, err = processProvChgUuMbmsSubscription(bodyBytes, link, subsIdStr, &provChgUuMbmsSubscription)
		if err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Location", provChgUuMbmsSubscription.Links.Self.Href)

	case PROV_CHG_PC5:
		var provChgPc5Subscription ProvChgPc5Subscription
		jsonResponse, err = processProvChgPc5Subscription(bodyBytes, link, subsIdStr, &provChgPc5Subscription)
		if err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Location", provChgPc5Subscription.Links.Self.Href)

	case V2X_MSG:
		var v2xSubscription V2xMsgSubscription
		jsonResponse, err = processV2xMsgSubscription(bodyBytes, link, subsIdStr, &v2xSubscription)
		if err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Location", v2xSubscription.Links.Self.Href)

	case PRED_QOS: // FIXME FSCOM There is no PredQosNotification defined by the standard ETSI GS ETSI GS MEC 040 V3.2.1 (2024-03)
		errHandlerProblemDetails(w, "There is no PredQosNotification defined by the standard ETSI GS ETSI GS MEC 040 V3.2.1 (2024-03)", http.StatusBadRequest)
		return
		// var predQosSubscription PredQosSubscription
		// jsonResponse, err = processPredQosSubscription(bodyBytes, link, subsIdStr, &predQosSubscription)
		// if err != nil {
		// 	log.Error(err.Error())
		// 	errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		// 	return
		// }
		//w.Header().Set("Location", predQosSubscription.Links.Self.Href)

	default:
		log.Error("Unsupported subscriptionType")
		return
	}
	log.Info("subscriptionsPost: jsonResponse: ", jsonResponse)

	// Prepare & send response
	w.WriteHeader(http.StatusCreated)
	fmt.Fprint(w, jsonResponse)

	if subscriptionCommon.RequestTestNotification {
		links := new(TestNotificationLinks)
		links.Subscription = self
		testNotification := TestNotification{
			Links:            links,
			NotificationType: TEST_NOTIF,
		}
		log.Info("subscriptionsPost: testNotification: ", testNotification)
		sendTestNotification(subscriptionCommon.CallbackReference, testNotification)
	}
}

/*
 * processProvChgUuUniSubscription is to create subscription at /subscriptions endpoint
 * @param {struct} w HTTP write reference
 * @param {struct} r contains the HTTP request
 * @see ETSI GS ETSI GS MEC 040 V3.2.1 (2024-03) Clause 7.9.3.4 POST
 */
func processProvChgUuUniSubscription(bodyBytes []byte, link *Links, subsIdStr string, provChgUuUniSubscription *ProvChgUuUniSubscription) (string, error) {

	err := json.Unmarshal(bodyBytes, provChgUuUniSubscription)
	if err != nil {
		log.Error(err.Error())
		return "", err
	}

	// Validating mandatory parameters provided in the request body
	if provChgUuUniSubscription.Links != nil {
		err = errors.New("Links attribute should not be present in request body")
		log.Error(err.Error())
		return "", err
	}

	// FIXME FSCOM Check filter values
	if provChgUuUniSubscription.FilterCriteria == nil {
		err = errors.New("Mandatory FilterCriteria parameter should be present")
		log.Error(err.Error())
		return "", err
	}

	if provChgUuUniSubscription.WebsockNotifConfig == nil && provChgUuUniSubscription.CallbackReference == "" {
		err = errors.New("Mandatory CallbackReference parameter should be present")
		log.Error(err.Error())
		return "", err
	}
	if provChgUuUniSubscription.WebsockNotifConfig != nil {
		err = errors.New("WebsockNotifConfig not supported")
		log.Error(err.Error())
		return "", err
	}
	if provChgUuUniSubscription.CallbackReference == "" {
		err = errors.New("CallbackReference parameter should be present")
		log.Error(err.Error())
		return "", err
	}

	registerProvChgUuUniSubscription(subsIdStr, nil, provChgUuUniSubscription)

	provChgUuUniSubscription.Links = link

	// Store subscription key in redis
	keyName := baseKey + "subscriptions:" + subsIdStr
	log.Info("processProvChgUuUniSubscription: keyName: ", keyName)
	log.Info("processProvChgUuUniSubscription: provChgUuUniSubscription: ", provChgUuUniSubscription)
	err = rc.JSONSetEntry(keyName, ".", convertProvChgUuUniSubscriptionToJson(provChgUuUniSubscription))
	if err != nil {
		log.Error(err.Error())
		return "", err
	}

	jsonResponse := convertProvChgUuUniSubscriptionToJson(provChgUuUniSubscription)

	return jsonResponse, nil
}

func processProvChgUuUniSubscriptionUpdate(bodyBytes []byte, subsIdStr string) ([]byte, bool, error) {
	log.Debug(">>> processProvChgUuUniSubscriptionUpdate: subsIdStr: ", subsIdStr)

	var provChgUuUniSubscription ProvChgUuUniSubscription
	err := json.Unmarshal(bodyBytes, &provChgUuUniSubscription)
	if err != nil {
		log.Error(err.Error())
		return nil, false, err
	}

	// Validating mandatory parameters specific to V2xMsgSubscription in the request body
	if provChgUuUniSubscription.SubscriptionType == "" {
		err = errors.New("subscription not found against the provided subscriptionId")
		log.Error(err.Error())
		return nil, false, err
	}

	if provChgUuUniSubscription.WebsockNotifConfig != nil {
		err = errors.New("WebsockNotifConfig not supported")
		log.Error(err.Error())
		return nil, false, err
	}

	if provChgUuUniSubscription.CallbackReference == "" {
		err = errors.New("Mandatory attribute CallbackReference is missing in the request body")
		log.Error(err.Error())
		return nil, false, err
	}

	// FIXME FSCOM Check filter values

	log.Info("processProvChgUuUniSubscriptionUpdate: Checks done")

	// registration
	if isSubscriptionIdRegisteredProvChgUuUni(subsIdStr) {
		// Retrieve the current subscription
		idx, err := strconv.Atoi(subsIdStr)
		if err != nil {
			log.Error(err.Error())
			return nil, false, err
		}
		currentProvChgUuUniSubscription := *provChgUuUniSubscriptionMap[idx]
		log.Info("processProvChgUuUniSubscriptionUpdate: current : ", currentProvChgUuUniSubscription)

		registerProvChgUuUniSubscription(subsIdStr, &currentProvChgUuUniSubscription, &provChgUuUniSubscription)
		// store updated subscription key in redis
		keyName := baseKey + "subscriptions:" + subsIdStr
		log.Info("processProvChgUuUniSubscriptionUpdate: keyName: ", keyName)
		log.Info("processProvChgUuUniSubscriptionUpdate: provChgUuUniSubscription: ", provChgUuUniSubscription)
		err = rc.JSONSetEntry(keyName, ".", convertProvChgUuUniSubscriptionToJson(&provChgUuUniSubscription))
		if err != nil {
			log.Error(err.Error())
			return nil, true, err
		}
		jsonResponse, err := json.Marshal(provChgUuUniSubscription)
		if err != nil {
			log.Error(err.Error())
			return nil, true, err
		}
		return jsonResponse, true, nil
	}

	return nil, false, errors.New("Not registered.")
}

/*
 * processProvChgUuMbmsSubscription is to create subscription at /subscriptions endpoint
 * @param {struct} w HTTP write reference
 * @param {struct} r contains the HTTP request
 * @see ETSI GS ETSI GS MEC 040 V3.2.1 (2024-03) Clause 7.9.3.4 POST
 */
func processProvChgUuMbmsSubscription(bodyBytes []byte, link *Links, subsIdStr string, provChgUuMbmsSubscription *ProvChgUuMbmsSubscription) (string, error) {
	log.Debug(">>> processProvChgUuMbmsSubscription")

	err := json.Unmarshal(bodyBytes, provChgUuMbmsSubscription)
	if err != nil {
		log.Error(err.Error())
		return "", err
	}

	// Validating mandatory parameters provided in the request body
	if provChgUuMbmsSubscription.Links != nil {
		log.Error("Links attribute should not be present in request body")
		return "", errors.New("Links attribute should not be present in request body")
	}

	// FIXME FSCOM Check filter values
	if provChgUuMbmsSubscription.FilterCriteria == nil {
		log.Error("Mandatory FilterCriteria parameter should be present")
		return "", errors.New("Mandatory FilterCriteria parameter should be present")
	}

	if provChgUuMbmsSubscription.WebsockNotifConfig == nil && provChgUuMbmsSubscription.CallbackReference == "" {
		err = errors.New("Mandatory CallbackReference parameter should be present")
		log.Error(err.Error())
		return "", err
	}
	if provChgUuMbmsSubscription.WebsockNotifConfig != nil {
		err = errors.New("WebsockNotifConfig not supported")
		log.Error(err.Error())
		return "", err
	}
	if provChgUuMbmsSubscription.CallbackReference == "" {
		err = errors.New("CallbackReference parameter should be present")
		log.Error(err.Error())
		return "", err
	}

	registerProvChgUuMbmsSubscription(subsIdStr, nil, provChgUuMbmsSubscription)

	provChgUuMbmsSubscription.Links = link

	// Store subscription key in redis
	keyName := baseKey + "subscriptions:" + subsIdStr
	log.Info("processProvChgUuMbmsSubscription: keyName: ", keyName)
	log.Info("processProvChgUuMbmsSubscription: provChgUuMbmsSubscription: ", provChgUuMbmsSubscription)
	err = rc.JSONSetEntry(keyName, ".", convertProvChgUuMbmsSubscriptionToJson(provChgUuMbmsSubscription))
	if err != nil {
		log.Error(err.Error())
		return "", err
	}

	jsonResponse := convertProvChgUuMbmsSubscriptionToJson(provChgUuMbmsSubscription)

	return jsonResponse, nil
}

func processProvChgUuMbmsSubscriptionUpdate(bodyBytes []byte, subsIdStr string) ([]byte, bool, error) {
	log.Debug(">>> processProvChgUuMbmsSubscriptionUpdate: subsIdStr: ", subsIdStr)

	var provChgUuMbmsSubscription ProvChgUuMbmsSubscription
	err := json.Unmarshal(bodyBytes, &provChgUuMbmsSubscription)
	if err != nil {
		log.Error(err.Error())
		return nil, false, err
	}

	// Validating mandatory parameters specific to V2xMsgSubscription in the request body
	if provChgUuMbmsSubscription.SubscriptionType == "" {
		err = errors.New("subscription not found against the provided subscriptionId")
		log.Error(err.Error())
		return nil, false, err
	}

	if provChgUuMbmsSubscription.WebsockNotifConfig != nil {
		err = errors.New("WebsockNotifConfig not supported")
		log.Error(err.Error())
		return nil, false, err
	}

	if provChgUuMbmsSubscription.CallbackReference == "" {
		err = errors.New("Mandatory attribute CallbackReference is missing in the request body")
		log.Error(err.Error())
		return nil, false, err
	}

	// FIXME FSCOM Check filter values

	log.Info("processProvChgUuMbmsSubscriptionUpdate: Checks done")

	// registration
	if isSubscriptionIdRegisteredProvChgUuMbms(subsIdStr) {
		// Retrieve the current subscription
		keyName := baseKey + "subscriptions:" + subsIdStr
		log.Info("processProvChgUuMbmsSubscriptionUpdate: keyName: ", keyName)
		var currentProvChgUuMbmsSubscription ProvChgUuMbmsSubscription
		subscription, err := rc.JSONGetEntry(keyName, ".")
		if err != nil {
			log.Error(err.Error())
			return nil, true, err
		}
		log.Info("processProvChgUuMbmsSubscriptionUpdate: current : ", subscription)
		err = json.Unmarshal([]byte(subscription), &currentProvChgUuMbmsSubscription)
		if err != nil {
			log.Error(err.Error())
			return nil, true, err
		}

		registerProvChgUuMbmsSubscription(subsIdStr, &currentProvChgUuMbmsSubscription, &provChgUuMbmsSubscription)
		// store updated subscription key in redis
		log.Info("processProvChgUuMbmsSubscriptionUpdate: provChgUuMbmsSubscription: ", provChgUuMbmsSubscription)
		err = rc.JSONSetEntry(keyName, ".", convertProvChgUuMbmsSubscriptionToJson(&provChgUuMbmsSubscription))
		if err != nil {
			log.Error(err.Error())
			return nil, true, err
		}
		jsonResponse, err := json.Marshal(provChgUuMbmsSubscription)
		if err != nil {
			log.Error(err.Error())
			return nil, true, err
		}
		return jsonResponse, true, nil
	}

	return nil, false, errors.New("Not registered.")
}

/*
 * processProvChgPc5Subscription is to create subscription at /subscriptions endpoint
 * @param {struct} w HTTP write reference
 * @param {struct} r contains the HTTP request
 * @see ETSI GS ETSI GS MEC 040 V3.2.1 (2024-03) Clause 7.9.3.4 POST
 */
func processProvChgPc5Subscription(bodyBytes []byte, link *Links, subsIdStr string, provChgPc5Subscription *ProvChgPc5Subscription) (string, error) {
	log.Debug(">>> processProvChgPc5Subscription")

	err := json.Unmarshal(bodyBytes, provChgPc5Subscription)
	if err != nil {
		log.Error(err.Error())
		return "", err
	}

	// Validating mandatory parameters provided in the request body
	if provChgPc5Subscription.Links != nil {
		log.Error("Links attribute should not be present in request body")
		return "", errors.New("Links attribute should not be present in request body")
	}

	// FIXME FSCOM Check filter values
	if provChgPc5Subscription.FilterCriteria == nil {
		log.Error("Mandatory FilterCriteria parameter should be present")
		return "", errors.New("Mandatory FilterCriteria parameter should be present")
	}

	if provChgPc5Subscription.WebsockNotifConfig == nil && provChgPc5Subscription.CallbackReference == "" {
		err = errors.New("Mandatory CallbackReference parameter should be present")
		log.Error(err.Error())
		return "", err
	}
	if provChgPc5Subscription.WebsockNotifConfig != nil {
		err = errors.New("WebsockNotifConfig not supported")
		log.Error(err.Error())
		return "", err
	}
	if provChgPc5Subscription.CallbackReference == "" {
		err = errors.New("CallbackReference parameter should be present")
		log.Error(err.Error())
		return "", err
	}

	registerProvChgPc5Subscription(subsIdStr, nil, provChgPc5Subscription)

	provChgPc5Subscription.Links = link

	// Store subscription key in redis
	keyName := baseKey + "subscriptions:" + subsIdStr
	log.Info("processProvChgPc5Subscription: keyName: ", keyName)
	log.Info("processProvChgPc5Subscription: provChgPc5Subscription: ", provChgPc5Subscription)
	err = rc.JSONSetEntry(keyName, ".", convertProvChgPc5SubscriptionToJson(provChgPc5Subscription))
	if err != nil {
		log.Error(err.Error())
		return "", err
	}

	jsonResponse := convertProvChgPc5SubscriptionToJson(provChgPc5Subscription)

	return jsonResponse, nil
}

func processProvChgPc5SubscriptionUpdate(bodyBytes []byte, subsIdStr string) ([]byte, bool, error) {
	log.Debug(">>> processProvChgPc5SubscriptionUpdate: subsIdStr: ", subsIdStr)

	var provChgPc5Subscription ProvChgPc5Subscription
	err := json.Unmarshal(bodyBytes, &provChgPc5Subscription)
	if err != nil {
		log.Error(err.Error())
		return nil, false, err
	}

	// Validating mandatory parameters specific to V2xMsgSubscription in the request body
	if provChgPc5Subscription.SubscriptionType == "" {
		err = errors.New("subscription not found against the provided subscriptionId")
		log.Error(err.Error())
		return nil, false, err
	}

	if provChgPc5Subscription.WebsockNotifConfig != nil {
		err = errors.New("WebsockNotifConfig not supported")
		log.Error(err.Error())
		return nil, false, err
	}

	if provChgPc5Subscription.CallbackReference == "" {
		err = errors.New("Mandatory attribute CallbackReference is missing in the request body")
		log.Error(err.Error())
		return nil, false, err
	}

	// FIXME FSCOM Check filter values

	log.Info("processProvChgPc5SubscriptionUpdate: Checks done")

	// registration
	if isSubscriptionIdRegisteredProvChgPc5(subsIdStr) {
		// Retrieve the current subscription
		keyName := baseKey + "subscriptions:" + subsIdStr
		log.Info("processProvChgPc5SubscriptionUpdate: keyName: ", keyName)
		var currentProvChgPc5Subscription ProvChgPc5Subscription
		subscription, err := rc.JSONGetEntry(keyName, ".")
		if err != nil {
			log.Error(err.Error())
			return nil, true, err
		}
		log.Info("processProvChgPc5SubscriptionUpdate: current : ", subscription)
		err = json.Unmarshal([]byte(subscription), &currentProvChgPc5Subscription)
		if err != nil {
			log.Error(err.Error())
			return nil, true, err
		}

		registerProvChgPc5Subscription(subsIdStr, &currentProvChgPc5Subscription, &provChgPc5Subscription)
		// store updated subscription key in redis
		log.Info("processProvChgPc5SubscriptionUpdate: provChgPc5Subscription: ", provChgPc5Subscription)
		err = rc.JSONSetEntry(keyName, ".", convertProvChgPc5SubscriptionToJson(&provChgPc5Subscription))
		if err != nil {
			log.Error(err.Error())
			return nil, true, err
		}
		jsonResponse, err := json.Marshal(provChgPc5Subscription)
		if err != nil {
			log.Error(err.Error())
			return nil, true, err
		}
		return jsonResponse, true, nil
	}

	return nil, false, errors.New("Not registered.")
}

// FIXME FSCOM There is no PredQosNotification defined by the standard ETSI GS ETSI GS MEC 040 V3.2.1 (2024-03)
// func processPredQosSubscription(bodyBytes []byte, link *Links, subsIdStr string, predQosSubscription *PredQosSubscription) (string, error) {
// 	log.Debug(">>> processPredQosSubscription")

// 	err := json.Unmarshal(bodyBytes, predQosSubscription)
// 	if err != nil {
// 		log.Error(err.Error())
// 		return "", err
// 	}

// 	// Validating mandatory parameters provided in the request body
// 	if predQosSubscription.Links != nil {
// 		log.Error("Links attribute should not be present in request body")
// 		return "", errors.New("Links attribute should not be present in request body")
// 	}

// 	if predQosSubscription.FilterCriteria == nil {
// 		log.Error("Mandatory FilterCriteria parameter should be present")
// 		return "", errors.New("Mandatory FilterCriteria parameter should be present")
// 	}

// 	return "", err
// }

func processPredQosSubscriptionUpdate(bodyBytes []byte, subsIdStr string) ([]byte, bool, error) {
	log.Debug(">>> processPredQosSubscriptionUpdate: subsIdStr: ", subsIdStr)

	return nil, false, errors.New("There is no PredQosNotification defined by the standard ETSI GS ETSI GS MEC 040 V3.2.1 (2024-03)")
	// FIXME FSCOM There is no PredQosNotification defined by the standard ETSI GS ETSI GS MEC 040 V3.2.1 (2024-03)
	// var provChgPc5Subscription PredQosSubscription
	// err := json.Unmarshal(bodyBytes, &provChgPc5Subscription)
	// if err != nil {
	// 	log.Error(err.Error())
	// 	return nil, false, err
	// }

	// // Validating mandatory parameters specific to V2xMsgSubscription in the request body
	// if provChgPc5Subscription.SubscriptionType == "" {
	// 	err = errors.New("subscription not found against the provided subscriptionId")
	// 	log.Error(err.Error())
	// 	return nil, false, err
	// }

	// if provChgPc5Subscription.WebsockNotifConfig != nil {
	// 	err = errors.New("WebsockNotifConfig not supported")
	// 	log.Error(err.Error())
	// 	return nil, false, err
	// }

	// if provChgPc5Subscription.CallbackReference == "" {
	// 	err = errors.New("Mandatory attribute CallbackReference is missing in the request body")
	// 	log.Error(err.Error())
	// 	return nil, false, err
	// }

	// // FIXME FSCOM Check filter values

	// log.Info("processPredQosSubscriptionUpdate: Checks done")

	// // registration
	// if isSubscriptionIdRegisteredPredQos(subsIdStr) {
	// 	// Retrieve the current subscription
	// 	keyName := baseKey + "subscriptions:" + subsIdStr
	// 	log.Info("processPredQosSubscriptionUpdate: keyName: ", keyName)
	// 	var currentPredQosSubscription PredQosSubscription
	// 	subscription, err := rc.JSONGetEntry(keyName, ".")
	// 	if err != nil {
	// 		log.Error(err.Error())
	// 		return nil, true, err
	// 	}
	// 	log.Info("processPredQosSubscriptionUpdate: current : ", subscription)
	// 	err = json.Unmarshal([]byte(subscription), &currentPredQosSubscription)
	// 	if err != nil {
	// 		log.Error(err.Error())
	// 		return nil, true, err
	// 	}

	// 	registerPredQosSubscription(subsIdStr, &currentPredQosSubscription, &provChgPc5Subscription)
	// 	// store updated subscription key in redis
	// 	log.Info("processPredQosSubscriptionUpdate: provChgPc5Subscription: ", provChgPc5Subscription)
	// 	err = rc.JSONSetEntry(keyName, ".", convertPredQosSubscriptionToJson(&provChgPc5Subscription))
	// 	if err != nil {
	// 		log.Error(err.Error())
	// 		return nil, true, err
	// 	}
	// 	jsonResponse, err := json.Marshal(provChgPc5Subscription)
	// 	if err != nil {
	// 		log.Error(err.Error())
	// 		return nil, true, err
	// 	}
	// 	return jsonResponse, true, nil
	// }

	// return nil, false, errors.New("Not registered.")
}

/*
 * processV2xMsgSubscription is to create subscription at /subscriptions endpoint
 * @param {struct} w HTTP write reference
 * @param {struct} r contains the HTTP request
 * @see ETSI GS ETSI GS MEC 040 V3.2.1 (2024-03) Clause 7.9.3.4 POST
 */
func processV2xMsgSubscription(bodyBytes []byte, link *Links, subsIdStr string, v2xSubscription *V2xMsgSubscription) (string, error) {
	log.Debug(">>> processV2xMsgSubscription: link: ", *link)
	log.Debug(">>> processV2xMsgSubscription: subsIdStr: ", subsIdStr)

	err := json.Unmarshal(bodyBytes, v2xSubscription)
	if err != nil {
		log.Error(err.Error())
		return "", err
	}

	// Validating mandatory paprocessV2xMsgSubscriptionrameters provided in the request body
	if v2xSubscription.Links != nil {
		err = errors.New("Links attribute should not be present in request body")
		log.Error(err.Error())
		return "", err
	}

	// Check filter values
	if v2xSubscription.FilterCriteria == nil {
		err = errors.New("Mandatory FilterCriteria parameter should be present")
		log.Error(err.Error())
		return "", err
	}
	if v2xSubscription.FilterCriteria.StdOrganization == "" {
		err = errors.New("Mandatory StdOrganization parameter should be present")
		log.Error(err.Error())
		return "", err
	}
	if !checkMsgTypeValue(v2xSubscription.FilterCriteria.MsgType) {
		err = errors.New("MsgType parameter should be between 1 and 13")
		log.Error(err.Error())
		return "", err
	}

	if v2xSubscription.WebsockNotifConfig == nil && v2xSubscription.CallbackReference == "" {
		err = errors.New("Mandatory CallbackReference parameter should be present")
		log.Error(err.Error())
		return "", err
	}
	if v2xSubscription.WebsockNotifConfig != nil {
		err = errors.New("WebsockNotifConfig not supported")
		log.Error(err.Error())
		return "", err
	}
	if v2xSubscription.CallbackReference == "" {
		err = errors.New("CallbackReference parameter should be present")
		log.Error(err.Error())
		return "", err
	}

	v2xSubscription.Links = link

	registerV2xMsgSubscription(v2xSubscription, subsIdStr)

	// Store subscription key in redis
	keyName := baseKey + "subscriptions:" + subsIdStr
	log.Info("processV2xMsgSubscription: keyName: ", keyName)
	log.Info("processV2xMsgSubscription: v2xSubscription: ", v2xSubscription)
	err = rc.JSONSetEntry(keyName, ".", convertV2xMsgSubscriptionToJson(v2xSubscription))
	if err != nil {
		log.Error(err.Error())
		return "", err
	}

	jsonResponse := convertV2xMsgSubscriptionToJson(v2xSubscription)

	return jsonResponse, nil
}

/*
 * sendTestNotification sends test notification to validate callback URI
 * @param {string} notifyUrl contains the call reference address
 * @param {struct} notification contains the test notification
 * @see ETSI GS ETSI GS MEC 040 V3.2.1 (2024-03) Clause 7.9
 * @see ETSI GS ETSI GS MEC 040 V3.2.1 (2024-03) Clause 6.4.6 Type: TestNotification
 */
func sendTestNotification(notifyUrl string, notification TestNotification) {
	log.Debug(">>> sendTestNotification: notifyUrl: ", notifyUrl)

	startTime := time.Now()
	jsonNotif, err := json.Marshal(notification)
	if err != nil {
		log.Error(err)
		return
	}
	log.Info("sendTestNotification: jsonNotif: ", string(jsonNotif))

	resp, err := http.Post(notifyUrl, "application/json", bytes.NewBuffer(jsonNotif))
	log.Info("sendTestNotification: resp: ", resp)
	duration := float64(time.Since(startTime).Microseconds()) / 1000.0
	_ = httpLog.LogNotification(notifyUrl, "POST", "", "", string(jsonNotif), resp, startTime)
	if err != nil {
		log.Error(err)
		met.ObserveNotification(sandboxName, serviceName, V2X_MSG_NOTIF, notifyUrl, nil, duration)
		return
	}
	met.ObserveNotification(sandboxName, serviceName, V2X_MSG_NOTIF, notifyUrl, resp, duration)
	defer resp.Body.Close()
}

func createSubscriptionLinkList(subType string) *SubscriptionLinkList {

	subscriptionLinkList := new(SubscriptionLinkList)

	link := new(Links2)
	self := new(LinkType)
	self.Href = hostUrl.String() + basePath + "subscriptions"

	link.Self = self

	//loop through all different types of subscription
	mutex.Lock()
	defer mutex.Unlock()

	if subType == "prov_chg_uu_uni" {
		for _, provChgUuUniSubscription := range provChgUuUniSubscriptionMap {
			if provChgUuUniSubscription != nil {
				var subscriptions Subscriptions
				subscriptions.Href = provChgUuUniSubscription.Links.Self.Href
				subscriptions.SubscriptionType = PROV_CHG_UU_UNI
				link.Subscriptions = append(link.Subscriptions, subscriptions)
			}
		} // End of 'for' statement
	} else if subType == "prov_chg_uu_mbms" {
		for _, provChgUuMbmsSubscription := range provChgUuMbmsSubscriptionMap {
			if provChgUuMbmsSubscription != nil {
				var subscriptions Subscriptions
				subscriptions.Href = provChgUuMbmsSubscription.Links.Self.Href
				subscriptions.SubscriptionType = PROV_CHG_UU_MBMS
				link.Subscriptions = append(link.Subscriptions, subscriptions)
			}
		} // End of 'for' statement
	} else if subType == "prov_chg_pc5" {
		for _, provChgPc5Subscription := range provChgPc5SubscriptionMap {
			if provChgPc5Subscription != nil {
				var subscriptions Subscriptions
				subscriptions.Href = provChgPc5Subscription.Links.Self.Href
				subscriptions.SubscriptionType = PROV_CHG_PC5
				link.Subscriptions = append(link.Subscriptions, subscriptions)
			}
		} // End of 'for' statement
	} else if subType == "v2x_msg" {
		for _, v2xSubscription := range v2xMsgSubscriptionMap {
			if v2xSubscription != nil {
				var subscription Subscriptions
				subscription.Href = v2xSubscription.Links.Self.Href
				subscription.SubscriptionType = V2X_MSG
				link.Subscriptions = append(link.Subscriptions, subscription)
			}
		} // End of 'for' statement
		// FIXME FSCOM There is no PredQosNotification defined by the standard ETSI GS ETSI GS MEC 040 V3.2.1 (2024-03)
		// } else if subType == "pred_qos" {
		// 	for _, predQosSubscription := range predQosSubscriptionMap {
		// 		if predQosSubscription != nil {
		// 			var subscription Subscriptions
		// 			subscription.Href = predQosSubscription.Links.Self.Href
		// 			subscription.SubscriptionType = PRED_QOS
		// 			link.Subscriptions = append(link.Subscriptions, subscription)
		// 		}
		// 	} // End of 'for' statement
	} else { // Build complete list
		for _, provChgUuUniSubscription := range provChgUuUniSubscriptionMap {
			if provChgUuUniSubscription != nil {
				var subscriptions Subscriptions
				subscriptions.Href = provChgUuUniSubscription.Links.Self.Href
				subscriptions.SubscriptionType = PROV_CHG_UU_UNI
				link.Subscriptions = append(link.Subscriptions, subscriptions)
			}
		} // End of 'for' statement
		for _, provChgUuMbmsSubscription := range provChgUuMbmsSubscriptionMap {
			if provChgUuMbmsSubscription != nil {
				var subscriptions Subscriptions
				subscriptions.Href = provChgUuMbmsSubscription.Links.Self.Href
				subscriptions.SubscriptionType = PROV_CHG_UU_MBMS
				link.Subscriptions = append(link.Subscriptions, subscriptions)
			}
		} // End of 'for' statement
		for _, provChgPc5Subscription := range provChgPc5SubscriptionMap {
			if provChgPc5Subscription != nil {
				var subscriptions Subscriptions
				subscriptions.Href = provChgPc5Subscription.Links.Self.Href
				subscriptions.SubscriptionType = PROV_CHG_PC5
				link.Subscriptions = append(link.Subscriptions, subscriptions)
			}
		} // End of 'for' statement
		for _, v2xSubscription := range v2xMsgSubscriptionMap {
			if v2xSubscription != nil {
				var subscription Subscriptions
				subscription.Href = v2xSubscription.Links.Self.Href
				subscription.SubscriptionType = V2X_MSG
				link.Subscriptions = append(link.Subscriptions, subscription)
			}
		} // End of 'for' statement
		// FIXME FSCOM There is no PredQosNotification defined by the standard ETSI GS ETSI GS MEC 040 V3.2.1 (2024-03)
		// for _, predQosSubscription := range predQosSubscriptionMap {
		// 	if predQosSubscription != nil {
		// 		var subscription Subscriptions
		// 		subscription.Href = predQosSubscription.Links.Self.Href
		// 		subscription.SubscriptionType = PRED_QOS
		// 		link.Subscriptions = append(link.Subscriptions, subscription)
		// 	}
		// } // End of 'for' statement
	}

	subscriptionLinkList.Links = link

	return subscriptionLinkList
}

/*
 * subscriptionsGET is to retrieve information about all existing subscriptions at /subscriptions endpoint
 * @param {struct} w HTTP write reference
 * @param {struct} r contains the HTTP request
 * @see ETSI GS ETSI GS MEC 040 V3.2.1 (2024-03) Clause 7.9.3.1
 */
func subscriptionsGET(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> subscriptionsGET: ", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	// get & validate query param values for subscription_type
	u, _ := url.Parse(r.URL.String())
	log.Info("url: ", u.RequestURI())
	q := u.Query()
	subType := q.Get("subscription_type")

	// look for all query parameters to reject if any invalid ones
	if !validateQueryParams(q, []string{"subscription_type"}) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if !validateQueryParamValue(q.Get("subscription_type"), []string{"prov_chg_uu_uni", "prov_chg_uu_mbms", "prov_chg_pc5", "v2x_msg", "pred_qos"}) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// get the response against particular subscription type
	response := createSubscriptionLinkList(subType)

	// prepare & send response
	jsonResponse, err := json.Marshal(response)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// success response code
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(jsonResponse))
}

/*
 * individualSubscriptionGET is to retrive a specific subscriptionsInfo at /subscriptions/{subscriptionId} endpoint
 * @param {struct} w HTTP write reference
 * @param {struct} r contains the HTTP request
 * @see ETSI GS ETSI GS MEC 040 V3.2.1 (2024-03) Clause 7.9
 */
func individualSubscriptionGET(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> individualSubscriptionGET: ", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	u, _ := url.Parse(r.URL.String())
	url := u.RequestURI()
	log.Info("url: ", url)
	subsIdStr := string(url[strings.LastIndex(url, "/")+1:])
	log.Info("subsIdStr: ", subsIdStr)

	// Find subscription entry in Redis DB
	keyName := baseKey + "subscriptions:" + subsIdStr
	log.Info("individualSubscriptionGET: keyName: ", keyName)
	subscription, err := rc.JSONGetEntry(keyName, ".")
	if err != nil {
		err = errors.New("subscription not found against the provided subscriptionId")
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusNotFound)
		return
	}
	log.Info("individualSubscriptionGET: subscription: ", subscription)

	var jsonResponse string
	if strings.Contains(subscription, PROV_CHG_UU_UNI) {
		var subResp ProvChgUuUniSubscription
		err = json.Unmarshal([]byte(subscription), &subResp)
		if err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jsonResponse = convertProvChgUuUniSubscriptionToJson(&subResp)
	} else if strings.Contains(subscription, PROV_CHG_UU_MBMS) {
		var subResp ProvChgUuMbmsSubscription
		err = json.Unmarshal([]byte(subscription), &subResp)
		if err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jsonResponse = convertProvChgUuMbmsSubscriptionToJson(&subResp)
	} else if strings.Contains(subscription, PROV_CHG_PC5) {
		var subResp ProvChgPc5Subscription
		err = json.Unmarshal([]byte(subscription), &subResp)
		if err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jsonResponse = convertProvChgPc5SubscriptionToJson(&subResp)
	} else if strings.Contains(subscription, V2X_MSG) {
		var subResp V2xMsgSubscription
		err = json.Unmarshal([]byte(subscription), &subResp)
		if err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jsonResponse = convertV2xMsgSubscriptionToJson(&subResp)
	} else if strings.Contains(subscription, PRED_QOS) {
		var subResp PredQosSubscription
		err = json.Unmarshal([]byte(subscription), &subResp)
		if err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jsonResponse = convertPredQosSubscriptionToJson(&subResp)
	}
	log.Info("individualSubscriptionGET: jsonResponse: ", jsonResponse)

	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, jsonResponse)
}

/*
 * registerV2xMsgSubscription to register new v2xMsgSubscription
 * @param {struct} v2xMsgSubscription contains request body send to /subscriptions endpoint
 * @param {string} subsIdStr contains an Id to uniquely subscription
 */
func registerV2xMsgSubscription(v2xMsgSubscription *V2xMsgSubscription, subId string) {
	log.Debug(">>> registerV2xMsgSubscription: subId: ", subId)

	subsId, _ := strconv.Atoi(subId)
	mutex.Lock()
	defer mutex.Unlock()

	log.Info("registerV2xMsgSubscription: Before subscriptionExpiryMap: ", subscriptionExpiryMap)
	v2xMsgSubscriptionMap[subsId] = v2xMsgSubscription
	if v2xMsgSubscription.ExpiryDeadline != nil {
		//get current list of subscription meant to expire at this time
		intList := subscriptionExpiryMap[int(v2xMsgSubscription.ExpiryDeadline.Seconds)]
		// FIXME expiryDeadline can be changed by PUT, we need originaland new value of expiryDeadline
		found := false
		for _, id := range intList {
			if id == subsId {
				found = true
			}
		}
		if !found {
			intList = append(intList, subsId)
			subscriptionExpiryMap[int(v2xMsgSubscription.ExpiryDeadline.Seconds)] = intList
		}
	}
	log.Info("registerV2xMsgSubscription: After subscriptionExpiryMap: ", subscriptionExpiryMap)
	log.Info("New registration: ", subsId, " type: ", V2X_MSG)

	log.Info("deregisterV2xMsgSubscription: len(v2xMsgSubscriptionMap): ", len(v2xMsgSubscriptionMap))
	if len(v2xMsgSubscriptionMap) == 1 { // Start V2X message broker server
		log.Info("registerV2xMsgSubscription: StartV2xMessageBrokerServer")
		_ = sbi.StartV2xMessageBrokerServer()
	}
}

/*
 * isSubscriptionIdRegisteredV2x to verify if subscription is already registered
 * @param {string} subsIdStr contains an Id to uniquely subscription
 * @return {bool} true on success, false otherwise
 */
func isSubscriptionIdRegisteredV2x(subsIdStr string) bool {
	var returnVal bool
	subsId, _ := strconv.Atoi(subsIdStr)
	mutex.Lock()
	defer mutex.Unlock()

	if v2xMsgSubscriptionMap[subsId] != nil {
		returnVal = true
	} else {
		returnVal = false
	}
	return returnVal
}

/*
 * registerProvChgUuUniSubscription to register new provChgUuUniSubscription
 * @param {string} subsIdStr contains an Id to uniquely subscription
 * @param {struct} currentProvChgUuUniSubscription contains the existing ProvChgUuUniSubscription
 * @param {struct} provChgUuUniSubscription contains request body send to /subscriptions endpoint
 */
func registerProvChgUuUniSubscription(subId string, currentProvChgUuUniSubscription *ProvChgUuUniSubscription, provChgUuUniSubscription *ProvChgUuUniSubscription) {
	log.Debug(">>> registerProvChgUuUniSubscription: subId: ", subId)
	log.Debug(">>> registerProvChgUuUniSubscription: currentProvChgUuUniSubscription: ", currentProvChgUuUniSubscription)
	log.Debug(">>> registerProvChgUuUniSubscription: provChgUuUniSubscription: ", provChgUuUniSubscription)

	subsId, _ := strconv.Atoi(subId)
	log.Info("registerProvChgUuUniSubscription: subsId: ", subsId)
	mutex.Lock()
	defer mutex.Unlock()

	log.Info("registerProvChgUuUniSubscription: Before subscriptionExpiryMap: ", subscriptionExpiryMap)
	// Replace the current subscription by the new one
	provChgUuUniSubscriptionMap[subsId] = provChgUuUniSubscription
	if currentProvChgUuUniSubscription != nil {
		// Update the subscriptionExpiryMap
		// 1. Find the old one if any
		if currentProvChgUuUniSubscription.ExpiryDeadline != nil {
			if *currentProvChgUuUniSubscription.ExpiryDeadline != *provChgUuUniSubscription.ExpiryDeadline {
				// 1.1 Remove the existing one
				intList := subscriptionExpiryMap[int(currentProvChgUuUniSubscription.ExpiryDeadline.Seconds)]
				// TODO FSCOM Common code with delSubscription, create a function to remove an expiruDeadline from the subscriptionExpiryMap
				for i, subsIndex := range intList {
					log.Info("registerProvChgUuUniSubscription: i: ", i)
					log.Info("registerProvChgUuUniSubscription: subsIndex: ", subsIndex)
					if subsIndex == subsId {
						//
						log.Info("registerProvChgUuUniSubscription: found index, delete entry")
						// Remove item and update subscriptionExpiryMap
						subscriptionExpiryMap[int(currentProvChgUuUniSubscription.ExpiryDeadline.Seconds)] = append(intList[:i], intList[i+1:]...)
						break
					}
				} // End of 'for' statement
				log.Info("delSubscription: After removal: subscriptionExpiryMap: ", subscriptionExpiryMap)

				// 1.2 And add the new one
				subscriptionExpiryMap[int(provChgUuUniSubscription.ExpiryDeadline.Seconds)] = append(subscriptionExpiryMap[int(provChgUuUniSubscription.ExpiryDeadline.Seconds)], subsId)
			}
		} else {
			// 2. Add new expiry if any
			if provChgUuUniSubscription.ExpiryDeadline != nil {
				intList := subscriptionExpiryMap[int(provChgUuUniSubscription.ExpiryDeadline.Seconds)]
				intList = append(intList, subsId)
				subscriptionExpiryMap[int(provChgUuUniSubscription.ExpiryDeadline.Seconds)] = intList
			}
		}
	} else { // First registration
		if provChgUuUniSubscription.ExpiryDeadline != nil {
			intList := subscriptionExpiryMap[int(provChgUuUniSubscription.ExpiryDeadline.Seconds)]
			intList = append(intList, subsId)
			subscriptionExpiryMap[int(provChgUuUniSubscription.ExpiryDeadline.Seconds)] = intList
		}
	}
	log.Info("registerProvChgUuUniSubscription: After subscriptionExpiryMap: ", subscriptionExpiryMap)
	log.Info("New registration: ", subsId, " type: ", PROV_CHG_UU_UNI)
}

/*
 * isSubscriptionIdRegisteredProvChgUuUni to verify if subscription is already registered
 * @param {string} subsIdStr contains an Id to uniquely subscription
 * @return {bool} true on success, false otherwise
 */
func isSubscriptionIdRegisteredProvChgUuUni(subsIdStr string) bool {
	var returnVal bool
	subsId, _ := strconv.Atoi(subsIdStr)
	mutex.Lock()
	defer mutex.Unlock()

	if provChgUuUniSubscriptionMap[subsId] != nil {
		returnVal = true
	} else {
		returnVal = false
	}
	return returnVal
}

/*
 * registerProvChgUuMbmsSubscription to register new provChgUuMbmsSubscription
 * @param {string} subsIdStr contains an Id to uniquely subscription
 * @param {struct} currentProvChgUuMbmsSubscription contains the existing ProvChgUuMbmsSubscription
 * @param {struct} provChgUuMbmsSubscription contains request body send to /subscriptions endpoint
 */
func registerProvChgUuMbmsSubscription(subId string, currentProvChgUuMbmsSubscription *ProvChgUuMbmsSubscription, provChgUuMbmsSubscription *ProvChgUuMbmsSubscription) {
	log.Debug(">>> registerProvChgUuMbmsSubscription: subId: ", subId)

	subsId, _ := strconv.Atoi(subId)
	mutex.Lock()
	defer mutex.Unlock()

	log.Info("registerProvChgUuMbmsSubscription: Before subscriptionExpiryMap: ", subscriptionExpiryMap)
	provChgUuMbmsSubscriptionMap[subsId] = provChgUuMbmsSubscription
	if provChgUuMbmsSubscription.ExpiryDeadline != nil {
		//get current list of subscription meant to expire at this time
		intList := subscriptionExpiryMap[int(provChgUuMbmsSubscription.ExpiryDeadline.Seconds)]
		if currentProvChgUuMbmsSubscription == nil {
			intList = append(intList, subsId)
			subscriptionExpiryMap[int(provChgUuMbmsSubscription.ExpiryDeadline.Seconds)] = intList
		} else {
			// FIXME expiryDeadline can be changed by PUT, we need originaland new value of expiryDeadline
			found := false
			for _, id := range intList {
				if id == subsId {
					found = true
				}
			}
			if !found {
				intList = append(intList, subsId)
				subscriptionExpiryMap[int(provChgUuMbmsSubscription.ExpiryDeadline.Seconds)] = intList
			}
		}
	}
	log.Info("registerProvChgUuMbmsSubscription: After subscriptionExpiryMap: ", subscriptionExpiryMap)
	log.Info("New registration: ", subsId, " type: ", PROV_CHG_UU_UNI)
}

/*
 * isSubscriptionIdRegisteredProvChgUuMbms to verify if subscription is already registered
 * @param {string} subsIdStr contains an Id to uniquely subscription
 * @return {bool} true on success, false otherwise
 */
func isSubscriptionIdRegisteredProvChgUuMbms(subsIdStr string) bool {
	var returnVal bool
	subsId, _ := strconv.Atoi(subsIdStr)
	mutex.Lock()
	defer mutex.Unlock()

	if provChgUuMbmsSubscriptionMap[subsId] != nil {
		returnVal = true
	} else {
		returnVal = false
	}
	return returnVal
}

/*
 * registerProvChgPc5Subscription to register new provChgPc5Subscription
 * @param {string} subsIdStr contains an Id to uniquely subscription
 * @param {struct} currentProvChgPc5Subscription contains the existing ProvChgPc5Subscription
 * @param {struct} provChgPc5Subscription contains request body send to /subscriptions endpoint
 */
func registerProvChgPc5Subscription(subId string, currentProvChgPc5Subscription *ProvChgPc5Subscription, provChgPc5Subscription *ProvChgPc5Subscription) {
	log.Debug(">>> registerProvChgPc5Subscription: subId: ", subId)

	subsId, _ := strconv.Atoi(subId)
	mutex.Lock()
	defer mutex.Unlock()

	log.Info("registerProvChgPc5Subscription: Before subscriptionExpiryMap: ", subscriptionExpiryMap)
	provChgPc5SubscriptionMap[subsId] = provChgPc5Subscription
	if provChgPc5Subscription.ExpiryDeadline != nil {
		//get current list of subscription meant to expire at this time
		intList := subscriptionExpiryMap[int(provChgPc5Subscription.ExpiryDeadline.Seconds)]
		if currentProvChgPc5Subscription == nil {
			intList = append(intList, subsId)
			subscriptionExpiryMap[int(provChgPc5Subscription.ExpiryDeadline.Seconds)] = intList
		} else {
			// FIXME expiryDeadline can be changed by PUT, we need originaland new value of expiryDeadline
			found := false
			for _, id := range intList {
				if id == subsId {
					found = true
				}
			}
			if !found {
				intList = append(intList, subsId)
				subscriptionExpiryMap[int(provChgPc5Subscription.ExpiryDeadline.Seconds)] = intList
			}
		}
	}
	log.Info("registerProvChgPc5Subscription: After subscriptionExpiryMap: ", subscriptionExpiryMap)
	log.Info("New registration: ", subsId, " type: ", PROV_CHG_UU_UNI)
}

/*
 * isSubscriptionIdRegisteredProvChgPc5 to verify if subscription is already registered
 * @param {string} subsIdStr contains an Id to uniquely subscription
 * @return {bool} true on success, false otherwise
 */
func isSubscriptionIdRegisteredProvChgPc5(subsIdStr string) bool {
	var returnVal bool
	subsId, _ := strconv.Atoi(subsIdStr)
	mutex.Lock()
	defer mutex.Unlock()

	if provChgPc5SubscriptionMap[subsId] != nil {
		returnVal = true
	} else {
		returnVal = false
	}
	return returnVal
}

/*
 * registerPredQosSubscription to register new predQosSubscription
 * @param {string} subsIdStr contains an Id to uniquely subscription
 * @param {struct} currentPredQosSubscription contains the existing PredQosSubscription
 * @param {struct} predQosSubscription contains request body send to /subscriptions endpoint
 */
// FIXME FSCOM There is no PredQosNotification defined by the standard ETSI GS ETSI GS MEC 040 V3.2.1 (2024-03)
// func registerPredQosSubscription(subId string, currentPredQosSubscription *PredQosSubscription, predQosSubscription *PredQosSubscription) {
// 	log.Debug(">>> registerPredQosSubscription: subId: ", subId)

// 	subsId, _ := strconv.Atoi(subId)
// 	mutex.Lock()
// 	defer mutex.Unlock()

// 	log.Info("registerPredQosSubscription: Before subscriptionExpiryMap: ", subscriptionExpiryMap)
// 	predQosSubscriptionMap[subsId] = predQosSubscription
// 	if predQosSubscription.ExpiryDeadline != nil {
// 		//get current list of subscription meant to expire at this time
// 		intList := subscriptionExpiryMap[int(predQosSubscription.ExpiryDeadline.Seconds)]
// 		if currentPredQosSubscription == nil {
// 			intList = append(intList, subsId)
// 			subscriptionExpiryMap[int(predQosSubscription.ExpiryDeadline.Seconds)] = intList
// 		} else {
// 			// FIXME expiryDeadline can be changed by PUT, we need originaland new value of expiryDeadline
// 			found := false
// 			for _, id := range intList {
// 				if id == subsId {
// 					found = true
// 				}
// 			}
// 			if !found {
// 				intList = append(intList, subsId)
// 				subscriptionExpiryMap[int(predQosSubscription.ExpiryDeadline.Seconds)] = intList
// 			}
// 		}
// 	}
// 	log.Info("registerPredQosSubscription: After subscriptionExpiryMap: ", subscriptionExpiryMap)
// 	log.Info("New registration: ", subsId, " type: ", PROV_CHG_UU_UNI)
// }

/*
 * isSubscriptionIdRegisteredPredQos to verify if subscription is already registered
 * @param {string} subsIdStr contains an Id to uniquely subscription
 * @return {bool} true on success, false otherwise
 */
// FIXME FSCOM There is no PredQosNotification defined by the standard ETSI GS ETSI GS MEC 040 V3.2.1 (2024-03)
// func isSubscriptionIdRegisteredPredQos(subsIdStr string) bool {
// 	var returnVal bool
// 	subsId, _ := strconv.Atoi(subsIdStr)
// 	mutex.Lock()
// 	defer mutex.Unlock()

// 	if predQosSubscriptionMap[subsId] != nil {
// 		returnVal = true
// 	} else {
// 		returnVal = false
// 	}
// 	return returnVal
// }

/*
 * checkForExpiredSubscriptions delete those subscriptions whose expiryTime is reached
 */
func checkForExpiredSubscriptions() {
	//log.Debug(">>> checkForExpiredSubscriptions")
	// log.Info("checkForExpiredSubscriptions: provChgUuUniSubscriptionMap: ", provChgUuUniSubscriptionMap)
	// log.Info("checkForExpiredSubscriptions: v2xMsgSubscriptionMap: ", v2xMsgSubscriptionMap)

	nowTime := int(time.Now().Unix())
	mutex.Lock()
	defer mutex.Unlock()
	for expiryTime, subsIndexList := range subscriptionExpiryMap {
		if expiryTime <= nowTime {
			subscriptionExpiryMap[expiryTime] = nil
			for _, subsId := range subsIndexList {
				subsIdStr := strconv.Itoa(subsId)
				keyName := baseKey + "subscriptions:" + subsIdStr
				log.Info("checkForExpiredSubscriptions: keyName: ", keyName)
				subscription, err := rc.JSONGetEntry(keyName, ".")
				if err != nil {
					log.Error(err.Error())
					continue
				}
				cbRef := ""
				if strings.Contains(subscription, PROV_CHG_UU_UNI) {
					if provChgUuUniSubscriptionMap[subsId] != nil {
						cbRef = provChgUuUniSubscriptionMap[subsId].CallbackReference
					} else {
						continue
					}
				} else if strings.Contains(subscription, PROV_CHG_UU_MBMS) {
					if provChgUuMbmsSubscriptionMap[subsId] != nil {
						cbRef = provChgUuMbmsSubscriptionMap[subsId].CallbackReference
					} else {
						continue
					}
				} else if strings.Contains(subscription, PROV_CHG_PC5) {
					if provChgPc5SubscriptionMap[subsId] != nil {
						cbRef = provChgPc5SubscriptionMap[subsId].CallbackReference
					} else {
						continue
					}
				} else if strings.Contains(subscription, V2X_MSG) {
					if v2xMsgSubscriptionMap[subsId] != nil {
						cbRef = v2xMsgSubscriptionMap[subsId].CallbackReference
					} else {
						continue
					}
					// FIXME FSCOM There is no PredQosNotification defined by the standard ETSI GS ETSI GS MEC 040 V3.2.1 (2024-03)
					// } else if strings.Contains(subscription, PRED_QOS) {
					// 	if predQosSubscriptionMap[subsId] != nil {
					// 		sendExpiryPredQosSubscription(predQosSubscriptionMap[subsId])
					// 	} else {
					// 		continue
					// 	}
				}

				var notif ExpiryNotification

				var expiryTimeStamp TimeStamp
				expiryTimeStamp.Seconds = int32(expiryTime)

				link := new(ExpiryNotificationLinks)
				link.Subscription.Href = cbRef
				notif.Links = link

				notif.ExpiryDeadline = &expiryTimeStamp
				sendExpiryNotification(link.Subscription.Href, notif)

				// Delete subscription
				err = delSubscription(subsIdStr, "", true)
				if err != nil {
					log.Error(err.Error())
				}
			} // End of 'for' statement
		}
	} // End of 'for' statement
}

func computeElapseTime(expiryDeadline TimeStamp) *TimeStamp {
	return &TimeStamp{NanoSeconds: 0, Seconds: int32(expiryDeadline.Seconds) - int32(time.Now().Unix())}
}

/*
* sendExpiryNotification send expiry notification to the the corresponding callback reference address
* @param {string} notifyUrl contains callback reference address of service consumer
* @param {struct} notification struct of type ExpiryNotification
 */
func sendExpiryNotification(notifyUrl string, notification ExpiryNotification) {
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
		met.ObserveNotification(sandboxName, serviceName, NOTIFY_EXPIRY, notifyUrl, nil, duration)
		return
	}
	met.ObserveNotification(sandboxName, serviceName, NOTIFY_EXPIRY, notifyUrl, resp, duration)
	defer resp.Body.Close()
}

/*
* delSubscription delete expired subscriptions from redis DB
 */
func delSubscription(subsId string, subscription string, mutexTaken bool) error {
	log.Debug(">>> delSubscription: ", subsId)

	keyName := baseKey + "subscriptions:" + subsId
	log.Info("delSubscription: keyName: ", keyName)
	err := rc.JSONDelEntry(keyName, ".")
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
			if strings.Compare(strconv.Itoa(subsIndex), subsId) == 0 {
				// FIXME FSCOM How to manage it subscriptionExpiryMap
				log.Info("delSubscription: found index, delete entry")
				subscriptionExpiryMap[i] = append(subscriptionExpiryMap[i][:j], subscriptionExpiryMap[i][j+1:]...)
				break
			}
		} // End of 'for' statement
	} // End of 'for' statement
	log.Info("delSubscription: After removal: subscriptionExpiryMap: ", subscriptionExpiryMap)

	if strings.Contains(subscription, PROV_CHG_UU_UNI) {
		deregisterProvChgUuUniSubscription(subsId, mutexTaken)
	} else if strings.Contains(subscription, PROV_CHG_UU_MBMS) {
		deregisterProvChgUuMbmsSubscription(subsId, mutexTaken)
	} else if strings.Contains(subscription, PROV_CHG_PC5) {
		deregisterProvChgPc5Subscription(subsId, mutexTaken)
	} else if strings.Contains(subscription, V2X_MSG) {
		deregisterV2xMsgSubscription(subsId, mutexTaken)
		// FIXME FSCOM There is no PredQosNotification defined by the standard ETSI GS ETSI GS MEC 040 V3.2.1 (2024-03)
		// } else if strings.Contains(subscription, PRED_QOS) {
		// 	deregisterPredQosSubscription(subsId, mutexTaken)
	}

	return nil
}

func deregisterProvChgUuUniSubscription(subsIdStr string, mutexTaken bool) {
	log.Debug(">>> deregisterProvChgUuUniSubscription: subsId: ", subsIdStr)

	subsId, _ := strconv.Atoi(subsIdStr)
	if !mutexTaken {
		mutex.Lock()
		defer mutex.Unlock()
	}
	log.Info("deregisterProvChgUuUniSubscription: Before provChgUuUniSubscriptionMap", provChgUuUniSubscriptionMap)
	delete(provChgUuUniSubscriptionMap, subsId)
	log.Info("deregisterProvChgUuUniSubscription: After provChgUuUniSubscriptionMap", provChgUuUniSubscriptionMap)

	log.Info("deregisterProvChgUuUniSubscription: ", subsId, " type: ", PROV_CHG_UU_UNI)
}

func deregisterProvChgUuMbmsSubscription(subsIdStr string, mutexTaken bool) {
	log.Debug(">>> deregisterProvChgUuMbmsSubscription: subsId: ", subsIdStr)

	subsId, _ := strconv.Atoi(subsIdStr)
	if !mutexTaken {
		mutex.Lock()
		defer mutex.Unlock()
	}
	delete(provChgUuMbmsSubscriptionMap, subsId)
	log.Info("deregisterProvChgUuMbmsSubscription: ", subsId, " type: ", PROV_CHG_UU_MBMS)
}

func deregisterProvChgPc5Subscription(subsIdStr string, mutexTaken bool) {
	log.Debug(">>> deregisterProvChgPc5Subscription: subsId: ", subsIdStr)

	subsId, _ := strconv.Atoi(subsIdStr)
	if !mutexTaken {
		mutex.Lock()
		defer mutex.Unlock()
	}
	delete(provChgPc5SubscriptionMap, subsId)
	log.Info("deregisterProvChgPc5Subscription: ", subsId, " type: ", PROV_CHG_PC5)
}

// FIXME FSCOM There is no PredQosNotification defined by the standard ETSI GS ETSI GS MEC 040 V3.2.1 (2024-03)
// func (subsIdStr string, mutexTaken bool) {
//	 log.Debug(">>> deregisterPredQosSubscription: subsId: ", subsIdStr)
//
// 	subsId, _ := strconv.Atoi(subsIdStr)
// 	if !mutexTaken {
// 		mutex.Lock()
// 		defer mutex.Unlock()
// 	}
// 	delete(predQosSubscriptionMap, subsId)
// 	log.Info("deregisterPredQosSubscription: ", subsId, " type: ", PRED_QOS)
// }

func deregisterV2xMsgSubscription(subsIdStr string, mutexTaken bool) {
	log.Debug(">>> deregisterV2xMsgSubscription: subsId: ", subsIdStr)

	subsId, _ := strconv.Atoi(subsIdStr)
	if !mutexTaken {
		mutex.Lock()
		defer mutex.Unlock()
	}
	log.Info("deregisterV2xMsgSubscription: Before v2xMsgSubscriptionMap", v2xMsgSubscriptionMap)
	delete(v2xMsgSubscriptionMap, subsId)
	log.Info("deregisterV2xMsgSubscription: After v2xMsgSubscriptionMap", v2xMsgSubscriptionMap)

	log.Info("deregisterV2xMsgSubscription: ", subsId, " type: ", V2X_MSG)

	log.Info("deregisterV2xMsgSubscription: len(v2xMsgSubscriptionMap): ", len(v2xMsgSubscriptionMap))
	if len(v2xMsgSubscriptionMap) == 0 { // Stop V2X message broker server
		sbi.StopV2xMessageBrokerServer()
	}
}

func repopulateV2xMsgSubscriptionMap(key string, jsonInfo string, userData interface{}) error {
	log.Debug(">>> repopulateV2xMsgSubscriptionMap: key: ", key)
	log.Debug(">>> repopulateV2xMsgSubscriptionMap: jsonInfo: ", jsonInfo)

	var v2xMsgSubscription V2xMsgSubscription

	// Format response
	err := json.Unmarshal([]byte(jsonInfo), &v2xMsgSubscription)
	if err != nil {
		return err
	}

	selfUrl := strings.Split(v2xMsgSubscription.Links.Self.Href, "/")
	subsIdStr := selfUrl[len(selfUrl)-1]
	subsId, _ := strconv.Atoi(subsIdStr)

	mutex.Lock()
	defer mutex.Unlock()

	v2xMsgSubscriptionMap[subsId] = &v2xMsgSubscription
	if v2xMsgSubscription.ExpiryDeadline != nil {
		intList := subscriptionExpiryMap[int(v2xMsgSubscription.ExpiryDeadline.Seconds)]
		intList = append(intList, subsId)
		subscriptionExpiryMap[int(v2xMsgSubscription.ExpiryDeadline.Seconds)] = intList
	}

	//reinitialisation of next available Id for future subscription request
	if subsId >= nextSubscriptionIdAvailable {
		nextSubscriptionIdAvailable = subsId + 1
	}

	return nil
}

// individualSubscriptionPut updates the information about a specific subscriptionInfo at /subscriptions/{subscriptionId} endpoint
func individualSubscriptionPut(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> individualSubscriptionPut: ", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	u, _ := url.Parse(r.URL.String())
	url := u.RequestURI()
	log.Info("url: ", url)
	subsIdStr := string(url[strings.LastIndex(url, "/")+1:])
	log.Info("subsIdStr: ", subsIdStr)

	var subscriptionCommon SubscriptionCommon
	// Read JSON input stream provided in the Request, and stores it in the bodyBytes as bytes
	bodyBytes, _ := ioutil.ReadAll(r.Body)
	// Unmarshal function to converts a JSON-formatted string into a SubscriptionCommon struct and store it in extractSubType
	err := json.Unmarshal(bodyBytes, &subscriptionCommon)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Info("subscriptionCommon: ", subscriptionCommon)
	// extract common body part
	subscriptionType := subscriptionCommon.SubscriptionType

	// validating common mandatory parameters provided in the request body
	if subscriptionCommon.SubscriptionType == "" {
		log.Error("Mandatory SubscriptionType parameter should be present")
		errHandlerProblemDetails(w, "Mandatory attribute SubscriptionType is missing in the request body.", http.StatusBadRequest)
		return
	}

	if subscriptionCommon.CallbackReference == "" && subscriptionCommon.WebsockNotifConfig == nil {
		log.Error("At least one of callbackReference and websockNotifConfig parameters should be present")
		errHandlerProblemDetails(w, "Both callbackReference and websockNotifConfig parameters are missing in the request body.", http.StatusBadRequest)
		return
	}

	link := subscriptionCommon.Links
	if link == nil || link.Self == nil {
		log.Error("Mandatory _links parameter should be present")
		errHandlerProblemDetails(w, "Mandatory attribute _links is missing in the request body.", http.StatusBadRequest)
		return
	}

	selfUrl := strings.Split(link.Self.Href, "/")
	subIdParamStr := selfUrl[len(selfUrl)-1]

	if subsIdStr != subIdParamStr {
		log.Error("SubscriptionId in endpoint and in body not matching")
		errHandlerProblemDetails(w, "SubscriptionId in endpoint and in body not matching", http.StatusNotFound)
		return
	}

	alreadyRegistered := false
	var jsonResponse []byte
	switch subscriptionType {
	case PROV_CHG_UU_UNI:
		jsonResponse, alreadyRegistered, err = processProvChgUuUniSubscriptionUpdate(bodyBytes, subsIdStr)

	case PROV_CHG_UU_MBMS:
		jsonResponse, alreadyRegistered, err = processProvChgUuMbmsSubscriptionUpdate(bodyBytes, subsIdStr)

	case PROV_CHG_PC5:
		jsonResponse, alreadyRegistered, err = processProvChgPc5SubscriptionUpdate(bodyBytes, subsIdStr)

	case V2X_MSG:
		jsonResponse, alreadyRegistered, err = processV2xMsgSubscriptionUpdate(bodyBytes, subsIdStr)

	case PRED_QOS:
		jsonResponse, alreadyRegistered, err = processPredQosSubscriptionUpdate(bodyBytes, subsIdStr)

	default:
		err = errors.New("Unsupported subscriptionType")
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return

	}
	log.Info("individualSubscriptionPut: alreadyRegistered: ", alreadyRegistered)

	if !alreadyRegistered {
		w.WriteHeader(http.StatusNotFound)
	} else {
		if err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(jsonResponse))
}

// individualSubscriptionDelete is to delete a specific subscriptionInfo at subscriptions/{subscriptionId} endpoint
func individualSubscriptionDelete(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> individualSubscriptionDelete: ", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	u, _ := url.Parse(r.URL.String())
	url := u.RequestURI()
	log.Info("url: ", url)
	subsIdStr := string(url[strings.LastIndex(url, "/")+1:])
	log.Info("subsIdStr: ", subsIdStr)

	// Find subscriptionInfo entry in redis DB
	keyName := baseKey + "subscriptions:" + subsIdStr
	log.Info("individualSubscriptionDelete: keyName: ", keyName)
	subscription, err := rc.JSONGetEntry(keyName, ".")
	if err != nil {
		err = errors.New("subscription not found against the provided subscriptionId")
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusNotFound)
		return
	}

	// Delete subscriptionInfo entry from redis DB
	err = delSubscription(subsIdStr, subscription, false)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Send response on successful deletion of subscription resource
	w.WriteHeader(http.StatusNoContent)
}

func v2xNotify(v2xMessage []byte, v2xType int32, msgProtocolVersion int32, stdOrganization string, longitude *float32, latitude *float32) {
	log.Debug(">>> v2xNotify: ", v2xMessage)

	var locationInfo *LocationInfo = nil
	if (longitude != nil) && (latitude != nil) {
		var locationInfoGeoArea = LocationInfoGeoArea{*latitude, *longitude}
		locationInfo = new(LocationInfo)
		locationInfo.Ecgi = nil
		locationInfo.GeoArea = &locationInfoGeoArea
	}
	msgPropertiesValues := V2xMsgPropertiesValues{locationInfo, msgProtocolVersion, string(v2xType), stdOrganization}
	v2xMsgNotification := V2xMsgNotification{
		Links:                   nil,
		MsgContent:              hex.EncodeToString(v2xMessage),
		MsgPropertiesValues:     &msgPropertiesValues,
		MsgRepresentationFormat: "hexadump",
		NotificationType:        V2X_MSG_NOTIF,
		TimeStamp: &TimeStamp{
			Seconds: int32(time.Now().Unix()),
		},
	}
	log.Info("v2xNotify: v2xMsgNotification: ", v2xMsgNotification)

	log.Info("v2xNotify: v2xMsgSubscriptionMap: ", v2xMsgSubscriptionMap)
	for i, sub := range v2xMsgSubscriptionMap {
		log.Info("v2xNotify: i: ", i)
		log.Info("v2xNotify: sub", sub)

		if sub.ExpiryDeadline != nil {
			v2xMsgNotification.TimeStamp = computeElapseTime(*sub.ExpiryDeadline)
		}
		if sub.FilterCriteria != nil && findMsgTypeId(sub.FilterCriteria.MsgType, v2xType) {
			if sub.Links != nil {
				v2xMsgNotification.Links = &Links3{
					Subscription: sub.Links.Self,
				}
			}
			notifyUrl := sub.CallbackReference
			log.Info("v2xNotify: v2xMsgNotification: ", v2xMsgNotification)
			log.Info("v2xNotify: notifyUrl: ", notifyUrl)
			sendV2xMsgNotification(notifyUrl, v2xMsgNotification)
		}
	}
}

func processV2xMsgSubscriptionUpdate(bodyBytes []byte, subsIdStr string) ([]byte, bool, error) {
	log.Debug(">>> processV2xMsgSubscriptionUpdate: subsIdStr: ", subsIdStr)

	var v2xSubscription V2xMsgSubscription
	err := json.Unmarshal(bodyBytes, &v2xSubscription)
	if err != nil {
		log.Error(err.Error())
		return nil, false, err
	}

	// Validating mandatory parameters specific to V2xMsgSubscription in the request body
	if v2xSubscription.SubscriptionType == "" {
		err = errors.New("subscription not found against the provided subscriptionId")
		log.Error(err.Error())
		return nil, false, err
	}

	if v2xSubscription.FilterCriteria.StdOrganization == "" {
		err = errors.New("Mandatory attribute StdOrganization is missing in the request body")
		log.Error(err.Error())
		return nil, false, err
	}

	if v2xSubscription.WebsockNotifConfig != nil {
		err = errors.New("WebsockNotifConfig not supported")
		log.Error(err.Error())
		return nil, false, err
	}

	if v2xSubscription.CallbackReference == "" {
		err = errors.New("Mandatory attribute CallbackReference is missing in the request body")
		log.Error(err.Error())
		return nil, false, err
	}

	if !checkMsgTypeValue(v2xSubscription.FilterCriteria.MsgType) {
		err = errors.New("MsgType parameter should be between 1 and 13")
		log.Error(err.Error())
		return nil, false, &json.UnmarshalTypeError{}
	}
	log.Info("processV2xMsgSubscriptionUpdate: Checks done")

	// registration
	if isSubscriptionIdRegisteredV2x(subsIdStr) {
		registerV2xMsgSubscription(&v2xSubscription, subsIdStr)
		// store subscription key in redis
		keyName := baseKey + "subscriptions:" + subsIdStr
		log.Info("processV2xMsgSubscriptionUpdate: keyName: ", keyName)
		log.Info("processV2xMsgSubscriptionUpdate: provChgUuUniSubscription: ", v2xSubscription)
		err = rc.JSONSetEntry(keyName, ".", convertV2xMsgSubscriptionToJson(&v2xSubscription))
		if err != nil {
			log.Error(err.Error())
			return nil, true, err
		}
		jsonResponse, err := json.Marshal(v2xSubscription)
		if err != nil {
			log.Error(err.Error())
			return nil, true, err
		}
		return jsonResponse, true, nil
	}

	return nil, false, errors.New("Not registered.")
}

func checkMsgTypeValue(msgType []string) bool {
	log.Info(">>> checkMsgTypeValue: ", msgType)

	for _, msgTypeInt := range msgType {
		m := parseMsgTypeToInt(msgTypeInt)
		if m < int32(DENM) || m > int32(MCM) {
			log.Error("checkMsgTypeValue: Invalid MsgType: ", m)
			return false
		}
	} // End of 'for' statement
	return true
}

func validateQueryParams(params url.Values, validParamList []string) bool {
	for param := range params {
		found := false
		for _, validParam := range validParamList {
			if param == validParam {
				found = true
				break
			}
		}
		if !found {
			log.Error("validateQueryParams: Invalid query param: ", param)
			return false
		}
	}

	log.Info("validateQueryParams: succeed")
	return true
}

func validateQueryParamValue(val string, validValues []string) bool {
	for _, validVal := range validValues {
		if val == validVal {
			log.Info("validateQueryParamValue: succeed")
			return true
		}
	}
	log.Error("validateQueryParamValue: Invalid query param value: ", val)
	return false
}

func findMsgTypeId(list []string, item int32) bool {
	if len(list) == 0 {
		return false
	}

	for _, v := range list {
		if parseMsgTypeToInt(v) == item {
			return true
		}
	}

	return false
}

func parseMsgTypeToInt(msgType string) int32 {
	m := -1
	switch msgType {
	case "denm":
		m = 1
	case "cam":
		m = 2
	case "poi":
		m = 3
	case "spatem":
		m = 4
	case "mapem":
		m = 5
	case "ivim":
		m = 6
	case "evrsr":
		m = 7
	case "tistpgtransaction":
		m = 8
	case "srem":
		m = 9
	case "ssem":
		m = 10
	case "evcsn":
		m = 11
	case "saem":
		m = 12
	case "rtcmem":
		m = 13
	case "cpm":
		m = 14
	case "imzm":
		m = 15
	case "vam":
		m = 16
	case "dsm":
		m = 17
	case "mim":
		m = 18
	case "mvm":
		m = 19
	case "mcm":
		m = 20
	}

	return int32(m)
}
