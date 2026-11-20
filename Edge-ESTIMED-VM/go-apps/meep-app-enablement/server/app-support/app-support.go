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
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	sm "github.com/InterDigitalInc/AdvantEDGE/go-apps/meep-app-enablement/server/service-mgmt"
	apps "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-applications"
	dkm "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-data-key-mgr"
	log "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-logger"
	mq "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-mq"
	redis "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-redis"
	subs "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-subscriptions"
	"github.com/google/uuid"

	"github.com/gorilla/mux"
)

const moduleName = "meep-app-enablement"
const appSupportBasePath = "mec_app_support/v2/"
const appEnablementKey = "app-enablement"
const globalLocationName = "global"
const APP_STATE_INITIALIZED = "INITIALIZED"
const APP_STATE_READY = "READY"
const APP_TERMINATION_NOTIF_SUB_TYPE = "AppTerminationNotificationSubscription"
const APP_TERMINATION_NOTIF_TYPE = "AppTerminationNotification"
const DEFAULT_GRACEFUL_TIMEOUT = 10

const serviceName = "App Enablement Service"

// App Info fields
const (
	fieldAppId   string = "id"
	fieldName    string = "name"
	fieldNode    string = "node"
	fieldType    string = "type"
	fieldPersist string = "persist"
	fieldState   string = "state"
)

// MQ payload fields
const (
	mqFieldAppId   string = "id"
	mqFieldPersist string = "persist"
)

var redisAddr string // = "meep-redis-master.default.svc.cluster.local:6379"
var APP_ENABLEMENT_DB = 0

var mutex *sync.Mutex
var rc *redis.Connector
var mqLocal *mq.MsgQueue
var hostUrl *url.URL
var sandboxName string
var mepName string
var basePath string
var baseKey string
var subMgr *subs.SubscriptionMgr
var appStore *apps.ApplicationStore
var appInfoMap map[string]map[string]string
var gracefulTerminateMap = map[string]chan bool{}

func notImplemented(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusNotImplemented)
}

func Init(sandbox string, mep string, host *url.URL, msgQueue *mq.MsgQueue, redisAddr_ string, globalMutex *sync.Mutex) (err error) {
	redisAddr = redisAddr_
	sandboxName = sandbox
	hostUrl = host
	mqLocal = msgQueue
	mutex = globalMutex
	mepName = mep

	// Initialize app info cache
	appInfoMap = make(map[string]map[string]string)

	// Set base path & base storage key
	if mepName == globalLocationName {
		basePath = "/" + sandboxName + "/" + appSupportBasePath
		baseKey = dkm.GetKeyRoot(sandboxName) + appEnablementKey + ":mep-global:"
	} else {
		basePath = "/" + sandboxName + "/" + mepName + "/" + appSupportBasePath
		baseKey = dkm.GetKeyRoot(sandboxName) + appEnablementKey + ":mep:" + mepName + ":"
	}

	// Connect to Redis DB
	rc, err = redis.NewConnector(redisAddr, APP_ENABLEMENT_DB)
	if err != nil {
		log.Error("Failed connection to Redis DB. Error: ", err)
		return err
	}
	log.Info("Connected to Redis DB")

	// Create Application Store
	cfg := &apps.ApplicationStoreCfg{
		Name:      moduleName,
		Namespace: sandboxName,
		RedisAddr: redisAddr,
	}
	appStore, err = apps.NewApplicationStore(cfg)
	if err != nil {
		log.Error("Failed to connect to Application Store. Error: ", err)
		return err
	}
	log.Info("Connected to Application Store")
	// Populate ECS Configuration
	err = setupECSConfiguration()
	if err != nil {
		log.Error("Failed to set up ECS configuration: ", err)
		return err
	}
	// Create Subscription Manager
	subMgrCfg := &subs.SubscriptionMgrCfg{
		Module:         moduleName,
		Sandbox:        sandboxName,
		Mep:            mepName,
		Service:        serviceName,
		Basekey:        baseKey,
		MetricsEnabled: true,
		ExpiredSubCb:   nil,
		PeriodicSubCb:  nil,
		TestNotifCb:    nil,
		NewWsCb:        nil,
	}
	subMgr, err = subs.NewSubscriptionMgr(subMgrCfg, redisAddr)
	if err != nil {
		log.Error("Failed to create Subscription Manager. Error: ", err)
		return err
	}
	log.Info("Created Subscription Manager")

	// TODO -- Initialize subscriptions from DB

	return nil
}

// Run - Start APP support
func Run() (err error) {

	// Register Message Queue handler
	handler := mq.MsgHandler{Handler: msgHandler, UserData: nil}
	_, err = mqLocal.RegisterHandler(handler)
	if err != nil {
		log.Error("Failed to listen for sandbox updates: ", err.Error())
		return err
	}

	// Update app info with latest apps from application store
	err = refreshApps()
	if err != nil {
		log.Error("Failed to sync & process apps with error: ", err.Error())
		return err
	}

	return nil
}

// Stop - Stop APP support
func Stop() (err error) {
	return nil
}

// Message Queue handler
func msgHandler(msg *mq.Msg, userData interface{}) {
	switch msg.Message {
	case mq.MsgAppUpdate:
		log.Debug("RX MSG: ", mq.PrintMsg(msg))
		appStore.Refresh()
		appId := msg.Payload[mqFieldAppId]
		_, _ = updateApp(appId)
	case mq.MsgAppRemove:
		log.Debug("RX MSG: ", mq.PrintMsg(msg))
		appStore.Refresh()
		appId := msg.Payload[mqFieldAppId]
		_ = terminateApp(appId)
	case mq.MsgAppFlush:
		log.Debug("RX MSG: ", mq.PrintMsg(msg))
		appStore.Refresh()
		persist, err := strconv.ParseBool(msg.Payload[mqFieldPersist])
		if err != nil {
			persist = false
		}
		_ = flushApps(persist)
	default:
	}
}

func applicationsConfirmReadyPOST(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> applicationsConfirmReadyPOST: ", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	vars := mux.Vars(r)
	appId := vars["appInstanceId"]

	mutex.Lock()
	defer mutex.Unlock()

	// Make sure App instance exists
	appInfo, err := getApp(appId)
	if err != nil {
		errHandlerProblemDetails(w, err.Error(), http.StatusNotFound)
		return
	}

	// Retrieve App Ready information from request
	var confirmation AppReadyConfirmation
	decoder := json.NewDecoder(r.Body)
	err = decoder.Decode(&confirmation)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Validate App Ready params
	if confirmation.Indication == "" {
		log.Error("Mandatory Indication not present")
		errHandlerProblemDetails(w, "Mandatory Indication not present", http.StatusBadRequest)
		return
	}
	switch confirmation.Indication {
	case "READY":
	default:
		log.Error("Mandatory OperationAction value not valid")
		errHandlerProblemDetails(w, "Mandatory OperationAction value not valid", http.StatusBadRequest)
		return
	}

	// Set App state
	appInfo[fieldState] = APP_STATE_READY

	// Set App Info
	err = setAppInfo(appInfo)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Send response
	w.WriteHeader(http.StatusNoContent)
}

func applicationsConfirmTerminationPOST(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> applicationsConfirmTerminationPOST: ", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	vars := mux.Vars(r)
	appId := vars["appInstanceId"]

	mutex.Lock()
	defer mutex.Unlock()

	// Get App instance
	appInfo, err := getApp(appId)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusNotFound)
		return
	}

	// Validate App info
	log.Info("applicationsConfirmTerminationPOST: appInfo: ", appInfo)
	code, problemDetails, err := validateAppInfo(appInfo)
	if err != nil {
		log.Error(err.Error())
		if problemDetails != "" {
			w.WriteHeader(code)
			fmt.Fprint(w, problemDetails)
		} else {
			errHandlerProblemDetails(w, err.Error(), code)
		}
		return
	}

	// Verify that confirmation is expected
	gracefulTerminateChannel, found := gracefulTerminateMap[appId]
	if !found {
		log.Error("Unexpected App Confirmation Termination Notification")
		errHandlerProblemDetails(w, "Unexpected App Confirmation Termination Notification", http.StatusBadRequest)
		return
	}

	// Retrieve Termination Confirmation data
	var confirmation AppTerminationConfirmation
	decoder := json.NewDecoder(r.Body)
	err = decoder.Decode(&confirmation)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Info("applicationsConfirmTerminationPOST: confirmation: ", confirmation)

	// Validate Termination Confirmation params
	if confirmation.OperationAction == nil {
		log.Error("Mandatory OperationAction not present")
		errHandlerProblemDetails(w, "Mandatory OperationAction not present", http.StatusBadRequest)
		return
	}
	switch *confirmation.OperationAction {
	case STOPPING_OperationActionType, TERMINATING_OperationActionType:
	default:
		log.Error("Mandatory OperationAction value not valid")
		errHandlerProblemDetails(w, "Mandatory OperationAction value not valid", http.StatusBadRequest)
		return
	}

	// Confirm termination
	gracefulTerminateChannel <- true

	// Send response
	w.WriteHeader(http.StatusNoContent)
}

func applicationsSubscriptionsPOST(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> applicationsSubscriptionsPOST: ", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	vars := mux.Vars(r)
	appId := vars["appInstanceId"]
	log.Info("applicationsSubscriptionsPOST: appInstanceId:", appId)

	mutex.Lock()
	defer mutex.Unlock()

	// Get App instance
	appInfo, err := getApp(appId)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusNotFound)
		return
	}
	log.Info("applicationsSubscriptionsPOST: appInfo:", appInfo)

	// Validate App info
	code, problemDetails, err := validateAppInfo(appInfo)
	if err != nil {
		log.Error(err.Error())
		if problemDetails != "" {
			w.WriteHeader(code)
			fmt.Fprint(w, problemDetails)
		} else {
			errHandlerProblemDetails(w, err.Error(), code)
		}
		return
	}

	// Retrieve subscription request
	var appTermNotifSub AppTerminationNotificationSubscription
	decoder := json.NewDecoder(r.Body)
	err = decoder.Decode(&appTermNotifSub)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Verify mandatory properties
	if appTermNotifSub.CallbackReference == "" {
		log.Error("Mandatory CallbackReference parameter not present")
		errHandlerProblemDetails(w, "Mandatory CallbackReference parameter not present", http.StatusBadRequest)
		return
	}
	if appTermNotifSub.SubscriptionType != APP_TERMINATION_NOTIF_SUB_TYPE {
		log.Error("SubscriptionType shall be AppTerminationNotificationSubscription")
		errHandlerProblemDetails(w, "SubscriptionType shall be AppTerminationNotificationSubscription", http.StatusBadRequest)
		return
	}
	if appTermNotifSub.AppInstanceId == "" {
		log.Error("Mandatory AppInstanceId parameter not present")
		errHandlerProblemDetails(w, "Mandatory AppInstanceId parameter not present", http.StatusBadRequest)
		return
	}
	if appTermNotifSub.AppInstanceId != appId {
		log.Error("AppInstanceId in endpoint and in body not matching")
		errHandlerProblemDetails(w, "AppInstanceId in endpoint and in body not matching", http.StatusBadRequest)
		return
	}

	// Get a new subscription ID
	subId := subMgr.GenerateSubscriptionId()

	appTermNotifSub.Links = &Self{
		Self: &LinkType{
			Href: hostUrl.String() + basePath + "applications/" + appId + "/subscriptions/" + subId,
		},
	}

	// Create & store subscription
	subCfg := newAppTerminationNotifSubCfg(&appTermNotifSub, subId, appId)
	jsonSub := convertAppTerminationNotifSubToJson(&appTermNotifSub)
	_, err = subMgr.CreateSubscription(subCfg, jsonSub)
	if err != nil {
		log.Error("Failed to create subscription")
		errHandlerProblemDetails(w, "Failed to create subscription", http.StatusInternalServerError)
		return
	}

	// Send response
	w.Header().Set("Location", appTermNotifSub.Links.Self.Href)
	w.WriteHeader(http.StatusCreated)
	fmt.Fprint(w, jsonSub)
}

func applicationsSubscriptionGET(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> applicationsSubscriptionGET: ", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	vars := mux.Vars(r)
	subId := vars["subscriptionId"]
	appId := vars["appInstanceId"]
	log.Debug("applicationsSubscriptionGET: appId: ", appId)
	log.Debug("applicationsSubscriptionGET: subId: ", subId)

	mutex.Lock()
	defer mutex.Unlock()

	// Get App instance info
	appInfo, err := getApp(appId)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusNotFound)
		return
	}
	log.Debug("applicationsSubscriptionGET: appInfo: ", appInfo)

	// Validate App info
	code, problemDetails, err := validateAppInfo(appInfo)
	if err != nil {
		log.Error(err.Error())
		if problemDetails != "" {
			w.WriteHeader(code)
			fmt.Fprint(w, problemDetails)
		} else {
			errHandlerProblemDetails(w, err.Error(), code)
		}
		return
	}

	// Find subscription by ID
	sub, err := subMgr.GetSubscription(subId)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusNotFound)
		return
	}

	// Validate subscription
	if sub.Cfg.AppId != appId || sub.Cfg.Type != APP_TERMINATION_NOTIF_SUB_TYPE {
		err = errors.New("subscription not found")
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusNotFound)
		return
	}
	log.Debug("applicationsSubscriptionGET: sub.JsonSubOrig: ", sub.JsonSubOrig)

	// Return original marshalled subscription
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, sub.JsonSubOrig)
}

func applicationsSubscriptionDELETE(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> applicationsSubscriptionDELETE: ", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	vars := mux.Vars(r)
	subId := vars["subscriptionId"]
	appId := vars["appInstanceId"]

	mutex.Lock()
	defer mutex.Unlock()

	// Get App instance info
	appInfo, err := getApp(appId)
	if err != nil {
		errHandlerProblemDetails(w, err.Error(), http.StatusNotFound)
		return
	}

	// Validate App info
	code, problemDetails, err := validateAppInfo(appInfo)
	if err != nil {
		log.Error(err.Error())
		if problemDetails != "" {
			w.WriteHeader(code)
			fmt.Fprint(w, problemDetails)
		} else {
			errHandlerProblemDetails(w, err.Error(), code)
		}
		return
	}

	// Find subscription by ID
	sub, err := subMgr.GetSubscription(subId)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusNotFound)
		return
	}

	// Validate subscription
	if sub.Cfg.AppId != appId || sub.Cfg.Type != APP_TERMINATION_NOTIF_SUB_TYPE {
		err = errors.New("subscription not found")
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusNotFound)
		return
	}

	// Delete subscription
	err = subMgr.DeleteSubscription(sub)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Send response
	w.WriteHeader(http.StatusNoContent)
}

func applicationsSubscriptionsGET(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> applicationsSubscriptionsGET: ", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	vars := mux.Vars(r)
	appId := vars["appInstanceId"]
	log.Info("applicationsSubscriptionsGET: appInstanceId: ", appId)

	mutex.Lock()
	defer mutex.Unlock()

	// Get App instance info
	appInfo, err := getApp(appId)
	if err != nil {
		errHandlerProblemDetails(w, err.Error(), http.StatusNotFound)
		return
	}
	log.Info("applicationsSubscriptionsGET: appInfo: ", appInfo)

	// Validate App info
	code, problemDetails, err := validateAppInfo(appInfo)
	if err != nil {
		log.Error(err.Error())
		if problemDetails != "" {
			w.WriteHeader(code)
			fmt.Fprint(w, problemDetails)
		} else {
			errHandlerProblemDetails(w, err.Error(), code)
		}
		return
	}

	// Get subscriptions for App instance
	subList, err := subMgr.GetFilteredSubscriptions(appId, APP_TERMINATION_NOTIF_SUB_TYPE)
	if err != nil {
		log.Error("Failed to get subscription list with err: ", err.Error())
		return
	}

	// Create subscription link list
	subscriptionLinkList := &MecAppSuptApiSubscriptionLinkList{
		Links: &MecAppSuptApiSubscriptionLinkListLinks{
			Self: &LinkType{
				Href: hostUrl.String() + basePath + "applications/" + appId + "/subscriptions",
			},
		},
	}

	for _, sub := range subList {
		// Create subscription reference & append it to link list
		subscription := MecAppSuptApiSubscriptionLinkListSubscription{
			SubscriptionType: APP_TERMINATION_NOTIF_SUB_TYPE,
			Href:             sub.Cfg.Self,
		}
		subscriptionLinkList.Links.Subscriptions = append(subscriptionLinkList.Links.Subscriptions, subscription)
	}

	// Send response
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, convertMecAppSuptApiSubscriptionLinkListToJson(subscriptionLinkList))
}

func timingCapsGET(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> timingCapsGET")

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	// Create timestamp
	seconds := time.Now().Unix()
	timingCaps := TimingCaps{
		TimeStamp: &TimingCapsTimeStamp{
			Seconds: int32(seconds),
		},
	}

	// Send response
	jsonResponse, err := json.Marshal(timingCaps)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(jsonResponse))
}

func timingCurrentTimeGET(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> timingCurrentTimeGET")

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	// Create timestamp
	seconds := time.Now().Unix()
	TimeSourceStatus := TRACEABLE_TimeSourceStatus
	currentTime := CurrentTime{
		Seconds:          int32(seconds),
		TimeSourceStatus: &TimeSourceStatus,
	}

	// Send response
	jsonResponse, err := json.Marshal(currentTime)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(jsonResponse))
}

// setupECSConfiguration stores the mandatory ECS configuration in Redis.
// It uses the hostUrl as the ECS address and returns an error if any step fails.
func setupECSConfiguration() error {
	// Create a map containing the mandatory ECS configuration
	ecsConfig := map[string]interface{}{
		"ECSAddress": hostUrl.String(), // Use the MEC Sandbox URL as the ECS Address
	}

	// Convert the ECS configuration to JSON
	ecsConfigJson, err := json.Marshal(ecsConfig)
	if err != nil {
		log.Error("setupECSConfiguration: failed to marshal ECS configuration", "error", err.Error())
		return fmt.Errorf("failed to marshal ECS configuration: %v", err)
	}

	// Convert JSON bytes to a string for storage
	ecsConfigJsonStr := string(ecsConfigJson)

	// Define the Redis key and store the JSON configuration in Redis
	ecsKey := baseKey + "ecs:config"
	err = rc.JSONSetEntry(ecsKey, ".", ecsConfigJsonStr)
	if err != nil {
		log.Error("setupECSConfiguration: failed to set ECS configuration in Redis", "error", err.Error())
		return fmt.Errorf("failed to set ECS configuration in Redis: %v", err)
	}

	log.Info("setupECSConfiguration: ECS configuration stored successfully in Redis", "key", ecsKey)
	return nil
}

// getECSConfig retrieves the ECS configuration from Redis and returns it as a map.
// An error is returned if the configuration is missing or cannot be unmarshaled.
// func getECSConfig() (map[string]interface{}, error) {
// 	// Define the Redis key for the ECS configuration
// 	ecsKey := baseKey + "ecs:config"
// 	ecsConfigJson, err := rc.JSONGetEntry(ecsKey, ".")
// 	if err != nil {
// 		log.Error("getECSConfig: failed to get ECS configuration from Redis", "error", err.Error())
// 		return nil, fmt.Errorf("failed to get ECS configuration: %v", err)
// 	}

// 	// Unmarshal the JSON configuration into a map
// 	var ecsConfig map[string]interface{}
// 	err = json.Unmarshal([]byte(ecsConfigJson), &ecsConfig)
// 	if err != nil {
// 		log.Error("getECSConfig: failed to unmarshal ECS configuration", "error", err.Error())
// 		return nil, fmt.Errorf("failed to unmarshal ECS configuration: %v", err)
// 	}

// 	log.Info("getECSConfig: successfully retrieved ECS configuration from Redis", "key", ecsKey)
// 	return ecsConfig, nil
// }

// getRegistration retrieves the EEC registration details for a given RegistrationId.
// It extracts the registrationId from the request URL, looks up the corresponding entry in Redis,
// and sends a JSON response with the registration information.
func getRegistration(w http.ResponseWriter, r *http.Request) {
	log.Info("getRegistration: Get EEC Registration by RegistrationId")
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	// Extract the registrationId from the URL variables
	vars := mux.Vars(r)
	registrationId := vars["registrationId"]
	keyName := baseKey + "app:" + registrationId

	// Retrieve the registration information from Redis
	eecPrevReg, err := rc.JSONGetEntry(keyName, ".")
	if err != nil {
		errMsg := "getRegistration: eecRegistration not found for the provided RegistrationId"
		log.Error(errMsg, "registrationId", registrationId, "error", err.Error())
		errHandlerProblemDetails(w, errMsg, http.StatusNotFound)
		return
	}
	if eecPrevReg == "" {
		log.Error("getRegistration: RegistrationId not found in Redis", "registrationId", registrationId)
		errHandlerProblemDetails(w, "Registration not found", http.StatusNotFound)
		return
	}

	// Convert the previously stored registration information to JSON format for the response
	sInfoJson := convertEecPrevRegReqInfoToJson(eecPrevReg)
	jsonResponse, err := json.Marshal(sInfoJson)
	if err != nil {
		log.Error("getRegistration: failed to marshal the response", "error", err.Error())
		errHandlerProblemDetails(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Send the JSON response with a 200 OK status
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(jsonResponse)
	if err != nil {
		log.Error("getRegistration: failed to create the response", "error", err.Error())
		errHandlerProblemDetails(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	log.Info("getRegistration: successfully retrieved registration", "registrationId", registrationId)
}

// updateRegistrationPut updates an existing EEC registration based on the RegistrationId provided in the URL.
// It decodes the update request, validates AcId fields, and updates the stored registration data in Redis.
func updateRegistrationPut(w http.ResponseWriter, r *http.Request) {
	log.Info("updateRegistrationPut: Update EEC Registration by RegistrationId")
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	// Extract registrationId from URL
	vars := mux.Vars(r)
	registrationId := vars["registrationId"]

	// Decode the update request body into ecsRegUpdateReq
	var ecsRegUpdateReq RegistrationsRegistrationIdBody
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&ecsRegUpdateReq); err != nil {
		log.Error("updateRegistrationPut: failed to decode the request body", "error", err.Error())
		errHandlerProblemDetails(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	keyName := baseKey + "app:" + registrationId

	// Retrieve the current registration entry from Redis
	eecPrevReg, err := rc.JSONGetEntry(keyName, ".")
	if err != nil {
		errMsg := "updateRegistrationPut: eecRegistration not found for the provided RegistrationId"
		log.Error(errMsg, "registrationId", registrationId, "error", err.Error())
		errHandlerProblemDetails(w, errMsg, http.StatusNotFound)
		return
	}
	if eecPrevReg == "" {
		log.Error("updateRegistrationPut: RegistrationId not found in Redis", "registrationId", registrationId)
		errHandlerProblemDetails(w, "Registration not found", http.StatusNotFound)
		return
	}

	// Convert the current registration info to a modifiable JSON structure
	sInfoJson := convertEecPrevRegReqInfoToJson(eecPrevReg)

	// Helper function to check if an AcId is valid
	isValidAcId := func(acId string) bool {
		return acId != "" && acId != "string"
	}

	// Validate that at least one valid AcId is provided in the update request
	hasAcId := false
	for _, acProf := range ecsRegUpdateReq.AcProfs {
		if isValidAcId(acProf.AcId) {
			hasAcId = true
			break
		}
	}

	// Process and validate each valid AcId in the update request
	if hasAcId {
		for _, acProf := range ecsRegUpdateReq.AcProfs {
			if isValidAcId(acProf.AcId) {
				appId := acProf.AcId
				log.Debug("updateRegistrationPut: processing AcId", "appId", appId)
				appInfo, err := getAppInfo(appId)
				if err != nil {
					log.Error("updateRegistrationPut: getAppInfo failed", "appId", appId, "error", err.Error())
					errHandlerProblemDetails(w, err.Error(), http.StatusNotFound)
					return
				}

				code, problemDetails, err := validateAppInfo(appInfo)
				if err != nil {
					log.Error("updateRegistrationPut: validateAppInfo error", "appId", appId, "error", err.Error())
					if problemDetails != "" {
						w.WriteHeader(code)
						fmt.Fprint(w, problemDetails)
					} else {
						errHandlerProblemDetails(w, err.Error(), code)
					}
					return
				}
			}
		}
	}

	// Update the registration JSON with new AcProfs, ExpTime, and UeMobilityReq values
	sInfoJson.AcProfs = ecsRegUpdateReq.AcProfs
	sInfoJson.ExpTime = ecsRegUpdateReq.ExpTime
	sInfoJson.UeMobilityReq = ecsRegUpdateReq.UeMobilityReq

	// Convert the updated registration structure back to JSON for storage
	sInfoJson_ := convertEecRegReqInfoToJson(sInfoJson)
	err = rc.JSONSetEntry(keyName, ".", sInfoJson_)
	if err != nil {
		log.Error("updateRegistrationPut: failed to set JSON entry in Redis DB", "registrationId", registrationId, "error", err.Error())
		errHandlerProblemDetails(w, "Failed to set JSON entry in Redis DB", http.StatusInternalServerError)
		return
	}

	// Prepare and send a response indicating a successful update
	response := InlineResponse201{
		ExpirationTime: time.Now(),
	}
	jsonResponse, err := json.Marshal(response)
	if err != nil {
		log.Error("updateRegistrationPut: failed to marshal the response", "error", err.Error())
		errHandlerProblemDetails(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, err = w.Write(jsonResponse)
	if err != nil {
		log.Error("updateRegistrationPut: failed to create the response", "error", err.Error())
		errHandlerProblemDetails(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	log.Info("updateRegistrationPut: registration updated successfully", "registrationId", registrationId)
}

// createEECReg creates a new EEC registration.
// It validates the incoming request, verifies that the request is stored in Redis,
// validates AcId fields, and finally stores the new registration while returning the registration details.
func createEECReg(w http.ResponseWriter, r *http.Request) {
	log.Info("createEECReg: Request to create EEC Registration")
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	mutex.Lock()
	defer mutex.Unlock()

	// Check if the request body is provided
	if r.Body == nil {
		err := errors.New("createEECReg: request body is missing")
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Decode the request body into an EecRegistration structure
	var ecsRegReq EecRegistration
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&ecsRegReq); err != nil {
		log.Error("createEECReg: failed to decode the request body", "error", err.Error())
		errHandlerProblemDetails(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	// Validate that a unique EecId is provided
	if ecsRegReq.EecId == "" || ecsRegReq.EecId == "string" {
		log.Error("createEECReg: invalid request - unique EecId missing or default value provided")
		errHandlerProblemDetails(w, "Please enter the unique EecId", http.StatusBadRequest)
		return
	}

	// Verify that the registration request is already stored in Redis
	key := baseKey + "app:" + ecsRegReq.EecId
	sInfoJson1, err := rc.JSONGetEntry(key, ".")
	if err != nil {
		log.Error("createEECReg: failed to get JSON entry from Redis DB", "error", err.Error())
		errHandlerProblemDetails(w, "Failed to get JSON entry from Redis DB", http.StatusInternalServerError)
		return
	}
	if sInfoJson1 == "" {
		log.Error("createEECReg: registration request is not stored in Redis", "EecId", ecsRegReq.EecId)
		errHandlerProblemDetails(w, "Request is not stored in Redis DB", http.StatusInternalServerError)
		return
	}

	// Helper function to check if an AcId is valid
	isValidAcId := func(acId string) bool {
		return acId != "" && acId != "string"
	}

	// Ensure that at least one valid AcId is provided in the registration request
	hasAcId := false
	for _, acProf := range ecsRegReq.AcProfs {
		if isValidAcId(acProf.AcId) {
			hasAcId = true
			break
		}
	}

	// Validate each valid AcId by retrieving and validating associated application info
	if hasAcId {
		for _, acProf := range ecsRegReq.AcProfs {
			if isValidAcId(acProf.AcId) {
				appId := acProf.AcId
				log.Debug("createEECReg: processing AcId", "appId", appId)
				appInfo, err := getAppInfo(appId)
				if err != nil {
					log.Error("createEECReg: getAppInfo failed", "appId", appId, "error", err.Error())
					errHandlerProblemDetails(w, err.Error(), http.StatusNotFound)
					return
				}

				code, problemDetails, err := validateAppInfo(appInfo)
				if err != nil {
					log.Error("createEECReg: validateAppInfo error", "appId", appId, "error", err.Error())
					if problemDetails != "" {
						w.WriteHeader(code)
						fmt.Fprint(w, problemDetails)
					} else {
						errHandlerProblemDetails(w, err.Error(), code)
					}
					return
				}
			}
		}
	}

	// Dynamically retrieve platform details to verify the registration endpoint and MEC platform ID
	ednConfig, err := getDynamicPlatformDetails(basePath)
	if err != nil {
		log.Error("createEECReg: failed to get EDN config information", "error", err.Error())
		errHandlerProblemDetails(w, "Failed to get the EDN config information", http.StatusBadRequest)
		return
	}

	// Validate that the registration endpoint and source EES ID match the expected EDN config values
	if ecsRegReq.EndPt == nil || ecsRegReq.EndPt.Uri != ednConfig.Eess[0].EndPt.Uri {
		log.Error("createEECReg: endpoint mismatch - EEC registration endpoint does not match EDN config endpoint",
			"provided", ecsRegReq.EndPt, "expected", ednConfig.Eess[0].EndPt)
		errHandlerProblemDetails(w, "Endpoint mismatch: EEC registration endpoint does not match EDN config endpoint", http.StatusBadRequest)
		return
	}
	if ecsRegReq.EndPt == nil || ecsRegReq.SrcEesId != ednConfig.Eess[0].EesId {
		log.Error("createEECReg: endpoint mismatch - SrcEesId does not match EDN config MEC Platform ID",
			"provided", ecsRegReq.SrcEesId, "expected", ednConfig.Eess[0].EesId)
		errHandlerProblemDetails(w, "Endpoint mismatch: SrcEesId does not match EDN config MEC Platform ID", http.StatusBadRequest)
		return
	}

	// Generate a new unique registrationId for the new registration
	registrationId := uuid.New().String()
	sInfoJson := convertEecRegReqInfoToJson(&ecsRegReq)
	key = baseKey + "app:" + registrationId
	err = rc.JSONSetEntry(key, ".", sInfoJson)
	if err != nil {
		log.Error("createEECReg: failed to set JSON entry in Redis DB", "registrationId", registrationId, "error", err.Error())
		errHandlerProblemDetails(w, "Failed to set JSON entry in Redis DB", http.StatusInternalServerError)
		return
	}

	// Prepare the response with registration details
	response := InlineResponse201{
		RegistrationID:             registrationId,
		ExpirationTime:             time.Now(),
		EECContextID:               "example-context-id", // Replace with actual context ID if available
		EECContextRelocationStatus: true,
		DiscoveredEASList:          []EasProfile{}, // Populate with actual EasProfile values if needed
	}

	jsonResponse, err := json.Marshal(response)
	if err != nil {
		log.Error("createEECReg: failed to marshal the response", "error", err.Error())
		errHandlerProblemDetails(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Send the successful creation response
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(jsonResponse)
	if err != nil {
		log.Error("createEECReg: failed to create the response", "error", err.Error())
		errHandlerProblemDetails(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	log.Info("createEECReg: registration created successfully", "registrationId", registrationId)
}

// deleteIndEECReg deletes an individual EEC registration identified by RegistrationId.
// It removes the corresponding entry from Redis and returns a 204 No Content response on success.
func deleteIndEECReg(w http.ResponseWriter, r *http.Request) {
	log.Info("deleteIndEECReg: Delete EEC Registration by RegistrationId")
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	// Extract registrationId from URL
	vars := mux.Vars(r)
	registrationId := vars["registrationId"]
	keyName := baseKey + "app:" + registrationId

	// Check if the registration exists in Redis
	_, err := rc.JSONGetEntry(keyName, ".")
	if err != nil {
		errMsg := "deleteIndEECReg: eecRegistration not found for the provided RegistrationId"
		log.Error(errMsg, "registrationId", registrationId, "error", err.Error())
		errHandlerProblemDetails(w, errMsg, http.StatusNotFound)
		return
	}

	// Delete the registration entry from Redis
	err = rc.JSONDelEntry(keyName, ".")
	if err != nil {
		log.Error("deleteIndEECReg: failed to delete JSON entry from Redis DB", "registrationId", registrationId, "error", err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusNotFound)
		return
	}

	log.Info("deleteIndEECReg: registration deleted successfully", "registrationId", registrationId)
	// Send a 204 No Content response on successful deletion
	w.WriteHeader(http.StatusNoContent)
}

// requestServProv handles requests for service provisioning. It validates the request payload,
// ensures proper synchronization, checks required fields, and finally stores the request in Redis
// before sending the platform configuration as a response.
func requestServProv(w http.ResponseWriter, r *http.Request) {
	log.Info("requestServProv: Received request to provide service")

	// Set response header to JSON
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	// Ensure thread safety when accessing shared resources
	mutex.Lock()
	defer mutex.Unlock()

	// Check if the request body is present
	if r.Body == nil {
		err := errors.New("requestServProv: request body is missing")
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Decode the incoming JSON request into an EcsServProvReq structure
	var ecsServReq EcsServProvReq
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&ecsServReq); err != nil {
		log.Error("requestServProv: failed to decode request body", "error", err.Error())
		errHandlerProblemDetails(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	// Validate that the EecId is provided and is not a placeholder value
	if ecsServReq.EecId == "" || ecsServReq.EecId == "string" {
		log.Error("requestServProv: invalid request - unique EecId missing or default value provided")
		errHandlerProblemDetails(w, "Please enter the unique EecId", http.StatusBadRequest)
		return
	}

	// Helper function to check if a provided AcId is valid (i.e. non-empty and not the default "string")
	isValidAcId := func(acId string) bool {
		return acId != "" && acId != "string"
	}

	// Verify that either a valid AcId is provided within AcProfs or LocationInfo is present.
	hasAcId := false
	for _, acProf := range ecsServReq.AcProfs {
		if isValidAcId(acProf.AcId) {
			hasAcId = true
			break
		}
	}
	if !hasAcId && ecsServReq.LocInf == nil {
		log.Error("requestServProv: invalid request - both AcId and LocationInfo are missing")
		errHandlerProblemDetails(w, "Either a valid AcId or LocationInfo must be provided", http.StatusBadRequest)
		return
	}

	// Process each valid AcId in the AcProfs slice
	if hasAcId {
		for _, acProf := range ecsServReq.AcProfs {
			if isValidAcId(acProf.AcId) {
				appId := acProf.AcId
				log.Debug("requestServProv: processing AcId", "appId", appId)
				// Retrieve application info for the given appId
				appInfo, err := getAppInfo(appId)
				if err != nil {
					log.Error("requestServProv: getAppInfo failed", "appId", appId, "error", err.Error())
					errHandlerProblemDetails(w, err.Error(), http.StatusNotFound)
					return
				}

				// Validate the retrieved appInfo; if invalid, send an appropriate response.
				code, problemDetails, err := validateAppInfo(appInfo)
				if err != nil {
					log.Error("requestServProv: validateAppInfo error", "appId", appId, "error", err.Error())
					if problemDetails != "" {
						w.WriteHeader(code)
						fmt.Fprint(w, problemDetails)
					} else {
						errHandlerProblemDetails(w, err.Error(), code)
					}
					return
				}
			}
		}
	}

	// Process LocationInfo if provided
	if ecsServReq.LocInf != nil {
		lat := ecsServReq.LocInf.GeographicArea.Point.Point.Lat
		lon := ecsServReq.LocInf.GeographicArea.Point.Point.Lon
		log.Info("requestServProv: received coordinates", "latitude", lat, "longitude", lon)

		// Validate the geographic coordinates to ensure they are within acceptable boundaries.
		if !isValidCoordinates(lat, lon) {
			log.Error("requestServProv: invalid location - MEC platform not found for provided coordinates",
				"latitude", lat, "longitude", lon)
			errHandlerProblemDetails(w, "MEC platform not found for this location", http.StatusNotFound)
			return
		}
	}

	// Convert the ECS service provisioning request info into JSON format for storage.
	sInfoJson := convertEcsServProvReqInfoToJson(&ecsServReq)
	key := baseKey + "app:" + ecsServReq.EecId

	// Store the request in Redis DB.
	if err := rc.JSONSetEntry(key, ".", sInfoJson); err != nil {
		log.Error("requestServProv: failed to set JSON entry in Redis DB", "error", err.Error())
		errHandlerProblemDetails(w, "Failed to set JSON entry in Redis DB", http.StatusInternalServerError)
		return
	}

	// Verify that the entry was successfully stored by retrieving it.
	sInfoJson1, err := rc.JSONGetEntry(key, ".")
	if err != nil {
		log.Error("requestServProv: failed to get JSON entry from Redis DB", "error", err.Error())
		errHandlerProblemDetails(w, "Failed to get JSON entry from Redis DB", http.StatusInternalServerError)
		return
	}
	if sInfoJson1 == "" {
		log.Error("requestServProv: request not stored in Redis DB", "key", key)
		errHandlerProblemDetails(w, "Request is not stored in Redis DB", http.StatusInternalServerError)
		return
	}

	// Dynamically retrieve platform configuration details using the basePath.
	ednConfig, err := getDynamicPlatformDetails(basePath)
	if err != nil {
		log.Error("requestServProv: failed to get dynamic platform details", "error", err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Marshal the platform configuration to JSON for the response.
	jsonResponse, err := json.Marshal(ednConfig)
	if err != nil {
		log.Error("requestServProv: failed to marshal response", "error", err.Error())
		errHandlerProblemDetails(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Send the successful response with HTTP 200 OK.
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(jsonResponse)
	if err != nil {
		log.Error("requestServProv: failed to create the response", "error", err.Error())
		errHandlerProblemDetails(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	log.Info("requestServProv: successfully processed service provisioning request", "EecId", ecsServReq.EecId)
}

// getDynamicPlatformDetails extracts platform configuration details dynamically based on the provided basePath.
// It returns an EdnConfigInfo structure containing the list of EES (Edge Services) configurations or an error if any occurs.
func getDynamicPlatformDetails(basePath string) (*EdnConfigInfo, error) {
	platformDetails, err := getPlatformDetailsFromBasePath(basePath)
	if err != nil {
		log.Error("getDynamicPlatformDetails: error retrieving platform details from basePath", "error", err)
		return nil, err
	}

	// Build EES information from the retrieved platform details.
	eesInfo := EesInfo{
		EesId: platformDetails.EesId,
		EndPt: platformDetails.EndPt,
	}

	return &EdnConfigInfo{
		Eess: []EesInfo{eesInfo},
	}, nil
}

// isValidCoordinates checks whether the given latitude and longitude fall within the expected geographic boundaries,
// allowing a small tolerance for minor discrepancies.
func isValidCoordinates(lat, lon float64) bool {
	const tolerance = 0.0001
	return lat >= (43.7244-tolerance) && lat <= (43.7515+tolerance) &&
		lon >= (7.4090-tolerance) && lon <= (7.4390+tolerance)
}

// getPlatformDetailsFromBasePath parses the basePath to extract the platform identifier and constructs the corresponding URL.
// It returns an EesInfo structure containing the platform's EES identifier and endpoint or an error if the parsing fails.
func getPlatformDetailsFromBasePath(basePath string) (*EesInfo, error) {
	// Locate the "/mep" segment in the path. This marks the beginning of the platform-specific portion.
	mepIndex := strings.Index(basePath, "/mep")
	if mepIndex == -1 {
		err := errors.New("getPlatformDetailsFromBasePath: invalid base path, '/mep' not found")
		log.Error(err.Error())
		return nil, err
	}

	// The namespace is the part of the basePath before "/mep".
	namespace := basePath[:mepIndex]
	// Extract the platform-specific segment after "/mep".
	platformPart := basePath[mepIndex+1:]
	nextSlashIndex := strings.Index(platformPart, "/")
	var platformIdentifier string
	if nextSlashIndex != -1 {
		platformIdentifier = platformPart[:nextSlashIndex]
	} else {
		platformIdentifier = platformPart
	}

	// Construct the full URL for the MEC platform by combining hostUrl with the namespace and platform identifier.
	mecPlatformUrl := strings.TrimSuffix(hostUrl.String(), "/") + "/" +
		strings.TrimPrefix(namespace, "/") + "/" + platformIdentifier

	log.Debug("getPlatformDetailsFromBasePath: constructed MEC platform URL", "url", mecPlatformUrl)

	return &EesInfo{
		EesId: platformIdentifier,
		EndPt: &EndPoint{
			Uri: mecPlatformUrl,
		},
	}, nil
}

// getEASDiscInfo handles HTTP requests to retrieve EAS (Edge Application Service) discovery information.
// It validates the request, retrieves application configuration from the datastore, and returns a JSON response.
func getEASDiscInfo(w http.ResponseWriter, r *http.Request) {
	log.Debug("getEASDiscInfo: request received", "method", r.Method, "url", r.URL)

	// Set the response content type to JSON.
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	// Decode the incoming JSON request into an EasDiscoveryReq structure.
	var discReq EasDiscoveryReq
	if err := json.NewDecoder(r.Body).Decode(&discReq); err != nil {
		log.Error("getEASDiscInfo: error decoding request body", "error", err)
		errHandlerProblemDetails(w, "Invalid request body.", http.StatusBadRequest)
		return
	}
	log.Info("getEASDiscInfo: decoded request info", "discReq", discReq)

	// Immediately reject requests that include an unsupported EasId.
	if discReq.RequestorId.EasId != "" {
		log.Error("getEASDiscInfo: unsupported EasId provided", "EasId", discReq.RequestorId.EasId)
		errHandlerProblemDetails(w, "EasId is not supported in this implementation", http.StatusBadRequest)
		return
	}

	// Validate that exactly one of EecId or EesId is provided in the request.
	idCount := 0
	if discReq.RequestorId.EecId != "" {
		idCount++
	}
	if discReq.RequestorId.EesId != "" {
		idCount++
	}
	if idCount != 1 {
		log.Error("getEASDiscInfo: request validation failed - exactly one identifier required", "EecId", discReq.RequestorId.EecId, "EesId", discReq.RequestorId.EesId)
		errHandlerProblemDetails(w, "Exactly one of eecId or eesId must be provided", http.StatusBadRequest)
		return
	}

	// Process EecId if provided.
	if discReq.RequestorId.EecId != "" {
		key := baseKey + "app:" + discReq.RequestorId.EecId
		sInfoJson1, err := rc.JSONGetEntry(key, ".")
		if err != nil {
			log.Error("getEASDiscInfo: invalid EecId", "EecId", discReq.RequestorId.EecId, "error", err)
			errHandlerProblemDetails(w, "Invalid EecId", http.StatusBadRequest)
			return
		}
		if sInfoJson1 == "" {
			log.Error("getEASDiscInfo: no data found for provided EecId", "EecId", discReq.RequestorId.EecId)
			errHandlerProblemDetails(w, "No data found for EecId", http.StatusNotFound)
			return
		}
	} else {
		// Validate the provided EesId against the dynamically generated platform configuration.
		ednConfig, err := getDynamicPlatformDetails(basePath)
		if err != nil {
			log.Error("getEASDiscInfo: error retrieving dynamic platform details", "error", err)
			errHandlerProblemDetails(w, "Platform configuration error: "+err.Error(), http.StatusInternalServerError)
			return
		}

		if len(ednConfig.Eess) == 0 || ednConfig.Eess[0].EesId == "" {
			log.Error("getEASDiscInfo: missing EES configuration in server settings")
			errHandlerProblemDetails(w, "Server configuration error", http.StatusInternalServerError)
			return
		}

		expectedEesId := ednConfig.Eess[0].EesId
		if discReq.RequestorId.EesId != expectedEesId {
			log.Error("getEASDiscInfo: provided EesId does not match expected configuration", "providedEesId", discReq.RequestorId.EesId, "expectedEesId", expectedEesId)
			errHandlerProblemDetails(w, "Invalid EesId", http.StatusBadRequest)
			return
		}
	}

	// Retrieve the appInstanceId from the first AcCharacteristics entry if present.
	var appInstanceId string
	if discReq.EasDiscoveryFilter != nil && len(discReq.EasDiscoveryFilter.AcChars) > 0 {
		acChar := discReq.EasDiscoveryFilter.AcChars[0]
		if acChar.AcProf != nil {
			appInstanceId = acChar.AcProf.AcId
		}
	}

	// Ensure that appInstanceId is present in the request.
	if appInstanceId == "" {
		err := errors.New("getEASDiscInfo: acId not found in the request")
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Construct the datastore key for retrieving app information.
	keyName := baseKey + "appInfo:" + appInstanceId
	log.Info("getEASDiscInfo: retrieving app information", "keyName", keyName)
	jsonAppInfo, err := rc.JSONGetEntry(keyName, ".")
	if err != nil {
		errMsg := "appInfo not found for the provided appInstanceId"
		log.Error("getEASDiscInfo:", errMsg, "appInstanceId", appInstanceId, "error", err)
		errHandlerProblemDetails(w, errMsg, http.StatusNotFound)
		return
	}

	// Unmarshal the retrieved JSON data into the AppInfo structure.
	var appInfo AppInfo
	if err := json.Unmarshal([]byte(jsonAppInfo), &appInfo); err != nil {
		log.Error("getEASDiscInfo: error unmarshaling appInfo", "error", err)
		errHandlerProblemDetails(w, "Internal server error.", http.StatusInternalServerError)
		return
	}

	// Map the AppInfo data to the EASProfile response format.
	easProfile := EasProfile{
		EasId:   appInfo.AppName,
		EndPt:   appInfo.Endpoint,
		ProvId:  appInfo.AppProvider,
		Type_:   appInfo.AppCategory,
		Scheds:  appInfo.Scheds,
		SvcArea: appInfo.SvcArea,
		SvcKpi:  appInfo.SvcKpi,
		PermLvl: appInfo.PermLvl,
		AcIds:   []string{appInfo.AppInstanceId},
	}

	// Build the full discovery response with the mapped EAS profile.
	resp := EasDiscoveryResp{
		DiscoveredEas: []DiscoveredEas{
			{
				Eas: &easProfile,
			},
		},
	}

	// Convert the response to JSON and send it with an HTTP 200 OK status.
	jsonResponse := convertEasDiscoveryRespToJson(&resp)
	w.WriteHeader(http.StatusOK)
	log.Info("getEASDiscInfo: successfully processed request", "appInstanceId", appInstanceId)
	fmt.Fprint(w, jsonResponse)
}

/*
* appRegistrationPOST handles the registration of applications.
* It decodes the request body into an AppInfo struct, validates mandatory parameters, retrieves app instance information, validates the app info, and stores it in Redis.
* @param {string} keyName	App Info stored with this key
* @param {jsonResponse} contains APP Info response
* @return {error} error An error will return if occurs
 */
func appRegistrationPOST(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> appRegistrationPOST:", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	// Parse request body into AppInfo struct
	var appInfo AppInfo
	if err := json.NewDecoder(r.Body).Decode(&appInfo); err != nil {
		log.Error("Error decoding request body:", err)
		errHandlerProblemDetails(w, "Invalid request body.", http.StatusBadRequest)
		return
	}
	log.Info("appRegistrationPOST: Received appInfo:", appInfo)

	// Validate required fields
	if appInfo.AppName == "" {
		log.Error("Missing mandatory parameter: AppName")
		errHandlerProblemDetails(w, "Mandatory attribute AppName is missing.", http.StatusBadRequest)
		return
	}

	if appInfo.AppInstanceId == "" {
		log.Error("Missing mandatory parameter: AppInstanceId")
		errHandlerProblemDetails(w, "Mandatory attribute AppInstanceId is missing.", http.StatusBadRequest)
		return
	}

	if !appInfo.IsInsByMec && appInfo.Endpoint == nil {
		log.Error("Endpoint is required when IsInsByMec is FALSE")
		errHandlerProblemDetails(w, "Endpoint is required when IsInsByMec is FALSE.", http.StatusBadRequest)
		return
	}
	if appInfo.IsInsByMec {
		// Process appProfile if provided
		if appInfo.AppProfile != nil {
			// Validate appProvider and other fields mapped to EASProfile
			if appInfo.AppProvider != appInfo.AppProfile.ProvId {
				log.Error("Mismatch between appProvider in AppInfo and provId in appProfile")
				errHandlerProblemDetails(w, "appProvider and provId must match.", http.StatusBadRequest)
				return
			}

			if !reflect.DeepEqual(getEndpointUris(appInfo.Endpoint), getProfileEndpointUris(appInfo.AppProfile.EndPt)) {
				log.Error("Mismatch between endpoint in AppInfo and endPt in appProfile")
				errHandlerProblemDetails(w, "Endpoint and endPt must match.", http.StatusBadRequest)
				return
			}

			if appInfo.AppProfile.EasId == "" {
				log.Error("Missing mandatory parameter: easId")
				errHandlerProblemDetails(w, "Mandatory attribute easId is missing.", http.StatusBadRequest)
				return
			}

			if appInfo.AppName != appInfo.AppProfile.EasId {
				log.Error("Mismatch between AppName in AppInfo and EasId in appProfile")
				errHandlerProblemDetails(w, "AppName and EasId must match.", http.StatusBadRequest)
				return
			}
			// Additional checks for attributes such as scheds, svcArea, etc., as required.
		}
	}

	// Retrieve App instance information
	log.Info("appRegistrationPOST: Processing AppInstanceId:", appInfo.AppInstanceId)
	appId, err := getAppInfo(appInfo.AppInstanceId)
	if err != nil {
		log.Error("Error retrieving app instance:", err)
		errHandlerProblemDetails(w, "App instance not found.", http.StatusNotFound)
		return
	}
	log.Info("appRegistrationPOST: Retrieved appId:", appId)

	// Validate the retrieved AppInfo
	code, problemDetails, err := validateAppInfo(appId)
	if err != nil {
		log.Error("Error validating app info:", err)
		if problemDetails != "" {
			w.WriteHeader(code)
			fmt.Fprint(w, problemDetails)
		} else {
			errHandlerProblemDetails(w, "Validation error.", code)
		}
		return
	}

	// Store AppInfo in Redis
	keyName := baseKey + "appInfo:" + appInfo.AppInstanceId
	log.Info("appRegistrationPOST: Storing data with key:", keyName)
	if err := rc.JSONSetEntry(keyName, ".", convertAppInfoToJson(&appInfo)); err != nil {
		log.Error("Failed to store registration in Redis:", err)
		errHandlerProblemDetails(w, "Server error. Could not store registration.", http.StatusInternalServerError)
		return
	}

	// Generate resource URI for the created app registration
	resourceURI := hostUrl.String() + basePath + "registrations/" + appInfo.AppInstanceId
	log.Info("appRegistrationPOST: Generated resource URI:", resourceURI)
	w.Header().Set("Location", resourceURI)

	// Send JSON response with status 201 Created
	jsonResponse := convertAppInfoToJson(&appInfo)
	w.WriteHeader(http.StatusCreated)
	fmt.Fprint(w, jsonResponse)
}

func getEndpointUris(endpoint *OneOfAppInfoEndpoint) []string {
	if endpoint == nil {
		return nil
	}

	// Access the `Uris` field directly from `EndPointInfoUris`
	return endpoint.EndPointInfoUris.Uris
}

func getProfileEndpointUris(endPt *OneOfAppProfileEndPt) []string {
	if endPt == nil {
		return nil
	}

	// Access the `Uris` field directly from `EndPointInfoUris`
	return endPt.EndPointInfoUris.Uris
}

/*
* appRegistrationGET handles retrieving the registration information of an application.
* It fetches the appInstanceId from the URL variables, retrieves the associated application info from Redis, and sends it back in the response. If the appInstanceId is not found or an error occurs, it responds with the appropriate status and error message.
* @param {http.ResponseWriter} w   HTTP response writer
* @param {string} jsonAppInfo Application information
* @return {error} error An error will return if occurs
 */
func appRegistrationGET(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> appRegistrationGET: ", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	vars := mux.Vars(r)
	appInstanceId := vars["appInstanceId"]
	log.Info("appRegistrationGET: appInstanceId: ", appInstanceId)

	mutex.Lock()
	defer mutex.Unlock()

	keyName := baseKey + "appInfo:" + appInstanceId
	log.Info("appRegistrationGET: keyName: ", keyName)
	jsonAppInfo, err := rc.JSONGetEntry(keyName, ".")
	if err != nil {
		err = errors.New("appInfo not found against the provided appInstanceId")
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusNotFound)
		return
	}

	var appInfo AppInfo
	err = json.Unmarshal([]byte(jsonAppInfo), &appInfo)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Info("appRegistrationGET: appInfo: ", appInfo)

	// Marshal the AppInfo struct to JSON format
	jsonResponse := convertAppInfoToJson(&appInfo)

	// write the JSON response
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, jsonResponse)
}

/*
* appRegistrationPUT modifies the registration information of an application.
* It fetches the appInstanceId from the URL variables, decodes the request body into an AppInfo struct, validates the parameters, checks for the existence of the app info in Redis, updates the Redis entry if validation passes,and responds with the appropriate status.
* @param {string} jsonAppInfo Application information
* @param {http.ResponseWriter} w   HTTP response writer
* @param {http.Request} r          HTTP request
* @return {error} error An error will return if occurs
 */
func appRegistrationPUT(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> appRegistrationPUT:", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	vars := mux.Vars(r)
	appInstanceId := vars["appInstanceId"]
	log.Info("appRegistrationPUT: Received appInstanceId:", appInstanceId)

	// Decode JSON input from request body into AppInfo structure
	var appInfoPut AppInfo
	if err := json.NewDecoder(r.Body).Decode(&appInfoPut); err != nil {
		log.Error("Error decoding request body:", err)
		errHandlerProblemDetails(w, "Invalid request body.", http.StatusBadRequest)
		return
	}
	log.Info("appRegistrationPUT: Parsed appInfoPut:", appInfoPut)

	// Check if the appInfo exists in Redis for the given appInstanceId
	keyName := baseKey + "appInfo:" + appInstanceId
	log.Info("appRegistrationPUT: Checking existence of key:", keyName)
	jsonAppInfo, _ := rc.JSONGetEntry(keyName, ".")
	if jsonAppInfo == "" {
		log.Error("No appInfo found for provided appInstanceId")
		errHandlerProblemDetails(w, "appInfo not found for the provided appInstanceId", http.StatusNotFound)
		return
	}

	// Validate appInstanceId consistency between URL and request body
	if appInfoPut.AppInstanceId != "" && appInstanceId != appInfoPut.AppInstanceId {
		log.Error("appInstanceId mismatch between endpoint and request body")
		errHandlerProblemDetails(w, "appInstanceId in endpoint and request body do not match", http.StatusBadRequest)
		return
	}

	// Validate required parameters
	if appInfoPut.AppName == "" {
		log.Error("Missing mandatory parameter: AppName")
		errHandlerProblemDetails(w, "Mandatory attribute AppName is missing.", http.StatusBadRequest)
		return
	}

	if !appInfoPut.IsInsByMec && appInfoPut.Endpoint == nil {
		log.Error("Endpoint is required when IsInsByMec is FALSE")
		errHandlerProblemDetails(w, "Endpoint is required when IsInsByMec is FALSE.", http.StatusBadRequest)
		return
	}

	// Process appProfile if provided
	if appInfoPut.AppProfile != nil {
		// Validate appProvider and associated fields as per EASProfile mapping
		if appInfoPut.AppProvider != appInfoPut.AppProfile.ProvId {
			log.Error("Mismatch between appProvider in AppInfo and provId in appProfile")
			errHandlerProblemDetails(w, "appProvider and provId must match.", http.StatusBadRequest)
			return
		}

		if !reflect.DeepEqual(getEndpointUris(appInfoPut.Endpoint), getProfileEndpointUris(appInfoPut.AppProfile.EndPt)) {
			log.Error("Mismatch between endpoint in AppInfo and endPt in appProfile")
			errHandlerProblemDetails(w, "Endpoint and endPt must match.", http.StatusBadRequest)
			return
		}

		if appInfoPut.AppProfile.EasId == "" {
			log.Error("Missing mandatory parameter: easId")
			errHandlerProblemDetails(w, "Mandatory attribute easId is missing.", http.StatusBadRequest)
			return
		}

		if appInfoPut.AppName != appInfoPut.AppProfile.EasId {
			log.Error("Mismatch between AppName in AppInfo and EasId in appProfile")
			errHandlerProblemDetails(w, "AppName and EasId must match.", http.StatusBadRequest)
			return
		}
		// Additional consistency checks for fields such as scheds, svcArea, etc., as required.
	}

	// Set appInstanceId in appInfoPut and store in Redis
	appInfoPut.AppInstanceId = appInstanceId
	log.Info("appRegistrationPUT: Storing updated appInfo for appInstanceId:", appInstanceId)
	if err := rc.JSONSetEntry(keyName, ".", convertAppInfoToJson(&appInfoPut)); err != nil {
		log.Error("Failed to store updated AppInfo in Redis:", err)
		errHandlerProblemDetails(w, "Server error. Could not store updated appInfo.", http.StatusInternalServerError)
		return
	}

	// Respond with status 204 No Content to indicate successful update
	w.WriteHeader(http.StatusNoContent)
}

/*
* appRegistrationDELETE handles the deletion of an application's registration information.
* It fetches the appInstanceId from the URL variables, checks if the app info exists in Redis, deletes the Redis entry if it exists, and responds with the appropriate status.
* @param {http.ResponseWriter} w   HTTP response writer
* @param {http.Request} r          HTTP request
* @return {error} error An error will return if occurs
 */
func appRegistrationDELETE(w http.ResponseWriter, r *http.Request) {
	log.Info("Delete appInfo by appInstanceId")

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	vars := mux.Vars(r)
	appInstanceId := vars["appInstanceId"]

	keyName := baseKey + "appInfo:" + appInstanceId

	// Find appInfo entry in redis DB
	_, err := rc.JSONGetEntry(keyName, ".")
	if err != nil {
		err = errors.New("appInfo not found against the provided appInstanceId")
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusNotFound)
		return
	}

	// Delete appInfo entry from redis DB
	err = rc.JSONDelEntry(keyName, ".")
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusNotFound)
		return
	}

	// Send response on successful deletion of registration
	w.WriteHeader(http.StatusNoContent)
}

func deleteAppInstance(appId string) {
	log.Debug(">>> deleteAppInstance: ", appId)

	// Delete app support subscriptions
	err := subMgr.DeleteFilteredSubscriptions(appId, APP_TERMINATION_NOTIF_SUB_TYPE)
	if err != nil {
		log.Error(err.Error())
	}

	// Clear App instance service subscriptions
	_ = sm.DeleteServiceSubscriptions(appId)

	// Clear App services
	_ = sm.DeleteServices(appId)

	// Flush App instance data
	keyName := baseKey + "app:" + appId
	log.Info("deleteAppInstance: keyName: ", keyName)
	_ = rc.DBFlush(keyName)

	// Confirm App removal
	sendAppRemoveCnf(appId)
}

func getAppList() ([]map[string]string, error) {
	log.Debug(">>> getAppList")

	var appInfoList []map[string]string

	// Get all applications from DB
	keyMatchStr := baseKey + "app:*:info"
	err := rc.ForEachEntry(keyMatchStr, populateAppInfo, &appInfoList)
	if err != nil {
		log.Error("Failed to get app info list with error: ", err.Error())
		return nil, err
	}
	return appInfoList, nil
}

func populateAppInfo(key string, entry map[string]string, userData interface{}) error {
	appInfoList := userData.(*[]map[string]string)

	// Copy entry
	appInfo := make(map[string]string, len(entry))
	for k, v := range entry {
		appInfo[k] = v
	}

	// Add app info to list
	*appInfoList = append(*appInfoList, appInfo)
	return nil
}

func validateAppInfo(appInfo map[string]string) (int, string, error) {
	// Make sure App is in ready state
	if appInfo[fieldState] != APP_STATE_READY {
		var problemDetails ProblemDetails
		problemDetails.Status = http.StatusForbidden
		problemDetails.Detail = "App Instance not ready. Waiting for AppReadyConfirmation."
		return http.StatusForbidden, convertProblemDetailsToJson(&problemDetails), errors.New("App Instance not ready")
	}
	return http.StatusOK, "", nil
}

func newAppInfo(app *apps.Application) (map[string]string, error) {
	log.Debug(">>> newAppInfo: ", app)

	// Validate app
	if app == nil {
		return nil, errors.New("nil application")
	}

	// Create App Info
	appInfo := make(map[string]string)
	appInfo[fieldAppId] = app.Id
	appInfo[fieldName] = app.Name
	appInfo[fieldNode] = app.Node
	appInfo[fieldType] = app.Type
	appInfo[fieldPersist] = strconv.FormatBool(app.Persist)
	appInfo[fieldState] = APP_STATE_INITIALIZED
	return appInfo, nil
}

func setAppInfo(appInfo map[string]string) error {
	log.Debug(">>> setAppInfo: ", appInfo)

	appId, found := appInfo[fieldAppId]
	if !found || appId == "" {
		return errors.New("missing app instance id")
	}

	// Convert value type to interface{} before storing app info
	entry := make(map[string]interface{}, len(appInfo))
	for k, v := range appInfo {
		entry[k] = v
	}

	// Store entry
	keyName := baseKey + "app:" + appId + ":info"
	log.Info("setAppInfo: keyName: ", keyName)
	err := rc.SetEntry(keyName, entry)
	if err != nil {
		return err
	}

	// Cache entry
	appInfoMap[appId] = appInfo

	return nil
}

func delAppInfo(appInfo map[string]string) error {
	log.Debug(">>> delAppInfo: ", appInfo)

	appId := appInfo[fieldAppId]

	// Clear graceful termination
	delete(gracefulTerminateMap, appId)

	// Remove from cache
	delete(appInfoMap, appId)

	// Delete app instance
	deleteAppInstance(appId)

	return nil
}

func refreshApps() error {
	// Refresh app store
	appStore.Refresh()

	// Get full App store app list
	fullAppList, err := appStore.GetAll()
	if err != nil {
		log.Error(err.Error())
		return err
	}

	// If MEP instance, ignore non-local apps
	appList := make([]*apps.Application, 0)
	for _, app := range fullAppList {
		if mepName != globalLocationName && app.Node != mepName {
			log.Debug("Ignoring update on non-local MEP for app: ", app.Id)
			continue
		}
		appList = append(appList, app)
	}

	mutex.Lock()
	defer mutex.Unlock()

	// Retrieve app info list from DB
	appInfoList, err := getAppList()
	if err != nil {
		log.Error(err.Error())
		return err
	}

	// Update app info
	for _, app := range appList {
		found := false
		for _, appInfo := range appInfoList {
			if appInfo[fieldAppId] == app.Id {
				found = true
				// Set existing app info to make sure cache is updated
				err = setAppInfo(appInfo)
				if err != nil {
					log.Error(err.Error())
				}
				break
			}
		}
		// Create & set app info for new apps
		if !found {
			appInfo, err := newAppInfo(app)
			if err != nil {
				log.Error(err.Error())
				continue
			}
			err = setAppInfo(appInfo)
			if err != nil {
				log.Error(err.Error())
				continue
			}
		}
	}

	// Remove deleted app info
	for _, appInfo := range appInfoList {
		found := false
		for _, app := range appList {
			if app.Id == appInfo[fieldAppId] {
				found = true
				break
			}
		}
		if !found {
			err := delAppInfo(appInfo)
			if err != nil {
				log.Error(err.Error())
			}
		}
	}
	return nil
}

func getApp(appId string) (map[string]string, error) {
	log.Debug(">>> getApp: ", appId)

	appInfo, found := appInfoMap[appId]
	if !found {
		return nil, errors.New("app instance not found")
	}
	return appInfo, nil
}

func updateApp(appId string) (map[string]string, error) {
	log.Debug(">>> updateApp: ", appId)

	// Get App information from app store
	app, err := appStore.Get(appId)
	if err != nil {
		log.Error(err.Error())
		return nil, err
	}
	log.Info("updateApp: app: ", app)

	// If MEP instance, ignore non-local apps
	if mepName != globalLocationName && app.Node != mepName {
		return nil, errors.New("ignoring app update on other MEP")
	}

	mutex.Lock()
	defer mutex.Unlock()

	// Store App Info
	appInfo, err := newAppInfo(app)
	if err != nil {
		log.Error(err.Error())
		return nil, err
	}
	err = setAppInfo(appInfo)
	if err != nil {
		log.Error(err.Error())
		return nil, err
	}
	log.Info("updateApp: appInfo: ", appInfo)

	// Store App Info
	return appInfo, nil
}

func terminateApp(appId string) error {
	log.Debug(">>> terminateApp: ", appId)

	mutex.Lock()
	defer mutex.Unlock()

	// Get App info
	appInfo, found := appInfoMap[appId]
	if !found {
		return errors.New("App info not found for: " + appId)
	}

	// Get subscriptions for App instance
	subList, err := subMgr.GetFilteredSubscriptions(appId, APP_TERMINATION_NOTIF_SUB_TYPE)
	if err != nil {
		log.Error("Failed to get subscription list with err: ", err.Error())
		return err
	}

	// Process graceful termination
	gracefulTermination := false
	for _, sub := range subList {
		gracefulTermination = true

		// Create notification payload
		operationAction := TERMINATING_OperationActionType
		notif := &AppTerminationNotification{
			NotificationType:   APP_TERMINATION_NOTIF_TYPE,
			OperationAction:    &operationAction,
			MaxGracefulTimeout: DEFAULT_GRACEFUL_TIMEOUT,
			Links: &AppTerminationNotificationLinks{
				Subscription: &LinkType{
					Href: sub.Cfg.Self,
				},
				ConfirmTermination: &LinkTypeConfirmTermination{
					Href: hostUrl.String() + basePath + "confirm_termination",
				},
			},
		}

		// Start graceful timeout timer prior to sending the app termination notification
		gracefulTerminateChannel := make(chan bool)
		gracefulTerminateMap[appId] = gracefulTerminateChannel

		go func(sub *subs.Subscription) {
			log.Info("Sending App Termination notification (" + sub.Cfg.Id + ") for " + appId)
			err := subMgr.SendNotification(sub, []byte(convertAppTerminationNotifToJson(notif)))
			if err != nil {
				log.Error("Failed to send App termination notif with err: ", err.Error())
			}

			// Wait for app termination confirmation or timeout
			select {
			case <-gracefulTerminateChannel:
				mutex.Lock()
				defer mutex.Unlock()
				log.Debug("Termination confirmation received for: ", appId)

			case <-time.After(time.Duration(DEFAULT_GRACEFUL_TIMEOUT) * time.Second):
				mutex.Lock()
				defer mutex.Unlock()
				delete(gracefulTerminateMap, appId)
			}

			// Delete App
			err = delAppInfo(appInfo)
			if err != nil {
				log.Error(err.Error())
			}
		}(sub)
	}

	// Delete App instance immediately if no graceful termination subscription
	if !gracefulTermination {
		err := delAppInfo(appInfo)
		if err != nil {
			log.Error(err.Error())
		}
	}

	return nil
}

func flushApps(persist bool) error {
	mutex.Lock()
	defer mutex.Unlock()

	// Delete App instances
	for appId, appInfo := range appInfoMap {
		// Ignore persistent apps unless required
		if !persist {
			appPersist, err := strconv.ParseBool(appInfo[fieldPersist])
			if err != nil {
				appPersist = false
			}
			if appPersist {
				continue
			}
		}

		// No need for graceful termination when flushing apps
		delete(gracefulTerminateMap, appId)

		// Delete app info
		err := delAppInfo(appInfo)
		if err != nil {
			log.Error(err.Error())
		}
	}
	return nil
}

func newAppTerminationNotifSubCfg(sub *AppTerminationNotificationSubscription, subId string, appId string) *subs.SubscriptionCfg {
	subCfg := &subs.SubscriptionCfg{
		Id:                  subId,
		AppId:               appId,
		Type:                APP_TERMINATION_NOTIF_SUB_TYPE,
		NotifType:           APP_TERMINATION_NOTIF_TYPE,
		Self:                sub.Links.Self.Href,
		NotifyUrl:           sub.CallbackReference,
		ExpiryTime:          nil,
		PeriodicInterval:    0,
		RequestTestNotif:    false,
		RequestWebsocketUri: false,
	}
	return subCfg
}

func sendAppRemoveCnf(id string) {
	// Create message to send on MQ
	msg := mqLocal.CreateMsg(mq.MsgAppRemoveCnf, mq.TargetAll, sandboxName)
	msg.Payload[mqFieldAppId] = id

	// Send message to inform other modules of app removal
	log.Debug("TX MSG: ", mq.PrintMsg(msg))
	err := mqLocal.SendMsg(msg)
	if err != nil {
		log.Error("Failed to send message. Error: ", err.Error())
		return
	}
}

/*
* getAppInfo gets application information using its application Instance Id stored in redis
* @param {string} appId application Instance Id used to retrive its information from redis DB
*	@return {map[string]string} appInfo application information
* @return {error} err It returns error if there is no information associated with this appId
 */
func getAppInfo(appId string) (map[string]string, error) {
	log.Debug(">>> getAppInfo: ", appId)

	var appInfo map[string]string

	// Get app instance from local MEP only
	keyName := baseKey + "app:" + appId + ":info"
	log.Info("getAppInfo: keyName: ", keyName)
	appInfo, err := rc.GetEntry(keyName)
	if err != nil {
		log.Error(err.Error())
		return nil, err
	} else if len(appInfo) == 0 {
		return nil, errors.New("app instance not found")
	}
	log.Info("getAppInfo: appInfo: ", appInfo)

	return appInfo, nil
}

func errHandlerProblemDetails(w http.ResponseWriter, error string, code int) {
	var pd ProblemDetails
	pd.Detail = error
	pd.Status = int32(code)

	jsonResponse := convertProblemDetailstoJson(&pd)

	w.WriteHeader(code)
	fmt.Fprint(w, jsonResponse)
}
