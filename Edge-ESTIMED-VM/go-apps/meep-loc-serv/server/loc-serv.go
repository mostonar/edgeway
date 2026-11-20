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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	sbi "github.com/InterDigitalInc/AdvantEDGE/go-apps/meep-loc-serv/sbi"
	asc "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-app-support-client"
	dkm "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-data-key-mgr"
	gisClient "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-gis-engine-client"
	httpLog "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-http-logger"
	log "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-logger"
	met "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-metrics"
	redis "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-redis"
	scc "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-sandbox-ctrl-client"
	smc "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-service-mgmt-client"
	"github.com/mitchellh/mapstructure"

	"github.com/gorilla/mux"
)

const moduleName = "meep-loc-serv"
const LocServBasePath = "location/v3/"
const locServKey = "loc-serv"
const serviceName = "Location Service"
const serviceCategory = "Location"
const defaultLocationName = "global"
const defaultScopeOfLocality = "MEC_SYSTEM"
const defaultConsumedLocalOnly = true
const appTerminationPath = "notifications/mec011/appTermination"

const typeZone = "zone"
const typeAccessPoint = "accessPoint"
const typeUser = "user"
const typeZonalSubscription = "zonalsubs"
const typeUserSubscription = "usersubs"
const typeZoneStatusSubscription = "zonestatus"
const typeDistanceSubscription = "distance"
const typeAreaCircleSubscription = "areacircle"
const typePeriodicSubscription = "periodic"

const (
	notifZonalPresence = "ZonalPresenceNotification"
	notifZoneStatus    = "ZoneStatusNotification"
	notifSubscription  = "SubscriptionNotification"
)

type UeUserData struct {
	queryZoneId  []string
	queryApId    []string
	queryAddress []string
	userList     *UserList
}

type ApUserData struct {
	queryInterestRealm string
	apList             *AccessPointList
}

type Pair struct {
	addr1 string
	addr2 string
}

var nextZonalSubscriptionIdAvailable int
var nextUserSubscriptionIdAvailable int
var nextZoneStatusSubscriptionIdAvailable int
var nextDistanceSubscriptionIdAvailable int
var nextAreaCircleSubscriptionIdAvailable int
var nextPeriodicSubscriptionIdAvailable int

var zonalSubscriptionEnteringMap = map[int]string{}
var zonalSubscriptionLeavingMap = map[int]string{}
var zonalSubscriptionTransferringMap = map[int]string{}
var zonalSubscriptionMap = map[int]string{}
var zonalSubscriptionMapLink = map[int]*ZoneCheck{}
var userSubscriptionEnteringMap = map[int]string{}
var userSubscriptionLeavingMap = map[int]string{}
var userSubscriptionTransferringMap = map[int]string{}
var userSubscriptionMap = map[int]string{}
var userSubscriptionMapLink = map[int]*EventCheck{}
var zoneStatusSubscriptionMap = map[int]*ZoneStatusCheck{}
var zoneStatusSubscriptionMapLink = map[int]*EventStatusCheck{}
var distanceSubscriptionMap1 = map[int]*DistanceCheck_{}
var periodicTicker *time.Ticker
var areaCircleSubscriptionMap = map[int]*AreaCircleCheck{}
var periodicSubscriptionMap1 = map[int]*PeriodicCheck1{}
var addressConnectedMap = map[string]bool{}

type ZoneStatusCheck struct {
	ZoneId                          string
	Serviceable                     bool
	Unserviceable                   bool
	Unknown                         bool
	upperNumberOfUsersZoneThreshold int32
	lowerNumberOfUsersZoneThreshold int32
	upperNumberOfUsersAPThreshold   int32
	lowerNumberOfUsersAPThreshold   int32
	Subscription                    *ZoneStatusSubscription
	Reporting_amount                int32
	Reporting_interval              int32
	NextTts                         int32
}

type DistanceCheck_ struct {
	NextTts                int32 //next time to send, derived from frequency
	NbNotificationsSent    int32
	NotificationCheckReady bool
	Reporting_amount       int32
	Reporting_interval     int32
	TimeStamp              int64
	Subscription           *UserDistanceSubscription
}

type AreaCircleCheck struct {
	NextTts                int32 //next time to send, derived from frequency
	AddrInArea             map[string]bool
	NbNotificationsSent    int32
	NotificationCheckReady bool
	Reporting_amount       int32
	Reporting_interval     int32
	TimeStamp              int64
	Subscription           *UserAreaSubscription
}

type EventCheck struct {
	TimeStamp    int64
	Subscription *UserLocationEventSubscription
}
type EventStatusCheck struct {
	TimeStamp          int64
	Subscription       *ZoneStatusSubscription
	Reporting_amount   int32
	Reporting_interval int32
	NextTts            int32 //next time to send, derived from frequency
}
type ZoneCheck struct {
	NextTts            int32 //next time to send, derived from frequency
	TimeStamp          int64
	Reporting_amount   int32
	Reporting_interval int32
	Subscription       *ZoneLocationEventSubscription
}
type PeriodicCheck1 struct {
	NextTts            int32 //next time to send, derived from frequency
	TimeStamp          int64
	Reporting_amount   float64
	Reporting_interval float64
	Subscription       *UserLocationPeriodicSubscription
}

var LOC_SERV_DB = 0
var currentStoreName = ""

var redisAddr string = "meep-redis-master.default.svc.cluster.local:6379"
var influxAddr string = "http://meep-influxdb.default.svc.cluster.local:8086"
var sbxCtrlUrl string = "http://meep-sandbox-ctrl"

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
var mutex sync.Mutex

var gisAppClient *gisClient.APIClient
var gisAppClientUrl string = "http://meep-gis-engine"

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

// Init - Location Service initialization
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

	// Get Sandbox name
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
		basePath = "/" + sandboxName + "/" + LocServBasePath
	} else {
		basePath = "/" + sandboxName + "/" + mepName + "/" + LocServBasePath
	}

	// Set base storage key
	baseKey = dkm.GetKeyRoot(sandboxName) + locServKey + ":mep:" + mepName + ":"

	// Connect to Redis DB
	rc, err = redis.NewConnector(redisAddr, LOC_SERV_DB)
	if err != nil {
		log.Error("Failed connection to Redis DB. Error: ", err)
		return err
	}
	_ = rc.DBFlush(baseKey)
	log.Info("Connected to Redis DB, location service table")

	gisAppClientCfg := gisClient.NewConfiguration()
	gisAppClientCfg.BasePath = gisAppClientUrl + "/gis/v1"

	gisAppClient = gisClient.NewAPIClient(gisAppClientCfg)
	if gisAppClient == nil {
		log.Error("Failed to create GIS App REST API client: ", gisAppClientCfg.BasePath)
		err := errors.New("Failed to create GIS App REST API client")
		return err
	}

	userLocationEventReInit()
	zoneLocationEventReInit()
	zoneStatusReInit()
	distanceReInit()
	areaCircleReInit()
	userLocationPeriodicReInit()

	// Initialize SBI
	sbiCfg := sbi.SbiCfg{
		ModuleName:     moduleName,
		SandboxName:    sandboxName,
		RedisAddr:      redisAddr,
		Locality:       locality,
		UserInfoCb:     updateUserInfo,
		ZoneInfoCb:     updateZoneInfo,
		ApInfoCb:       updateAccessPointInfo,
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
		// Create App Info client
		sbxCtrlClientCfg := scc.NewConfiguration()
		sbxCtrlClientCfg.BasePath = sbxCtrlUrl + "/sandbox-ctrl/v1"
		sbxCtrlClient = scc.NewAPIClient(sbxCtrlClientCfg)
		if sbxCtrlClient == nil {
			return errors.New("Failed to create App Info REST API client")
		}

		// Create App Support client
		appSupportClientCfg := asc.NewConfiguration()
		appSupportClientCfg.BasePath = appEnablementUrl + "/mec_app_support/v2"
		appSupportClient = asc.NewAPIClient(appSupportClientCfg)
		if appSupportClient == nil {
			return errors.New("Failed to create App Enablement App Support REST API client")
		}

		// Create Service Management client
		srvMgmtClientCfg := smc.NewConfiguration()
		srvMgmtClientCfg.BasePath = appEnablementUrl + "/mec_service_mgmt/v1"
		svcMgmtClient = smc.NewAPIClient(srvMgmtClientCfg)
		if svcMgmtClient == nil {
			return errors.New("Failed to create App Enablement Service Management REST API client")
		}
	}

	log.Info("Location Service successfully initialized")
	return nil
}

// Run - Start Location Service
func Run() (err error) {

	// Start MEC Service registration ticker
	if appEnablementEnabled {
		startRegistrationTicker()
	}

	periodicTicker = time.NewTicker(time.Second)
	go func() {
		for range periodicTicker.C {
			updateNotificationAreaCirclePeriodicTrigger()
			checkNotificationPeriodicTrigger1()
			checkNotificationDistancePeriodicTrigger1()
		}
	}()

	return sbi.Run()
}

// Stop - Stop Location
func Stop() (err error) {
	// Stop MEC Service registration ticker
	if appEnablementEnabled {
		stopRegistrationTicker()
	}

	periodicTicker.Stop()
	err = sbi.Stop()
	if err != nil {
		return err
	}
	return nil
}

// startRegistrationTicker initiates a periodic process to register the application with the App Enablement Service.
// It ensures that the registration ticker is not already running, waits for the App Enablement Service to start,
// and then periodically attempts to send readiness, registration, and subscription messages until successful.
//
// The function works as follows:
// 1. Checks if the registration ticker is already running; if it is, logs a warning and exits.
// 2. Waits for a few seconds to allow the App Enablement Service to start, avoiding long TCP socket connect timeouts.
// 3. Starts a new ticker that triggers every 5 seconds and runs a goroutine to handle the registration process.
//
// Inside the goroutine:
// - It retrieves the application instance ID, either from a sandbox service or a scenario-provisioned instance.
// - Sends an "App Ready" message if not already sent.
// - Registers the service instance with the App Enablement Service if not already registered.
// - Subscribes for graceful termination notifications if not already subscribed.
//
// If all registration steps are successful, it logs a success message and stops the ticker.
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

// stopRegistrationTicker stops the registration ticker if it is currently running.
// It logs an informational message indicating that the App Enablement registration ticker is being stopped,
// then stops the ticker and sets the ticker variable to nil to indicate it is no longer active.
//
// This function ensures that the periodic registration attempts are halted cleanly.
func stopRegistrationTicker() {
	if registrationTicker != nil {
		log.Info("Stopping App Enablement registration ticker")
		registrationTicker.Stop()
		registrationTicker = nil
	}
}

// getAppInstanceId requests a new application instance ID from the Sandbox Controller.
// It constructs an ApplicationInfo object with necessary details such as instance ID, service category,
// node name, and type. Depending on the node name, it sets the persistence flag.
// The function then makes an API call to the Sandbox Controller to obtain the instance ID.
//
// Parameters:
// - None
//
// Returns:
// - id (string): The obtained application instance ID.
// - err (error): An error object if the API call fails, otherwise nil.
func getAppInstanceId() (id string, err error) {
	var appInfo scc.ApplicationInfo
	appInfo.Id = instanceId
	appInfo.Name = serviceCategory
	appInfo.NodeName = mepName
	appInfo.Type_ = "SYSTEM"
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

// deregisterService unregisters a service instance from the App Enablement registry.
// It makes an API call to delete the service identified by serviceId for the given appInstanceId.
//
// Parameters:
// - appInstanceId (string): The application instance ID associated with the service.
// - serviceId (string): The ID of the service to be deregistered.
//
// Returns:
// - error: An error object if the deregistration API call fails, otherwise nil.
func deregisterService(appInstanceId string, serviceId string) error {
	_, err := svcMgmtClient.MecServiceMgmtApi.AppServicesServiceIdDELETE(context.TODO(), appInstanceId, serviceId)
	if err != nil {
		log.Error("Failed to unregister the service to app enablement registry: ", err)
		return err
	}
	return nil
}

// registerService registers a service instance with the App Enablement registry for the given application instance ID.
// It constructs a ServiceInfoPost object with the necessary service details and makes an API call to register the service.
//
// Parameters:
// - appInstanceId (string): The application instance ID associated with the service.
//
// Returns:
// - error: An error object if the registration API call fails, otherwise nil.
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
			Id:      "locationId",
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

// sendReadyConfirmation sends a readiness confirmation message for the given application instance ID
// to indicate that the application is ready to the App Support API.
// It constructs an AppReadyConfirmation object with the readiness indication and makes an API call.
//
// Parameters:
// - appInstanceId (string): The application instance ID to send the readiness confirmation for.
//
// Returns:
// - error: An error object if the API call to send the readiness confirmation fails, otherwise nil.
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
// func isValidIPAddress(ip string) bool {
// 	return net.ParseIP(ip) != nil
// }

// sendTerminationConfirmation sends a termination confirmation message for the given application instance ID
// to indicate that the application is terminating to the App Support API.
// It constructs an AppTerminationConfirmation object with the termination action and makes an API call.
//
// Parameters:
// - appInstanceId (string): The application instance ID to send the termination confirmation for.
//
// Returns:
// - error: An error object if the API call to send the termination confirmation fails, otherwise nil.
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

// subscribeAppTermination subscribes to termination notifications for the given application instance ID.
// It constructs an AppTerminationNotificationSubscription object with the necessary subscription details
// and makes an API call to register the subscription.
//
// Parameters:
// - appInstanceId (string): The application instance ID to subscribe for termination notifications.
//
// Returns:
// - error: An error object if the API call to register the subscription fails, otherwise nil.
func subscribeAppTermination(appInstanceId string) error {
	var sub asc.AppTerminationNotificationSubscription
	sub.SubscriptionType = "AppTerminationNotificationSubscription"
	sub.AppInstanceId = appInstanceId
	if mepName == defaultLocationName {
		sub.CallbackReference = "http://" + moduleName + "/" + LocServBasePath + appTerminationPath
	} else {
		sub.CallbackReference = "http://" + mepName + "-" + moduleName + "/" + LocServBasePath + appTerminationPath
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

// unsubscribeAppTermination unsubscribes from termination notifications for the given application instance ID and subscription ID.
// It makes an API call to delete the specified subscription.
//
// Parameters:
// - appInstanceId (string): The application instance ID associated with the subscription.
// - subId (string): The subscription ID to be deleted.
//
// Returns:
// - error: An error object if the API call to delete the subscription fails, otherwise nil.
func unsubscribeAppTermination(appInstanceId string, subId string) error {
	//only subscribe to one subscription, so we force number to be one, couldn't be anything else
	_, err := appSupportClient.MecAppSupportApi.ApplicationsSubscriptionDELETE(context.TODO(), appInstanceId, subId)
	if err != nil {
		log.Error("Failed to unregister to App Support subscription: ", err)
		return err
	}
	return nil
}

// deregisterZoneStatus removes a zone status subscription from the zoneStatusSubscriptionMap.
// It converts the subscription ID string to an integer and then removes the corresponding entry from the map.
//
// Parameters:
// - subsIdStr (string): The subscription ID as a string to be removed from the map.
//
// Returns:
// - None
func deregisterZoneStatus(subsIdStr string) {
	subsId, err := strconv.Atoi(subsIdStr)
	if err != nil {
		log.Error(err)
	}

	mutex.Lock()
	defer mutex.Unlock()
	zoneStatusSubscriptionMap[subsId] = nil
}

// registerZoneStatus registers zone status information and its corresponding subscription details.
// It converts the subscription ID string to an integer, calculates the expiry time if available,
// constructs ZoneStatusCheck and EventStatusCheck objects, and adds them to the zoneStatusSubscriptionMap.
//
// Parameters:
// - zoneId (string): The ID of the zone.
// - upNbOfUsersZoneThreshold (int32): Upper number of users threshold for the zone.
// - upNbOfUsersAPThreshold (int32): Upper number of users threshold for the access point.
// - opStatus ([]OperationStatus): Operation status of the zone.
// - subsIdStr (string): The subscription ID as a string.
// - loNbOfUsersZoneThreshold (int32): Lower number of users threshold for the zone.
// - loNbOfUsersAPThreshold (int32): Lower number of users threshold for the access point.
// - zoneStatusSub (*ZoneStatusSubscription): Pointer to the ZoneStatusSubscription object.
//
// Returns:
// - None
func registerZoneStatus(zoneId string, upNbOfUsersZoneThreshold int32, upNbOfUsersAPThreshold int32, opStatus []OperationStatus, subsIdStr string, loNbOfUsersZoneThreshold int32, loNbOfUsersAPThreshold int32, zoneStatusSub *ZoneStatusSubscription) {

	subsId, err := strconv.Atoi(subsIdStr)
	if err != nil {
		log.Error(err)
	}
	var expiryTime int64
	if zoneStatusSub != nil && zoneStatusSub.ExpiryDeadline != nil {
		expiryTime = time.Now().Unix() + int64(zoneStatusSub.ExpiryDeadline.Seconds)
	}
	mutex.Lock()
	defer mutex.Unlock()
	var zoneStatus ZoneStatusCheck
	if opStatus != nil {
		for i := 0; i < len(opStatus); i++ {
			switch opStatus[i] {
			case SERVICEABLE:
				zoneStatus.Serviceable = true
			case UNSERVICEABLE:
				zoneStatus.Unserviceable = true
			case UNKNOWN:
				zoneStatus.Unknown = true
			default:
			}
		}
	}
	var StatusSub EventStatusCheck
	zoneStatus.upperNumberOfUsersZoneThreshold = upNbOfUsersZoneThreshold
	zoneStatus.upperNumberOfUsersAPThreshold = upNbOfUsersAPThreshold
	zoneStatus.lowerNumberOfUsersZoneThreshold = loNbOfUsersZoneThreshold
	zoneStatus.lowerNumberOfUsersAPThreshold = loNbOfUsersAPThreshold
	zoneStatus.ZoneId = zoneId
	StatusSub.Subscription = zoneStatusSub
	StatusSub.TimeStamp = expiryTime
	if zoneStatusSub != nil && zoneStatusSub.ReportingCtrl != nil {
		StatusSub.Reporting_amount = zoneStatusSub.ReportingCtrl.MaximumCount
		StatusSub.Reporting_interval = zoneStatusSub.ReportingCtrl.MinimumInterval
		// StatusSub.NextTts = zoneStatusSub.ReportingCtrl.MaximumFrequency
	}
	zoneStatusSubscriptionMap[subsId] = &zoneStatus
	zoneStatusSubscriptionMapLink[subsId] = &StatusSub
}

// deregisterZonal removes zonal subscriptions from various zonal subscription maps based on the given subscription ID string.
// It converts the subscription ID string to an integer and then removes the corresponding entries from the maps.
//
// Parameters:
// - subsIdStr (string): The subscription ID as a string to be removed from the maps.
//
// Returns:
// - None
func deregisterZonal(subsIdStr string) {
	subsId, err := strconv.Atoi(subsIdStr)
	if err != nil {
		log.Error(err)
	}

	mutex.Lock()
	defer mutex.Unlock()
	zonalSubscriptionMap[subsId] = ""
	zonalSubscriptionEnteringMap[subsId] = ""
	zonalSubscriptionLeavingMap[subsId] = ""
	zonalSubscriptionTransferringMap[subsId] = ""
}

// registerZonal1 registers zonal events and their corresponding subscription details.
// It converts the subscription ID string to an integer, calculates the expiry time if available,
// assigns zone IDs to various zonal subscription maps based on the events, constructs ZoneCheck objects,
// and adds them to the zonalSubscriptionMap.
//
// Parameters:
// - zoneId (string): The ID of the zone.
// - event ([]LocationEventType): Array of location event types.
// - subsIdStr (string): The subscription ID as a string.
// - zoneSub (*ZoneLocationEventSubscription): Pointer to the ZoneLocationEventSubscription object.
//
// Returns:
// - None
func registerZonal1(zoneId string, event []LocationEventType, subsIdStr string, zoneSub *ZoneLocationEventSubscription) {

	subsId, err := strconv.Atoi(subsIdStr)
	if err != nil {
		log.Error(err)
	}
	// Calculate expiry time by adding seconds to the current time
	var expiryTime int64
	if zoneSub != nil && zoneSub.ExpiryDeadline != nil {
		expiryTime = time.Now().Unix() + int64(zoneSub.ExpiryDeadline.Seconds)
	}
	mutex.Lock()
	defer mutex.Unlock()
	if event != nil {
		for i := 0; i < len(event); i++ {
			switch event[i] {
			case ENTERING_AREA_EVENT:
				zonalSubscriptionEnteringMap[subsId] = zoneId
			case LEAVING_AREA_EVENT:
				zonalSubscriptionLeavingMap[subsId] = zoneId
			default:
			}
		}
	} else {
		zonalSubscriptionEnteringMap[subsId] = zoneId
		zonalSubscriptionLeavingMap[subsId] = zoneId
		zonalSubscriptionTransferringMap[subsId] = zoneId
	}
	var ZoneSub ZoneCheck
	ZoneSub.Subscription = zoneSub
	ZoneSub.TimeStamp = expiryTime
	if zoneSub != nil && zoneSub.ReportingCtrl != nil {
		ZoneSub.Reporting_amount = zoneSub.ReportingCtrl.MaximumCount
		ZoneSub.Reporting_interval = zoneSub.ReportingCtrl.MinimumInterval
		// ZoneSub.NextTts = zoneSub.ReportingCtrl.MaximumFrequency
	}

	zonalSubscriptionMap[subsId] = zoneId
	zonalSubscriptionMapLink[subsId] = &ZoneSub
}

// deregisterUser removes user subscriptions from various user subscription maps based on the given subscription ID string.
// It converts the subscription ID string to an integer and then removes the corresponding entries from the maps.
//
// Parameters:
// - subsIdStr (string): The subscription ID as a string to be removed from the maps.
//
// Returns:
// - None
func deregisterUser(subsIdStr string) {
	subsId, err := strconv.Atoi(subsIdStr)
	if err != nil {
		log.Error(err)
	}

	mutex.Lock()
	defer mutex.Unlock()
	userSubscriptionMap[subsId] = ""
	userSubscriptionEnteringMap[subsId] = ""
	userSubscriptionLeavingMap[subsId] = ""
	userSubscriptionTransferringMap[subsId] = ""
}

// registerUser1 registers user events and their corresponding subscription details.
// It converts the subscription ID string to an integer, calculates the expiry time if available,
// assigns user addresses to various user subscription maps based on the events, constructs EventCheck objects,
// and adds them to the userSubscriptionMap.
//
// Parameters:
// - userAddress (string): The address of the user.
// - event ([]LocationEventType): Array of location event types.
// - subsIdStr (string): The subscription ID as a string.
// - eventSub (*UserLocationEventSubscription): Pointer to the UserLocationEventSubscription object.
//
// Returns:
// - None
func registerUser1(userAddress string, event []LocationEventType, subsIdStr string, eventSub *UserLocationEventSubscription) {
	subsId, err := strconv.Atoi(subsIdStr)
	if err != nil {
		log.Error(err)
	}
	// Calculate expiry time by adding seconds to the current time
	var expiryTime int64
	if eventSub != nil && eventSub.ExpiryDeadline != nil {
		expiryTime = time.Now().Unix() + int64(eventSub.ExpiryDeadline.Seconds)
	}
	mutex.Lock()
	defer mutex.Unlock()
	if event != nil {
		for i := 0; i < len(event); i++ {
			switch event[i] {
			case ENTERING_AREA_EVENT:
				userSubscriptionEnteringMap[subsId] = userAddress
			case LEAVING_AREA_EVENT:
				userSubscriptionLeavingMap[subsId] = userAddress
			}
		}
	} else {
		userSubscriptionEnteringMap[subsId] = userAddress
		userSubscriptionLeavingMap[subsId] = userAddress
	}
	var EventSub EventCheck
	EventSub.Subscription = eventSub
	userSubscriptionMap[subsId] = userAddress
	EventSub.TimeStamp = expiryTime
	userSubscriptionMapLink[subsId] = &EventSub
}

// updateNotificationAreaCirclePeriodicTrigger updates the notification trigger for area circle subscriptions.
// It checks if there is at least one subscription, then iterates through the areaCircleSubscriptionMap,
// decrements the NextTts value if it's not zero, and sets the NotificationCheckReady flag accordingly.
//
// Parameters:
// - None
//
// Returns:
// - None
func updateNotificationAreaCirclePeriodicTrigger() {
	//only check if there is at least one subscription
	mutex.Lock()
	defer mutex.Unlock()

	for _, areaCircleCheck := range areaCircleSubscriptionMap {
		if areaCircleCheck != nil {
			if areaCircleCheck.NextTts != 0 {
				areaCircleCheck.NextTts--
			}
			if areaCircleCheck.NextTts == 0 {
				areaCircleCheck.NotificationCheckReady = true
			} else {
				areaCircleCheck.NotificationCheckReady = false
			}
		}
	}
}

// checkNotificationDistancePeriodicTrigger1 checks for distance notifications based on specified subscriptions.
// It iterates through distanceSubscriptionMap1, retrieves distance data from the GIS engine for each pair
// of monitored and reference addresses, evaluates the criteria specified in the subscription, and sends notifications accordingly.
// The function also handles expiration of subscriptions and updates reporting control parameters.
//
// If the current time exceeds the expiry time of a subscription, it removes the subscription from the map.
// For each pair of addresses, it checks if both addresses are connected and retrieves the distance data from the GIS engine.
// It then compares the distance against the specified criteria in the subscription, and if met, adds the monitored address to the return address map.
//
// Parameters:
// None
//
// Returns:
// None
func checkNotificationDistancePeriodicTrigger1() {

	//only check if there is at least one subscription
	mutex.Lock()
	defer mutex.Unlock()
	currentTime := time.Now().Unix()
	//check all that applies
	for subsId, distanceCheck := range distanceSubscriptionMap1 {
		if distanceCheck != nil && distanceCheck.Subscription != nil {

			// Check if the current time exceeds the expiry time
			if distanceCheck.Subscription.ExpiryDeadline != nil && time.Now().Unix() > int64(distanceSubscriptionMap1[subsId].TimeStamp) {
				subsIdStr := strconv.Itoa(subsId)
				log.Info("Expiry deadline passed for subscription: ")
				// Optionally, you can remove the subscription from the map or perform other cleanup actions
				err := rc.JSONDelEntry(baseKey+typeDistanceSubscription+":"+subsIdStr, ".")
				if err != nil {
					log.Error(err.Error())
				}
				continue
			}

			//loop through every reference address
			returnAddr := make(map[string]*gisClient.Distance)
			skipThisSubscription := false

			//if reference address is specified, reference addresses are checked agains each monitored address
			//if reference address is nil, each pair of the monitored address should be checked
			//creating address pairs to check
			//e.g. refAddr = A, B ; monitoredAddr = C, D, E ; resultingPairs {A,C - A,D - A,E - B,C - B,D - B-E}
			//e.g. monitoredAddr = A, B, C ; resultingPairs {A,B - B,A - A,C - C,A - B,C - C,B}

			var addressPairs []Pair
			if distanceCheck.Subscription.ReferenceAddress != nil {
				for _, refAddr := range distanceCheck.Subscription.ReferenceAddress {
					//loop through every monitored address
					for _, monitoredAddr := range distanceCheck.Subscription.MonitoredAddress {
						pair := Pair{addr1: refAddr, addr2: monitoredAddr}
						addressPairs = append(addressPairs, pair)
					}
				}
			} else {
				nbIndex := len(distanceCheck.Subscription.MonitoredAddress)
				for i := 0; i < nbIndex-1; i++ {
					for j := i + 1; j < nbIndex; j++ {
						pair := Pair{addr1: distanceCheck.Subscription.MonitoredAddress[i], addr2: distanceCheck.Subscription.MonitoredAddress[j]}
						addressPairs = append(addressPairs, pair)
						//need pair to be symmetrical so that each is used as reference point and monitored address
						pair = Pair{addr1: distanceCheck.Subscription.MonitoredAddress[j], addr2: distanceCheck.Subscription.MonitoredAddress[i]}
						addressPairs = append(addressPairs, pair)
					}
				}
			}

			for _, pair := range addressPairs {
				refAddr := pair.addr1
				monitoredAddr := pair.addr2

				//check if one of the address if both addresses are connected, if not, disregard this pair
				if !addressConnectedMap[refAddr] || !addressConnectedMap[monitoredAddr] {
					//ignore that pair and continue processing
					continue
				}

				var distParam gisClient.TargetPoint
				distParam.AssetName = monitoredAddr

				distResp, httpResp, err := gisAppClient.GeospatialDataApi.GetDistanceGeoDataByName(context.TODO(), refAddr, distParam)
				if err != nil {
					//getting distance of an element that is not in the DB (not in scenario, not connected) returns error code 400 (bad parameters) in the API. Using that error code to track that request made it to GIS but no good result, so ignore that address (monitored or ref)
					if httpResp.StatusCode == http.StatusBadRequest {
						//ignore that pair and continue processing
						continue
					} else {
						log.Error("Failed to communicate with gis engine: ", err)
						return
					}
				}

				distance := int32(distResp.Distance)

				switch *distanceCheck.Subscription.Criteria {
				case ALL_WITHIN_DISTANCE_DistanceCriteria:
					if float32(distance) < distanceCheck.Subscription.Distance {
						returnAddr[monitoredAddr] = &distResp
					} else {
						skipThisSubscription = true
					}
				case ALL_BEYOND_DISTANCE_DistanceCriteria:
					if float32(distance) > distanceCheck.Subscription.Distance {
						returnAddr[monitoredAddr] = &distResp
					} else {
						skipThisSubscription = true
					}
				case ANY_WITHIN_DISTANCE_DistanceCriteria:
					if float32(distance) < distanceCheck.Subscription.Distance {
						returnAddr[monitoredAddr] = &distResp
					}
				case ANY_BEYOND_DISTANCE_DistanceCriteria:
					if float32(distance) > distanceCheck.Subscription.Distance {
						returnAddr[monitoredAddr] = &distResp
					}
				default:
				}
				if skipThisSubscription {
					break
				}
			}
			if skipThisSubscription {
				continue
			}
			if distanceCheck.Subscription.ReportingCtrl != nil {
				// If NextTts has passed, send notification

				if currentTime >= int64(distanceCheck.NextTts) {
					// Update NextTts for the next notification
					distanceCheck.NextTts = int32(currentTime + int64(distanceCheck.Reporting_interval))
					// Check if reporting amount is reached
					if distanceCheck.Reporting_amount <= 0 {
						// If reporting amount is zero, no more notifications should be sent
						continue
					}
					// Decrement reporting amount
					distanceCheck.Reporting_amount--
					sendDistanceNotification(subsId, returnAddr, distanceCheck)
				}
			} else {
				// If no reporting control parameters, send notification without conditions
				sendDistanceNotification(subsId, returnAddr, distanceCheck)
			}
		}
	}
}

// sendDistanceNotification sends distance notifications to the specified subscription callback reference.
// It constructs a UserDistanceNotification containing information about the monitored users and their distances,
// and sends it as an inline subscription notification using sendSubscriptionNotification3.
//
// Parameters:
// - subsId: The subscription ID.
// - returnAddr: A map containing the monitored addresses and their corresponding distance information.
// - distanceCheck: A pointer to the DistanceCheck_ struct containing information about the subscription.
//
// Returns:
// None
func sendDistanceNotification(subsId int, returnAddr map[string]*gisClient.Distance, distanceCheck *DistanceCheck_) {
	if len(returnAddr) > 0 {
		//update nb of notification sent anch check if valid
		subsIdStr := strconv.Itoa(subsId)

		var distanceNotif UserDistanceNotification
		distanceNotif.DistanceEvent = distanceCheck.Subscription.Criteria
		distanceNotif.Links = &SubscriptionLinks{
			Subscription: distanceCheck.Subscription.Links.Self,
		}

		var userData UeUserData
		var ul UserList
		ul.ResourceURL = hostUrl.String() + basePath + "queries/users"
		userData.userList = &ul
		keyName := baseKey + typeUser + ":*"
		err := rc.ForEachJSONEntry(keyName, populateUserList, &userData)
		if err != nil {
			log.Warn(err.Error())
			return
		}
		if userData.userList == nil {
			log.Warn("No user found")
			return
		}
		if len(userData.userList.User) == 0 {
			log.Warn("No user info available")
			return
		}
		var userList UserList
		var userInfoList []UserInfo
		for terminalAddr, distanceInfo := range returnAddr {
			var userInfo UserInfo
			userInfo.Address = terminalAddr
			for _, v := range userData.userList.User {
				if v.Address == userInfo.Address {
					userInfo.AccessPointId = v.AccessPointId
					userInfo.ZoneId = v.ZoneId
					break
				}
			} // End of 'for' statement
			if userInfo.AccessPointId == "" || userInfo.ZoneId == "" {
				log.Error("User ot found: " + userInfo.Address)
				return
			}
			var locationInfo LocationInfo
			locationInfo.Latitude = nil
			locationInfo.Latitude = append(locationInfo.Latitude, distanceInfo.DstLatitude)
			locationInfo.Longitude = nil
			locationInfo.Longitude = append(locationInfo.Longitude, distanceInfo.DstLongitude)
			locationInfo.Shape = 2
			seconds := time.Now().Unix()
			var timeStamp TimeStamp
			timeStamp.Seconds = int32(seconds)
			userInfo.Timestamp = &timeStamp
			userInfo.LocationInfo = &locationInfo
			userInfoList = append(userInfoList, userInfo)
		}
		userList.User = userInfoList
		distanceNotif.MonitoredUsers = &userList
		distanceNotif.TimeStamp = &TimeStamp{
			Seconds:     int32(time.Now().Unix()),
			NanoSeconds: 0,
		}
		distanceNotif.NotificationType = "UserDistanceNotification"
		var inlineDistanceSubscriptionNotification InlineUserDistanceNotification
		inlineDistanceSubscriptionNotification.UserDistanceNotification = &distanceNotif
		distanceCheck.NbNotificationsSent++
		sendSubscriptionNotification3(distanceCheck.Subscription.CallbackReference, inlineDistanceSubscriptionNotification)
		log.Info("Distance Notification"+"("+subsIdStr+") For ", returnAddr)
	}
}

// checkNotificationAreaCircle checks if the specified address is within the area of any active subscriptions.
// If the address is within the area and meets the subscription criteria, it sends a notification to the callback reference.
//
// Parameters:
// - addressToCheck: The address to be checked for area circle subscriptions.
//
// Returns:
// None
func checkNotificationAreaCircle(addressToCheck string) {
	//only check if there is at least one subscription
	mutex.Lock()
	defer mutex.Unlock()
	currentTime := time.Now().Unix()
	//check all that applies
	for subsId, areaCircleCheck := range areaCircleSubscriptionMap {
		if areaCircleCheck != nil && areaCircleCheck.Subscription != nil {
			// if areaCircleCheck.Subscription.Count == 0 || (areaCircleCheck.Subscription.Count != 0 && areaCircleCheck.NbNotificationsSent < areaCircleCheck.Subscription.Count) {
			// 	if !areaCircleCheck.NotificationCheckReady {
			// 		continue
			// 	}

			//loop through every reference address
			if areaCircleCheck.Subscription.ExpiryDeadline != nil && time.Now().Unix() > int64(areaCircleSubscriptionMap[subsId].TimeStamp) {
				subsIdStr := strconv.Itoa(subsId)
				log.Info("Expiry deadline passed for subscription: ")
				// Optionally, you can remove the subscription from the map or perform other cleanup actions
				err := rc.JSONDelEntry(baseKey+typeAreaCircleSubscription+":"+subsIdStr, ".")
				if err != nil {
					log.Error(err.Error())
				}
				continue
			}
			for _, addr := range areaCircleCheck.Subscription.AddressList {
				if addr != addressToCheck {
					continue
				}
				if !addressConnectedMap[addr] {
					continue
				}
				//check if address is already inside the area or not based on the subscription
				var withinRangeParam gisClient.TargetRange
				withinRangeParam.Latitude = areaCircleCheck.Subscription.AreaDefine.Points[0].Latitude
				withinRangeParam.Longitude = areaCircleCheck.Subscription.AreaDefine.Points[0].Longitude
				withinRangeParam.Radius = float32(areaCircleCheck.Subscription.AreaDefine.Radius)

				withinRangeResp, httpResp, err := gisAppClient.GeospatialDataApi.GetWithinRangeByName(context.TODO(), addr, withinRangeParam)
				if err != nil {
					//getting element that is not in the DB (not in scenario, not connected) returns error code 400 (bad parameters) in the API. Using that error code to track that request made it to GIS but no good result, so ignore that address (monitored or ref)
					if httpResp.StatusCode == http.StatusBadRequest {
						//if the UE was within the zone, continue processing to send a LEAVING notification, otherwise, go to next subscription
						if !areaCircleCheck.AddrInArea[addr] {
							continue
						}
					} else {
						log.Error("Failed to communicate with gis engine: ", err)
						return
					}
				}
				//check if there is a change
				var event LocationEventType
				if withinRangeResp.Within {
					if areaCircleCheck.AddrInArea[addr] {
						//no change
						continue
					} else {
						areaCircleCheck.AddrInArea[addr] = true
						event = ENTERING_AREA_EVENT
					}
				} else {
					if !areaCircleCheck.AddrInArea[addr] {
						//no change
						continue
					} else {
						areaCircleCheck.AddrInArea[addr] = false
						event = LEAVING_AREA_EVENT
					}
				}
				//no tracking this event, stop looking for this UE
				if !contains(areaCircleCheck.Subscription.LocationEventCriteria, event) {
					continue
				}
				subsIdStr := strconv.Itoa(subsId)
				var areaCircleNotif UserAreaNotification
				//areaCircleNotif.UserLocationEvent = areaCircleCheck.Subscription.LocationEventCriteria
				areaCircleNotif.UserLocationEvent = &event
				areaCircleNotif.Links = &SubscriptionLinks{
					Subscription: areaCircleCheck.Subscription.Links.Self,
				}
				areaCircleNotif.Address = addr
				var locationInfo LocationInfo
				locationInfo.Latitude = nil
				locationInfo.Latitude = append(locationInfo.Latitude, withinRangeResp.SrcLatitude)
				locationInfo.Longitude = nil
				locationInfo.Longitude = append(locationInfo.Longitude, withinRangeResp.SrcLongitude)
				locationInfo.Shape = 2
				seconds := time.Now().Unix()
				areaCircleNotif.LocationInfo = &locationInfo
				var timeStamp TimeStamp
				timeStamp.Seconds = int32(seconds)
				areaCircleNotif.TimeStamp = &timeStamp
				areaCircleNotif.NotificationType = "UserAreaNotification"
				var inlineCircleSubscriptionNotification InlineUserAreaNotification
				inlineCircleSubscriptionNotification.UserAreaNotification = &areaCircleNotif
				areaCircleCheck.NbNotificationsSent++
				if areaCircleCheck.Subscription.ReportingCtrl != nil {
					// If NextTts has passed, send notification

					if currentTime >= int64(areaCircleCheck.NextTts) {
						// Update NextTts for the next notification
						areaCircleCheck.NextTts = int32(currentTime + int64(areaCircleCheck.Reporting_interval))
						// Check if reporting amount is reached
						if areaCircleCheck.Reporting_amount <= 0 {
							// If reporting amount is zero, no more notifications should be sent
							continue
						}
						// Decrement reporting amount
						areaCircleCheck.Reporting_amount--
						sendSubscriptionNotification5(areaCircleCheck.Subscription.CallbackReference, inlineCircleSubscriptionNotification)
					}
				} else {
					// If no reporting control parameters, send notification without conditions
					sendSubscriptionNotification5(areaCircleCheck.Subscription.CallbackReference, inlineCircleSubscriptionNotification)
				}
				log.Info("Area Circle Notification" + "(" + subsIdStr + ") For " + addr + " when " + " area")
			}
		}
	}
}

// }
func contains(s []LocationEventType, str LocationEventType) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}
	return false
}

// checkNotificationPeriodicTrigger1 checks if it's time to send periodic notifications for active subscriptions.
// It iterates over each subscription, checks if it's time to send the next notification, and sends it if conditions are met.
//
// Returns:
// None
func checkNotificationPeriodicTrigger1() {
	mutex.Lock()
	defer mutex.Unlock()

	currentTime := time.Now().Unix()

	for subsId, periodicCheck := range periodicSubscriptionMap1 {
		if periodicCheck == nil || periodicCheck.Subscription == nil {
			continue
		}

		addr := periodicCheck.Subscription.Address

		if !addressConnectedMap[addr] {
			continue
		}
		// Check if the current time exceeds the expiry time
		if periodicCheck.Subscription.ExpiryDeadline != nil && time.Now().Unix() > int64(periodicSubscriptionMap1[subsId].TimeStamp) {
			subsIdStr := strconv.Itoa(subsId)
			log.Info("Expiry deadline passed for subscription: ")
			// Optionally, you can remove the subscription from the map or perform other cleanup actions
			err := rc.JSONDelEntry(baseKey+typePeriodicSubscription+":"+subsIdStr, ".")
			if err != nil {
				log.Error(err.Error())
			}
			continue
		}

		// Check if it's time to send the next notification
		timeDifference := currentTime - int64(periodicCheck.NextTts)
		reportingIntervalInSeconds := int64(periodicCheck.Reporting_interval)

		if timeDifference >= reportingIntervalInSeconds {
			// Update NextTts for the next notification
			periodicCheck.NextTts = int32(currentTime + reportingIntervalInSeconds)

			// Check if reporting amount is reached
			if periodicCheck.Reporting_amount <= 0 {
				// If reporting amount is zero, no more notifications should be sent
				continue
			}

			// Decrement reporting amount
			periodicCheck.Reporting_amount--
			if periodicCheck.Reporting_amount != 0 {
				// Prepare and send the notification
				sendNotification(subsId, periodicCheck)

				// Log the notification
				subsIdStr := strconv.Itoa(subsId)
				log.Info("Periodic Notification (" + subsIdStr + ") For " + addr)
			} else {
				continue
			}
		}
	}
}

// sendNotification prepares and sends a periodic location notification for a subscription.
//
// Parameters:
// - subsId (int): The subscription ID.
// - periodicCheck (*PeriodicCheck1): The periodic check details.
//
// Returns:
// None
func sendNotification(subsId int, periodicCheck *PeriodicCheck1) {
	var periodicNotif UserLocationPeriodicNotification
	addr := periodicCheck.Subscription.Address

	geoDataInfo, _, err := gisAppClient.GeospatialDataApi.GetGeoDataByName(context.TODO(), addr, nil)
	if err != nil {
		log.Error("Failed to communicate with gis engine: ", err)
		return
	}
	periodicNotif.Address = addr
	var locationInfo LocationInfo
	locationInfo.Latitude = nil
	locationInfo.Latitude = append(locationInfo.Latitude, geoDataInfo.Location.Coordinates[1])
	locationInfo.Longitude = nil
	locationInfo.Longitude = append(locationInfo.Longitude, geoDataInfo.Location.Coordinates[0])
	locationInfo.Shape = 2
	seconds := time.Now().Unix()
	periodicNotif.LocationInfo = &locationInfo
	periodicNotif.IsFinalNotification = false
	periodicNotif.Links = &SubscriptionLinks{
		Subscription: periodicCheck.Subscription.Links.Self,
	}
	var timeStamp TimeStamp
	timeStamp.Seconds = int32(seconds)
	periodicNotif.TimeStamp = &timeStamp
	periodicNotif.NotificationType = "UserLocationPeriodicNotification"
	result := new(NotificationResult)
	*result = SUCCESS
	periodicNotif.Result = result
	subsIdStr := strconv.Itoa(subsId)
	var inlinePeriodicSubscriptionNotification InlineUserLocationPeriodicNotification
	inlinePeriodicSubscriptionNotification.UserLocationPeriodicNotification = &periodicNotif
	sendSubscriptionNotification1(periodicCheck.Subscription.CallbackReference, inlinePeriodicSubscriptionNotification)
	log.Info("Periodic Notification"+"("+subsIdStr+") For ", periodicCheck.Subscription.Address)
}

// deregisterDistance removes a distance subscription identified by its subscription ID.
//
// Parameters:
// - subsIdStr (string): The subscription ID in string format.
//
// Returns:
// None
func deregisterDistance(subsIdStr string) {
	subsId, err := strconv.Atoi(subsIdStr)
	if err != nil {
		log.Error(err)
	}

	mutex.Lock()
	defer mutex.Unlock()
	distanceSubscriptionMap1[subsId] = nil
}

// registerDistance1 registers a new user distance subscription.
//
// Parameters:
// - distanceSub (*UserDistanceSubscription): Pointer to the user distance subscription object.
// - subsIdStr (string): The subscription ID in string format.
//
// Returns:
// None
func registerDistance1(distanceSub *UserDistanceSubscription, subsIdStr string) {

	subsId, err := strconv.Atoi(subsIdStr)
	if err != nil {
		log.Error(err)
	}
	var expiryTime int64
	if distanceSub != nil && distanceSub.ExpiryDeadline != nil {
		expiryTime = time.Now().Unix() + int64(distanceSub.ExpiryDeadline.Seconds)
	}
	mutex.Lock()
	defer mutex.Unlock()
	var distanceCheck DistanceCheck_
	if distanceSub != nil && distanceSub.ReportingCtrl != nil {
		distanceCheck.Reporting_amount = distanceSub.ReportingCtrl.MaximumCount
		distanceCheck.Reporting_interval = distanceSub.ReportingCtrl.MinimumInterval
	}
	distanceCheck.TimeStamp = expiryTime
	distanceCheck.Subscription = distanceSub
	distanceSubscriptionMap1[subsId] = &distanceCheck
}

// deregisterAreaCircle removes an area circle subscription.
//
// Parameters:
// - subsIdStr (string): The subscription ID in string format.
//
// Returns:
// None
func deregisterAreaCircle(subsIdStr string) {
	subsId, err := strconv.Atoi(subsIdStr)
	if err != nil {
		log.Error(err)
	}

	mutex.Lock()
	defer mutex.Unlock()
	areaCircleSubscriptionMap[subsId] = nil
}

// registerAreaCircle registers a new area circle subscription.
//
// Parameters:
// - areaSub (*UserAreaSubscription): The user area subscription information.
// - subsIdStr (string): The subscription ID in string format.
//
// Returns:
// None
func registerAreaCircle(areaSub *UserAreaSubscription, subsIdStr string) {

	subsId, err := strconv.Atoi(subsIdStr)
	if err != nil {
		log.Error(err)
	}
	var expiryTime int64
	if areaSub != nil && areaSub.ExpiryDeadline != nil {
		expiryTime = time.Now().Unix() + int64(areaSub.ExpiryDeadline.Seconds)
	}
	mutex.Lock()
	defer mutex.Unlock()
	var areaCircleCheck AreaCircleCheck
	areaCircleCheck.Subscription = areaSub
	areaCircleCheck.NbNotificationsSent = 0
	areaCircleCheck.AddrInArea = map[string]bool{}
	if areaSub != nil && areaSub.ReportingCtrl != nil {
		areaCircleCheck.Reporting_amount = areaSub.ReportingCtrl.MaximumCount
		areaCircleCheck.Reporting_interval = areaSub.ReportingCtrl.MinimumInterval
	}
	areaCircleCheck.TimeStamp = expiryTime
	areaCircleCheck.NextTts = 0
	areaCircleSubscriptionMap[subsId] = &areaCircleCheck
}

// deregisterPeriodic removes a periodic subscription.
//
// Parameters:
// - subsIdStr (string): The subscription ID in string format.
//
// Returns:
// None
func deregisterPeriodic(subsIdStr string) {
	subsId, err := strconv.Atoi(subsIdStr)
	if err != nil {
		log.Error(err)
	}

	mutex.Lock()
	defer mutex.Unlock()
	periodicSubscriptionMap1[subsId] = nil
}

// registerPeriodic1 registers a periodic location subscription.
//
// Parameters:
// - periodicSub (*UserLocationPeriodicSubscription): The periodic location subscription to register.
// - subsIdStr (string): The subscription ID in string format.
//
// Returns:
// None
func registerPeriodic1(periodicSub *UserLocationPeriodicSubscription, subsIdStr string) {

	subsId, err := strconv.Atoi(subsIdStr)
	if err != nil {
		log.Error(err)
	}
	var expiryTime int64
	if periodicSub != nil && periodicSub.ExpiryDeadline != nil {
		expiryTime = time.Now().Unix() + int64(periodicSub.ExpiryDeadline.Seconds)
	}

	mutex.Lock()
	defer mutex.Unlock()
	var periodicCheck PeriodicCheck1
	periodicCheck.Subscription = periodicSub
	periodicCheck.Reporting_amount = periodicSub.PeriodicEventInfo.ReportingAmount
	periodicCheck.Reporting_interval = periodicSub.PeriodicEventInfo.ReportingInterval
	periodicCheck.TimeStamp = expiryTime
	periodicSubscriptionMap1[subsId] = &periodicCheck
}

// checkNotificationRegisteredZoneStatus1 checks the registered zone status and sends notifications if threshold conditions are met.
//
// Parameters:
// - zoneId (string): The ID of the zone to check.
// - apId (string): The ID of the access point.
// - nbUsersInAP (int32): The number of users in the access point.
// - nbUsersInZone (int32): The number of users in the zone.
// - previousNbUsersInAP (int32): The previous number of users in the access point.
// - previousNbUsersInZone (int32): The previous number of users in the zone.
//
// Returns:
// None
func checkNotificationRegisteredZoneStatus1(zoneId string, apId string, nbUsersInAP int32, nbUsersInZone int32, previousNbUsersInAP int32, previousNbUsersInZone int32) {
	mutex.Lock()
	defer mutex.Unlock()

	//check all that applies
	for subsId, zoneStatus := range zoneStatusSubscriptionMap {
		if zoneStatus == nil {
			continue
		}
		// Check if the current time exceeds the expiry time
		if zoneStatusSubscriptionMapLink[subsId].Subscription.ExpiryDeadline != nil && time.Now().Unix() > int64(zoneStatusSubscriptionMapLink[subsId].TimeStamp) {
			subsIdStr := strconv.Itoa(subsId)
			log.Info("Expiry deadline passed for subscription: ")
			// Optionally, you can remove the subscription from the map or perform other cleanup actions
			err := rc.JSONDelEntry(baseKey+typeZoneStatusSubscription+":"+subsIdStr, ".")
			if err != nil {
				log.Error(err.Error())
			}
			continue
		}
		if zoneStatus.ZoneId == zoneId {
			zoneWarning := false
			apWarning := false
			zoneWarning_Lower := false
			apWarning_Lower := false

			if nbUsersInZone != -1 {
				if previousNbUsersInZone != nbUsersInZone && nbUsersInZone >= zoneStatus.upperNumberOfUsersZoneThreshold {
					zoneWarning = true
				}
			}
			if nbUsersInZone != -1 {
				if previousNbUsersInZone != nbUsersInZone && nbUsersInZone <= zoneStatus.lowerNumberOfUsersZoneThreshold {
					zoneWarning_Lower = true
				}
			}
			if nbUsersInAP != -1 {
				if previousNbUsersInAP != nbUsersInAP && nbUsersInAP >= zoneStatus.upperNumberOfUsersAPThreshold {
					apWarning = true
				}
			}
			if nbUsersInAP != -1 {
				if previousNbUsersInAP != nbUsersInAP && nbUsersInAP <= zoneStatus.lowerNumberOfUsersAPThreshold {
					apWarning_Lower = true
				}
			}

			if zoneWarning || apWarning || apWarning_Lower || zoneWarning_Lower {
				subsIdStr := strconv.Itoa(subsId)
				jsonInfo, _ := rc.JSONGetEntry(baseKey+typeZoneStatusSubscription+":"+subsIdStr, ".")
				if jsonInfo == "" {
					return
				}

				subscription := convertJsonToZoneStatusSubscription(jsonInfo)

				var zoneStatusNotif ZoneStatusNotification
				zoneStatusNotif.ZoneId = zoneId
				if apWarning {
					zoneStatusNotif.AccessPointId = apId
					zoneStatusNotif.UserNumEvent = "OVER_AP_UPPER_THD"
				}
				if apWarning_Lower {
					zoneStatusNotif.AccessPointId = apId
					zoneStatusNotif.UserNumEvent = "UNDER_AP_LOWER_THD"
				}
				if zoneWarning {
					zoneStatusNotif.UserNumEvent = "OVER_ZONE_UPPER_THD"
				}
				if zoneWarning_Lower {
					zoneStatusNotif.UserNumEvent = "UNDER_ZONE_LOWER_THD"
				}
				seconds := time.Now().Unix()
				zoneStatusNotif.Links = &SubscriptionLinks{} // Initialize Links outside the loop
				for _, value_ := range zoneStatusSubscriptionMapLink {
					zoneStatusNotif.Links.Subscription = &LinkType{
						Href: value_.Subscription.Links.Self.Href,
					}
				}
				var timeStamp TimeStamp
				timeStamp.Seconds = int32(seconds)
				zoneStatusNotif.TimeStamp = &timeStamp
				zoneStatusNotif.NotificationType = "ZoneStatusNotification"
				var inlineZoneStatusNotification InlineZoneStatusNotification
				inlineZoneStatusNotification.ZoneStatusNotification = &zoneStatusNotif
				sendStatusNotification(subscription.CallbackReference, inlineZoneStatusNotification)
				if apWarning {
					log.Info("Zone Status Notification" + "(" + subsIdStr + "): " + "For event in zone " + zoneId + " which has " + strconv.Itoa(int(nbUsersInAP)) + " users in AP " + apId)
				} else {
					log.Info("Zone Status Notification" + "(" + subsIdStr + "): " + "For event in zone " + zoneId + " which has " + strconv.Itoa(int(nbUsersInZone)) + " users in total")
				}
			}
		}
	}

}

// checkNotificationRegisteredUsers checks the registered users' location and sends notifications for zone transitions.
//
// Parameters:
// - oldZoneId (string): The ID of the old zone.
// - newZoneId (string): The ID of the new zone.
// - oldApId (string): The ID of the old access point.
// - newApId (string): The ID of the new access point.
// - userId (string): The ID of the user.
//
// Returns:
// None
func checkNotificationRegisteredUsers(oldZoneId string, newZoneId string, oldApId string, newApId string, userId string) {
	mutex.Lock()
	defer mutex.Unlock()
	//check all that applies
	for subsId, value := range userSubscriptionMap {
		if value == userId {
			subsIdStr := strconv.Itoa(subsId)
			jsonInfo, _ := rc.JSONGetEntry(baseKey+typeUserSubscription+":"+subsIdStr, ".")
			if jsonInfo == "" {
				return
			}
			subscription := convertJsonToUserSubscription1(jsonInfo)
			// Check if the current time exceeds the expiry time
			if userSubscriptionMapLink[subsId].Subscription.ExpiryDeadline != nil && time.Now().Unix() > int64(userSubscriptionMapLink[subsId].TimeStamp) {
				log.Info("Expiry deadline passed for subscription: " + subsIdStr)
				// Optionally, you can remove the subscription from the map or perform other cleanup actions
				err := rc.JSONDelEntry(baseKey+typeUserSubscription+":"+subsIdStr, ".")
				if err != nil {
					log.Error(err.Error())
				}
				continue
			}
			var zonal UserLocationEventNotification
			zonal.Address = userId
			zonal.Links = &SubscriptionLinks{} // Initialize Links outside the loop
			for _, value_ := range userSubscriptionMapLink {
				zonal.Links.Subscription = &LinkType{
					Href: value_.Subscription.Links.Self.Href,
				}
			}
			zonal.NotificationType = "UserLocationEventNotification"
			seconds := time.Now().Unix()
			var timeStamp TimeStamp
			timeStamp.Seconds = int32(seconds)
			zonal.TimeStamp = &timeStamp
			if newZoneId != oldZoneId {
				//process LEAVING events prior to entering ones
				if oldZoneId != "" {
					if userSubscriptionLeavingMap[subsId] != "" {
						zonal.ZoneId = oldZoneId
						zonal.AccessPointId = oldApId
						event := new(LocationEventType)
						*event = LEAVING_AREA_EVENT
						zonal.UserLocationEvent = event
						var inlineZonal InlineUserLocationEventNotification
						inlineZonal.UserLocationEventNotification = &zonal
						sendZonalPresenceNotification(subscription.CallbackReference, inlineZonal)
						log.Info("User Notification" + "(" + subsIdStr + "): " + "Leaving event in zone " + oldZoneId + " for user " + userId)
					}
				}
				if userSubscriptionEnteringMap[subsId] != "" && newZoneId != "" {
					zonal.ZoneId = newZoneId
					zonal.AccessPointId = newApId
					event := new(LocationEventType)
					*event = ENTERING_AREA_EVENT
					zonal.UserLocationEvent = event
					var inlineZonal InlineUserLocationEventNotification
					inlineZonal.UserLocationEventNotification = &zonal
					sendZonalPresenceNotification(subscription.CallbackReference, inlineZonal)
					log.Info("User Notification" + "(" + subsIdStr + "): " + "Entering event in zone " + newZoneId + " for user " + userId)
				}
			}
		}
	}
}

// sendZonalPresenceNotification sends a zonal presence notification to the specified URL.
//
// Parameters:
// - notifyUrl (string): The URL to notify.
// - notification (InlineUserLocationEventNotification): The inline user location event notification to send.
//
// Returns:
// None
func sendZonalPresenceNotification(notifyUrl string, notification InlineUserLocationEventNotification) {
	startTime := time.Now()
	jsonNotif, err := json.Marshal(notification)
	if err != nil {
		log.Error(err)
		return
	}

	resp, err := http.Post(notifyUrl, "application/json", bytes.NewBuffer(jsonNotif))
	duration := float64(time.Since(startTime).Microseconds()) / 1000.0
	_ = httpLog.LogNotification(notifyUrl, "POST", "", "", string(jsonNotif), resp, startTime)
	if err != nil {
		log.Error(err)
		met.ObserveNotification(sandboxName, serviceName, notifZonalPresence, notifyUrl, nil, duration)
		return
	}
	met.ObserveNotification(sandboxName, serviceName, notifZonalPresence, notifyUrl, resp, duration)
	defer resp.Body.Close()
}

// sendZonalPresenceNotification_L sends a zonal presence notification to the specified URL.
//
// Parameters:
// - notifyUrl (string): The URL to notify.
// - notification (InlineZoneLocationEventNotification): The inline zone location event notification to send.
//
// Returns:
// None
func sendZonalPresenceNotification_L(notifyUrl string, notification InlineZoneLocationEventNotification) {
	startTime := time.Now()
	jsonNotif, err := json.Marshal(notification)
	if err != nil {
		log.Error(err)
		return
	}

	resp, err := http.Post(notifyUrl, "application/json", bytes.NewBuffer(jsonNotif))
	duration := float64(time.Since(startTime).Microseconds()) / 1000.0
	_ = httpLog.LogNotification(notifyUrl, "POST", "", "", string(jsonNotif), resp, startTime)
	if err != nil {
		log.Error(err)
		met.ObserveNotification(sandboxName, serviceName, notifZonalPresence, notifyUrl, nil, duration)
		return
	}
	met.ObserveNotification(sandboxName, serviceName, notifZonalPresence, notifyUrl, resp, duration)
	defer resp.Body.Close()
}

// sendStatusNotification sends a zone status notification to the specified URL.
//
// Parameters:
// - notifyUrl (string): The URL to notify.
// - notification (InlineZoneStatusNotification): The inline zone status notification to send.
//
// Returns:
// None
func sendStatusNotification(notifyUrl string, notification InlineZoneStatusNotification) {
	startTime := time.Now()
	jsonNotif, err := json.Marshal(notification)
	if err != nil {
		log.Error(err)
		return
	}

	resp, err := http.Post(notifyUrl, "application/json", bytes.NewBuffer(jsonNotif))
	duration := float64(time.Since(startTime).Microseconds()) / 1000.0
	_ = httpLog.LogNotification(notifyUrl, "POST", "", "", string(jsonNotif), resp, startTime)
	if err != nil {
		log.Error(err)
		met.ObserveNotification(sandboxName, serviceName, notifZoneStatus, notifyUrl, nil, duration)
		return
	}
	met.ObserveNotification(sandboxName, serviceName, notifZoneStatus, notifyUrl, resp, duration)
	defer resp.Body.Close()
}

// sendSubscriptionNotification1 sends a user location periodic notification to the specified URL.
//
// Parameters:
// - notifyUrl (string): The URL to notify.
// - notification (InlineUserLocationPeriodicNotification): The inline user location periodic notification to send.
//
// Returns:
// None
func sendSubscriptionNotification1(notifyUrl string, notification InlineUserLocationPeriodicNotification) {
	startTime := time.Now()
	jsonNotif, err := json.Marshal(notification)
	if err != nil {
		log.Error(err)
		return
	}

	resp, err := http.Post(notifyUrl, "application/json", bytes.NewBuffer(jsonNotif))
	duration := float64(time.Since(startTime).Microseconds()) / 1000.0
	_ = httpLog.LogNotification(notifyUrl, "POST", "", "", string(jsonNotif), resp, startTime)
	if err != nil {
		log.Error(err)
		met.ObserveNotification(sandboxName, serviceName, notifSubscription, notifyUrl, nil, duration)
		return
	}
	met.ObserveNotification(sandboxName, serviceName, notifSubscription, notifyUrl, resp, duration)
	defer resp.Body.Close()
}

// sendSubscriptionNotification3 sends a user distance notification to the specified URL.
//
// Parameters:
// - notifyUrl (string): The URL to notify.
// - notification (InlineUserDistanceNotification): The inline user distance notification to send.
//
// Returns:
// None
func sendSubscriptionNotification3(notifyUrl string, notification InlineUserDistanceNotification) {
	startTime := time.Now()
	jsonNotif, err := json.Marshal(notification)
	if err != nil {
		log.Error(err)
		return
	}

	resp, err := http.Post(notifyUrl, "application/json", bytes.NewBuffer(jsonNotif))
	duration := float64(time.Since(startTime).Microseconds()) / 1000.0
	_ = httpLog.LogNotification(notifyUrl, "POST", "", "", string(jsonNotif), resp, startTime)
	if err != nil {
		log.Error(err)
		met.ObserveNotification(sandboxName, serviceName, notifSubscription, notifyUrl, nil, duration)
		return
	}
	met.ObserveNotification(sandboxName, serviceName, notifSubscription, notifyUrl, resp, duration)
	defer resp.Body.Close()
}

// sendSubscriptionNotification5 sends a subscription notification to the specified URL.
//
// This function marshals the given notification into JSON format and sends it as an HTTP POST
// request to the specified notifyUrl. It logs the notification, measures the time taken for the
// operation, and records the metrics.
//
// Parameters:
//   - notifyUrl (string): The URL to which the notification should be sent.
//   - notification (InlineUserAreaNotification): The notification data to be sent.
//
// The function performs the following steps:
//  1. Records the start time of the operation.
//  2. Marshals the notification into JSON format.
//  3. Sends an HTTP POST request with the JSON data to the notifyUrl.
//  4. Logs the notification, including the request and response details.
//  5. Observes and records the metrics for the notification process, including any errors.
//
// If there is an error during JSON marshalling or the HTTP request, the function logs the error and
// records the metrics accordingly. The response body is closed after the operation to free resources.
func sendSubscriptionNotification5(notifyUrl string, notification InlineUserAreaNotification) {
	startTime := time.Now()
	jsonNotif, err := json.Marshal(notification)
	if err != nil {
		log.Error(err)
		return
	}

	resp, err := http.Post(notifyUrl, "application/json", bytes.NewBuffer(jsonNotif))
	duration := float64(time.Since(startTime).Microseconds()) / 1000.0
	_ = httpLog.LogNotification(notifyUrl, "POST", "", "", string(jsonNotif), resp, startTime)
	if err != nil {
		log.Error(err)
		met.ObserveNotification(sandboxName, serviceName, notifSubscription, notifyUrl, nil, duration)
		return
	}
	met.ObserveNotification(sandboxName, serviceName, notifSubscription, notifyUrl, resp, duration)
	defer resp.Body.Close()
}

// checkNotificationRegisteredZones1 checks and handles notifications for user zone changes.
//
// This function verifies and processes zone-based notifications for a user based on their
// movement between zones and access points. It locks the necessary resources to ensure
// thread safety, checks for expired subscriptions, and sends notifications for entering
// or leaving zones based on subscription criteria.
//
// Parameters:
//   - oldZoneId (string): The ID of the previous zone the user was in.
//   - newZoneId (string): The ID of the current zone the user is in.
//   - oldApId (string): The ID of the previous access point the user was connected to.
//   - newApId (string): The ID of the current access point the user is connected to.
//   - userId (string): The ID of the user whose zone change is being processed.
//
// The function performs the following steps:
//  1. Locks the mutex to ensure thread safety.
//  2. Records the current time for checking expiry and scheduling notifications.
//  3. Iterates through all subscriptions to check if any have expired, and removes them if they have.
//  4. Checks if the user's new zone matches any subscription criteria, and sends notifications for
//     entering or leaving events based on the subscription settings and reporting control parameters.
//
// If a subscription has reporting control parameters, it ensures notifications are sent at the appropriate
// intervals and decrements the reporting amount accordingly. Notifications are sent only if the conditions
// specified in the subscription are met.
func checkNotificationRegisteredZones1(oldZoneId string, newZoneId string, oldApId string, newApId string, userId string) {
	mutex.Lock()
	defer mutex.Unlock()
	currentTime := time.Now().Unix()

	// Check all subscriptions
	for subsId, value := range zonalSubscriptionMap {
		subsIdStr := strconv.Itoa(subsId)

		// Check if subscription has expired
		if zonalSubscriptionMapLink[subsId].Subscription.ExpiryDeadline != nil && time.Now().Unix() > int64(zonalSubscriptionMapLink[subsId].TimeStamp) {
			log.Info("Expiry deadline passed for subscription: " + subsIdStr)
			// Optionally, you can remove the subscription from the map or perform other cleanup actions
			err := rc.JSONDelEntry(baseKey+typeZonalSubscription+":"+subsIdStr, ".")
			if err != nil {
				log.Error(err.Error())
			}
			continue
		}
		if zonalSubscriptionMapLink[subsId].Subscription.AddressList == nil {
			// Check if the current zone matches the subscription zone
			if value == newZoneId {
				if newZoneId != oldZoneId {
					// Check if entering event subscription exists
					if zonalSubscriptionEnteringMap[subsId] != "" {
						// Check if reporting control parameters are provided
						// if zonalSubscriptionMapLink[subsId].Reporting_amount > 0 && zonalSubscriptionMapLink[subsId].Reporting_interval > 0 {
						if zonalSubscriptionMapLink[subsId].Subscription.ReportingCtrl != nil {
							// If NextTts has passed, send notification

							if currentTime >= int64(zonalSubscriptionMapLink[subsId].NextTts) {
								// Update NextTts for the next notification
								zonalSubscriptionMapLink[subsId].NextTts = int32(currentTime + int64(zonalSubscriptionMapLink[subsId].Reporting_interval))
								// Check if reporting amount is reached
								if zonalSubscriptionMapLink[subsId].Reporting_amount <= 0 {
									// If reporting amount is zero, no more notifications should be sent
									continue
								}
								// Decrement reporting amount
								zonalSubscriptionMapLink[subsId].Reporting_amount--
								sendNotification_1(subsId, newZoneId, userId, ENTERING_AREA_EVENT)
							}
						} else {
							// If no reporting control parameters, send notification without conditions
							sendNotification_1(subsId, newZoneId, userId, ENTERING_AREA_EVENT)
						}
					}
				}
			} else {
				// Check if leaving event subscription exists
				if value == oldZoneId {
					if zonalSubscriptionLeavingMap[subsId] != "" {

						// if zonalSubscriptionMapLink[subsId].Reporting_amount > 0 && zonalSubscriptionMapLink[subsId].Reporting_interval > 0 {
						if zonalSubscriptionMapLink[subsId].Subscription.ReportingCtrl != nil {
							// If NextTts has passed, send notification
							if currentTime >= int64(zonalSubscriptionMapLink[subsId].NextTts) {
								// Update NextTts for the next notification
								zonalSubscriptionMapLink[subsId].NextTts = int32(currentTime + int64(zonalSubscriptionMapLink[subsId].Reporting_interval))
								// Check if reporting amount is reached
								if zonalSubscriptionMapLink[subsId].Reporting_amount <= 0 {
									// If reporting amount is zero, no more notifications should be sent
									continue
								}
								// Decrement reporting amount
								zonalSubscriptionMapLink[subsId].Reporting_amount--
								sendNotification_1(subsId, oldZoneId, userId, LEAVING_AREA_EVENT)
							}
						} else {
							// If no reporting control parameters, send notification without conditions
							sendNotification_1(subsId, oldZoneId, userId, LEAVING_AREA_EVENT)
						}
					}
				}
			}
		} else {
			// Check if the current zone matches the subscription zone
			for _, addr := range zonalSubscriptionMapLink[subsId].Subscription.AddressList {
				if addr == userId {
					if value == newZoneId {
						if newZoneId != oldZoneId {
							// Check if entering event subscription exists
							if zonalSubscriptionEnteringMap[subsId] != "" {
								// Check if reporting control parameters are provided
								// if zonalSubscriptionMapLink[subsId].Reporting_amount > 0 && zonalSubscriptionMapLink[subsId].Reporting_interval > 0 {
								if zonalSubscriptionMapLink[subsId].Subscription.ReportingCtrl != nil {
									// If NextTts has passed, send notification

									if currentTime >= int64(zonalSubscriptionMapLink[subsId].NextTts) {
										// Update NextTts for the next notification
										zonalSubscriptionMapLink[subsId].NextTts = int32(currentTime + int64(zonalSubscriptionMapLink[subsId].Reporting_interval))
										// Check if reporting amount is reached
										if zonalSubscriptionMapLink[subsId].Reporting_amount <= 0 {
											// If reporting amount is zero, no more notifications should be sent
											continue
										}
										// Decrement reporting amount
										zonalSubscriptionMapLink[subsId].Reporting_amount--
										sendNotification_1(subsId, newZoneId, userId, ENTERING_AREA_EVENT)
									}
								} else {
									// If no reporting control parameters, send notification without conditions
									sendNotification_1(subsId, newZoneId, userId, ENTERING_AREA_EVENT)
								}
							}
						}
					} else {
						// Check if leaving event subscription exists
						if value == oldZoneId {
							if zonalSubscriptionLeavingMap[subsId] != "" {

								// if zonalSubscriptionMapLink[subsId].Reporting_amount > 0 && zonalSubscriptionMapLink[subsId].Reporting_interval > 0 {
								if zonalSubscriptionMapLink[subsId].Subscription.ReportingCtrl != nil {
									// If NextTts has passed, send notification
									if currentTime >= int64(zonalSubscriptionMapLink[subsId].NextTts) {
										// Update NextTts for the next notification
										zonalSubscriptionMapLink[subsId].NextTts = int32(currentTime + int64(zonalSubscriptionMapLink[subsId].Reporting_interval))
										// Check if reporting amount is reached
										if zonalSubscriptionMapLink[subsId].Reporting_amount <= 0 {
											// If reporting amount is zero, no more notifications should be sent
											continue
										}
										// Decrement reporting amount
										zonalSubscriptionMapLink[subsId].Reporting_amount--
										sendNotification_1(subsId, oldZoneId, userId, LEAVING_AREA_EVENT)
									}
								} else {
									// If no reporting control parameters, send notification without conditions
									sendNotification_1(subsId, oldZoneId, userId, LEAVING_AREA_EVENT)
								}
							}
						}
					}
				}
			}
		}
	}
}

// sendNotification_1 sends a notification for a zone-related event.
//
// This function retrieves the subscription details for the given subscription ID, constructs
// a notification for the specified event (entering or leaving a zone), and sends it to the
// callback URL defined in the subscription. It also logs the notification event.
//
// Parameters:
//   - subsId (int): The ID of the subscription associated with the notification.
//   - zoneId (string): The ID of the zone where the event occurred.
//   - userId (string): The ID of the user related to the event.
//   - eventType (LocationEventType): The type of location event (e.g., entering or leaving a zone).
//
// The function performs the following steps:
//  1. Converts the subscription ID to a string.
//  2. Retrieves the subscription details from the JSON store using the subscription ID.
//  3. Constructs the ZoneLocationEventNotification object with relevant event details.
//  4. Sends the notification to the callback URL specified in the subscription.
//  5. Logs the notification event, indicating whether it is an entering or leaving event.
//
// If the event type is ENTERING_AREA_EVENT, it logs an entering event message. If the event type
// is LEAVING_AREA_EVENT, it logs a leaving event message.
func sendNotification_1(subsId int, zoneId string, userId string, eventType LocationEventType) {
	subsIdStr := strconv.Itoa(subsId)
	jsonInfo, _ := rc.JSONGetEntry(baseKey+typeZonalSubscription+":"+subsIdStr, ".")
	if jsonInfo != "" {
		subscription := convertJsonToZonalSubscription_1(jsonInfo)

		var zonal ZoneLocationEventNotification
		zonal.Links = &SubscriptionLinks{} // Initialize Links outside the loop
		for _, value_ := range zonalSubscriptionMapLink {
			zonal.Links.Subscription = &LinkType{
				Href: value_.Subscription.Links.Self.Href,
			}
		}
		zonal.ZoneId = zoneId
		// zonal.CurrentAccessPointId = newApId
		zonal.Address = userId
		zonal.NotificationType = "ZoneLocationEventNotification"
		event := new(LocationEventType)
		*event = eventType
		zonal.UserLocationEvent = event
		seconds := time.Now().Unix()
		var timeStamp TimeStamp
		timeStamp.Seconds = int32(seconds)
		zonal.TimeStamp = &timeStamp
		var inlineZonal InlineZoneLocationEventNotification
		inlineZonal.ZoneLocationEventNotification = &zonal
		sendZonalPresenceNotification_L(subscription.CallbackReference, inlineZonal)

		if eventType == ENTERING_AREA_EVENT {
			log.Info("Zonal Notify Entering event in zone " + zoneId + " for user " + userId)
		} else {
			log.Info("Zonal Notify Leaving event in zone " + zoneId + " for user " + userId)
		}
	}
}

// usersGet handles the GET request for retrieving user data based on specified query parameters.
// It first sets the content type of the response to JSON.
//
// It then retrieves the query parameters from the request URL and validates them.
// If any invalid query parameters are found, it returns a 400 Bad Request response.
//
// Next, it retrieves the user list from the database based on the valid query parameters.
// The user list is populated by iterating over the database entries and filtering based on the query parameters.
// If an error occurs during database retrieval, it returns a 500 Internal Server Error response.
//
// Finally, it constructs the response containing the user list and sends it as a JSON response.
//
// Parameters:
//
//	w: http.ResponseWriter - an interface used to manipulate the HTTP response.
//	r: *http.Request - a pointer to the HTTP request received.
//
// The response contains a JSON representation of the user list along with its resource URL.
func usersGet(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	var userData UeUserData

	// Retrieve query parameters
	u, _ := url.Parse(r.URL.String())
	log.Info("url: ", u.RequestURI())
	q := u.Query()
	userData.queryZoneId = q["zoneId"]
	userData.queryApId = q["accessPointId"]
	userData.queryAddress = q["address"]

	validQueryParams := []string{"zoneId", "accessPointId", "address"}

	//look for all query parameters to reject if any invalid ones
	found := false
	for queryParam := range q {
		found = false
		for _, validQueryParam := range validQueryParams {
			if queryParam == validQueryParam {
				found = true
				break
			}
		}
		if !found {
			log.Error("Query param not valid: ", queryParam)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}

	// Get user list from DB
	var response InlineUserList
	var userList UserList
	userList.ResourceURL = hostUrl.String() + basePath + "queries/users"
	response.UserList = &userList
	userData.userList = &userList

	keyName := baseKey + typeUser + ":*"
	err := rc.ForEachJSONEntry(keyName, populateUserList, &userData)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Send response
	jsonResponse, err := json.Marshal(response)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(jsonResponse))
}

// populateUserList populates the user list with user information based on query parameters.
//
// This function filters and adds user information to the user list in the provided userData object
// based on matching query parameters such as zone ID, access point ID, and address. If the user
// information does not match the query parameters or if required fields are missing, the function
// returns without adding the user to the list.
//
// Parameters:
//   - key (string): A key associated with the user data (not used in the function).
//   - jsonInfo (string): JSON-encoded string containing user information.
//   - userData (interface{}): An interface containing user data, which should be of type *UeUserData.
//
// Returns:
//   - error: An error if the user data is invalid or if JSON unmarshalling fails. Returns nil if
//     the user information is successfully added to the list or if it does not match the
//     query parameters.
//
// The function performs the following steps:
//  1. Retrieves the user list from the userData object.
//  2. Unmarshals the JSON-encoded user information.
//  3. Filters user information based on query parameters (zone ID, access point ID, address).
//  4. Adds the user information to the user list if it matches the query parameters.
//
// The function ignores user entries that do not have a zone ID or access point ID.
func populateUserList(key string, jsonInfo string, userData interface{}) error {
	// Get query params & userlist from user data
	data := userData.(*UeUserData)
	if data == nil || data.userList == nil {
		return errors.New("userList not found in userData")
	}

	// Retrieve user info from DB
	var userInfo UserInfo
	err := json.Unmarshal([]byte(jsonInfo), &userInfo)
	if err != nil {
		return err
	}

	// Ignore entries with no zoneID or AP ID
	if userInfo.ZoneId == "" || userInfo.AccessPointId == "" {
		return nil
	}

	//query parameters looked through using OR within same query parameter and AND between different query parameters
	//example returning users matching zoneId : (zone01 OR zone02) AND accessPointId : (ap1 OR ap2 OR ap3) AND address: (ipAddress1 OR ipAddress2)
	foundAMatch := false
	// Filter using query params
	if len(data.queryZoneId) > 0 {
		foundAMatch = false
		for _, queryZoneId := range data.queryZoneId {
			if userInfo.ZoneId == queryZoneId {
				foundAMatch = true
			}
		}
		if !foundAMatch {
			return nil
		}
	}

	if len(data.queryApId) > 0 {
		foundAMatch = false
		for _, queryApId := range data.queryApId {
			if userInfo.AccessPointId == queryApId {
				foundAMatch = true
			}
		}
		if !foundAMatch {
			return nil
		}
	}

	if len(data.queryAddress) > 0 {
		foundAMatch = false
		for _, queryAddress := range data.queryAddress {
			if userInfo.Address == queryAddress {
				foundAMatch = true
			}
		}
		if !foundAMatch {
			return nil
		}
	}

	// Add user info to list
	data.userList.User = append(data.userList.User, userInfo)
	return nil
}

// apGet retrieves information about access points within a specific zone and returns it as a JSON response.
//
// Parameters:
//
//	w: http.ResponseWriter - an interface used to manipulate the HTTP response.
//	r: *http.Request - a pointer to the HTTP request received.
//
// The function first sets the content type of the response to "application/json; charset=UTF-8".
// It then extracts variables from the request's URL path using mux.Vars(r), specifically the 'zoneId'.
//
// Next, it parses query parameters from the request's URL to retrieve access point IDs.
// The function then initializes an InlineAccessPointList struct and an AccessPointList struct, setting the 'ZoneId'
// and 'ResourceURL' fields of the latter based on the provided 'zoneId'.
//
// It checks if the zone exists in the database. If not, it returns a 404 Not Found status code.
//
// The function then constructs a Redis key to fetch access point data for the specified zone and retrieves
// this data using rc.ForEachJSONEntry, populating the AccessPointList struct.
//
// If accessPointId parameters are provided, the function filters the access point list based on these IDs.
//
// Finally, it sets the 'ResourceURL' of the response AccessPointList and sends the JSON response with a 200 OK status code.
// If there's an error during JSON marshaling, it logs the error and returns a 500 Internal Server Error status code
// using errHandlerProblemDetails.
func apGet(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	var userData ApUserData
	vars := mux.Vars(r)

	// Parse query parameters
	accessPointIDs := r.URL.Query()["accessPointId"]

	// Get user list from DB
	var response InlineAccessPointList
	var apList AccessPointList
	apList.ZoneId = vars["zoneId"]
	apList.ResourceURL = hostUrl.String() + basePath + "queries/zones/" + vars["zoneId"] + "/accessPoints"
	response.AccessPointList = &apList
	userData.apList = &apList

	// Make sure the zone exists first
	jsonZoneInfo, _ := rc.JSONGetEntry(baseKey+typeZone+":"+vars["zoneId"], ".")
	if jsonZoneInfo == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	keyName := baseKey + typeZone + ":" + vars["zoneId"] + ":*"
	err := rc.ForEachJSONEntry(keyName, populateApList, &userData)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Filter access point list if accessPointId parameters are provided
	if len(accessPointIDs) > 0 {
		filteredAccessPointList := AccessPointList{}
		for _, accessPointID := range accessPointIDs {
			for _, accessPoint := range apList.AccessPoint {
				if accessPoint.AccessPointId == accessPointID {
					filteredAccessPointList.AccessPoint = append(filteredAccessPointList.AccessPoint, accessPoint)
					break // Assuming accessPoint IDs are unique, stop after finding the match
				}
			}
		}
		response.AccessPointList = &filteredAccessPointList
	}

	// Set the resourceURL to include base URL with or without parameters
	response.AccessPointList.ResourceURL = buildResourceURL_(hostUrl.String(), basePath, "queries/zones/"+vars["zoneId"]+"/accessPoints", accessPointIDs)

	// Send response
	jsonResponse, err := json.Marshal(response)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(jsonResponse))
}

// Function to build resource URL with or without parameters
func buildResourceURL_(baseURL, basePath, endpoint string, parameters []string) string {
	url := baseURL + basePath + endpoint
	if len(parameters) > 0 {
		url += "?" + strings.Join(parameters, "&")
	}
	return url
}

// apByIdGet retrieves information about a specific access point by its ID from the database and returns it as a JSON response.
//
// It takes an http.ResponseWriter and an *http.Request as parameters. The http.ResponseWriter is used to manipulate the HTTP response,
// and the *http.Request represents the HTTP request received.
//
// The function first sets the content type of the response to "application/json; charset=UTF-8".
// It then extracts the variables from the request's URL path using mux.Vars(r), specifically the 'zoneId' and 'accessPointId'.
// Next, it retrieves the access point information from the database based on these IDs using rc.JSONGetEntry.
// If the information is not found (jsonApInfo == ""), it returns a 404 Not Found status code.
// If there's an error during JSON unmarshaling or marshaling, it logs the error and returns a 500 Internal Server Error status code
// using errHandlerProblemDetails.
//
// Finally, it writes the JSON response containing the access point information to the response writer using fmt.Fprint(w, string(jsonResponse))
// and sets the status code to 200 OK using w.WriteHeader(http.StatusOK).
func apByIdGet(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	vars := mux.Vars(r)

	var response InlineAccessPointInfo
	var apInfo AccessPointInfo
	response.AccessPointInfo = &apInfo

	jsonApInfo, _ := rc.JSONGetEntry(baseKey+typeZone+":"+vars["zoneId"]+":"+typeAccessPoint+":"+vars["accessPointId"], ".")
	if jsonApInfo == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	err := json.Unmarshal([]byte(jsonApInfo), &apInfo)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}

	jsonResponse, err := json.Marshal(response)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(jsonResponse))
}

// zonesGet handles the GET request for retrieving zone information.
//
// It sets the Content-Type header of the HTTP response to application/json; charset=UTF-8.
// Then, it parses the query parameters from the request URL to extract zone IDs.
// Next, it initializes a response object and a zone list.
// It constructs the resource URL for the zone list based on the host URL, base path, and query parameters.
// It retrieves zone data from Redis and populates the zone list using the populateZoneList function.
// If an error occurs during this process, it logs the error, constructs an error response, and returns.
//
// If zone ID parameters are provided in the request, it filters the zone list to include only the zones
// matching the provided IDs.
//
// Finally, it marshals the response object to JSON format, sets the HTTP status code to OK, and writes
// the JSON response to the HTTP response writer.
//
// Parameters:
//
//	w: http.ResponseWriter - an interface used to manipulate the HTTP response.
//	r: *http.Request - a pointer to the HTTP request received.
func zonesGet(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	// Parse query parameters
	zoneIDs := r.URL.Query()["zoneId"]

	var response InlineZoneList
	var zoneList ZoneList
	zoneList.ResourceURL = hostUrl.String() + basePath + "queries/zones"
	response.ZoneList = &zoneList

	keyName := baseKey + typeZone + ":*"
	err := rc.ForEachJSONEntry(keyName, populateZoneList, &zoneList)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Filter zone list if zoneId parameters are provided
	if len(zoneIDs) > 0 {
		filteredZoneList := ZoneList{}
		for _, zoneID := range zoneIDs {
			for _, zone := range zoneList.Zone {
				if zone.ZoneId == zoneID {
					filteredZoneList.Zone = append(filteredZoneList.Zone, zone)
					break // Assuming zone IDs are unique, stop after finding the match
				}
			}
		}
		response.ZoneList = &filteredZoneList
	}

	// Set the resourceURL to include base URL with or without parameters
	response.ZoneList.ResourceURL = buildResourceURL(hostUrl.String(), basePath, zoneIDs)

	jsonResponse, err := json.Marshal(response)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(jsonResponse))
}

// Function to build resource URL with or without parameters
func buildResourceURL(baseURL, basePath string, zoneIDs []string) string {
	url := baseURL + basePath + "queries/zones"
	if len(zoneIDs) > 0 {
		url += "?zoneId=" + strings.Join(zoneIDs, "&zoneId=")
	}
	return url
}

// zonesByIdGet handles the GET request for retrieving zone information by zone ID.
//
// It sets the Content-Type header of the HTTP response to application/json; charset=UTF-8.
// Then, it extracts the zone ID from the request URL using the mux package.
// Next, it initializes a response object and a zone info object.
// It retrieves zone information from Redis based on the provided zone ID.
// If the zone information is not found, it sets the HTTP status code to NotFound and returns.
//
// If the zone information is found, it unmarshals the JSON data into the zone info object.
// If an error occurs during unmarshaling, it logs the error, constructs an error response, and returns.
//
// Finally, it marshals the response object to JSON format, sets the HTTP status code to OK, and writes
// the JSON response to the HTTP response writer.
//
// Parameters:
//
//	w: http.ResponseWriter - an interface used to manipulate the HTTP response.
//	r: *http.Request - a pointer to the HTTP request received.
func zonesByIdGet(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	vars := mux.Vars(r)

	var response InlineZoneInfo
	var zoneInfo ZoneInfo
	response.ZoneInfo = &zoneInfo

	jsonZoneInfo, _ := rc.JSONGetEntry(baseKey+typeZone+":"+vars["zoneId"], ".")
	if jsonZoneInfo == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	err := json.Unmarshal([]byte(jsonZoneInfo), &zoneInfo)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}

	jsonResponse, err := json.Marshal(response)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(jsonResponse))
}

func populateZoneList(key string, jsonInfo string, userData interface{}) error {

	zoneList := userData.(*ZoneList)
	var zoneInfo ZoneInfo

	// Format response
	err := json.Unmarshal([]byte(jsonInfo), &zoneInfo)
	if err != nil {
		return err
	}
	if zoneInfo.ZoneId != "" {
		zoneList.Zone = append(zoneList.Zone, zoneInfo)
	}
	return nil
}

// populateApList populates the access point list with access point information based on query parameters.
//
// This function filters and adds access point information to the access point list in the provided
// userData object based on matching query parameters such as interest realm. If the access point
// information does not match the query parameters or if required fields are missing, the function
// returns without adding the access point to the list.
//
// Parameters:
//   - key (string): A key associated with the access point data (not used in the function).
//   - jsonInfo (string): JSON-encoded string containing access point information.
//   - userData (interface{}): An interface containing access point data, which should be of type *ApUserData.
//
// Returns:
//   - error: An error if the user data is invalid or if JSON unmarshalling fails. Returns nil if
//     the access point information is successfully added to the list or if it does not match
//     the query parameters.
//
// The function performs the following steps:
//  1. Retrieves the access point list from the userData object.
//  2. Unmarshals the JSON-encoded access point information.
//  3. Filters access point information based on the query parameter (interest realm).
//  4. Adds the access point information to the access point list if it matches the query parameters.
//
// The function ignores access point entries that do not have an access point ID.
func populateApList(key string, jsonInfo string, userData interface{}) error {
	// Get query params & aplist from user data
	data := userData.(*ApUserData)
	if data == nil || data.apList == nil {
		return errors.New("apList not found in userData")
	}

	// Retrieve AP info from DB
	var apInfo AccessPointInfo
	err := json.Unmarshal([]byte(jsonInfo), &apInfo)
	if err != nil {
		return err
	}

	// Ignore entries with no AP ID
	if apInfo.AccessPointId == "" {
		return nil
	}

	// Filter using query params
	if data.queryInterestRealm != "" && apInfo.InterestRealm != data.queryInterestRealm {
		return nil
	}

	// Add AP info to list
	data.apList.AccessPoint = append(data.apList.AccessPoint, apInfo)
	return nil
}

// distanceSubDelete deletes a distance subscription identified by its ID and returns a JSON response with no content.
//
// Parameters:
//
//	w: http.ResponseWriter - an interface used to manipulate the HTTP response.
//	r: *http.Request - a pointer to the HTTP request received.
//
// The function first sets the content type of the response to "application/json; charset=UTF-8".
//
// It extracts the subscription ID from the request's URL path using mux.Vars(r).
//
// It checks if the subscription ID exists in the database. If not, it returns a 404 Not Found status code.
//
// The function then attempts to delete the subscription entry from the database using rc.JSONDelEntry.
// If there's an error during deletion, it returns a 500 Internal Server Error status code using errHandlerProblemDetails.
//
// Additionally, it calls deregisterDistance to remove the subscription from the registry.
//
// Finally, it sets the HTTP status code to 204 No Content and returns an empty response body.
func distanceSubDelete(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	vars := mux.Vars(r)

	present, _ := rc.JSONGetEntry(baseKey+typeDistanceSubscription+":"+vars["subscriptionId"], ".")
	if present == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	err := rc.JSONDelEntry(baseKey+typeDistanceSubscription+":"+vars["subscriptionId"], ".")
	if err != nil {
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}

	deregisterDistance(vars["subscriptionId"])
	w.WriteHeader(http.StatusNoContent)
}

// distanceSubListGet retrieves a list of distance subscriptions and returns them as a JSON response.
//
// Parameters:
//
//	w: http.ResponseWriter - an interface used to manipulate the HTTP response.
//	r: *http.Request - a pointer to the HTTP request received.
//
// The function first sets the content type of the response to "application/json; charset=UTF-8".
//
// It initializes an InlineNotificationSubscriptionList struct and a slice to hold Subscription structs.
// It constructs the key name pattern to query distance subscriptions from the database.
// It then iterates over each distance subscription entry in the database using rc.ForEachJSONEntry,
// populating the subscriptions slice with Subscription structs.
// If there's an error during iteration, it returns a 500 Internal Server Error status code using errHandlerProblemDetails.
//
// Next, it constructs the response InlineNotificationSubscriptionList containing the list of subscriptions,
// along with the resource URL.
//
// Finally, it marshals the response into JSON format, sets the HTTP status code to 200 OK,
// and writes the JSON response to the response writer.
// If there's an error during JSON marshaling, it returns a 500 Internal Server Error status code using errHandlerProblemDetails.
func distanceSubListGet(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	// Check for query parameters
	if len(r.URL.Query()) > 0 {
		errHandlerProblemDetails(w, "Query parameters are not allowed", http.StatusBadRequest)
		return
	}
	var response InlineNotificationSubscriptionList
	var subscriptions []Subscription

	keyName := baseKey + typeDistanceSubscription + "*"
	err := rc.ForEachJSONEntry(keyName, populateDistanceList, &subscriptions)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}
	response = InlineNotificationSubscriptionList{
		NotificationSubscriptionList: &NotificationSubscriptionList{
			Subscription: subscriptions,
			ResourceURL: &LinkType{
				Href: hostUrl.String() + basePath + "subscriptions/distance",
			},
		},
	}

	jsonResponse, err := json.Marshal(response)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(jsonResponse))
}

// distanceSubGet retrieves a distance subscription identified by its ID and returns it as a JSON response.
//
// Parameters:
//
//	w: http.ResponseWriter - an interface used to manipulate the HTTP response.
//	r: *http.Request - a pointer to the HTTP request received.
//
// The function first sets the content type of the response to "application/json; charset=UTF-8".
//
// It extracts the subscription ID from the request's URL path using mux.Vars(r).
//
// It initializes an InlineUserDistanceSubscription struct and a UserDistanceSubscription struct.
// It then retrieves the JSON representation of the distance subscription from the database using rc.JSONGetEntry.
// If the subscription ID does not exist, it returns a 404 Not Found status code.
//
// The function unmarshals the JSON data into the distanceSub struct.
// If there's an error during unmarshaling, it returns a 500 Internal Server Error status code using errHandlerProblemDetails.
//
// Finally, it constructs the response InlineUserDistanceSubscription containing the distance subscription,
// marshals it into JSON format, sets the HTTP status code to 200 OK, and writes the JSON response to the response writer.
// If there's an error during JSON marshaling, it returns a 500 Internal Server Error status code using errHandlerProblemDetails.
func distanceSubGet(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	vars := mux.Vars(r)

	var response InlineUserDistanceSubscription
	var distanceSub UserDistanceSubscription
	response.UserDistanceSubscription = &distanceSub

	jsonDistanceSub, _ := rc.JSONGetEntry(baseKey+typeDistanceSubscription+":"+vars["subscriptionId"], ".")
	if jsonDistanceSub == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	err := json.Unmarshal([]byte(jsonDistanceSub), &distanceSub)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}

	jsonResponse, err := json.Marshal(response)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(jsonResponse))
}

// distanceSubPost creates a new distance subscription based on the provided data in the request body and returns the created subscription as a JSON response.
//
// Parameters:
//
//	w: http.ResponseWriter - an interface used to manipulate the HTTP response.
//	r: *http.Request - a pointer to the HTTP request received.
//
// The function first sets the content type of the response to "application/json; charset=UTF-8".
//
// It initializes an InlineUserDistanceSubscription struct for both the response and the request body.
// It decodes the request body into the InlineUserDistanceSubscription struct using json.NewDecoder.
// If there's an error during decoding, it logs the error and returns a 500 Internal Server Error status code using errHandlerProblemDetails.
//
// Next, it extracts the UserDistanceSubscription struct from the decoded request body.
// If the body is empty or the UserDistanceSubscription struct is nil, it returns a 400 Bad Request status code with an error message.
//
// The function then checks for the presence of mandatory properties in the UserDistanceSubscription struct:
// - CallbackReference
// - Criteria
// - SubscriptionType
// - MonitoredAddress
// - Distance
// - TrackingAccuracy
// If any mandatory property is missing, it returns a 400 Bad Request status code with an appropriate error message.
//
// If all mandatory properties are present, the function generates a new subscription ID, constructs the self link for the subscription,
// sets the subscription type, stores the subscription in the database using rc.JSONSetEntry, and registers the subscription using registerDistance1.
//
// Finally, it constructs the response InlineUserDistanceSubscription containing the created subscription,
// marshals it into JSON format, sets the HTTP status code to 201 Created, and writes the JSON response to the response writer.
// If there's an error during JSON marshaling, it logs the error and returns a 500 Internal Server Error status code using errHandlerProblemDetails.
func distanceSubPost(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	var response InlineUserDistanceSubscription
	var body InlineUserDistanceSubscription
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&body)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}
	distanceSub := body.UserDistanceSubscription
	if distanceSub == nil {
		log.Error("Body not present")
		errHandlerProblemDetails(w, "Body not present", http.StatusBadRequest)
		return
	}
	//checking for mandatory properties
	if distanceSub.CallbackReference == "" {
		log.Error("Mandatory CallbackReference parameter not present")
		errHandlerProblemDetails(w, "Mandatory CallbackReference parameter not present", http.StatusBadRequest)
		return
	}
	if distanceSub.Criteria == nil {
		log.Error("Mandatory DistanceCriteria parameter not present")
		errHandlerProblemDetails(w, "Mandatory DistanceCriteria parameter not present", http.StatusBadRequest)
		return
	}
	if distanceSub.SubscriptionType != "UserDistanceSubscription" {
		log.Error("Mandatory SubscriptionType parameter not present or invalid")
		errHandlerProblemDetails(w, "Mandatory SubscriptionType parameter not present or invalid", http.StatusBadRequest)
		return
	}
	if distanceSub.MonitoredAddress == nil {
		log.Error("Mandatory MonitoredAddress parameter not present")
		errHandlerProblemDetails(w, "Mandatory MonitoredAddress parameter not present", http.StatusBadRequest)
		return
	}
	if distanceSub.Distance == 0 {
		log.Error("Mandatory Distance parameter not present or its value is 0")
		errHandlerProblemDetails(w, "Mandatory Distance parameter not present or its value is 0", http.StatusBadRequest)
		return
	}
	if distanceSub.TrackingAccuracy == 0 {
		log.Error("Mandatory TrackingAccuracy parameter not present or its value is 0")
		errHandlerProblemDetails(w, "Mandatory TrackingAccuracy parameter not present or its value is 0", http.StatusBadRequest)
		return
	}
	newSubsId := nextDistanceSubscriptionIdAvailable
	nextDistanceSubscriptionIdAvailable++
	subsIdStr := strconv.Itoa(newSubsId)
	location := hostUrl.String() + basePath + "subscriptions/distance/" + subsIdStr
	w.Header().Set("Location", location)
	distanceSub.Links = &Links{
		Self: &LinkType{
			Href: location,
		},
	}
	_ = rc.JSONSetEntry(baseKey+typeDistanceSubscription+":"+subsIdStr, ".", convertDistanceSubscriptionToJson1(distanceSub))
	registerDistance1(distanceSub, subsIdStr)
	response.UserDistanceSubscription = distanceSub
	jsonResponse, err := json.Marshal(response)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	fmt.Fprint(w, string(jsonResponse))
}

// distanceSubPut updates an existing distance subscription with the provided data in the request body and returns the updated subscription as a JSON response.
//
// Parameters:
//
//	w: http.ResponseWriter - an interface used to manipulate the HTTP response.
//	r: *http.Request - a pointer to the HTTP request received.
//
// The function first sets the content type of the response to "application/json; charset=UTF-8".
//
// It initializes an InlineUserDistanceSubscription struct for both the response and the request body.
// It decodes the request body into the InlineUserDistanceSubscription struct using json.NewDecoder.
// If there's an error during decoding, it logs the error and returns a 500 Internal Server Error status code using errHandlerProblemDetails.
//
// Next, it extracts the UserDistanceSubscription struct from the decoded request body.
// If the body is empty or the UserDistanceSubscription struct is nil, it returns a 400 Bad Request status code with an error message.
//
// The function then checks for the presence of mandatory properties in the UserDistanceSubscription struct:
// - CallbackReference
// - Criteria
// - SubscriptionType
// - MonitoredAddress
// - Links.Self.Href
// - Distance
// - TrackingAccuracy
// If any mandatory property is missing, it returns a 400 Bad Request status code with an appropriate error message.
//
// It extracts the subscription ID from the URL path parameters and compares it with the subscription ID extracted from the request body.
// If they don't match, it returns a 400 Bad Request status code with an error message.
//
// If the subscription ID is valid and corresponds to an existing subscription, the function updates the subscription details in the database using rc.JSONSetEntry.
// It deregisters the old subscription, registers the updated subscription, and updates the dynamic states of the subscription.
//
// Finally, it constructs the response InlineUserDistanceSubscription containing the updated subscription,
// marshals it into JSON format, sets the HTTP status code to 200 OK, and writes the JSON response to the response writer.
// If there's an error during JSON marshaling, it logs the error and returns a 500 Internal Server Error status code using errHandlerProblemDetails.
func distanceSubPut(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	vars := mux.Vars(r)
	var response InlineUserDistanceSubscription

	var body InlineUserDistanceSubscription
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&body)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}
	distanceSub := body.UserDistanceSubscription

	if distanceSub == nil {
		log.Error("Body not present")
		errHandlerProblemDetails(w, "Body not present", http.StatusBadRequest)
		return
	}

	//checking for mandatory properties
	if distanceSub.CallbackReference == "" {
		log.Error("Mandatory CallbackReference parameter not present")
		errHandlerProblemDetails(w, "Mandatory CallbackReference parameter not present", http.StatusBadRequest)
		return
	}
	if distanceSub.Criteria == nil {
		log.Error("Mandatory DistanceCriteria parameter not present")
		errHandlerProblemDetails(w, "Mandatory DistanceCriteria parameter not present", http.StatusBadRequest)
		return
	}
	if distanceSub.SubscriptionType != "UserDistanceSubscription" {
		log.Error("Mandatory SubscriptionType parameter not present or invalid")
		errHandlerProblemDetails(w, "Mandatory SubscriptionType parameter not present or invalid", http.StatusBadRequest)
		return
	}
	if distanceSub.MonitoredAddress == nil {
		log.Error("Mandatory MonitoredAddress parameter not present")
		errHandlerProblemDetails(w, "Mandatory MonitoredAddress parameter not present", http.StatusBadRequest)
		return
	}
	if distanceSub.Links == nil || distanceSub.Links.Self == nil || distanceSub.Links.Self.Href == "" {
		log.Error("Mandatory Links.Self.Href parameter not present")
		errHandlerProblemDetails(w, "Mandatory Links.Self.Href parameter not present", http.StatusBadRequest)
		return
	}
	if distanceSub.Distance == 0 {
		log.Error("Mandatory Distance parameter not present or its value is 0")
		errHandlerProblemDetails(w, "Mandatory Distance parameter not present or its value is 0", http.StatusBadRequest)
		return
	}
	if distanceSub.TrackingAccuracy == 0 {
		log.Error("Mandatory TrackingAccuracy parameter not present or its value is 0")
		errHandlerProblemDetails(w, "Mandatory TrackingAccuracy parameter not present or its value is 0", http.StatusBadRequest)
		return
	}
	subsIdParamStr := vars["subscriptionId"]

	selfUrl := strings.Split(distanceSub.Links.Self.Href, "/")
	subsIdStr := selfUrl[len(selfUrl)-1]

	//Body content not matching parameters
	if subsIdStr != subsIdParamStr {
		log.Error("SubscriptionId in endpoint and in body not matching")
		errHandlerProblemDetails(w, "SubscriptionId in endpoint and in body not matching", http.StatusNotFound)
		return
	}

	distanceSub.Links = &Links{
		Self: &LinkType{
			Href: hostUrl.String() + basePath + "subscriptions/distance/" + subsIdStr,
		},
	}

	subsId, err := strconv.Atoi(subsIdStr)
	if err != nil {
		log.Error(err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if distanceSubscriptionMap1[subsId] == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	_ = rc.JSONSetEntry(baseKey+typeDistanceSubscription+":"+subsIdStr, ".", convertDistanceSubscriptionToJson1(distanceSub))

	//store the dynamic states of the subscription
	notifSent := distanceSubscriptionMap1[subsId].NbNotificationsSent
	deregisterDistance(subsIdStr)
	registerDistance1(distanceSub, subsIdStr)
	distanceSubscriptionMap1[subsId].NbNotificationsSent = notifSent

	response.UserDistanceSubscription = distanceSub

	jsonResponse, err := json.Marshal(response)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(jsonResponse))
}

// populateDistanceList populates the subscription list with distance-based subscription information.
//
// This function unmarshals the JSON-encoded distance subscription information and adds it to the
// subscription list in the provided userData object.
//
// Parameters:
//   - key (string): A key associated with the distance subscription data (not used in the function).
//   - jsonInfo (string): JSON-encoded string containing distance subscription information.
//   - userData (interface{}): An interface containing a slice of Subscription objects, which should
//     be of type *[]Subscription.
//
// Returns:
//   - error: An error if JSON unmarshalling fails. Returns nil if the distance subscription information
//     is successfully added to the list.
//
// The function performs the following steps:
//  1. Retrieves the subscription list from the userData object.
//  2. Unmarshals the JSON-encoded distance subscription information.
//  3. Extracts the href from the links if available.
//  4. Creates a Subscription instance with the extracted href and subscription type.
//  5. Adds the Subscription instance to the subscription list.
func populateDistanceList(key string, jsonInfo string, userData interface{}) error {

	subscriptions := userData.(*[]Subscription)
	var distanceInfo UserDistanceSubscription

	// Format response
	err := json.Unmarshal([]byte(jsonInfo), &distanceInfo)
	if err != nil {
		return err
	}
	href := ""
	if distanceInfo.Links != nil && distanceInfo.Links.Self != nil {
		href = distanceInfo.Links.Self.Href
	}
	// Create a Subscription instance
	sub := Subscription{
		Href:             href,
		SubscriptionType: distanceInfo.SubscriptionType,
	}
	*subscriptions = append(*subscriptions, sub)
	return nil
}

// areaSubDELETE deletes a subscription for a specific area or circle identified by its ID and returns a JSON response.
//
// Parameters:
//
//	w: http.ResponseWriter - an interface used to manipulate the HTTP response.
//	r: *http.Request - a pointer to the HTTP request received.
//
// The function first sets the content type of the response to "application/json; charset=UTF-8".
// It then extracts variables from the request's URL path using mux.Vars(r), specifically the 'subscriptionId'.
//
// It checks if the subscription ID exists in the database. If not, it returns a 404 Not Found status code.
//
// The function then attempts to delete the subscription entry from the database using rc.JSONDelEntry.
// If there's an error during deletion, it logs the error and returns a 500 Internal Server Error status code
// using errHandlerProblemDetails.
//
// Additionally, the function calls deregisterAreaCircle to deregister the subscription.
// Finally, it sets the HTTP status code to 204 No Content and returns an empty response body.
func areaSubDELETE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	vars := mux.Vars(r)

	present, _ := rc.JSONGetEntry(baseKey+typeAreaCircleSubscription+":"+vars["subscriptionId"], ".")
	if present == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	err := rc.JSONDelEntry(baseKey+typeAreaCircleSubscription+":"+vars["subscriptionId"], ".")
	if err != nil {
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}

	deregisterAreaCircle(vars["subscriptionId"])
	w.WriteHeader(http.StatusNoContent)
}

// areaSubListGET retrieves a list of area or circle subscriptions and returns it as a JSON response.
//
// Parameters:
//
//	w: http.ResponseWriter - an interface used to manipulate the HTTP response.
//	r: *http.Request - a pointer to the HTTP request received.
//
// The function first sets the content type of the response to "application/json; charset=UTF-8".
//
// It initializes an InlineNotificationSubscriptionList struct and a slice of Subscription structs.
// The function then constructs a Redis key pattern to retrieve all area or circle subscriptions and
// iterates over each entry, populating the slice of Subscription structs using rc.ForEachJSONEntry.
// If there's an error during iteration, it logs the error and returns a 500 Internal Server Error status code
// using errHandlerProblemDetails.
//
// After populating the Subscription slice, the function constructs the response InlineNotificationSubscriptionList,
// setting its NotificationSubscriptionList field to point to the slice of subscriptions and its ResourceURL field
// to the base URL with the path "/subscriptions/area".
//
// Finally, it marshals the response into JSON format, sets the HTTP status code to 200 OK, and writes the JSON response to the response writer.
// If there's an error during JSON marshaling, it logs the error and returns a 500 Internal Server Error status code
// using errHandlerProblemDetails.
func areaSubListGET(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	var response InlineNotificationSubscriptionList
	var subscriptions []Subscription
	keyName := baseKey + typeAreaCircleSubscription + "*"
	err := rc.ForEachJSONEntry(keyName, populateUserAreaList, &subscriptions)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}
	response = InlineNotificationSubscriptionList{
		NotificationSubscriptionList: &NotificationSubscriptionList{
			Subscription: subscriptions,
			ResourceURL: &LinkType{
				Href: hostUrl.String() + basePath + "subscriptions/area",
			},
		},
	}
	jsonResponse, err := json.Marshal(response)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(jsonResponse))
}

// areaSubGET retrieves information about a subscription for a specific area or circle identified by its ID and returns it as a JSON response.
//
// Parameters:
//
//	w: http.ResponseWriter - an interface used to manipulate the HTTP response.
//	r: *http.Request - a pointer to the HTTP request received.
//
// The function first sets the content type of the response to "application/json; charset=UTF-8".
// It then extracts variables from the request's URL path using mux.Vars(r), specifically the 'subscriptionId'.
//
// The function initializes an InlineUserAreaSubscription struct and a UserAreaSubscription struct, setting the 'UserAreaSubscription' field
// of the former to point to the latter.
//
// It retrieves the JSON representation of the area or circle subscription from the database based on the subscription ID.
// If the subscription is not found, it returns a 404 Not Found status code.
//
// The function then unmarshals the JSON data into the UserAreaSubscription struct.
// If there's an error during unmarshaling, it logs the error and returns a 500 Internal Server Error status code
// using errHandlerProblemDetails.
//
// Finally, it marshals the response into JSON format, sets the HTTP status code to 200 OK, and writes the JSON response to the response writer.
// If there's an error during JSON marshaling, it logs the error and returns a 500 Internal Server Error status code
// using errHandlerProblemDetails.
func areaSubGET(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	vars := mux.Vars(r)

	var response InlineUserAreaSubscription
	var areaCircleSub UserAreaSubscription
	response.UserAreaSubscription = &areaCircleSub
	jsonAreaCircleSub, _ := rc.JSONGetEntry(baseKey+typeAreaCircleSubscription+":"+vars["subscriptionId"], ".")
	if jsonAreaCircleSub == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	err := json.Unmarshal([]byte(jsonAreaCircleSub), &areaCircleSub)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}

	jsonResponse, err := json.Marshal(response)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(jsonResponse))
}

// areaSubPOST creates a new subscription for a specific area or circle and returns the created subscription as a JSON response.
//
// Parameters:
//
//	w: http.ResponseWriter - an interface used to manipulate the HTTP response.
//	r: *http.Request - a pointer to the HTTP request received.
//
// The function first sets the content type of the response to "application/json; charset=UTF-8".
//
// It decodes the JSON request body into an InlineUserAreaSubscription struct.
// If there's an error during decoding, it logs the error and returns a 500 Internal Server Error status code
// using errHandlerProblemDetails.
//
// The function then validates the mandatory properties of the subscription:
// CallbackReference, AddressList, Latitude and Longitude, Radius, LocationEventCriteria, SubscriptionType, and TrackingAccuracy.
// If any of these properties are missing or invalid, it logs the error and returns a 400 Bad Request status code
// using errHandlerProblemDetails.
//
// Next, it generates a new subscription ID and constructs the subscription's self-reference link.
// It sets the subscription type to "UserAreaSubscription" and stores the subscription data in the database using rc.JSONSetEntry.
// Additionally, it registers the area circle subscription using registerAreaCircle.
//
// Finally, it constructs the response InlineUserAreaSubscription, marshals it into JSON format, sets the HTTP status code to 201 Created,
// and writes the JSON response to the response writer.
// If there's an error during JSON marshaling, it logs the error and returns a 500 Internal Server Error status code
// using errHandlerProblemDetails.
func areaSubPOST(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	var response InlineUserAreaSubscription

	var body InlineUserAreaSubscription
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&body)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}
	areaCircleSub := body.UserAreaSubscription

	if areaCircleSub == nil {
		log.Error("Body not present")
		errHandlerProblemDetails(w, "Body not present", http.StatusBadRequest)
		return
	}

	//checking for mandatory properties
	if areaCircleSub.CallbackReference == "" {
		log.Error("Mandatory CallbackReference parameter not present")
		errHandlerProblemDetails(w, "Mandatory CallbackReference parameter not present", http.StatusBadRequest)
		return
	}
	if areaCircleSub.AddressList == nil {
		log.Error("Mandatory Address parameter not present")
		errHandlerProblemDetails(w, "Mandatory Address parameter not present", http.StatusBadRequest)
		return
	}
	if len(areaCircleSub.AreaDefine.Points) == 0 {
		log.Error("Latitude and longitude not present")
		errHandlerProblemDetails(w, "Latitude and longitude not present", http.StatusBadRequest)
		return
	}
	if areaCircleSub.AreaDefine.Radius == 0 {
		log.Error("Mandatory Radius parameter not present")
		errHandlerProblemDetails(w, "Mandatory Radius parameter not present", http.StatusBadRequest)
		return
	}

	if areaCircleSub.SubscriptionType != "UserAreaSubscription" {
		log.Error("Mandatory SubscriptionType parameter not present or invalid")
		errHandlerProblemDetails(w, "Mandatory SubscriptionType parameter not present or invalid", http.StatusBadRequest)
		return
	}
	// Check if EnteringLeavingCriteria values are valid

	if len(areaCircleSub.LocationEventCriteria) == 0 && areaCircleSub.LocationEventCriteria == nil {
		locationEventType := []LocationEventType{"ENTERING_AREA_EVENT", "LEAVING_AREA_EVENT"}
		//locationEventType := {ENTERING_AREA_EVENT, LEAVING_AREA_EVENT}
		areaCircleSub.LocationEventCriteria = locationEventType
	} else {
		for _, criteria := range areaCircleSub.LocationEventCriteria {
			if criteria != ENTERING_AREA_EVENT && criteria != LEAVING_AREA_EVENT {
				log.Error("Invalid EnteringLeavingCriteria parameter value")
				errHandlerProblemDetails(w, "Invalid EnteringLeavingCriteria parameter value", http.StatusBadRequest)
				return
			}
		}
	}

	if areaCircleSub.TrackingAccuracy == 0 {
		log.Error("Mandatory TrackingAccuracy parameter not present")
		errHandlerProblemDetails(w, "Mandatory TrackingAccuracy parameter not present", http.StatusBadRequest)
		return
	}

	newSubsId := nextAreaCircleSubscriptionIdAvailable
	nextAreaCircleSubscriptionIdAvailable++
	subsIdStr := strconv.Itoa(newSubsId)
	location := hostUrl.String() + basePath + "subscriptions/area/" + subsIdStr
	w.Header().Set("Location", location)
	areaCircleSub.Links = &Links{
		Self: &LinkType{
			Href: location,
		},
	}

	_ = rc.JSONSetEntry(baseKey+typeAreaCircleSubscription+":"+subsIdStr, ".", convertAreaSubscriptionToJson(areaCircleSub))

	registerAreaCircle(areaCircleSub, subsIdStr)

	response.UserAreaSubscription = areaCircleSub

	jsonResponse, err := json.Marshal(response)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	fmt.Fprint(w, string(jsonResponse))
}

// areaSubPUT updates an existing subscription for a specific area or circle and returns the updated subscription as a JSON response.
//
// Parameters:
//
//	w: http.ResponseWriter - an interface used to manipulate the HTTP response.
//	r: *http.Request - a pointer to the HTTP request received.
//
// The function first sets the content type of the response to "application/json; charset=UTF-8".
//
// It decodes the JSON request body into an InlineUserAreaSubscription struct.
// If there's an error during decoding, it logs the error and returns a 500 Internal Server Error status code
// using errHandlerProblemDetails.
//
// The function then validates the mandatory properties of the subscription:
// CallbackReference, AddressList, Latitude and Longitude, Radius, LocationEventCriteria, SubscriptionType, TrackingAccuracy, and Links.Self.Href.
// If any of these properties are missing or invalid, it logs the error and returns a 400 Bad Request status code
// using errHandlerProblemDetails.
//
// It extracts the subscription ID from the request URL path and compares it with the ID in the request body.
// If they don't match, it logs the error and returns a 400 Bad Request status code using errHandlerProblemDetails.
//
// The function updates the subscription data in the database using rc.JSONSetEntry.
// Additionally, it deregisters the existing area circle subscription, registers the updated subscription, and restores any dynamic states.
//
// Finally, it constructs the response InlineUserAreaSubscription, marshals it into JSON format, sets the HTTP status code to 200 OK,
// and writes the JSON response to the response writer.
// If there's an error during JSON marshaling, it logs the error and returns a 500 Internal Server Error status code
// using errHandlerProblemDetails.
func areaSubPUT(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	var response InlineUserAreaSubscription
	vars := mux.Vars(r)
	var body InlineUserAreaSubscription
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&body)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}
	areaCircleSub := body.UserAreaSubscription

	if areaCircleSub == nil {
		log.Error("Body not present")
		errHandlerProblemDetails(w, "Body not present", http.StatusBadRequest)
		return
	}

	//checking for mandatory properties
	if areaCircleSub.CallbackReference == "" {
		log.Error("Mandatory CallbackReference parameter not present")
		errHandlerProblemDetails(w, "Mandatory CallbackReference parameter not present", http.StatusBadRequest)
		return
	}
	if areaCircleSub.AddressList == nil {
		log.Error("Mandatory Address parameter not present")
		errHandlerProblemDetails(w, "Mandatory Address parameter not present", http.StatusBadRequest)
		return
	}
	if len(areaCircleSub.AreaDefine.Points) == 0 {
		log.Error("Latitude and longitude not present")
		errHandlerProblemDetails(w, "Latitude and longitude not present", http.StatusBadRequest)
		return
	}
	if areaCircleSub.AreaDefine.Radius == 0 {
		log.Error("Mandatory Radius parameter not present")
		errHandlerProblemDetails(w, "Mandatory Radius parameter not present", http.StatusBadRequest)
		return
	}
	// if len(areaCircleSub.LocationEventCriteria) == 0 {
	// 	log.Error("LocationEventCriteria not present")
	// 	errHandlerProblemDetails(w, "LocationEventCriteria not present", http.StatusBadRequest)
	// 	return
	// }
	if areaCircleSub.Links == nil || areaCircleSub.Links.Self == nil || areaCircleSub.Links.Self.Href == "" {
		log.Error("Mandatory Links.Self.Href parameter not present")
		errHandlerProblemDetails(w, "Mandatory Links.Self.Href parameter not present", http.StatusBadRequest)
		return
	}
	if areaCircleSub.SubscriptionType != "UserAreaSubscription" {
		log.Error("Mandatory SubscriptionType parameter not present or invalid")
		errHandlerProblemDetails(w, "Mandatory SubscriptionType parameter not present or invalid", http.StatusBadRequest)
		return
	}
	// Check if EnteringLeavingCriteria values are valid
	if len(areaCircleSub.LocationEventCriteria) == 0 && areaCircleSub.LocationEventCriteria == nil {
		locationEventType := []LocationEventType{"ENTERING_AREA_EVENT", "LEAVING_AREA_EVENT"}
		//locationEventType := {ENTERING_AREA_EVENT, LEAVING_AREA_EVENT}
		areaCircleSub.LocationEventCriteria = locationEventType
	} else {
		for _, criteria := range areaCircleSub.LocationEventCriteria {
			if criteria != ENTERING_AREA_EVENT && criteria != LEAVING_AREA_EVENT {
				log.Error("Invalid EnteringLeavingCriteria parameter value")
				errHandlerProblemDetails(w, "Invalid EnteringLeavingCriteria parameter value", http.StatusBadRequest)
				return
			}
		}
	}

	if areaCircleSub.TrackingAccuracy == 0 {
		log.Error("Mandatory TrackingAccuracy parameter not present")
		errHandlerProblemDetails(w, "Mandatory TrackingAccuracy parameter not present", http.StatusBadRequest)
		return
	}

	subsIdParamStr := vars["subscriptionId"]

	selfUrl := strings.Split(areaCircleSub.Links.Self.Href, "/")
	subsIdStr := selfUrl[len(selfUrl)-1]

	//body content not matching parameters
	if subsIdStr != subsIdParamStr {
		log.Error("SubscriptionId in endpoint and in body not matching")
		errHandlerProblemDetails(w, "SubscriptionId in endpoint and in body not matching", http.StatusNotFound)
		return
	}
	areaCircleSub.Links = &Links{
		Self: &LinkType{
			Href: hostUrl.String() + basePath + "subscriptions/area/" + subsIdStr,
		},
	}

	subsId, err := strconv.Atoi(subsIdStr)
	if err != nil {
		log.Error(err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if areaCircleSubscriptionMap[subsId] == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	_ = rc.JSONSetEntry(baseKey+typeAreaCircleSubscription+":"+subsIdStr, ".", convertAreaSubscriptionToJson(areaCircleSub))

	//store the dynamic states fo the subscription
	notifSent := areaCircleSubscriptionMap[subsId].NbNotificationsSent
	addrInArea := areaCircleSubscriptionMap[subsId].AddrInArea
	deregisterAreaCircle(subsIdStr)
	registerAreaCircle(areaCircleSub, subsIdStr)
	areaCircleSubscriptionMap[subsId].NbNotificationsSent = notifSent
	areaCircleSubscriptionMap[subsId].AddrInArea = addrInArea

	response.UserAreaSubscription = areaCircleSub

	jsonResponse, err := json.Marshal(response)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(jsonResponse))
}

// populateUserAreaList populates the subscription list with user area subscription information.
//
// This function unmarshals the JSON-encoded user area subscription information and adds it to the
// subscription list in the provided userData object.
//
// Parameters:
//   - key (string): A key associated with the user area subscription data (not used in the function).
//   - jsonInfo (string): JSON-encoded string containing user area subscription information.
//   - userData (interface{}): An interface containing a slice of Subscription objects, which should
//     be of type *[]Subscription.
//
// Returns:
//   - error: An error if JSON unmarshalling fails. Returns nil if the user area subscription information
//     is successfully added to the list.
//
// The function performs the following steps:
//  1. Retrieves the subscription list from the userData object.
//  2. Unmarshals the JSON-encoded user area subscription information.
//  3. Extracts the href from the links if available.
//  4. Creates a Subscription instance with the extracted href and subscription type.
//  5. Adds the Subscription instance to the subscription list.
func populateUserAreaList(key string, jsonInfo string, userData interface{}) error {

	subscriptions := userData.(*[]Subscription)
	var areaInfo UserAreaSubscription

	// Format response
	err := json.Unmarshal([]byte(jsonInfo), &areaInfo)
	if err != nil {
		return err
	}
	href := ""
	if areaInfo.Links != nil && areaInfo.Links.Self != nil {
		href = areaInfo.Links.Self.Href
	}
	// Create a Subscription instance
	sub := Subscription{
		Href:             href,
		SubscriptionType: areaInfo.SubscriptionType,
	}
	*subscriptions = append(*subscriptions, sub)
	return nil
}

// userSubListGET retrieves a list of user subscriptions based on the provided query parameters 'subscription_type' and 'address'.
// It returns the list of subscriptions as a JSON response.
//
// Parameters:
//
//	w: http.ResponseWriter - an interface used to manipulate the HTTP response.
//	r: *http.Request - a pointer to the HTTP request received.
//
// The function first sets the content type of the response to "application/json; charset=UTF-8".
//
// It retrieves the query parameters from the URL using r.URL.Query().
// The 'subscription_type' parameter specifies the type of subscription ('periodic' or 'event').
// The 'address' parameter filters subscriptions based on the provided address.
//
// Based on the 'subscription_type' parameter, the function retrieves subscriptions from Redis by iterating over the appropriate keys.
// If 'subscription_type' is 'periodic', it retrieves periodic subscriptions; if 'event', it retrieves event subscriptions.
// If 'subscription_type' is not provided or invalid, it retrieves both types of subscriptions.
// It populates the list of subscriptions using the appropriate callback function: populateUserSubList for periodic subscriptions and populateUserSubList1 for event subscriptions.
//
// If there's an error during the iteration or population of subscriptions, it logs the error and returns a 500 Internal Server Error status code using errHandlerProblemDetails.
//
// Finally, it constructs the response InlineNotificationSubscriptionList containing the list of subscriptions,
// marshals it into JSON format, sets the HTTP status code to 200 OK, and writes the JSON response to the response writer.
// If there's an error during JSON marshaling, it logs the error and returns a 500 Internal Server Error status code using errHandlerProblemDetails.
func userSubListGET(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	// Check if the 'subscription_type' and 'address' query parameters exist and get their values
	queryParams := r.URL.Query()
	subscriptionType := queryParams.Get("subscription_type")
	address := queryParams.Get("address")
	// If subscriptionType is not empty, validate it
	if subscriptionType != "" {
		// Validating subscriptionType parameter
		validSubscriptionTypes := map[string]bool{"event": true, "periodic": true}
		if _, ok := validSubscriptionTypes[strings.ToLower(subscriptionType)]; !ok {
			http.Error(w, "Invalid subscription type. Allowed values are 'event' and 'periodic'.", http.StatusBadRequest)
			return
		}
	}
	var response InlineNotificationSubscriptionList

	// Create a slice to hold subscriptions
	var subscriptions []Subscription

	var err error
	switch subscriptionType {
	case "periodic":
		keyName := baseKey + typePeriodicSubscription + "*"
		err = rc.ForEachJSONEntry(keyName, func(key string, jsonInfo string, userData interface{}) error {
			return populateUserSubList([]byte(jsonInfo), address, &subscriptions)
		}, nil)
	case "event":
		keyName := baseKey + typeUserSubscription + "*"
		err = rc.ForEachJSONEntry(keyName, func(key string, jsonInfo string, userData interface{}) error {
			return populateUserSubList1([]byte(jsonInfo), address, &subscriptions)
		}, nil)
	default:
		var userSubListPeriodic []Subscription
		keyNamePeriodic := baseKey + typePeriodicSubscription + "*"
		errPeriodic := rc.ForEachJSONEntry(keyNamePeriodic, func(key string, jsonInfo string, userData interface{}) error {
			return populateUserSubList([]byte(jsonInfo), address, &userSubListPeriodic)
		}, nil)
		if errPeriodic != nil {
			log.Error(errPeriodic.Error())
			errHandlerProblemDetails(w, errPeriodic.Error(), http.StatusInternalServerError)
			return
		}

		var userSubListEvent []Subscription
		keyNameEvent := baseKey + typeUserSubscription + "*"
		errEvent := rc.ForEachJSONEntry(keyNameEvent, func(key string, jsonInfo string, userData interface{}) error {
			return populateUserSubList1([]byte(jsonInfo), address, &userSubListEvent)
		}, nil)
		if errEvent != nil {
			log.Error(errEvent.Error())
			errHandlerProblemDetails(w, errEvent.Error(), http.StatusInternalServerError)
			return
		}

		subscriptions = append(subscriptions, userSubListPeriodic...)
		subscriptions = append(subscriptions, userSubListEvent...)

		err = nil
	}

	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Constructing the response
	response = InlineNotificationSubscriptionList{
		NotificationSubscriptionList: &NotificationSubscriptionList{
			Subscription: subscriptions,
			ResourceURL: &LinkType{
				Href: hostUrl.String() + basePath + "subscriptions/users",
			},
		},
	}

	// Marshaling the response
	jsonResponse, err := json.Marshal(response)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Writing the response
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(jsonResponse); err != nil {
		log.Error("Can't write response: ", err.Error())
		http.Error(w, fmt.Sprintf("could not write response: %v", err), http.StatusInternalServerError)
	}
}

// userSubDELETE handles the DELETE request for deleting user subscriptions by subscription ID.
// It first checks if a subscription exists for the given ID in both event-based and periodic-based subscriptions.
// If a subscription is found in either type, it deletes the subscription entry and deregisters it from the respective tracking system.
//
// Parameters:
//
//	w: http.ResponseWriter - an interface used to manipulate the HTTP response.
//	r: *http.Request - a pointer to the HTTP request received.
//
// The function first sets the content type of the response to JSON.
//
// It then retrieves the subscription ID from the request parameters.
//
// Next, it checks if there's a user subscription or a periodic subscription corresponding to the given subscription ID.
// If neither type of subscription is found, it returns a 404 Not Found response.
//
// If a user subscription is found, it deletes the subscription entry from the Redis database and deregisters it from the user tracking system.
// If the deletion operation encounters an error, it returns a 500 Internal Server Error response.
// Otherwise, it returns a 204 No Content response.
//
// If a periodic subscription is found, it performs similar deletion and deregistration operations as for the user subscription.
//
// If the deletion operation for a periodic subscription encounters an error, it returns a 500 Internal Server Error response.
// Otherwise, it returns a 204 No Content response.
//
// If there's an error during any of these steps, it logs the error and returns a 500 Internal Server Error response.
func userSubDELETE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	vars := mux.Vars(r)

	subscriptionID := vars["subscriptionId"]
	presentUser, _ := rc.JSONGetEntry(baseKey+typeUserSubscription+":"+subscriptionID, ".")
	presentPeriodic, _ := rc.JSONGetEntry(baseKey+typePeriodicSubscription+":"+subscriptionID, ".")

	if presentUser == "" && presentPeriodic == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	if presentUser != "" {
		err := rc.JSONDelEntry(baseKey+typeUserSubscription+":"+subscriptionID, ".")
		if err != nil {
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}
		deregisterUser(subscriptionID)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if presentPeriodic != "" {
		err := rc.JSONDelEntry(baseKey+typePeriodicSubscription+":"+subscriptionID, ".")
		if err != nil {
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}
		deregisterPeriodic(subscriptionID)
		w.WriteHeader(http.StatusNoContent)
		return
	}

}

// userSubGET handles the GET request for retrieving user subscriptions by subscription ID.
// It first checks if the subscription ID corresponds to a periodic-based subscription.
// If a periodic-based subscription is found, it retrieves the subscription details,
// constructs an InlineUserLocationPeriodicSubscription response, and returns it.
//
// If the subscription ID does not correspond to a periodic-based subscription,
// it assumes it's an event-based subscription, retrieves the subscription details,
// constructs an InlineUserLocationEventSubscription response, and returns it.
//
// If no subscription is found for the given ID, it returns a 404 Not Found response.
//
// Parameters:
//
//	w: http.ResponseWriter - an interface used to manipulate the HTTP response.
//	r: *http.Request - a pointer to the HTTP request received.
//
// The function first sets the content type of the response to JSON.
//
// It then retrieves the subscription ID from the request parameters and attempts to fetch the corresponding periodic-based subscription.
// If found, it unmarshals the subscription details into a UserLocationPeriodicSubscription struct,
// constructs an InlineUserLocationPeriodicSubscription response, and assigns it to the 'response' variable.
//
// If no periodic-based subscription is found, it assumes the subscription ID corresponds to an event-based subscription,
// attempts to fetch the subscription details, unmarshals them into a UserLocationEventSubscription struct,
// constructs an InlineUserLocationEventSubscription response, and assigns it to the 'response' variable.
//
// If no subscription is found for the given ID, it returns a 404 Not Found response.
//
// Finally, it marshals the response into JSON format, writes the JSON response to the response writer,
// and sets the HTTP status code to 200 OK.
//
// If there's an error during any of these steps, it logs the error and returns a 500 Internal Server Error response.
func userSubGET(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	vars := mux.Vars(r)
	var response interface{}

	// Check if the subscription ID corresponds to a periodic-based subscription
	jsonPeriodicSub, _ := rc.JSONGetEntry(baseKey+typePeriodicSubscription+":"+vars["subscriptionId"], ".")
	if jsonPeriodicSub != "" {
		var periodicSub UserLocationPeriodicSubscription
		if err := json.Unmarshal([]byte(jsonPeriodicSub), &periodicSub); err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}
		response = InlineUserLocationPeriodicSubscription{UserLocationPeriodicSubscription: &periodicSub}
	} else {
		// If the subscription ID does not correspond to a periodic-based subscription,
		// assume it's an event-based subscription
		var eventSub UserLocationEventSubscription
		jsonEventSub, _ := rc.JSONGetEntry(baseKey+typeUserSubscription+":"+vars["subscriptionId"], ".")
		if jsonEventSub == "" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if err := json.Unmarshal([]byte(jsonEventSub), &eventSub); err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}
		response = InlineUserLocationEventSubscription{UserLocationEventSubscription: &eventSub}
	}

	jsonResponse, err := json.Marshal(response)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(jsonResponse))
}

// userSubPOST handles the POST request for creating user subscriptions.
// It decodes the request body into a map and determines the type of subscription based on its presence in the map.
// If both event-based and periodic-based subscriptions are present in the request body, it returns a 400 Bad Request response.
// Otherwise, it delegates the handling to the appropriate function based on the type of subscription.
//
// Parameters:
//
//	w: http.ResponseWriter - an interface used to manipulate the HTTP response.
//	r: *http.Request - a pointer to the HTTP request received.
//
// The function first decodes the request body into a map[string]interface{} using json.NewDecoder.
// If there's an error during decoding, it logs the error and returns a 400 Bad Request response with an error message.
//
// It then checks if either 'userLocationEventSubscription' or 'userLocationPeriodicSubscription' is present in the request body.
// If both are present, it returns a 400 Bad Request response with an error message.
//
// If 'userLocationEventSubscription' is present, it delegates the handling to the handleUserLocationEventSubscription function.
// If 'userLocationPeriodicSubscription' is present, it delegates the handling to the handleUserLocationPeriodicSubscription function.
//
// If neither subscription type is present in the request body, it returns a 400 Bad Request response with an error message.
func userSubPOST(w http.ResponseWriter, r *http.Request) {

	var requestBody map[string]interface{}

	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&requestBody); err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, "Invalid request body format", http.StatusBadRequest)
		return
	}

	hasEventSubscription := false
	hasPeriodicSubscription := false

	if _, ok := requestBody["userLocationEventSubscription"]; ok {
		hasEventSubscription = true
	} else if _, ok := requestBody["userLocationPeriodicSubscription"]; ok {
		hasPeriodicSubscription = true
	}

	// Check if both types of subscriptions are present
	if hasEventSubscription && hasPeriodicSubscription {
		errHandlerProblemDetails(w, "Please send only one type of subscription at a time", http.StatusBadRequest)
		return
	}

	if hasEventSubscription {
		handleUserLocationEventSubscription(w, requestBody)
	} else if hasPeriodicSubscription {
		handleUserLocationPeriodicSubscription(w, requestBody)
	} else {
		errHandlerProblemDetails(w, "No valid subscription found in the request body", http.StatusBadRequest)
		return
	}
}

// handleUserLocationEventSubscription handles the creation of event-based user location subscriptions.
// It expects the subscription information to be present in the requestBody map under the key 'userLocationEventSubscription'.
// It validates the mandatory properties of the subscription and registers it accordingly.
// If successful, it returns a 201 Created response with the created subscription details.
//
// Parameters:
//
//	w: http.ResponseWriter - an interface used to manipulate the HTTP response.
//	requestBody: map[string]interface{} - a map containing the request body decoded from JSON.
//
// The function first checks if 'userLocationEventSubscription' is present in the requestBody map.
// If not present, it returns a 400 Bad Request response with an error message.
//
// It then converts the event subscription map to a struct, validates its mandatory properties,
// generates a unique subscription ID, registers the subscription, constructs the response, marshals it into JSON format,
// and writes the JSON response to the response writer.
// If there's an error during any of these steps, it logs the error and returns a 500 Internal Server Error response.
func handleUserLocationEventSubscription(w http.ResponseWriter, requestBody map[string]interface{}) {

	if eventSubscription, ok := requestBody["userLocationEventSubscription"]; ok {
		// Convert the event subscription to a map
		eventSubscriptionMap := eventSubscription.(map[string]interface{})

		// Decode the event subscription map into a struct
		var userSubBody UserLocationEventSubscription
		err := mapstructure.Decode(eventSubscriptionMap, &userSubBody)
		if err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Validate mandatory properties
		if userSubBody.CallbackReference == "" {
			log.Error("Mandatory CallbackReference parameter not present")
			errHandlerProblemDetails(w, "Mandatory CallbackReference parameter not present", http.StatusBadRequest)
			return
		}
		if userSubBody.Address == "" {
			log.Error("Mandatory Address parameter not present")
			errHandlerProblemDetails(w, "Mandatory Address parameter not present", http.StatusBadRequest)
			return
		}
		// if !isValidIPAddress(userSubBody.Address) {
		// 	log.Error("Invalid Address parameter")
		// 	errHandlerProblemDetails(w, "Invalid Address parameter", http.StatusBadRequest)
		// 	return
		// }
		if userSubBody.SubscriptionType != "UserLocationEventSubscription" {
			log.Error("Mandatory SubscriptionType parameter not present or invalid")
			errHandlerProblemDetails(w, "Mandatory SubscriptionType parameter not present or invalid", http.StatusBadRequest)
			return
		}
		// Add your logic to register the event-based subscription
		newSubsId := nextUserSubscriptionIdAvailable
		if newSubsId%2 != 0 {
			newSubsId++ // Ensure newSubsId is even
		}
		nextUserSubscriptionIdAvailable = newSubsId + 2 // Increment by 2 to ensure the next even number
		subsIdStr := strconv.Itoa(newSubsId)
		location := hostUrl.String() + basePath + "subscriptions/users/" + subsIdStr
		w.Header().Set("Location", location)
		userSubBody.Links = &Links{} // Initialize Links outside the loop

		userSubBody.Links.Self = &LinkType{
			Href: location,
		}
		registerUser1(userSubBody.Address, userSubBody.LocationEventCriteria, subsIdStr, &userSubBody)

		_ = rc.JSONSetEntry(baseKey+typeUserSubscription+":"+subsIdStr, ".", convertUserSubscriptionToJson1(&userSubBody))

		// Prepare response
		var response InlineUserLocationEventSubscription
		response.UserLocationEventSubscription = &userSubBody

		// Marshal response
		jsonResponse, err := json.Marshal(response)
		if err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Write response
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, string(jsonResponse))
		return
	}
}

// handleUserLocationPeriodicSubscription handles the creation of periodic-based user location subscriptions.
// It expects the subscription information to be present in the requestBody map under the key 'userLocationPeriodicSubscription'.
// It validates the mandatory properties of the subscription and registers it accordingly.
// If successful, it returns a 201 Created response with the created subscription details.
//
// Parameters:
//
//	w: http.ResponseWriter - an interface used to manipulate the HTTP response.
//	requestBody: map[string]interface{} - a map containing the request body decoded from JSON.
//
// The function first checks if 'userLocationPeriodicSubscription' is present in the requestBody map.
// If not present, it returns a 400 Bad Request response with an error message.
//
// It then converts the periodic subscription map to a struct, validates its mandatory properties,
// generates a unique subscription ID, registers the subscription, constructs the response, marshals it into JSON format,
// and writes the JSON response to the response writer.
// If there's an error during any of these steps, it logs the error and returns a 500 Internal Server Error response.
func handleUserLocationPeriodicSubscription(w http.ResponseWriter, requestBody map[string]interface{}) {

	if periodicSubscription, ok := requestBody["userLocationPeriodicSubscription"]; ok {
		// Convert the periodic subscription to a map
		periodicSubscriptionMap := periodicSubscription.(map[string]interface{})

		// Decode the periodic subscription map into a struct
		var periodicSub UserLocationPeriodicSubscription
		err := mapstructure.Decode(periodicSubscriptionMap, &periodicSub)
		if err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Validate mandatory properties
		if periodicSub.CallbackReference == "" {
			log.Error("Mandatory CallbackReference parameter not present")
			errHandlerProblemDetails(w, "Mandatory CallbackReference parameter not present", http.StatusBadRequest)
			return
		}
		if periodicSub.Address == "" {
			log.Error("Mandatory Address parameter not present")
			errHandlerProblemDetails(w, "Mandatory Address parameter not present", http.StatusBadRequest)
			return
		}
		if periodicSub.SubscriptionType != "UserLocationPeriodicSubscription" {
			log.Error("Mandatory SubscriptionType parameter not present or invalid")
			errHandlerProblemDetails(w, "Mandatory SubscriptionType parameter not present or invalid", http.StatusBadRequest)
			return
		}
		if periodicSub.PeriodicEventInfo == nil {
			log.Error("Mandatory periodicEventInfo parameter not present")
			errHandlerProblemDetails(w, "Mandatory periodicEventInfo parameter not present", http.StatusBadRequest)
			return
		}
		// Add your logic to register the periodic-based subscription
		newSubsId := nextPeriodicSubscriptionIdAvailable
		nextPeriodicSubscriptionIdAvailable += 2
		subsIdStr := strconv.Itoa(newSubsId)
		location := hostUrl.String() + basePath + "subscriptions/users/" + subsIdStr
		w.Header().Set("Location", location)
		periodicSub.Links = &Links{} // Initialize Links outside the loop

		periodicSub.Links.Self = &LinkType{
			Href: location,
		}
		_ = rc.JSONSetEntry(baseKey+typePeriodicSubscription+":"+subsIdStr, ".", convertPeriodicSubscriptionToJson1(&periodicSub))
		registerPeriodic1(&periodicSub, subsIdStr)

		// Prepare response
		var response InlineUserLocationPeriodicSubscription
		response.UserLocationPeriodicSubscription = &periodicSub

		// Marshal response
		jsonResponse, err := json.Marshal(response)
		if err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Write response
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, string(jsonResponse))
		return
	}
}

// zoneSubPUT handles the PUT request for updating zone subscriptions.
//
// It first decodes the request body into a map[string]interface{} to extract the subscription details.
// If the decoding process encounters an error, it logs the error, constructs and returns a Bad Request response.
//
// Next, it determines whether the request body contains a zone location event subscription or a zone status subscription.
// Based on this determination, it calls the corresponding handler function to process the subscription update.
// If neither type of subscription is found in the request body, it constructs and returns a Bad Request response.
//
// Parameters:
//
//	w: http.ResponseWriter - an interface used to manipulate the HTTP response.
//	r: *http.Request - a pointer to the HTTP request received.
func zoneSubPUT(w http.ResponseWriter, r *http.Request) {
	var requestBody map[string]interface{}
	vars := mux.Vars(r)
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&requestBody); err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, "Invalid request body format", http.StatusBadRequest)
		return
	}

	hasZoneSubscription := false
	hasStatusSubscription := false

	if _, ok := requestBody["zoneLocationEventSubscription"]; ok {
		hasZoneSubscription = true
	} else if _, ok := requestBody["zoneStatusSubscription"]; ok {
		hasStatusSubscription = true
	}

	// Check if both types of subscriptions are present
	if hasZoneSubscription && hasStatusSubscription {
		errHandlerProblemDetails(w, "Please send only one type of subscription at a time", http.StatusBadRequest)
		return
	}

	if hasZoneSubscription {
		handlezoneLocationEventSubscriptionPut(w, requestBody, vars["subscriptionId"])
	} else if hasStatusSubscription {
		handlezoneStatusSubscriptionPut(w, requestBody, vars["subscriptionId"])
	} else {
		errHandlerProblemDetails(w, "No valid subscription found in the request body", http.StatusBadRequest)
		return
	}
}

// userSubPUT handles the PUT request for updating user subscriptions by subscription ID.
// It first decodes the request body into a map[string]interface{} to determine the type of subscription.
// Then, it checks if both event-based and periodic-based subscriptions are present in the request body.
// If both types are present, it returns a 400 Bad Request response.
// If only one type of subscription is present, it calls the corresponding handler function to process the update.
//
// Parameters:
//
//	w: http.ResponseWriter - an interface used to manipulate the HTTP response.
//	r: *http.Request - a pointer to the HTTP request received.
//
// The function first sets the content type of the response to JSON.
//
// It then decodes the request body into a map[string]interface{}.
//
// Next, it checks if both event-based and periodic-based subscriptions are present in the request body.
// If both types are present, it returns a 400 Bad Request response.
// If only one type of subscription is present, it calls the corresponding handler function to process the update.
// If no valid subscription is found in the request body, it returns a 400 Bad Request response.
func userSubPUT(w http.ResponseWriter, r *http.Request) {
	var requestBody map[string]interface{}
	vars := mux.Vars(r)
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&requestBody); err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, "Invalid request body format", http.StatusBadRequest)
		return
	}

	hasEventSubscription := false
	hasPeriodicSubscription := false

	if _, ok := requestBody["userLocationEventSubscription"]; ok {
		hasEventSubscription = true
	} else if _, ok := requestBody["userLocationPeriodicSubscription"]; ok {
		hasPeriodicSubscription = true
	}

	// Check if both types of subscriptions are present
	if hasEventSubscription && hasPeriodicSubscription {
		errHandlerProblemDetails(w, "Please send only one type of subscription at a time", http.StatusBadRequest)
		return
	}

	if hasEventSubscription {
		handleUserLocationEventSubscriptionPut(w, requestBody, vars["subscriptionId"])
	} else if hasPeriodicSubscription {
		handleUserLocationPeriodicSubscriptionPut(w, requestBody, vars["subscriptionId"])
	} else {
		errHandlerProblemDetails(w, "No valid subscription found in the request body", http.StatusBadRequest)
		return
	}
}

// handlezoneLocationEventSubscriptionPut handles the update of zone location event subscriptions.
//
// It takes the HTTP response writer, the request body containing the subscription details,
// and the subscription ID as input parameters.
//
// It first checks if the request body contains a zone location event subscription.
// If found, it converts the subscription data to a map, decodes it into a ZoneLocationEventSubscription struct,
// and performs validation checks on mandatory properties.
// If any mandatory property is missing or invalid, it constructs and returns a Bad Request response.
// It also compares the subscription ID in the request body with the subscription ID in the endpoint URL.
// If they do not match, it constructs and returns a Bad Request response.
//
// After validation, it updates the subscription details in Redis, deregisters the previous subscription,
// registers the updated subscription, constructs the response, marshals it into JSON format, and writes it to the HTTP response.
//
// Parameters:
//
//	w: http.ResponseWriter - an interface used to manipulate the HTTP response.
//	requestBody: map[string]interface{} - a map containing the subscription details extracted from the request body.
//	subscriptionID: string - the ID of the subscription extracted from the endpoint URL.
func handlezoneLocationEventSubscriptionPut(w http.ResponseWriter, requestBody map[string]interface{}, subscriptionID string) {

	if zoneSubscription, ok := requestBody["zoneLocationEventSubscription"]; ok {
		// Convert the event subscription to a map
		zoneSubscriptionMap := zoneSubscription.(map[string]interface{})
		if links, ok := zoneSubscriptionMap["_links"]; ok {
			zoneSubscriptionMap["Links"] = links
			delete(zoneSubscriptionMap, "_links")
		}
		// Decode the event subscription map into a struct
		var zoneSubBody ZoneLocationEventSubscription
		err := mapstructure.Decode(zoneSubscriptionMap, &zoneSubBody)
		if err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}

		//checking for mandatory properties
		if zoneSubBody.CallbackReference == "" {
			log.Error("Mandatory CallbackReference parameter not present")
			errHandlerProblemDetails(w, "Mandatory CallbackReference parameter not present", http.StatusBadRequest)
			return
		}
		if zoneSubBody.ZoneId == "" {
			log.Error("Mandatory ZoneId parameter not present")
			errHandlerProblemDetails(w, "Mandatory ZoneId parameter not present", http.StatusBadRequest)
			return
		}
		if zoneSubBody.SubscriptionType != "ZoneLocationEventSubscription" {
			log.Error("Mandatory SubscriptionType parameter not present or invalid")
			errHandlerProblemDetails(w, "Mandatory SubscriptionType parameter not present or invalid", http.StatusBadRequest)
			return
		}
		if zoneSubBody.Links == nil || zoneSubBody.Links.Self == nil || zoneSubBody.Links.Self.Href == "" {
			log.Error("Mandatory Links.Self.Href parameter not present")
			errHandlerProblemDetails(w, "Mandatory Links.Self.Href parameter not present", http.StatusBadRequest)
			return
		}
		subsIdParamStr := subscriptionID
		selfUrl := strings.Split(zoneSubBody.Links.Self.Href, "/")
		subsIdStr := selfUrl[len(selfUrl)-1]

		//body content not matching parameters
		if subsIdStr != subsIdParamStr {
			log.Error("SubscriptionId in endpoint and in body not matching")
			errHandlerProblemDetails(w, "SubscriptionId in endpoint and in body not matching", http.StatusNotFound)
			return
		}
		zoneSubBody.Links = &Links{} // Initialize Links outside the loop

		zoneSubBody.Links.Self = &LinkType{
			Href: hostUrl.String() + basePath + "subscriptions/zones/" + subsIdStr,
		}

		subsId, err := strconv.Atoi(subsIdStr)
		if err != nil {
			log.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if zonalSubscriptionMap[subsId] == "" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		_ = rc.JSONSetEntry(baseKey+typeZonalSubscription+":"+subsIdStr, ".", convertZonalSubscriptionToJson1(&zoneSubBody))

		deregisterZonal(subsIdStr)

		registerZonal1(zoneSubBody.ZoneId, zoneSubBody.LocationEventCriteria, subsIdStr, &zoneSubBody)
		var response InlineZoneLocationEventSubscription
		response.ZoneLocationEventSubscription = &zoneSubBody

		// Marshal response
		jsonResponse, err := json.Marshal(response)
		if err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Write response
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, string(jsonResponse))
		return

	}
}

// handlezoneStatusSubscriptionPut handles the update of zone status subscriptions.
//
// It takes the HTTP response writer, the request body containing the subscription details,
// and the subscription ID as input parameters.
//
// It first checks if the request body contains a zone status subscription.
// If found, it converts the subscription data to a map, decodes it into a ZoneStatusSubscription struct,
// and performs validation checks on mandatory properties.
// If any mandatory property is missing or invalid, it constructs and returns a Bad Request response.
// It also compares the subscription ID in the request body with the subscription ID in the endpoint URL.
// If they do not match, it constructs and returns a Bad Request response.
//
// After validation, it updates the subscription details in Redis, deregisters the previous subscription,
// registers the updated subscription, constructs the response, marshals it into JSON format, and writes it to the HTTP response.
//
// Parameters:
//
//	w: http.ResponseWriter - an interface used to manipulate the HTTP response.
//	requestBody: map[string]interface{} - a map containing the subscription details extracted from the request body.
//	subscriptionID: string - the ID of the subscription extracted from the endpoint URL.
func handlezoneStatusSubscriptionPut(w http.ResponseWriter, requestBody map[string]interface{}, subscriptionID string) {

	if statusSubscription, ok := requestBody["zoneStatusSubscription"]; ok {
		// Convert the event subscription to a map
		statusSubscriptionMap := statusSubscription.(map[string]interface{})
		if links, ok := statusSubscriptionMap["_links"]; ok {
			statusSubscriptionMap["Links"] = links
			delete(statusSubscriptionMap, "_links")
		}
		// Decode the event subscription map into a struct
		var zoneStatusSub ZoneStatusSubscription
		err := mapstructure.Decode(statusSubscriptionMap, &zoneStatusSub)
		if err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}

		//checking for mandatory properties
		if zoneStatusSub.CallbackReference == "" {
			log.Error("Mandatory CallbackReference parameter not present")
			errHandlerProblemDetails(w, "Mandatory CallbackReference parameter not present", http.StatusBadRequest)
			return
		}
		if zoneStatusSub.ZoneId == "" {
			log.Error("Mandatory ZoneId parameter not present")
			errHandlerProblemDetails(w, "Mandatory ZoneId parameter not present", http.StatusBadRequest)
			return
		}
		if zoneStatusSub.Links == nil || zoneStatusSub.Links.Self == nil || zoneStatusSub.Links.Self.Href == "" {
			log.Error("Mandatory Ref parameter not present")
			errHandlerProblemDetails(w, "Mandatory ResourceURL parameter not present", http.StatusBadRequest)
			return
		}
		if zoneStatusSub.SubscriptionType != "ZoneStatusSubscription" {
			log.Error("Mandatory SubscriptionType parameter not present or invalid")
			errHandlerProblemDetails(w, "Mandatory SubscriptionType parameter not present or invalid", http.StatusBadRequest)
			return
		}

		subsIdParamStr := subscriptionID
		selfUrl := strings.Split(zoneStatusSub.Links.Self.Href, "/")
		subsIdStr := selfUrl[len(selfUrl)-1]

		//body content not matching parameters
		if subsIdStr != subsIdParamStr {
			log.Error("SubscriptionId in endpoint and in body not matching")
			errHandlerProblemDetails(w, "SubscriptionId in endpoint and in body not matching", http.StatusNotFound)
			return
		}
		zoneStatusSub.Links = &Links{} // Initialize Links outside the loop

		zoneStatusSub.Links.Self = &LinkType{
			Href: hostUrl.String() + basePath + "subscriptions/zones/" + subsIdStr,
		}
		subsId, err := strconv.Atoi(subsIdStr)
		if err != nil {
			log.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if zoneStatusSubscriptionMap[subsId] == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		_ = rc.JSONSetEntry(baseKey+typeZoneStatusSubscription+":"+subsIdStr, ".", convertZoneStatusSubscriptionToJson(&zoneStatusSub))

		deregisterZoneStatus(subsIdStr)

		registerZoneStatus(zoneStatusSub.ZoneId, zoneStatusSub.UpperNumberOfUsersZoneThreshold, zoneStatusSub.UpperNumberOfUsersAPThreshold,
			zoneStatusSub.OperationStatus, subsIdStr, zoneStatusSub.LowerNumberOfUsersZoneThreshold, zoneStatusSub.LowerNumberOfUsersAPThreshold, &zoneStatusSub)
		var response InlineZoneStatusSubscription
		response.ZoneStatusSubscription = &zoneStatusSub

		// Marshal response
		jsonResponse, err := json.Marshal(response)
		if err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Write response
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, string(jsonResponse))
		return

	}
}

// handleUserLocationEventSubscriptionPut handles the PUT request for updating event-based user subscriptions by subscription ID.
// It decodes the event subscription map from the request body into a struct, validates the mandatory parameters,
// and performs the update operation.
//
// Parameters:
//
//	w: http.ResponseWriter - an interface used to manipulate the HTTP response.
//	requestBody: map[string]interface{} - the request body containing the event-based subscription information.
//	subscriptionID: string - the ID of the subscription to be updated.
//
// The function first converts the event subscription map to a struct and validates the mandatory parameters.
// If the parameters are valid, it updates the subscription entry in the Redis database and deregisters the existing subscription.
// If the update operation encounters an error, it returns a 500 Internal Server Error response.
// Otherwise, it returns a 200 OK response with the updated subscription details.
func handleUserLocationEventSubscriptionPut(w http.ResponseWriter, requestBody map[string]interface{}, subscriptionID string) {

	if eventSubscription, ok := requestBody["userLocationEventSubscription"]; ok {
		// Convert the event subscription to a map
		eventSubscriptionMap := eventSubscription.(map[string]interface{})
		// Convert _links to Links
		if links, ok := eventSubscriptionMap["_links"]; ok {
			eventSubscriptionMap["Links"] = links
			delete(eventSubscriptionMap, "_links")
		}
		// Decode the event subscription map into a struct
		var userSubBody UserLocationEventSubscription
		err := mapstructure.Decode(eventSubscriptionMap, &userSubBody)
		if err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if userSubBody.CallbackReference == "" {
			log.Error("Mandatory CallbackReference parameter not present")
			errHandlerProblemDetails(w, "Mandatory CallbackReference parameter not present", http.StatusBadRequest)
			return
		}
		if userSubBody.Address == "" {
			log.Error("Mandatory Address parameter not present")
			errHandlerProblemDetails(w, "Mandatory Address parameter not present", http.StatusBadRequest)
			return
		}
		if userSubBody.SubscriptionType != "UserLocationEventSubscription" {
			log.Error("Mandatory SubscriptionType parameter not present or invalid")
			errHandlerProblemDetails(w, "Mandatory SubscriptionType parameter not present or invalid", http.StatusBadRequest)
			return
		}
		if userSubBody.Links == nil || userSubBody.Links.Self == nil || userSubBody.Links.Self.Href == "" {
			log.Error("Mandatory Links.Self.Href parameter not present")
			errHandlerProblemDetails(w, "Mandatory Links.Self.Href parameter not present", http.StatusBadRequest)
			return
		}
		subsIdParamStr := subscriptionID
		selfUrl := strings.Split(userSubBody.Links.Self.Href, "/")
		subsIdStr := selfUrl[len(selfUrl)-1]
		// //Body content not matching parameters
		if subsIdStr != subsIdParamStr {
			log.Error("SubscriptionId in endpoint and in body not matching")
			errHandlerProblemDetails(w, "SubscriptionId in endpoint and in body not matching", http.StatusNotFound)
			return
		}
		userSubBody.Links = &Links{
			Self: &LinkType{
				Href: hostUrl.String() + basePath + "subscriptions/users/" + subsIdStr,
			},
		}

		subsId, err := strconv.Atoi(subsIdStr)
		if err != nil {
			log.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if userSubscriptionMap[subsId] == "" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		_ = rc.JSONSetEntry(baseKey+typeUserSubscription+":"+subsIdStr, ".", convertUserSubscriptionToJson1(&userSubBody))

		deregisterUser(subsIdStr)

		registerUser1(userSubBody.Address, userSubBody.LocationEventCriteria, subsIdStr, &userSubBody)
		var response InlineUserLocationEventSubscription
		response.UserLocationEventSubscription = &userSubBody

		// Marshal response
		jsonResponse, err := json.Marshal(response)
		if err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Write response
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, string(jsonResponse))
		return

	}
}

// handleUserLocationPeriodicSubscriptionPut handles the PUT request for updating periodic-based user subscriptions by subscription ID.
// It decodes the periodic subscription map from the request body into a struct, validates the mandatory parameters,
// and performs the update operation.
//
// Parameters:
//
//	w: http.ResponseWriter - an interface used to manipulate the HTTP response.
//	requestBody: map[string]interface{} - the request body containing the periodic-based subscription information.
//	subscriptionID: string - the ID of the subscription to be updated.
//
// The function first converts the periodic subscription map to a struct and validates the mandatory parameters.
// If the parameters are valid, it updates the subscription entry in the Redis database and deregisters the existing subscription.
// If the update operation encounters an error, it returns a 500 Internal Server Error response.
// Otherwise, it returns a 200 OK response with the updated subscription details.
func handleUserLocationPeriodicSubscriptionPut(w http.ResponseWriter, requestBody map[string]interface{}, subscriptionID string) {

	if periodicSubscription, ok := requestBody["userLocationPeriodicSubscription"]; ok {
		// Convert the periodic subscription to a map
		periodicSubscriptionMap := periodicSubscription.(map[string]interface{})
		// Decode the periodic subscription map into a struct
		if links, ok := periodicSubscriptionMap["_links"]; ok {
			periodicSubscriptionMap["Links"] = links
			delete(periodicSubscriptionMap, "_links")
		}
		var periodicSub UserLocationPeriodicSubscription
		err := mapstructure.Decode(periodicSubscriptionMap, &periodicSub)
		if err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if periodicSub.CallbackReference == "" {
			log.Error("Mandatory CallbackReference parameter not present")
			errHandlerProblemDetails(w, "Mandatory CallbackReference parameter not present", http.StatusBadRequest)
			return
		}
		if periodicSub.Address == "" {
			log.Error("Mandatory Address parameter not present")
			errHandlerProblemDetails(w, "Mandatory Address parameter not present", http.StatusBadRequest)
			return
		}
		if periodicSub.Links == nil || periodicSub.Links.Self == nil || periodicSub.Links.Self.Href == "" {
			log.Error("Mandatory Links.Self.Href parameter not present")
			errHandlerProblemDetails(w, "Mandatory Links.Self.Href parameter not present", http.StatusBadRequest)
			return
		}
		if periodicSub.SubscriptionType != "UserLocationPeriodicSubscription" {
			log.Error("Mandatory SubscriptionType parameter not present or invalid")
			errHandlerProblemDetails(w, "Mandatory SubscriptionType parameter not present or invalid", http.StatusBadRequest)
			return
		}

		subsIdParamStr := subscriptionID
		selfUrl := strings.Split(periodicSub.Links.Self.Href, "/")
		subsIdStr := selfUrl[len(selfUrl)-1]
		// //Body content not matching parameters
		if subsIdStr != subsIdParamStr {
			log.Error("SubscriptionId in endpoint and in body not matching")
			errHandlerProblemDetails(w, "SubscriptionId in endpoint and in body not matching", http.StatusNotFound)
			return
		}
		periodicSub.Links = &Links{
			Self: &LinkType{
				Href: hostUrl.String() + basePath + "subscriptions/users/" + subsIdStr,
			},
		}

		subsId, err := strconv.Atoi(subsIdStr)
		if err != nil {
			log.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if periodicSubscriptionMap1[subsId] == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		_ = rc.JSONSetEntry(baseKey+typePeriodicSubscription+":"+subsIdStr, ".", convertPeriodicSubscriptionToJson1(&periodicSub))

		deregisterPeriodic(subsIdStr)
		registerPeriodic1(&periodicSub, subsIdStr)
		var response InlineUserLocationPeriodicSubscription
		response.UserLocationPeriodicSubscription = &periodicSub

		// Marshal response
		jsonResponse, err := json.Marshal(response)
		if err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Write response
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, string(jsonResponse))
		return

	}
}

// populateUserSubList1 populates the subscription list with user location event subscription information.
//
// This function unmarshals the JSON-encoded user location event subscription information and adds it to the
// subscription list in the provided subscriptions slice, filtering by address if provided.
//
// Parameters:
//   - data ([]byte): JSON-encoded byte slice containing user location event subscription information.
//   - address (string): Address to filter the subscriptions. If empty, no filtering by address is performed.
//   - subscriptions (*[]Subscription): A pointer to a slice of Subscription objects to be populated.
//
// Returns:
//   - error: An error if JSON unmarshalling fails. Returns nil if the user location event subscription information
//     is successfully added to the list or if it does not match the address filter.
//
// The function performs the following steps:
//  1. Unmarshals the JSON-encoded user location event subscription information.
//  2. Filters the subscription by the address if provided.
//  3. Extracts the href from the links if available.
//  4. Creates a Subscription instance with the extracted href and subscription type.
//  5. Adds the Subscription instance to the subscription list.
func populateUserSubList1(data []byte, address string, subscriptions *[]Subscription) error {
	var userInfo UserLocationEventSubscription
	if err := json.Unmarshal(data, &userInfo); err != nil {
		return err
	}

	// Filter subscriptions by address
	if address != "" && userInfo.Address != address {
		return nil
	}

	href := ""
	if userInfo.Links != nil && userInfo.Links.Self != nil {
		href = userInfo.Links.Self.Href
	}
	sub := Subscription{
		Href:             href,
		SubscriptionType: userInfo.SubscriptionType,
	}
	*subscriptions = append(*subscriptions, sub)
	return nil
}

// populateUserSubList populates the subscription list with user location periodic subscription information.
//
// This function unmarshals the JSON-encoded user location periodic subscription information and adds it to the
// subscription list in the provided subscriptions slice, filtering by address if provided.
//
// Parameters:
//   - data ([]byte): JSON-encoded byte slice containing user location periodic subscription information.
//   - address (string): Address to filter the subscriptions. If empty, no filtering by address is performed.
//   - subscriptions (*[]Subscription): A pointer to a slice of Subscription objects to be populated.
//
// Returns:
//   - error: An error if JSON unmarshalling fails. Returns nil if the user location periodic subscription information
//     is successfully added to the list or if it does not match the address filter.
//
// The function performs the following steps:
//  1. Unmarshals the JSON-encoded user location periodic subscription information.
//  2. Filters the subscription by the address if provided.
//  3. Extracts the href from the links if available.
//  4. Creates a Subscription instance with the extracted href and subscription type.
//  5. Adds the Subscription instance to the subscription list.
func populateUserSubList(data []byte, address string, subscriptions *[]Subscription) error {
	var userInfo UserLocationPeriodicSubscription
	if err := json.Unmarshal(data, &userInfo); err != nil {
		return err
	}

	// Filter subscriptions by address
	if address != "" && userInfo.Address != address {
		return nil
	}

	href := ""
	if userInfo.Links != nil && userInfo.Links.Self != nil {
		href = userInfo.Links.Self.Href
	}
	sub := Subscription{
		Href:             href,
		SubscriptionType: userInfo.SubscriptionType,
	}
	*subscriptions = append(*subscriptions, sub)
	return nil
}

// zoneSubListGET handles the GET request for retrieving subscription data based on specified query parameters related to zones.
// It first sets the content type of the response to JSON.
//
// It then retrieves the query parameters from the request URL and validates them.
// If any invalid query parameters are found, it returns a 400 Bad Request response.
//
// Next, it retrieves the subscription list from the database based on the valid query parameters.
// The subscription list is populated by iterating over the database entries and filtering based on the query parameters.
// If an error occurs during database retrieval, it returns a 500 Internal Server Error response.
//
// Finally, it constructs the response containing the subscription list and sends it as a JSON response.
//
// Parameters:
//
//	w: http.ResponseWriter - an interface used to manipulate the HTTP response.
//	r: *http.Request - a pointer to the HTTP request received.
//
// The response contains a JSON representation of the subscription list along with its resource URL.
func zoneSubListGET(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	// Check if the 'subscription_type' and 'zoneId' query parameters exist and get their values
	queryParams := r.URL.Query()
	subscriptionType := queryParams.Get("subscription_type")
	zoneId := queryParams.Get("zoneId")
	if subscriptionType != "" {
		// Validating subscriptionType parameter
		validSubscriptionTypes := map[string]bool{"event": true, "status": true}
		if _, ok := validSubscriptionTypes[strings.ToLower(subscriptionType)]; !ok {
			http.Error(w, "Invalid subscription type. Allowed values are 'event' and 'status'.", http.StatusBadRequest)
			return
		}
	}
	var response InlineNotificationSubscriptionList

	// Create a slice to hold subscriptions
	var subscriptions []Subscription

	var err error
	switch subscriptionType {
	case "event":
		keyName := baseKey + typeZonalSubscription + "*"
		err = rc.ForEachJSONEntry(keyName, func(key string, jsonInfo string, userData interface{}) error {
			return populateZonalTrafficList([]byte(jsonInfo), zoneId, &subscriptions)
		}, nil)
	case "status":
		keyName := baseKey + typeZoneStatusSubscription + "*"
		err = rc.ForEachJSONEntry(keyName, func(key string, jsonInfo string, userData interface{}) error {
			return populateZoneStatusList([]byte(jsonInfo), zoneId, &subscriptions)
		}, nil)
	default:
		var userSubListZoneEvent []Subscription
		keyNameEventZone := baseKey + typeZonalSubscription + "*"
		errEvent := rc.ForEachJSONEntry(keyNameEventZone, func(key string, jsonInfo string, userData interface{}) error {
			return populateZonalTrafficList([]byte(jsonInfo), zoneId, &userSubListZoneEvent)
		}, nil)
		if errEvent != nil {
			log.Error(errEvent.Error())
			errHandlerProblemDetails(w, errEvent.Error(), http.StatusInternalServerError)
			return
		}

		var userSubListStatus []Subscription
		keyNameEventZone = baseKey + typeZoneStatusSubscription + "*"
		errStatus := rc.ForEachJSONEntry(keyNameEventZone, func(key string, jsonInfo string, userData interface{}) error {
			return populateZoneStatusList([]byte(jsonInfo), zoneId, &userSubListStatus)
		}, nil)
		if errStatus != nil {
			log.Error(errStatus.Error())
			errHandlerProblemDetails(w, errStatus.Error(), http.StatusInternalServerError)
			return
		}

		// Append subscriptions from both lists
		subscriptions = append(subscriptions, userSubListZoneEvent...)
		subscriptions = append(subscriptions, userSubListStatus...)

		// No error since we're combining the lists
		err = nil
	}

	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Constructing the response
	response = InlineNotificationSubscriptionList{
		NotificationSubscriptionList: &NotificationSubscriptionList{
			Subscription: subscriptions,
			ResourceURL: &LinkType{
				Href: hostUrl.String() + basePath + "subscriptions/zones",
			},
		},
	}

	// Marshaling the response
	jsonResponse, err := json.Marshal(response)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Writing the response
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(jsonResponse); err != nil {
		log.Error("Can't write response: ", err.Error())
		http.Error(w, fmt.Sprintf("could not write response: %v", err), http.StatusInternalServerError)
	}
}

// zoneSubDELETE handles the DELETE request for deleting zone subscriptions.
//
// It first sets the Content-Type header of the HTTP response to application/json; charset=UTF-8.
// Then, it extracts the subscription ID from the request URL using mux.Vars(r).
// Next, it retrieves the JSON data for both zonal traffic subscription and zone status subscription from Redis
// based on the subscription ID.
//
// If neither zonal traffic subscription nor zone status subscription is found for the given subscription ID,
// it constructs and returns a Not Found response.
//
// If a zonal traffic subscription is found, it deletes the subscription data from Redis, deregisters the subscription,
// sets the HTTP status code to No Content, and returns.
//
// If a zone status subscription is found, it performs similar deletion and deregistration steps as above.
//
// Parameters:
//
//	w: http.ResponseWriter - an interface used to manipulate the HTTP response.
//	r: *http.Request - a pointer to the HTTP request received.
func zoneSubDELETE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	vars := mux.Vars(r)
	jsonZonalTrafficSub, _ := rc.JSONGetEntry(baseKey+typeZonalSubscription+":"+vars["subscriptionId"], ".")
	jsonZoneStatusSub, _ := rc.JSONGetEntry(baseKey+typeZoneStatusSubscription+":"+vars["subscriptionId"], ".")

	if jsonZonalTrafficSub == "" && jsonZoneStatusSub == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	if jsonZonalTrafficSub != "" {
		err := rc.JSONDelEntry(baseKey+typeZonalSubscription+":"+vars["subscriptionId"], ".")
		if err != nil {
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}
		deregisterZonal(vars["subscriptionId"])
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if jsonZoneStatusSub != "" {
		err := rc.JSONDelEntry(baseKey+typeZoneStatusSubscription+":"+vars["subscriptionId"], ".")
		if err != nil {
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}
		deregisterZoneStatus(vars["subscriptionId"])
		w.WriteHeader(http.StatusNoContent)
		return
	}
}

// zoneSubGET handles the GET request for retrieving zone subscriptions by subscription ID.
//
// It first sets the Content-Type header of the HTTP response to application/json.
// Then, it extracts the subscription ID from the request URL parameters.
//
// Next, it retrieves the subscription information from Redis based on the subscription ID.
// If the subscription ID corresponds to a zone location event subscription, it unmarshals the JSON data
// into a ZoneLocationEventSubscription struct and constructs the response accordingly.
// If the subscription ID corresponds to a zone status subscription, it unmarshals the JSON data
// into a ZoneStatusSubscription struct and constructs the response accordingly.
// If no subscription is found for the given ID, it returns a 404 Not Found response.
//
// Finally, it marshals the response into JSON format and writes it to the HTTP response body.
//
// Parameters:
//
//	w: http.ResponseWriter - an interface used to manipulate the HTTP response.
//	r: *http.Request - a pointer to the HTTP request received.
func zoneSubGET(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	vars := mux.Vars(r)
	var response interface{}

	jsonZonalTrafficSub, _ := rc.JSONGetEntry(baseKey+typeZonalSubscription+":"+vars["subscriptionId"], ".")
	if jsonZonalTrafficSub != "" {
		var jsonZoneSub ZoneLocationEventSubscription
		if err := json.Unmarshal([]byte(jsonZonalTrafficSub), &jsonZoneSub); err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}
		response = InlineZoneLocationEventSubscription{ZoneLocationEventSubscription: &jsonZoneSub}
	} else {
		var statusSub ZoneStatusSubscription
		jsonZoneStatusSub, _ := rc.JSONGetEntry(baseKey+typeZoneStatusSubscription+":"+vars["subscriptionId"], ".")
		if jsonZoneStatusSub == "" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if err := json.Unmarshal([]byte(jsonZoneStatusSub), &statusSub); err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}
		response = InlineZoneStatusSubscription{ZoneStatusSubscription: &statusSub}
	}
	jsonResponse, err := json.Marshal(response)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(jsonResponse))

}

// zoneSubPOST handles the POST request for creating zone subscriptions.
// It first decodes the request body into a map[string]interface{}.
//
// Then, it checks if the request body contains either a zone location event subscription or a zone status subscription.
// Based on the type of subscription found, it delegates to the appropriate handler function.
//
// If both types of subscriptions are present in the request body, it returns a 400 Bad Request response.
//
// Parameters:
//
//	w: http.ResponseWriter - an interface used to manipulate the HTTP response.
//	r: *http.Request - a pointer to the HTTP request received.
func zoneSubPOST(w http.ResponseWriter, r *http.Request) {

	var requestBody map[string]interface{}

	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&requestBody); err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, "Invalid request body format", http.StatusBadRequest)
		return
	}
	hasZoneSubscription := false
	hasStatusSubscription := false

	if _, ok := requestBody["zoneLocationEventSubscription"]; ok {
		hasZoneSubscription = true
	} else if _, ok := requestBody["zoneStatusSubscription"]; ok {
		hasStatusSubscription = true
	}

	// Check if both types of subscriptions are present
	if hasZoneSubscription && hasStatusSubscription {
		errHandlerProblemDetails(w, "Please send only one type of subscription at a time", http.StatusBadRequest)
		return
	}
	if hasZoneSubscription {
		handleZoneLocationEventSubscription(w, requestBody)
	} else if hasStatusSubscription {
		handleZoneStatusSubscription(w, requestBody)
	} else {
		errHandlerProblemDetails(w, "No valid subscription found in the request body", http.StatusBadRequest)
		return
	}
}

// handleZoneStatusSubscription handles the creation of zone status subscriptions.
// It expects the request body to contain a zone status subscription.
//
// First, it decodes the zone status subscription map from the request body into a struct.
// Then, it validates the mandatory parameters of the subscription.
//
// If any mandatory parameters are missing or invalid, it returns a 400 Bad Request response.
// Otherwise, it registers the subscription and returns a 201 Created response with the subscription details.
//
// Parameters:
//
//	w: http.ResponseWriter - an interface used to manipulate the HTTP response.
//	requestBody: map[string]interface{} - the request body containing the subscription details.
func handleZoneStatusSubscription(w http.ResponseWriter, requestBody map[string]interface{}) {

	if statusSubscription, ok := requestBody["zoneStatusSubscription"]; ok {
		// Convert the Status subscription to a map
		statusSubscriptionMap := statusSubscription.(map[string]interface{})
		// Decode the Zone subscription map into a struct
		var zoneStatusSub ZoneStatusSubscription
		err := mapstructure.Decode(statusSubscriptionMap, &zoneStatusSub)
		if err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}
		//checking for mandatory properties
		if zoneStatusSub.CallbackReference == "" {
			log.Error("Mandatory CallbackReference parameter not present")
			errHandlerProblemDetails(w, "Mandatory CallbackReference parameter not present", http.StatusBadRequest)
			return
		}
		if zoneStatusSub.ZoneId == "" {
			log.Error("Mandatory ZoneId parameter not present")
			errHandlerProblemDetails(w, "Mandatory ZoneId parameter not present", http.StatusBadRequest)
			return
		}
		if zoneStatusSub.SubscriptionType != "ZoneStatusSubscription" {
			log.Error("Mandatory SubscriptionType parameter not present or invalid")
			errHandlerProblemDetails(w, "Mandatory SubscriptionType parameter not present or invalid", http.StatusBadRequest)
			return
		}
		// Add your logic to register the event-based subscription
		newSubsId := nextZoneStatusSubscriptionIdAvailable
		if newSubsId%2 != 0 {
			newSubsId++ // Ensure newSubsId is even
		}
		nextZoneStatusSubscriptionIdAvailable = newSubsId + 2 // Increment by 2 to ensure the next even number
		subsIdStr := strconv.Itoa(newSubsId)
		location := hostUrl.String() + basePath + "subscriptions/zones/" + subsIdStr
		w.Header().Set("Location", location)
		zoneStatusSub.Links = &Links{
			Self: &LinkType{
				Href: location,
			},
		}
		_ = rc.JSONSetEntry(baseKey+typeZoneStatusSubscription+":"+subsIdStr, ".", convertZoneStatusSubscriptionToJson(&zoneStatusSub))
		registerZoneStatus(zoneStatusSub.ZoneId, zoneStatusSub.UpperNumberOfUsersZoneThreshold, zoneStatusSub.UpperNumberOfUsersAPThreshold,
			zoneStatusSub.OperationStatus, subsIdStr, zoneStatusSub.LowerNumberOfUsersZoneThreshold, zoneStatusSub.LowerNumberOfUsersAPThreshold, &zoneStatusSub)

		// Prepare response
		var response InlineZoneStatusSubscription
		response.ZoneStatusSubscription = &zoneStatusSub
		// Marshal response
		jsonResponse, err := json.Marshal(response)
		if err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Write response
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, string(jsonResponse))
		return
	}
}

// handleZoneLocationEventSubscription handles the creation of zone location event subscriptions.
// It expects the request body to contain a zone location event subscription.
//
// First, it decodes the zone location event subscription map from the request body into a struct.
// Then, it validates the mandatory parameters of the subscription.
//
// If any mandatory parameters are missing or invalid, it returns a 400 Bad Request response.
// Otherwise, it registers the subscription and returns a 201 Created response with the subscription details.
//
// Parameters:
//
//	w: http.ResponseWriter - an interface used to manipulate the HTTP response.
//	requestBody: map[string]interface{} - the request body containing the subscription details.
func handleZoneLocationEventSubscription(w http.ResponseWriter, requestBody map[string]interface{}) {

	if zoneLocationEventSubscription, ok := requestBody["zoneLocationEventSubscription"]; ok {
		zoneLocationEventSubscriptionMap := zoneLocationEventSubscription.(map[string]interface{})
		// Decode the zone subscription map into a struct
		var zonalSub ZoneLocationEventSubscription
		err := mapstructure.Decode(zoneLocationEventSubscriptionMap, &zonalSub)
		if err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}
		//checking for mandatory properties
		if zonalSub.CallbackReference == "" {
			log.Error("Mandatory CallbackReference parameter not present")
			errHandlerProblemDetails(w, "Mandatory CallbackReference parameter not present", http.StatusBadRequest)
			return
		}
		if zonalSub.ZoneId == "" {
			log.Error("Mandatory ZoneId parameter not present")
			errHandlerProblemDetails(w, "Mandatory ZoneId parameter not present", http.StatusBadRequest)
			return
		}
		if zonalSub.SubscriptionType != "ZoneLocationEventSubscription" {
			log.Error("Mandatory SubscriptionType parameter not present or invalid")
			errHandlerProblemDetails(w, "Mandatory SubscriptionType parameter not present or invalid", http.StatusBadRequest)
			return
		}

		// Add your logic to register the periodic-based subscription
		newSubsId := nextZonalSubscriptionIdAvailable
		nextZonalSubscriptionIdAvailable += 2
		subsIdStr := strconv.Itoa(newSubsId)
		location := hostUrl.String() + basePath + "subscriptions/zones/" + subsIdStr
		w.Header().Set("Location", location)
		zonalSub.Links = &Links{
			Self: &LinkType{
				Href: location,
			},
		}
		_ = rc.JSONSetEntry(baseKey+typeZonalSubscription+":"+subsIdStr, ".", convertZonalSubscriptionToJson1(&zonalSub))
		registerZonal1(zonalSub.ZoneId, zonalSub.LocationEventCriteria, subsIdStr, &zonalSub)
		var response InlineZoneLocationEventSubscription
		response.ZoneLocationEventSubscription = &zonalSub
		// Marshal response
		jsonResponse, err := json.Marshal(response)
		if err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Write response
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, string(jsonResponse))
		return
	}
}

// populateZonalTrafficList populates the subscription list with zonal traffic subscription information.
//
// This function unmarshals the JSON-encoded zonal traffic subscription information and adds it to the
// subscription list in the provided subscriptions slice, filtering by zone ID if provided.
//
// Parameters:
//   - data ([]byte): JSON-encoded byte slice containing zonal traffic subscription information.
//   - zoneId (string): Zone ID to filter the subscriptions. If empty, no filtering by zone ID is performed.
//   - subscriptions (*[]Subscription): A pointer to a slice of Subscription objects to be populated.
//
// Returns:
//   - error: An error if JSON unmarshalling fails. Returns nil if the zonal traffic subscription information
//     is successfully added to the list or if it does not match the zone ID filter.
//
// The function performs the following steps:
//  1. Unmarshals the JSON-encoded zonal traffic subscription information.
//  2. Filters the subscription by the zone ID if provided.
//  3. Extracts the href from the links if available.
//  4. Creates a Subscription instance with the extracted href and subscription type.
//  5. Adds the Subscription instance to the subscription list.
func populateZonalTrafficList(data []byte, zoneId string, subscriptions *[]Subscription) error {
	var zoneInfo ZoneLocationEventSubscription
	if err := json.Unmarshal(data, &zoneInfo); err != nil {
		return err
	}

	// Filter subscriptions by zoneId
	if zoneId != "" && zoneInfo.ZoneId != zoneId {
		return nil
	}

	href := ""
	if zoneInfo.Links != nil && zoneInfo.Links.Self != nil {
		href = zoneInfo.Links.Self.Href
	}
	sub := Subscription{
		Href:             href,
		SubscriptionType: zoneInfo.SubscriptionType,
	}
	*subscriptions = append(*subscriptions, sub)
	return nil
}

// populateZoneStatusList populates the subscription list with zone status subscription information.
//
// This function unmarshals the JSON-encoded zone status subscription information and adds it to the
// subscription list in the provided subscriptions slice, filtering by zone ID if provided.
//
// Parameters:
//   - data ([]byte): JSON-encoded byte slice containing zone status subscription information.
//   - zoneId (string): Zone ID to filter the subscriptions. If empty, no filtering by zone ID is performed.
//   - subscriptions (*[]Subscription): A pointer to a slice of Subscription objects to be populated.
//
// Returns:
//   - error: An error if JSON unmarshalling fails. Returns nil if the zone status subscription information
//     is successfully added to the list or if it does not match the zone ID filter.
//
// The function performs the following steps:
//  1. Unmarshals the JSON-encoded zone status subscription information.
//  2. Filters the subscription by the zone ID if provided.
//  3. Extracts the href from the links if available.
//  4. Creates a Subscription instance with the extracted href and subscription type.
//  5. Adds the Subscription instance to the subscription list.
func populateZoneStatusList(data []byte, zoneId string, subscriptions *[]Subscription) error {
	var zoneInfo ZoneStatusSubscription
	if err := json.Unmarshal(data, &zoneInfo); err != nil {
		return err
	}

	// Filter subscriptions by zoneId
	if zoneId != "" && zoneInfo.ZoneId != zoneId {
		return nil
	}

	href := ""
	if zoneInfo.Links != nil && zoneInfo.Links.Self != nil {
		href = zoneInfo.Links.Self.Href
	}
	sub := Subscription{
		Href:             href,
		SubscriptionType: zoneInfo.SubscriptionType,
	}
	*subscriptions = append(*subscriptions, sub)
	return nil
}

// cleanUp terminates all ongoing operations and clears the database and subscription-related maps.
//
// This function logs a termination message, flushes the Redis database, and resets global variables
// related to subscription IDs and maps. It also resets the connected address map and updates the
// store name to an empty string.
//
// Note: This function should be used with caution as it clears all data and resets the system state.
func cleanUp() {
	log.Info("Terminate all")
	rc.DBFlush(baseKey)
	nextZonalSubscriptionIdAvailable = 1
	nextUserSubscriptionIdAvailable = 1
	nextZoneStatusSubscriptionIdAvailable = 1
	nextDistanceSubscriptionIdAvailable = 1
	nextAreaCircleSubscriptionIdAvailable = 1
	nextPeriodicSubscriptionIdAvailable = 1

	mutex.Lock()
	defer mutex.Unlock()
	zonalSubscriptionEnteringMap = map[int]string{}
	zonalSubscriptionLeavingMap = map[int]string{}
	zonalSubscriptionTransferringMap = map[int]string{}
	zonalSubscriptionMap = map[int]string{}

	userSubscriptionEnteringMap = map[int]string{}
	userSubscriptionLeavingMap = map[int]string{}
	userSubscriptionTransferringMap = map[int]string{}
	userSubscriptionMap = map[int]string{}

	zoneStatusSubscriptionMap = map[int]*ZoneStatusCheck{}
	distanceSubscriptionMap1 = map[int]*DistanceCheck_{}
	areaCircleSubscriptionMap = map[int]*AreaCircleCheck{}
	periodicSubscriptionMap1 = map[int]*PeriodicCheck1{}

	addressConnectedMap = map[string]bool{}

	updateStoreName("")
}

// updateStoreName updates the current store name used for logging and monitoring.
//
// This function updates the current store name used for logging and monitoring. If the provided store name
// is different from the current store name, it updates the current store name and reinitializes the logging
// component with the new store name.
//
// Parameters:
//   - storeName (string): The new store name to be used for logging and monitoring.
func updateStoreName(storeName string) {
	if currentStoreName != storeName {
		currentStoreName = storeName

		logComponent := moduleName
		if mepName != defaultLocationName {
			logComponent = moduleName + "-" + mepName
		}
		_ = httpLog.ReInit(logComponent, sandboxName, storeName, redisAddr, influxAddr)
	}
}

// updateUserInfo updates the user information in the database and triggers relevant notifications.
//
// This function updates the user information in the database with the provided address, zone ID, access point ID,
// location details, and other optional information. It also triggers notifications based on the changes in
// zone or access point for the user.
//
// Parameters:
//   - address (string): The address of the user.
//   - zoneId (string): The new zone ID of the user.
//   - accessPointId (string): The new access point ID of the user.
//   - longitude (*float32): The longitude of the user's location. If nil, the user's location info is removed.
//   - latitude (*float32): The latitude of the user's location. If nil, the user's location info is removed.
//   - country (*string): The country of the user's location. Optional.
//   - mapid (*string): The map ID of the user's location. Optional.
//   - x (*float32): The x-coordinate of the user's location in the map. Optional.
//   - y (*float32): The y-coordinate of the user's location in the map. Optional.
//   - originLatitude (*float32): The latitude of the origin point of the map. Optional.
//   - originLongitude (*float32): The longitude of the origin point of the map. Optional.
//
// The function performs the following steps:
//  1. Retrieves the current user information from the database based on the provided address.
//  2. Updates the user information with the new zone ID, access point ID, and location details.
//  3. Determines if the user is connected based on the provided access point ID.
//  4. Updates the user's timeStamp to the current time.
//  5. Updates the user's relative location information if map ID is provided.
//  6. Updates the user's civic address information if country is provided.
//  7. Updates the user's location information if longitude and latitude are provided.
//  8. Updates the user information in the database.
//  9. Checks and triggers notifications for zone and access point changes.
//  10. Checks and triggers notifications for area circle subscriptions.
func updateUserInfo(address string, zoneId string, accessPointId string, longitude *float32, latitude *float32, country *string, mapid *string, x *float32, y *float32, originLatitude *float32, originLongitude *float32) {
	var oldZoneId string
	var oldApId string

	// Get User Info from DB
	jsonUserInfo, _ := rc.JSONGetEntry(baseKey+typeUser+":"+address, ".")
	userInfo := convertJsonToUserInfo(jsonUserInfo)

	// Create new user info if necessary
	if userInfo == nil {
		userInfo = new(UserInfo)
		userInfo.Address = address
		userInfo.ResourceURL = hostUrl.String() + basePath + "queries/users?address=" + address
	} else {
		// Get old zone & AP IDs
		oldZoneId = userInfo.ZoneId
		oldApId = userInfo.AccessPointId
	}
	userInfo.ZoneId = zoneId
	userInfo.AccessPointId = accessPointId

	//dtermine if ue is connected or not based on POA connectivity
	if accessPointId != "" {
		addressConnectedMap[address] = true
	} else {
		addressConnectedMap[address] = false
	}

	seconds := time.Now().Unix()
	var timeStamp TimeStamp
	timeStamp.Seconds = int32(seconds)

	userInfo.Timestamp = &timeStamp
	if mapid != nil {
		if userInfo.RelativeLocationInfo == nil {
			userInfo.RelativeLocationInfo = new(RelativeLocationInfo)
		}
		userInfo.RelativeLocationInfo.X = *x
		userInfo.RelativeLocationInfo.Y = *y
		if userInfo.RelativeLocationInfo.MapInfo == nil {
			userInfo.RelativeLocationInfo.MapInfo = new(MapInfo)
		}
		userInfo.RelativeLocationInfo.MapInfo.MapId = *mapid

		if userInfo.RelativeLocationInfo.MapInfo.Origin == nil {
			userInfo.RelativeLocationInfo.MapInfo.Origin = new(Origin)
			userInfo.RelativeLocationInfo.MapInfo.Origin.Latitude = *originLatitude
			userInfo.RelativeLocationInfo.MapInfo.Origin.Longitude = *originLongitude
		}
	}
	if country != nil {
		if userInfo.CivicInfo == nil {
			userInfo.CivicInfo = new(CivicAddress)
		}
		userInfo.CivicInfo.Country = *country
	}
	// Update position
	if longitude == nil || latitude == nil {
		userInfo.LocationInfo = nil
	} else {
		if userInfo.LocationInfo == nil {
			userInfo.LocationInfo = new(LocationInfo)
		}
		//we only support shape == 2 in locationInfo, so we ignore any conditional parameters based on shape
		userInfo.LocationInfo.Shape = 2
		userInfo.LocationInfo.Longitude = nil
		userInfo.LocationInfo.Longitude = append(userInfo.LocationInfo.Longitude, *longitude)
		userInfo.LocationInfo.Latitude = nil
		userInfo.LocationInfo.Latitude = append(userInfo.LocationInfo.Latitude, *latitude)

	}

	// Update User info in DB & Send notifications
	_ = rc.JSONSetEntry(baseKey+typeUser+":"+address, ".", convertUserInfoToJson(userInfo))
	checkNotificationRegisteredUsers(oldZoneId, zoneId, oldApId, accessPointId, address)
	checkNotificationRegisteredZones1(oldZoneId, zoneId, oldApId, accessPointId, address)
	// checkNotificationRegisteredZones(oldZoneId, zoneId, oldApId, accessPointId, address)
	checkNotificationAreaCircle(address)
}

// updateZoneInfo updates the zone information in the database and triggers relevant notifications.
//
// This function updates the zone information in the database with the provided zone ID, number of access points,
// number of unserviceable access points, and number of users. It also triggers notifications based on the changes
// in the zone's number of users.
//
// Parameters:
//   - zoneId (string): The zone ID of the zone to update.
//   - nbAccessPoints (int): The new number of access points in the zone. Set to -1 to keep the current value.
//   - nbUnsrvAccessPoints (int): The new number of unserviceable access points in the zone. Set to -1 to keep the current value.
//   - nbUsers (int): The new number of users in the zone. Set to -1 to keep the current value.
//
// The function performs the following steps:
//  1. Retrieves the current zone information from the database based on the provided zone ID.
//  2. Updates the zone information with the new number of access points, number of unserviceable access points,
//     and number of users.
//  3. Updates the zone information in the database.
//  4. Checks and triggers notifications for zone status changes based on the new number of users.
func updateZoneInfo(zoneId string, nbAccessPoints int, nbUnsrvAccessPoints int, nbUsers int) {
	// Get Zone Info from DB
	jsonZoneInfo, _ := rc.JSONGetEntry(baseKey+typeZone+":"+zoneId, ".")
	zoneInfo := convertJsonToZoneInfo(jsonZoneInfo)

	// Create new zone info if necessary
	if zoneInfo == nil {
		zoneInfo = new(ZoneInfo)
		zoneInfo.ZoneId = zoneId
		zoneInfo.ResourceURL = hostUrl.String() + basePath + "queries/zones/" + zoneId
	}

	previousNbUsers := zoneInfo.NumberOfUsers

	// Update info
	if nbAccessPoints != -1 {
		zoneInfo.NumberOfAccessPoints = int32(nbAccessPoints)
	}
	if nbUnsrvAccessPoints != -1 {
		zoneInfo.NumberOfUnserviceableAccessPoints = int32(nbUnsrvAccessPoints)
	}
	if nbUsers != -1 {
		zoneInfo.NumberOfUsers = int32(nbUsers)
	}

	// Update Zone info in DB & Send notifications
	_ = rc.JSONSetEntry(baseKey+typeZone+":"+zoneId, ".", convertZoneInfoToJson(zoneInfo))
	// checkNotificationRegisteredZoneStatus(zoneId, "", int32(-1), int32(nbUsers), int32(-1), previousNbUsers)
	checkNotificationRegisteredZoneStatus1(zoneId, "", int32(-1), int32(nbUsers), int32(-1), previousNbUsers)
}

// updateAccessPointInfo updates the access point information in the database and triggers relevant notifications.
//
// This function updates the access point information in the database with the provided zone ID, access point ID,
// connection type, operation status, number of users, longitude, and latitude. It also triggers notifications based
// on the changes in the access point's operation status or number of users.
//
// Parameters:
//   - zoneId (string): The zone ID of the access point's zone.
//   - apId (string): The access point ID of the access point.
//   - conTypeStr (string): The connection type of the access point. Use empty string to keep the current value.
//   - opStatusStr (string): The operation status of the access point. Use empty string to keep the current value.
//   - nbUsers (int): The new number of users connected to the access point. Set to -1 to keep the current value.
//   - longitude (*float32): The longitude of the access point's location. Set to nil to remove the location info.
//   - latitude (*float32): The latitude of the access point's location. Set to nil to remove the location info.
//
// The function performs the following steps:
//  1. Retrieves the current access point information from the database based on the provided zone ID and access point ID.
//  2. Updates the access point information with the new connection type, operation status, and number of users.
//  3. Updates the access point's location information if longitude and latitude are provided.
//  4. Updates the access point information in the database.
//  5. Checks and triggers notifications for access point status changes based on the new operation status or number of users.
func updateAccessPointInfo(zoneId string, apId string, conTypeStr string, opStatusStr string, nbUsers int, longitude *float32, latitude *float32) {
	// Get AP Info from DB
	jsonApInfo, _ := rc.JSONGetEntry(baseKey+typeZone+":"+zoneId+":"+typeAccessPoint+":"+apId, ".")
	apInfo := convertJsonToAccessPointInfo(jsonApInfo)

	// Create new AP info if necessary
	if apInfo == nil {
		apInfo = new(AccessPointInfo)
		apInfo.AccessPointId = apId
		apInfo.ResourceURL = hostUrl.String() + basePath + "queries/zones/" + zoneId + "/accessPoints/" + apId
	}

	previousNbUsers := apInfo.NumberOfUsers

	// Update info
	if opStatusStr != "" {
		opStatus := convertStringToOperationStatus(opStatusStr)
		apInfo.OperationStatus = &opStatus
	}
	if conTypeStr != "" {
		conType := convertStringToConnectionType(conTypeStr)
		apInfo.ConnectionType = &conType
	}
	if nbUsers != -1 {
		apInfo.NumberOfUsers = int32(nbUsers)
	}

	// Update position
	if longitude == nil || latitude == nil {
		apInfo.LocationInfo = nil
	} else {
		if apInfo.LocationInfo == nil {
			apInfo.LocationInfo = new(LocationInfo)
		}

		//we only support shape != 7 in locationInfo
		//Accuracy supported for shape 4, 5, 6 only, so ignoring it in our case (only support shape == 2)
		//apInfo.LocationInfo.Accuracy = 1
		apInfo.LocationInfo.Shape = 2
		apInfo.LocationInfo.Longitude = nil
		apInfo.LocationInfo.Longitude = append(apInfo.LocationInfo.Longitude, *longitude)
		apInfo.LocationInfo.Latitude = nil
		apInfo.LocationInfo.Latitude = append(apInfo.LocationInfo.Latitude, *latitude)

		seconds := time.Now().Unix()
		var timeStamp TimeStamp
		timeStamp.Seconds = int32(seconds)

	}

	// Update AP info in DB & Send notifications
	_ = rc.JSONSetEntry(baseKey+typeZone+":"+zoneId+":"+typeAccessPoint+":"+apId, ".", convertAccessPointInfoToJson(apInfo))
	// checkNotificationRegisteredZoneStatus(zoneId, apId, int32(nbUsers), int32(-1), previousNbUsers, int32(-1))
	checkNotificationRegisteredZoneStatus1(zoneId, apId, int32(nbUsers), int32(-1), previousNbUsers, int32(-1))
}

// zoneStatusReInit reinitializes the zone status subscriptions.
//
// This function reinitializes the zone status subscriptions by fetching all existing zone status subscriptions
// from the database and populating the zone status subscription map. It also updates the next available zone status
// subscription ID.
//
// The function performs the following steps:
//  1. Retrieves all existing zone status subscriptions from the database.
//  2. Populates the zone status subscription map with the retrieved subscriptions.
//  3. Updates the next available zone status subscription ID based on the maximum subscription ID found.
func zoneStatusReInit() {
	//reusing the object response for the get multiple zoneStatusSubscription
	var zoneList NotificationSubscriptionList

	keyName := baseKey + typeZoneStatusSubscription + "*"
	_ = rc.ForEachJSONEntry(keyName, func(key string, jsonInfo string, userData interface{}) error {
		return populateZoneStatusList([]byte(jsonInfo), "", userData.(*[]Subscription))
	}, &zoneList.ZoneStatusSubscription)

	maxZoneStatusSubscriptionId := 0
	mutex.Lock()
	defer mutex.Unlock()
	for _, zone := range zoneList.ZoneStatusSubscription {
		resourceUrl := strings.Split(zone.Links.Self.Href, "/")
		subscriptionId, err := strconv.Atoi(resourceUrl[len(resourceUrl)-1])
		if err != nil {
			log.Error(err)
		} else {
			if subscriptionId > maxZoneStatusSubscriptionId {
				maxZoneStatusSubscriptionId = subscriptionId
			}

			var zoneStatus ZoneStatusCheck
			opStatus := zone.OperationStatus
			if opStatus != nil {
				for i := 0; i < len(opStatus); i++ {
					switch opStatus[i] {
					case SERVICEABLE:
						zoneStatus.Serviceable = true
					case UNSERVICEABLE:
						zoneStatus.Unserviceable = true
					case UNKNOWN:
						zoneStatus.Unknown = true
					default:
					}
				}
			}
			zoneStatus.lowerNumberOfUsersAPThreshold = zone.LowerNumberOfUsersAPThreshold
			zoneStatus.lowerNumberOfUsersZoneThreshold = zone.LowerNumberOfUsersZoneThreshold
			zoneStatus.upperNumberOfUsersAPThreshold = zone.UpperNumberOfUsersAPThreshold
			zoneStatus.upperNumberOfUsersZoneThreshold = zone.UpperNumberOfUsersZoneThreshold

			zoneStatus.ZoneId = zone.ZoneId
			zoneStatusSubscriptionMap[subscriptionId] = &zoneStatus
		}
	}
	nextZoneStatusSubscriptionIdAvailable = maxZoneStatusSubscriptionId + 1
}

// zoneLocationEventReInit reinitializes the zone location event subscriptions.
//
// This function reinitializes the zone location event subscriptions by fetching all existing zonal subscriptions
// from the database and populating the zonal subscription maps. It also updates the next available zonal subscription
// ID.
//
// The function performs the following steps:
//  1. Retrieves all existing zonal subscriptions from the database.
//  2. Populates the zonal subscription maps with the retrieved subscriptions.
//  3. Updates the next available zonal subscription ID based on the maximum subscription ID found.
func zoneLocationEventReInit() {
	//reusing the object response for the get multiple zonalSubscription
	var zoneList NotificationSubscriptionList

	keyName := baseKey + typeZonalSubscription + "*"
	_ = rc.ForEachJSONEntry(keyName, func(key string, jsonInfo string, userData interface{}) error {
		return populateZonalTrafficList([]byte(jsonInfo), "", userData.(*[]Subscription))
	}, &zoneList.ZoneLocationEventSubscription)

	maxZonalSubscriptionId := 0
	mutex.Lock()
	defer mutex.Unlock()
	for _, zone := range zoneList.ZoneLocationEventSubscription {
		resourceUrl := strings.Split(zone.Links.Self.Href, "/")
		subscriptionId, err := strconv.Atoi(resourceUrl[len(resourceUrl)-1])
		if err != nil {
			log.Error(err)
		} else {
			if subscriptionId > maxZonalSubscriptionId {
				maxZonalSubscriptionId = subscriptionId
			}

			for i := 0; i < len(zone.LocationEventCriteria); i++ {
				switch zone.LocationEventCriteria[i] {
				case ENTERING_AREA_EVENT:
					zonalSubscriptionEnteringMap[subscriptionId] = zone.ZoneId
				case LEAVING_AREA_EVENT:
					zonalSubscriptionLeavingMap[subscriptionId] = zone.ZoneId
				default:
				}
			}
			zonalSubscriptionMap[subscriptionId] = zone.ZoneId
		}
	}
	nextZonalSubscriptionIdAvailable = maxZonalSubscriptionId + 1
}

// userLocationEventReInit reinitializes the user location event subscriptions.
//
// This function reinitializes the user location event subscriptions by fetching all existing user subscriptions
// from the database and populating the user subscription maps. It also updates the next available user subscription
// ID.
//
// The function performs the following steps:
//  1. Retrieves all existing user subscriptions from the database.
//  2. Populates the user subscription maps with the retrieved subscriptions.
//  3. Updates the next available user subscription ID based on the maximum subscription ID found.
func userLocationEventReInit() {
	//reusing the object response for the get multiple zonalSubscription
	var userList NotificationSubscriptionList

	keyName := baseKey + typeUserSubscription + "*"
	_ = rc.ForEachJSONEntry(keyName, func(key string, jsonInfo string, userData interface{}) error {
		return populateUserSubList([]byte(jsonInfo), "", userData.(*[]Subscription))
	}, &userList)

	maxUserSubscriptionId := 0
	mutex.Lock()
	defer mutex.Unlock()

	for _, user := range userList.UserLocationEventSubscription {
		resourceUrl := strings.Split(user.Links.Self.Href, "/")
		subscriptionId, err := strconv.Atoi(resourceUrl[len(resourceUrl)-1])
		if err != nil {
			log.Error(err)
		} else {
			if subscriptionId > maxUserSubscriptionId {
				maxUserSubscriptionId = subscriptionId
			}

			for i := 0; i < len(user.LocationEventCriteria); i++ {
				switch user.LocationEventCriteria[i] {
				case ENTERING_AREA_EVENT:
					userSubscriptionEnteringMap[subscriptionId] = user.Address
				case LEAVING_AREA_EVENT:
					userSubscriptionLeavingMap[subscriptionId] = user.Address
				default:
				}
			}
			userSubscriptionMap[subscriptionId] = user.Address
		}
	}
	nextUserSubscriptionIdAvailable = maxUserSubscriptionId + 1
}

// distanceReInit reinitializes the distance subscriptions.
//
// This function reinitializes the distance subscriptions by fetching all existing distance subscriptions
// from the database and populating the distance subscription map. It also updates the next available distance
// subscription ID.
//
// The function performs the following steps:
//  1. Retrieves all existing distance subscriptions from the database.
//  2. Populates the distance subscription map with the retrieved subscriptions.
//  3. Updates the next available distance subscription ID based on the maximum subscription ID found.
func distanceReInit() {
	//reusing the object response for the get multiple zonalSubscription
	var distanceList NotificationSubscriptionList

	keyName := baseKey + typeDistanceSubscription + "*"
	_ = rc.ForEachJSONEntry(keyName, populateDistanceList, &distanceList)

	maxDistanceSubscriptionId := 0
	mutex.Lock()
	defer mutex.Unlock()

	for _, distanceSub := range distanceList.UserDistanceSubscription {
		resourceUrl := strings.Split(distanceSub.Links.Self.Href, "/")
		subscriptionId, err := strconv.Atoi(resourceUrl[len(resourceUrl)-1])
		if err != nil {
			log.Error(err)
		} else {
			if subscriptionId > maxDistanceSubscriptionId {
				maxDistanceSubscriptionId = subscriptionId
			}
			var distanceCheck DistanceCheck_
			distanceCheck.Subscription = &distanceSub
			distanceCheck.NbNotificationsSent = 0
			if distanceSub.CheckImmediate {
				distanceCheck.NextTts = 0 //next time periodic trigger hits, will be forced to trigger
			} else {
				distanceCheck.NextTts = distanceSub.ReportingCtrl.MaximumFrequency
			}
			distanceSubscriptionMap1[subscriptionId] = &distanceCheck
		}
	}
	nextDistanceSubscriptionIdAvailable = maxDistanceSubscriptionId + 1
}

// areaCircleReInit reinitializes the area circle subscriptions.
//
// This function reinitializes the area circle subscriptions by fetching all existing subscriptions
// from the database and populating the area circle subscription map. It also updates the next available
// area circle subscription ID.
//
// The function performs the following steps:
//  1. Retrieves all existing area circle subscriptions from the database.
//  2. Populates the area circle subscription map with the retrieved subscriptions.
//  3. Updates the next available area circle subscription ID based on the maximum subscription ID found.
func areaCircleReInit() {
	//reusing the object response for the get multiple zonalSubscription
	var areaCircleList NotificationSubscriptionList

	keyName := baseKey + typeAreaCircleSubscription + "*"
	_ = rc.ForEachJSONEntry(keyName, populateUserAreaList, &areaCircleList)

	maxAreaCircleSubscriptionId := 0
	mutex.Lock()
	defer mutex.Unlock()
	for _, areaCircleSub := range areaCircleList.UserAreaSubscription {
		resourceUrl := strings.Split(areaCircleSub.Links.Self.Href, "/")
		subscriptionId, err := strconv.Atoi(resourceUrl[len(resourceUrl)-1])
		if err != nil {
			log.Error(err)
		} else {
			if subscriptionId > maxAreaCircleSubscriptionId {
				maxAreaCircleSubscriptionId = subscriptionId
			}
			var areaCircleCheck AreaCircleCheck
			areaCircleCheck.Subscription = &areaCircleSub
			areaCircleCheck.NbNotificationsSent = 0
			areaCircleCheck.AddrInArea = map[string]bool{}
			if areaCircleSub.RequestTestNotification {
				areaCircleCheck.NextTts = 0 //next time periodic trigger hits, will be forced to trigger
			} else {
				areaCircleCheck.NextTts = areaCircleSub.ReportingCtrl.MaximumFrequency
			}
			areaCircleSubscriptionMap[subscriptionId] = &areaCircleCheck
		}
	}
	nextAreaCircleSubscriptionIdAvailable = maxAreaCircleSubscriptionId + 1
}

// userLocationPeriodicReInit reinitializes the user location periodic subscriptions.
//
// This function reinitializes the user location periodic subscriptions by fetching all existing subscriptions
// from the database and populating the user location periodic subscription map. It also updates the next available
// user location periodic subscription ID.
//
// The function performs the following steps:
//  1. Retrieves all existing user location periodic subscriptions from the database.
//  2. Populates the user location periodic subscription map with the retrieved subscriptions.
//  3. Updates the next available user location periodic subscription ID based on the maximum subscription ID found.
func userLocationPeriodicReInit() {
	//reusing the object response for the get multiple zonalSubscription
	var periodicList NotificationSubscriptionList

	keyName := baseKey + typePeriodicSubscription + "*"
	_ = rc.ForEachJSONEntry(keyName, func(key string, jsonInfo string, userData interface{}) error {
		return populateUserSubList1([]byte(jsonInfo), "", userData.(*[]Subscription))
	}, &periodicList)

	maxPeriodicSubscriptionId := 0
	mutex.Lock()
	defer mutex.Unlock()
	for _, periodicSub := range periodicList.UserLocationPeriodicSubscription {
		resourceUrl := strings.Split(periodicSub.Links.Self.Href, "/")
		subscriptionId, err := strconv.Atoi(resourceUrl[len(resourceUrl)-1])
		if err != nil {
			log.Error(err)
		} else {
			if subscriptionId > maxPeriodicSubscriptionId {
				maxPeriodicSubscriptionId = subscriptionId
			}
			var periodicCheck PeriodicCheck1
			periodicCheck.Subscription = &periodicSub
			// periodicCheck.NextTts = periodicSub.Frequency
			periodicSubscriptionMap1[subscriptionId] = &periodicCheck

		}
	}
	nextPeriodicSubscriptionIdAvailable = maxPeriodicSubscriptionId + 1
}

// distanceGet calculates the distance between two geographical points or addresses and returns the result as a JSON response.
//
// Parameters:
//
//	w: http.ResponseWriter - an interface used to manipulate the HTTP response.
//	r: *http.Request - a pointer to the HTTP request received.
//
// The function first sets the content type of the response to "application/json; charset=UTF-8".
//
// It retrieves query parameters from the request's URL and performs validation checks on them:
// - At least one 'address' parameter is required, and there can be a maximum of two 'address' parameters.
// - If two 'address' parameters are provided, 'latitude' and 'longitude' parameters cannot be present.
// - If 'latitude' or 'longitude' parameters are present, both must be provided.
// - If only one 'address' parameter is provided, at least one of 'latitude' or 'longitude' parameters must also be provided.
// - It rejects any invalid query parameters.
//
// Next, it verifies the validity of the provided addresses using the addressConnectedMap.
//
// It constructs a gisClient.TargetPoint struct with the destination address and optionally latitude and longitude parameters.
// It then calls the gisAppClient.GeospatialDataApi.GetDistanceGeoDataByName method to get the distance between the source address and the target point.
//
// If there's an error during communication with the GIS engine API, it logs the error and returns an appropriate HTTP status code.
//
// Finally, it constructs the response InlineTerminalDistance containing the calculated distance and timeStamp,
// marshals it into JSON format, sets the HTTP status code to 200 OK, and writes the JSON response to the response writer.
// If there's an error during JSON marshaling, it logs the error and returns a 500 Internal Server Error status code.
func distanceGet(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	// Retrieve query parameters
	u, _ := url.Parse(r.URL.String())
	log.Info("url: ", u.RequestURI())
	q := u.Query()
	//requester := q.Get("requester")
	latitudeStr := q.Get("latitude")
	longitudeStr := q.Get("longitude")
	address := q["address"]

	if len(address) == 0 {
		log.Error("Query should have at least 1 'address' parameter")
		errHandlerProblemDetails(w, "Query should have at least 1 'address' parameter", http.StatusBadRequest)
		return
	}
	if len(address) > 2 {
		log.Error("Query cannot have more than 2 'address' parameters")
		errHandlerProblemDetails(w, "Query cannot have more than 2 'address' parameters", http.StatusBadRequest)
		return
	}
	if len(address) == 2 && (latitudeStr != "" || longitudeStr != "") {
		log.Error("Query cannot have 2 'address' parameters and 'latitude'/'longitude' parameters")
		errHandlerProblemDetails(w, "Query cannot have 2 'address' parameters and 'latitude'/'longitude' parameters", http.StatusBadRequest)
		return
	}
	if (latitudeStr != "" && longitudeStr == "") || (latitudeStr == "" && longitudeStr != "") {
		log.Error("Query must provide a latitude and a longitude for a point to be valid")
		errHandlerProblemDetails(w, "Query must provide a latitude and a longitude for a point to be valid", http.StatusBadRequest)
		return
	}
	if len(address) == 1 && latitudeStr == "" && longitudeStr == "" {
		log.Error("Query must provide either 2 'address' parameters or 1 'address' parameter and 'latitude'/'longitude' parameters")
		errHandlerProblemDetails(w, "Query must provide either 2 'address' parameters or 1 'address' parameter and 'latitude'/'longitude' parameters", http.StatusBadRequest)
		return
	}

	validQueryParams := []string{"requester", "address", "latitude", "longitude"}

	//look for all query parameters to reject if any invalid ones
	found := false
	for queryParam := range q {
		found = false
		for _, validQueryParam := range validQueryParams {
			if queryParam == validQueryParam {
				found = true
				break
			}
		}
		if !found {
			log.Error("Query param not valid: ", queryParam)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}

	srcAddress := address[0]
	dstAddress := ""
	if len(address) > 1 {
		dstAddress = address[1]
	}

	// Verify address validity
	if !addressConnectedMap[srcAddress] || (dstAddress != "" && !addressConnectedMap[dstAddress]) {
		log.Error("Invalid address")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var distParam gisClient.TargetPoint
	distParam.AssetName = dstAddress

	if longitudeStr != "" {
		longitude, err := strconv.ParseFloat(longitudeStr, 32)
		if err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}
		distParam.Longitude = float32(longitude)
	}

	if latitudeStr != "" {
		latitude, err := strconv.ParseFloat(latitudeStr, 32)
		if err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}
		distParam.Latitude = float32(latitude)
	}
	distResp, _, err := gisAppClient.GeospatialDataApi.GetDistanceGeoDataByName(context.TODO(), srcAddress, distParam)
	if err != nil {
		errCodeStr := strings.Split(err.Error(), " ")
		if len(errCodeStr) > 0 {
			errCode, errStr := strconv.Atoi(errCodeStr[0])
			if errStr == nil {
				log.Error("Error code from gis-engine API : ", err)
				errHandlerProblemDetails(w, err.Error(), errCode)
			} else {
				log.Error("Failed to communicate with gis engine: ", err)
				errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			}
		} else {
			log.Error("Failed to communicate with gis engine: ", err)
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	var response InlineTerminalDistance
	var terminalDistance TerminalDistance
	terminalDistance.Distance = int32(distResp.Distance)

	seconds := time.Now().Unix()
	var timeStamp TimeStamp
	timeStamp.Seconds = int32(seconds)
	terminalDistance.Timestamp = &timeStamp

	response.TerminalDistance = &terminalDistance

	// Send response
	jsonResponse, err := json.Marshal(response)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(jsonResponse))
}

// mec011AppTerminationPost handles the POST request for app termination notifications.
//
// This function is responsible for processing app termination notifications received from MEC011-compliant applications.
// It decodes the notification from the request body, deregisters the service associated with the notification,
// deletes subscriptions related to app termination, and confirms app termination if necessary.
//
// The function performs the following steps:
//  1. Decodes the app termination notification from the request body.
//  2. If the app enablement is disabled, it ignores the notification and returns a status code 204 (No Content).
//  3. If the app enablement is enabled, it:
//     - Deregisters the service associated with the notification.
//     - Deletes subscriptions related to app termination.
//     - Confirms app termination if specified by the configuration.
//  4. Responds with a status code 204 (No Content).
func mec011AppTerminationPost(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	var notification AppTerminationNotification
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&notification)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if !appEnablementEnabled {
		//just ignore the message
		w.WriteHeader(http.StatusNoContent)
		return
	}

	//using a go routine to quickly send the response to the requestor
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

// errHandlerProblemDetails writes a JSON response with a ProblemDetails object to the provided http.ResponseWriter.
//
// This function is used to handle errors by constructing a ProblemDetails object with the provided error message
// and HTTP status code. It then writes the JSON representation of the ProblemDetails object to the response writer
// along with the specified HTTP status code.
//
// Parameters:
//   - w: http.ResponseWriter to write the response to.
//   - error: The error message to include in the ProblemDetails object.
//   - code: The HTTP status code to set in the response.
func errHandlerProblemDetails(w http.ResponseWriter, error string, code int) {
	var pd ProblemDetails
	pd.Detail = error
	pd.Status = int32(code)

	jsonResponse := convertProblemDetailstoJson(&pd)

	w.WriteHeader(code)
	fmt.Fprint(w, jsonResponse)
}
