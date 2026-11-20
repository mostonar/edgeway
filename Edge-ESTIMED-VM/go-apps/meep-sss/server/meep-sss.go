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

	sbi "github.com/InterDigitalInc/AdvantEDGE/go-apps/meep-sss/sbi"
	asc "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-app-support-client"
	dkm "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-data-key-mgr"
	httpLog "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-http-logger"
	iot "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-iot-client"
	log "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-logger"
	met "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-metrics"
	redis "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-redis"
	scc "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-sandbox-ctrl-client"
	smc "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-service-mgmt-client"

	"github.com/gorilla/mux"
	//uuid "github.com/google/uuid"
)

const moduleName = "meep-sss"
const sssBasePath = "sens/v1/"
const sssKey = "sens"

const serviceName = "SSS Service"
const serviceCategory = "SSS"
const defaultLocationName = "global"
const defaultScopeOfLocality = "MEC_SYSTEM"
const defaultConsumedLocalOnly = true
const appTerminationPath = "notifications/mec011/appTermination"

const iotBasePath = "iots/v1"

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
var iotClient *iot.APIClient

var registrationTicker *time.Ticker = nil
var iotPlatfomrDiscoveryTicker *time.Ticker = nil

const SENS_DISCOVERY = "SensorDiscoveryEventSubscription"
const SENS_STATUS = "SensorStatusSubscription"
const SENS_DATA = "SensorDataSubscription"

const SENS_DISCOVERY_NOTIF = "SensorDiscoveryEventNotification"
const SENS_SATUS_NOTIF = "SensorStatusNotification"
const SENS_DATA_NOTIF = "SensorDataNotification"

const TEST_NOTIF = "TestNotification"
const NOTIFY_EXPIRY = "ExpiryNotification"

var sensorDiscoveryEventSubscriptionMap = map[int]*SensorDiscoveryEventSubscription{}
var sensorStatusSubscriptionMap = map[int]*SensorStatusSubscription{}
var sensorDataSubscriptionMap = map[int]*SensorDataSubscription{}

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
			Id:      "sssId",
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
		sub.CallbackReference = "http://" + moduleName + "/" + sssBasePath + appTerminationPath
	} else {
		sub.CallbackReference = "http://" + mepName + "-" + moduleName + "/" + sssBasePath + appTerminationPath
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

// Init - SSS Service initialization
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
		basePath = "/" + sandboxName + "/" + sssBasePath
	} else {
		basePath = "/" + sandboxName + "/" + mepName + "/" + sssBasePath
	}

	// Set base storage key
	baseKey = dkm.GetKeyRoot(sandboxName) + sssKey + ":mep:" + mepName + ":"

	// Connect to Redis DB (IOT_DB)
	rc, err = redis.NewConnector(redisAddr, IOT_DB)
	if err != nil {
		log.Error("Failed connection to Redis DB (IOT_DB). Error: ", err)
		return err
	}
	_ = rc.DBFlush(baseKey)
	log.Info("Connected to Redis DB, SSS service table")

	expiryTicker = time.NewTicker(time.Second)
	go func() {
		for range expiryTicker.C {
			checkForExpiredSubscriptions()
		}
	}()

	// Initialize SBI
	sbiCfg := sbi.SbiCfg{
		ModuleName:      moduleName,
		SandboxName:     sandboxName,
		RedisAddr:       redisAddr,
		Locality:        locality,
		ScenarioNameCb:  updateStoreName,
		CleanUpCb:       cleanUp,
		Protocol:        "",
		Host:            "",
		Port:            0,
		Name:            "",
		HostId:          "",
		DiscoveryNotify: discoveryNotify,
		StatusNotify:    statusNotify,
		DataNotify:      dataNotify,
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

	iotClientCfg := iot.NewConfiguration()
	iotClientCfg.BasePath = "http://" + mepName + "-meep-iot/" + iotBasePath
	iotClient = iot.NewAPIClient(iotClientCfg)
	if iotClient == nil {
		return errors.New("Failed to create IoT REST API client")
	}
	log.Info("Create IoT REST API client")

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

	log.Info("SSS successfully initialized")
	return nil
}

// Run - Start SSS
func Run() (err error) {
	// Start Iot platfomr discovery ticker
	startIotPlatformDiscoveryTicker()

	// Start MEC Service registration ticker
	if appEnablementEnabled {
		startRegistrationTicker()
	}
	return sbi.Run()
}

// Stop - Stop SSS
func Stop() (err error) {
	// Stop Iot platfomr discovery ticker
	stopIotPlatfomrDiscoveryTicker()

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
	registrationTicker = time.NewTicker(time.Duration(5) * time.Second)
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

func startIotPlatformDiscoveryTicker() {
	// Make sure ticker is not running
	if iotPlatfomrDiscoveryTicker != nil {
		log.Warn("IoT platform discovery ticker already running")
		return
	}

	// Wait a few seconds to allow App Enablement Service to start.
	// This is done to avoid the default 20 second TCP socket connect timeout
	// if the App Enablement Service is not yet running.
	log.Info("Waiting stable state start")
	time.Sleep(5 * time.Second)

	// Start registration ticker
	iotPlatfomrDiscoveryTicker = time.NewTicker(time.Duration(60) * time.Second)
	go func() {
		for range iotPlatfomrDiscoveryTicker.C {
			log.Debug("startIotPlatformDiscoveryTicker: Launch IoT platform discovery")
			iotPlatformInfoList, _, err := iotClient.RegIotPlatApi.RegisterediotplatformsGET(context.TODO(), nil)
			if err != nil {
				log.Error("Failed to initiate the IoT platform discovery: ", err)
			} else if len(iotPlatformInfoList) > 0 {
				l := make([]sbi.IotPlatformInfo, 0)
				for _, v := range iotPlatformInfoList {
					if v.Enabled {
						var p sbi.IotPlatformInfo
						log.Debug("startIotPlatformDiscoveryTicker: Processing IoT platform: ", v.CustomServicesTransportInfo)
						if len(v.CustomServicesTransportInfo) == 0 {
							log.Warn("startIotPlatformDiscoveryTicker: Empty CustomServicesTransportInfo, skipping IoT platform ", v.IotPlatformId)
							continue
						}
						if v.CustomServicesTransportInfo[0].Endpoint.Addresses != nil {
							p = sbi.IotPlatformInfo{
								/* For HTTP */
								Host:     v.CustomServicesTransportInfo[0].Endpoint.Addresses[0].Host,
								Port:     int(v.CustomServicesTransportInfo[0].Endpoint.Addresses[0].Port),
								Path:     "",
								CseName:  v.CustomServicesTransportInfo[0].Name,
								Id:       v.IotPlatformId,
								Protocol: v.CustomServicesTransportInfo[0].Protocol,
								/* For MQTT, issues :( send req
								Host:     v.UserTransportInfo[0].Endpoint.Addresses[0].Host,
								Port:     int(v.UserTransportInfo[0].Endpoint.Addresses[0].Port),
								CseName:  v.UserTransportInfo[0].Name,
								Id:       v.IotPlatformId,
								Protocol: v.UserTransportInfo[0].Protocol,
								*/
							}
						} else if v.CustomServicesTransportInfo[0].Endpoint.Uris != nil {
							u, err := url.Parse(v.CustomServicesTransportInfo[0].Endpoint.Uris[0])
							if err != nil {
								log.Error("startIotPlatformDiscoveryTicker: ", err.Error())
								continue
							}
							log.Debug("startIotPlatformDiscoveryTicker: uri: ", u)

							log.Debug("startIotPlatformDiscoveryTicker: host: ", u.Host)
							log.Debug("startIotPlatformDiscoveryTicker: path: ", u.Path)
							log.Debug("startIotPlatformDiscoveryTicker: query: ", u.Query())
							log.Debug("startIotPlatformDiscoveryTicker: scheme: ", u.Scheme)
							log.Debug("startIotPlatformDiscoveryTicker: raw: ", u.RawQuery)
							port := u.Port()
							u.Host = u.Scheme + "://" + u.Hostname()
							log.Debug("startIotPlatformDiscoveryTicker: address: ", u.Host)
							log.Debug("startIotPlatformDiscoveryTicker: port: ", port)
							// Need to send request......
							p = sbi.IotPlatformInfo{
								/* For HTTP */
								Host:     u.Host,
								Path:     u.Path,
								CseName:  v.CustomServicesTransportInfo[0].Name,
								Id:       v.IotPlatformId,
								Protocol: v.CustomServicesTransportInfo[0].Protocol,
								/* For MQTT, issues :( send req
								Host:     v.UserTransportInfo[0].Endpoint.Addresses[0].Host,
								Port:     int(v.UserTransportInfo[0].Endpoint.Addresses[0].Port),
								CseName:  v.UserTransportInfo[0].Name,
								Id:       v.IotPlatformId,
								Protocol: v.UserTransportInfo[0].Protocol,
								*/
							}
							p.Port, err = strconv.Atoi(port)
							if err != nil {
								log.Error("startIotPlatformDiscoveryTicker: Port comversion: ", err.Error())
								//
								if u.Scheme == "https" {
									p.Port = 443
									log.Debug("startIotPlatformDiscoveryTicker: Force port to 443")
								} else {
									p.Port = 80
									log.Debug("startIotPlatformDiscoveryTicker: Force port to 80")
								}
								continue
							}
						} else {
							log.Error("startIotPlatformDiscoveryTicker: Unsupported endpoint found for IoT platform: ", v.IotPlatformId)
							continue
						}
						l = append(l, p)
						log.Debug("startIotPlatformDiscoveryTicker: Added IoT platform: l: ", l)
					} // else, skip it
				} // End of 'for'statement
				if len(l) != 0 {
					_ = sbi.UpdatePlatformList(l)
				}
			} else {
				log.Debug("startIotPlatformDiscoveryTicker: No IoT platform found")
			}
		} // End of 'for' statement
	}()
}

func stopIotPlatfomrDiscoveryTicker() {
	if iotPlatfomrDiscoveryTicker != nil {
		log.Info("Stopping IoT platform discovery ticker")
		iotPlatfomrDiscoveryTicker.Stop()
		iotPlatfomrDiscoveryTicker = nil
	}
}

func cleanUp() {
	log.Info("Terminate all")

	// Flush all service data
	rc.DBFlush(baseKey)

	// Reset metrics store name
	updateStoreName("")

	sensorDiscoveryEventSubscriptionMap = map[int]*SensorDiscoveryEventSubscription{}
	sensorStatusSubscriptionMap = map[int]*SensorStatusSubscription{}
	sensorDataSubscriptionMap = map[int]*SensorDataSubscription{}
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

func sensorDiscoveryLookupGET(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> sensorDiscoveryLookupGET: ", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	// Validate query parameters
	u, _ := url.Parse(r.URL.String())
	q := u.Query()
	log.Debug("sensorDiscoveryLookupGET: q: ", q)
	if len(q) == 0 {
		err := errors.New("Invalid query parameters")
		log.Error("sensorDiscoveryLookupGET: ", err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	} else if _, ok := q["type"]; !ok {
		err := errors.New("Invalid query parameters: 'type' parameter is mandatory")
		log.Error("sensorDiscoveryLookupGET: ", err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	} else if len(q["type"][0]) == 0 {
		err := errors.New("Invalid query parameters: Wrong 'type' parameter value")
		log.Error("sensorDiscoveryLookupGET: ", err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	} else if _, ok := q["sensorPropertyList"]; !ok {
		err := errors.New("Invalid query parameters: 'sensorPropertyList' parameter is mandatory, at leasr one item")
		log.Error("sensorDiscoveryLookupGET: ", err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	} else if len(q["sensorPropertyList"][0]) == 0 {
		err := errors.New("Invalid query parameters: Wrong 'sensorPropertyList' parameter value")
		log.Error("sensorDiscoveryLookupGET: ", err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}

	s, err := sbi.SensorDiscoveryInfoAll()
	if err != nil {
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(s) == 0 {
		err := errors.New("No sensor found")
		errHandlerProblemDetails(w, err.Error(), http.StatusNotFound)
		return
	}
	sensors := convertSensorDiscoveryInfoListFromSbi_with_filter(s, q)
	if len(sensors) == 0 {
		err := errors.New("No sensor found")
		errHandlerProblemDetails(w, err.Error(), http.StatusNotFound)
		return
	}

	// Prepare response and send
	jsonResponse := convertSensorDiscoveryInfoListToJson(sensors)
	if jsonResponse == "" {
		log.Error("sensorDiscoveryLookupGET: Marshalling failure")
		errHandlerProblemDetails(w, "Marshalling failure", http.StatusInternalServerError)
		return
	}
	log.Info("sensorDiscoveryLookupGET: jsonResponse: ", jsonResponse)
	fmt.Fprint(w, jsonResponse)
	w.WriteHeader(http.StatusOK)
}

func sensorStatusLookupGET(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> sensorStatusLookupGET: ", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	// Validate query parameters
	u, _ := url.Parse(r.URL.String())
	q := u.Query()
	log.Debug("sensorStatusLookupGET: q: ", q)
	if _, ok := q["sensorIdentifier"]; !ok {
		err := errors.New("Invalid query parameters")
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	} else if len(q["sensorIdentifier"][0]) == 0 {
		err := errors.New("Invalid query parameters")
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}
	log.Debug("sensorStatusLookupGET: q[sensorIdentifier][0]: ", q["sensorIdentifier"][0])

	s, err := sbi.GetSensorStatus(q["sensorIdentifier"][0])
	if err != nil {
		if err.Error() == "Wrong Device identifier" {
			errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		} else {
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	log.Debug("sensorStatusLookupGET: s: ", s)

	// Prepare response and send
	jsonResponse := convertSensorStatusToJson(convertSensorStatusInfoFromSbi(s))
	if jsonResponse == "" {
		log.Error("sensorDiscoveryLookupGET: Marshalling failure")
		errHandlerProblemDetails(w, "Marshalling failure", http.StatusInternalServerError)
		return
	}
	log.Info("sensorDiscoveryLookupGET: jsonResponse: ", jsonResponse)
	fmt.Fprint(w, jsonResponse)
	w.WriteHeader(http.StatusOK)
}

func sensorDataLookupGET(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> sensorDataLookupGET: ", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	// Validate query parameters
	u, _ := url.Parse(r.URL.String())
	q := u.Query()
	log.Debug("sensorDataLookupGET: q: ", q)
	if _, ok := q["sensorIdentifier"]; !ok {
		err := errors.New("Invalid query parameters")
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	} else if len(q["sensorIdentifier"][0]) == 0 {
		err := errors.New("Invalid query parameters")
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}
	log.Debug("sensorDataLookupGET: q[sensorIdentifier][0]: ", q["sensorIdentifier"][0])

	s, err := sbi.GetSensorData(q["sensorIdentifier"][0])
	if err != nil {
		if err.Error() == "Wrong Device identifier" {
			errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		} else {
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	log.Debug("sensorDataLookupGET: s: ", s)

	// Prepare response and send
	jsonResponse := convertSensorDataToJson(convertSensorDataFromSbi(s))
	if jsonResponse == "" {
		log.Error("sensorDataLookupGET: Marshalling failure")
		errHandlerProblemDetails(w, "Marshalling failure", http.StatusInternalServerError)
		return
	}
	log.Info("sensorDataLookupGET: jsonResponse: ", jsonResponse)
	fmt.Fprint(w, jsonResponse)
	w.WriteHeader(http.StatusOK)
}

func sensorDiscoverySubscriptionGET(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> sensorDiscoverySubscriptionGET: ", r)

	subscriptionsGET(SENS_DISCOVERY, w, r)
}

func sensorStatusSubscriptionGET(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> sensorStatusSubscriptionPOST: ", r)

	subscriptionsGET(SENS_STATUS, w, r)
}

func sensorDataSubscriptionGET(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> sensorDataSubscriptionGET: ", r)

	subscriptionsGET(SENS_DATA, w, r)
}

func sensorDiscoverySubscriptionPOST(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> sensorDiscoverySubscriptionPOST: ", r)

	subscriptionsPOST(SENS_DISCOVERY, w, r)
}

func sensorStatusSubscriptionPOST(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> sensorStatusSubscriptionPOST: ", r)

	subscriptionsPOST(SENS_STATUS, w, r)
}

func sensorDataSubscriptionPOST(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> sensorDataSubscriptionPOST: ", r)

	subscriptionsPOST(SENS_DATA, w, r)
}

func sensorDiscoveryIndividualSubscriptionGET(w http.ResponseWriter, r *http.Request) {
	subscriptionsByIdGET(SENS_DISCOVERY, w, r)
}

func sensorStatusIndividualSubscriptionGET(w http.ResponseWriter, r *http.Request) {
	subscriptionsByIdGET(SENS_STATUS, w, r)
}

func sensorDataIndividualSubscriptionGET(w http.ResponseWriter, r *http.Request) {
	subscriptionsByIdGET(SENS_DATA, w, r)
}

func sensorDiscoverySubscriptionDELETE(w http.ResponseWriter, r *http.Request) {
	subscriptionDelete(w, r)
}

func sensorStatusSubscriptionDELETE(w http.ResponseWriter, r *http.Request) {
	subscriptionDelete(w, r)
}

func sensorDataSubscriptionDELETE(w http.ResponseWriter, r *http.Request) {
	subscriptionDelete(w, r)
}

func subscriptionsPOST(subType string, w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> subscriptionsPOST: ", subType)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	var subscriptionCommon SubscriptionCommon
	var bodyBytes []byte
	var err error
	switch subType {
	case SENS_DISCOVERY:
		var sub SensorDiscoverySubscriptionIdBody
		// Read JSON input stream provided in the Request, and stores it in the bodyBytes as bytes
		bodyBytes, err = ioutil.ReadAll(r.Body)
		if err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}
		log.Info("subscriptionsPost: bodyBytes: ", string(bodyBytes))
		// Unmarshal function to converts a JSON-formatted string into a SubscriptionCommon struct and store it in extractSubType
		err = json.Unmarshal(bodyBytes, &sub)
		if err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}
		log.Info("subscriptionsPost: sub: ", sub)
		if sub.SensorDiscoveryEventSubscription == nil {
			errHandlerProblemDetails(w, "Missing SensorDiscoveryEventSubscription", http.StatusBadRequest)
			return
		}
		subscriptionType := ""
		if sub.SensorDiscoveryEventSubscription.SubscriptionType != nil {
			subscriptionType = string(*sub.SensorDiscoveryEventSubscription.SubscriptionType)
		}
		subscriptionCommon = SubscriptionCommon{
			SubscriptionType:        subscriptionType,
			CallbackReference:       sub.SensorDiscoveryEventSubscription.CallbackReference,
			WebsockNotifConfig:      sub.SensorDiscoveryEventSubscription.WebsockNotifConfig,
			RequestTestNotification: sub.SensorDiscoveryEventSubscription.RequestTestNotification,
		}
	case SENS_STATUS:
		var sub SensorStatusSubscriptionIdBody
		// Read JSON input stream provided in the Request, and stores it in the bodyBytes as bytes
		bodyBytes, err = ioutil.ReadAll(r.Body)
		if err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}
		log.Info("subscriptionsPost: bodyBytes: ", string(bodyBytes))
		// Unmarshal function to converts a JSON-formatted string into a SubscriptionCommon struct and store it in extractSubType
		err = json.Unmarshal(bodyBytes, &sub)
		if err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}
		log.Info("subscriptionsPost: sub: ", sub)
		if sub.SensorStatusSubscription == nil {
			errHandlerProblemDetails(w, "Missing SensorStatusSubscription", http.StatusBadRequest)
			return
		}
		subscriptionType := ""
		if sub.SensorStatusSubscription.SubscriptionType != nil {
			subscriptionType = string(*sub.SensorStatusSubscription.SubscriptionType)
		}
		subscriptionCommon = SubscriptionCommon{
			SubscriptionType:        subscriptionType,
			CallbackReference:       sub.SensorStatusSubscription.CallbackReference,
			WebsockNotifConfig:      sub.SensorStatusSubscription.WebsockNotifConfig,
			RequestTestNotification: sub.SensorStatusSubscription.RequestTestNotification,
		}
	case SENS_DATA:
		var sub SensorDataSubscriptionIdBody
		// Read JSON input stream provided in the Request, and stores it in the bodyBytes as bytes
		bodyBytes, err = ioutil.ReadAll(r.Body)
		if err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}
		log.Info("subscriptionsPost: bodyBytes: ", string(bodyBytes))
		// Unmarshal function to converts a JSON-formatted string into a SubscriptionCommon struct and store it in extractSubType
		err = json.Unmarshal(bodyBytes, &sub)
		if err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}
		log.Info("subscriptionsPost: sub: ", sub)
		if sub.SensorDataSubscription == nil {
			errHandlerProblemDetails(w, "Missing SensorDataSubscription", http.StatusBadRequest)
			return
		}
		subscriptionType := ""
		if sub.SensorDataSubscription.SubscriptionType != nil {
			subscriptionType = string(*sub.SensorDataSubscription.SubscriptionType)
		}
		subscriptionCommon = SubscriptionCommon{
			SubscriptionType:        subscriptionType,
			CallbackReference:       sub.SensorDataSubscription.CallbackReference,
			WebsockNotifConfig:      sub.SensorDataSubscription.WebsockNotifConfig,
			RequestTestNotification: sub.SensorDataSubscription.RequestTestNotification,
		}
	}

	// Validating mandatory parameters provided in the request body
	if subscriptionCommon.SubscriptionType == "" {
		log.Error("Mandatory SubscriptionType parameter should be present")
		errHandlerProblemDetails(w, "Mandatory attribute SubscriptionType is missing in the request body.", http.StatusBadRequest)
		return
	}

	if subscriptionCommon.SubscriptionType != SENS_DISCOVERY && subscriptionCommon.SubscriptionType != SENS_STATUS && subscriptionCommon.SubscriptionType != SENS_DATA {
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
	link := new(SubscriptionLinks)
	self := new(LinkType)

	// switch statement is based on provided subscriptionType in the request body
	var jsonResponse string
	switch subscriptionType {
	case SENS_DISCOVERY:
		self.Href = hostUrl.String() + basePath + "subscriptions/sensor_discovery/" + subsIdStr
		link.Self = self
		var sensorDiscoveryEventSubscription SensorDiscoveryEventSubscription
		jsonResponse, err = processSensorDiscoveryEventSubscription(bodyBytes, link, subsIdStr, &sensorDiscoveryEventSubscription)
		if err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Location", sensorDiscoveryEventSubscription.Links.Self.Href)

	case SENS_STATUS:
		self.Href = hostUrl.String() + basePath + "subscriptions/sensor_status/" + subsIdStr
		link.Self = self
		var sensorStatusSubscription SensorStatusSubscription
		jsonResponse, err = processSensorStatusSubscription(bodyBytes, link, subsIdStr, &sensorStatusSubscription)
		if err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Location", sensorStatusSubscription.Links.Self.Href)

	case SENS_DATA:
		self.Href = hostUrl.String() + basePath + "subscriptions/sensor_data/" + subsIdStr
		link.Self = self
		var sensorDataSubscription SensorDataSubscription
		jsonResponse, err = processSensorDataSubscription(bodyBytes, link, subsIdStr, &sensorDataSubscription)
		if err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Location", sensorDataSubscription.Links.Self.Href)

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
 * subscriptionsGET is to retrieve information about all existing subscriptions at /subscriptions endpoint
 * @param {struct} w HTTP write reference
 * @param {struct} r contains the HTTP request
 * @see ETSI GS MEC 046 V3.1.1 (2024-04) Clause 7
 */
func subscriptionsGET(subType string, w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> subscriptionsGET: ", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	// get & validate query param values for sensorIdentifier
	u, _ := url.Parse(r.URL.String())
	log.Info("url: ", u.RequestURI())
	q := u.Query()
	sensorIdentifiers := []string{}
	if len(q) != 0 {
		sensorIdentifier := q.Get("sensorIdentifier")
		if len(sensorIdentifier) == 0 {
			err := errors.New("Wrong query parameters")
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
			return
		}
		log.Debug("subscriptionsGET: sensorIdentifier: ", sensorIdentifier)
		sensorIdentifiers = strings.Split(sensorIdentifier, ",")
	}
	log.Debug("subscriptionsGET: sensorIdentifiers: ", sensorIdentifiers)

	// get the response against particular subscription type
	response, err := createSubscriptionLinkList(subType, sensorIdentifiers)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}

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
 * subscriptionsByIdGET is to retrieve information about all existing subscriptions at /subscriptions endpoint
 * @param {struct} w HTTP write reference
 * @param {struct} r contains the HTTP request
 * @see ETSI GS MEC 046 V3.1.1 (2024-04) Clause 7
 */
func subscriptionsByIdGET(subType string, w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> subscriptionsByIdGET: ", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	// get & validate query param values for sensorIdentifier
	u, _ := url.Parse(r.URL.String())
	log.Info("url: ", u.RequestURI())

	vars := mux.Vars(r)
	subsIdStr := vars["subscriptionId"]
	log.Info("subsIdStr: ", subsIdStr)

	// Find subscription entry in Redis DB
	keyName := baseKey + "subscriptions:" + subsIdStr
	log.Info("subscriptionsByIdGET: keyName: ", keyName)
	subscription, err := rc.JSONGetEntry(keyName, ".")
	if err != nil {
		err = errors.New("subscription not found against the provided subscriptionId")
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusNotFound)
		return
	}
	log.Info("subscriptionsByIdGET: subscription: ", subscription)

	var jsonResponse []byte
	if subType == SENS_DISCOVERY && isSubscriptionIdRegisteredSensorDiscoveryEvent(subsIdStr) {
		var subResp SensorDiscoveryEventSubscription
		err = json.Unmarshal([]byte(subscription), &subResp)
		if err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if *subResp.SubscriptionType != SENS_DISCOVERY {
			err = errors.New("subscriptionType mismatch")
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
			return
		}
		// prepare response
		jsonResponse, err = json.Marshal(&subResp)
		if err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else if subType == SENS_STATUS && isSubscriptionIdRegisteredSensorStatus(subsIdStr) {
		var subResp SensorStatusSubscription
		err = json.Unmarshal([]byte(subscription), &subResp)
		if err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if *subResp.SubscriptionType != SENS_STATUS {
			err = errors.New("subscriptionType mismatch")
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
			return
		}
		// prepare response
		jsonResponse, err = json.Marshal(&subResp)
		if err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else if subType == SENS_DATA && isSubscriptionIdRegisteredSensorData(subsIdStr) {
		var subResp SensorDataSubscription
		err = json.Unmarshal([]byte(subscription), &subResp)
		if err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if *subResp.SubscriptionType != SENS_DATA {
			err = errors.New("subscriptionType mismatch")
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
			return
		}
		// prepare response
		jsonResponse, err = json.Marshal(&subResp)
		if err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		err = errors.New("subscriptionType does not exist")
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}
	log.Info("subscriptionsByIdGET: jsonResponse: ", string(jsonResponse))

	// send response
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(jsonResponse))
}

func subscriptionDelete(w http.ResponseWriter, r *http.Request) {
	log.Debug(">>> subscriptionDelete: ", r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	u, _ := url.Parse(r.URL.String())
	log.Info("url: ", u.RequestURI())

	vars := mux.Vars(r)
	subsIdStr := vars["subscriptionId"]
	log.Info("subsIdStr: ", subsIdStr)

	// Find subscriptionInfo entry in redis DB
	keyName := baseKey + "subscriptions:" + subsIdStr
	log.Info("subscriptionDelete: keyName: ", keyName)
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

/*
 * processSensorDiscoveryEventSubscription is to create subscription at /subscriptions endpoint
 * @param {struct} w HTTP write reference
 * @param {struct} r contains the HTTP request
 * @see ETSI GS MEC 046 V3.1.1 (2024-04) Clause 7.4.3.4 POST
 */
func processSensorDiscoveryEventSubscription(bodyBytes []byte, link *SubscriptionLinks, subsIdStr string, sensorDiscoveryEventSubscription *SensorDiscoveryEventSubscription) (string, error) {

	err := json.Unmarshal(bodyBytes, sensorDiscoveryEventSubscription)
	if err != nil {
		log.Error(err.Error())
		return "", err
	}

	// Validating mandatory parameters provided in the request body
	if sensorDiscoveryEventSubscription.SubscriptionType != nil && *sensorDiscoveryEventSubscription.SubscriptionType != SENSOR_DISCOVERY_EVENT_SUBSCRIPTION {
		err = errors.New("SubscriptionType attribute shall be present in request body")
		log.Error(err.Error())
		return "", err
	}

	if sensorDiscoveryEventSubscription.Links != nil {
		err = errors.New("Links attribute should not be present in request body")
		log.Error(err.Error())
		return "", err
	}

	// FIXME FSCOM Check filter values

	err = registerSensorDiscoveryEventSubscription(subsIdStr, nil, sensorDiscoveryEventSubscription)
	if err != nil {
		log.Error("processSensorDiscoveryEventSubscription: Failed to register: ", err.Error())
		return "", err
	}

	sensorDiscoveryEventSubscription.SubscriptionType = new(SubscriptionType)
	*sensorDiscoveryEventSubscription.SubscriptionType = SENSOR_DISCOVERY_EVENT_SUBSCRIPTION
	sensorDiscoveryEventSubscription.Links = link

	// Store subscription key in redis
	keyName := baseKey + "subscriptions:" + subsIdStr
	log.Info("processSensorDiscoveryEventSubscription: keyName: ", keyName)
	log.Info("processSensorDiscoveryEventSubscription: sensorDiscoveryEventSubscription: ", sensorDiscoveryEventSubscription)
	jsonResponse, err := json.Marshal(sensorDiscoveryEventSubscription)
	if err != nil {
		log.Error(err.Error())
		return "", err
	}
	jsonResponse_str := string(jsonResponse)
	err = rc.JSONSetEntry(keyName, ".", jsonResponse_str)
	if err != nil {
		log.Error(err.Error())
		return "", err
	}

	return jsonResponse_str, nil
}

/*
 * registerSensorDiscoveryEventSubscription to register new sensorDiscoveryEventSubscription
 * @param {string} subsIdStr contains an Id to uniquely subscription
 * @param {struct} currentSensorDiscoveryEventSubscription contains the existing SensorDiscoveryEventSubscription
 * @param {struct} sensorDiscoveryEventSubscription contains request body send to /subscriptions endpoint
 */
func registerSensorDiscoveryEventSubscription(subId string, currentSensorDiscoveryEventSubscription *SensorDiscoveryEventSubscription, sensorDiscoveryEventSubscription *SensorDiscoveryEventSubscription) error {
	log.Debug(">>> registerSensorDiscoveryEventSubscription: subId: ", subId)
	log.Debug(">>> registerSensorDiscoveryEventSubscription: currentSensorDiscoveryEventSubscription: ", currentSensorDiscoveryEventSubscription)
	log.Debug(">>> registerSensorDiscoveryEventSubscription: sensorDiscoveryEventSubscription: ", sensorDiscoveryEventSubscription)

	subsId, _ := strconv.Atoi(subId)
	log.Info("registerSensorDiscoveryEventSubscription: subsId: ", subsId)
	mutex.Lock()
	defer mutex.Unlock()

	log.Info("registerSensorDiscoveryEventSubscription: Before subscriptionExpiryMap: ", subscriptionExpiryMap)
	// Replace the current subscription by the new one
	sensorDiscoveryEventSubscriptionMap[subsId] = sensorDiscoveryEventSubscription
	if currentSensorDiscoveryEventSubscription != nil {
		// Update the subscriptionExpiryMap
		// 1. Find the old one if any
		if currentSensorDiscoveryEventSubscription.ExpiryDeadline != nil {
			if *currentSensorDiscoveryEventSubscription.ExpiryDeadline != *sensorDiscoveryEventSubscription.ExpiryDeadline {
				// 1.1 Remove the existing one
				intList := subscriptionExpiryMap[int(currentSensorDiscoveryEventSubscription.ExpiryDeadline.Seconds)]
				// TODO FSCOM Common code with delSubscription, create a function to remove an expiruDeadline from the subscriptionExpiryMap
				for i, subsIndex := range intList {
					log.Info("registerSensorDiscoveryEventSubscription: i: ", i)
					log.Info("registerSensorDiscoveryEventSubscription: subsIndex: ", subsIndex)
					if subsIndex == subsId {
						//
						log.Info("registerSensorDiscoveryEventSubscription: found index, delete entry")
						// Remove item and update subscriptionExpiryMap
						subscriptionExpiryMap[int(currentSensorDiscoveryEventSubscription.ExpiryDeadline.Seconds)] = append(intList[:i], intList[i+1:]...)
						break
					}
				} // End of 'for' statement
				log.Info("delSubscription: After removal: subscriptionExpiryMap: ", subscriptionExpiryMap)

				// 1.2 And add the new one
				subscriptionExpiryMap[int(sensorDiscoveryEventSubscription.ExpiryDeadline.Seconds)] = append(subscriptionExpiryMap[int(sensorDiscoveryEventSubscription.ExpiryDeadline.Seconds)], subsId)
			}
		} else {
			// 2. Add new expiry if any
			if sensorDiscoveryEventSubscription.ExpiryDeadline != nil {
				intList := subscriptionExpiryMap[int(sensorDiscoveryEventSubscription.ExpiryDeadline.Seconds)]
				intList = append(intList, subsId)
				subscriptionExpiryMap[int(sensorDiscoveryEventSubscription.ExpiryDeadline.Seconds)] = intList
			}
		}
	} else { // First registration
		if sensorDiscoveryEventSubscription.ExpiryDeadline != nil {
			intList := subscriptionExpiryMap[int(sensorDiscoveryEventSubscription.ExpiryDeadline.Seconds)]
			intList = append(intList, subsId)
			subscriptionExpiryMap[int(sensorDiscoveryEventSubscription.ExpiryDeadline.Seconds)] = intList
		}
		sub := sbi.SensorDiscoveryEventSubscription{
			SubscriptionType: string(SENSOR_DISCOVERY_EVENT_SUBSCRIPTION),
			SensorInfoList:   sensorDiscoveryEventSubscription.SensorInfoList,
		}
		// TODO FSCOM Add GeographicalArea field & create a convert function
		err := sbi.ProcessSensorDiscoveryEventSubscription(sub)
		if err != nil {
			log.Error("Failed to register sensorDiscoveryEventSubscription: ", err)
			return err
		}
	}
	log.Info("registerSensorDiscoveryEventSubscription: After subscriptionExpiryMap: ", subscriptionExpiryMap)
	log.Info("New registration: ", subsId, " type: ", SENS_DISCOVERY)
	return nil
}

/*
 * isSubscriptionIdRegisteredSensorDiscoveryEvent to verify if subscription is already registered
 * @param {string} subsIdStr contains an Id to uniquely subscription
 * @return {bool} true on success, false otherwise
 */
func isSubscriptionIdRegisteredSensorDiscoveryEvent(subsIdStr string) bool {
	var returnVal bool
	subsId, _ := strconv.Atoi(subsIdStr)
	mutex.Lock()
	defer mutex.Unlock()

	if sensorDiscoveryEventSubscriptionMap[subsId] != nil {
		returnVal = true
	} else {
		returnVal = false
	}
	return returnVal
}

/*
 * processSensorDiscoveryEventSubscription is to create subscription at /subscriptions endpoint
 * @param {struct} w HTTP write reference
 * @param {struct} r contains the HTTP request
 * @see ETSI GS MEC 046 V3.1.1 (2024-04) Clause 7.7.3.4 POST
 */
func processSensorStatusSubscription(bodyBytes []byte, link *SubscriptionLinks, subsIdStr string, sensorStatusSubscription *SensorStatusSubscription) (string, error) {

	err := json.Unmarshal(bodyBytes, sensorStatusSubscription)
	if err != nil {
		log.Error(err.Error())
		return "", err
	}

	// Validating mandatory parameters provided in the request body
	if sensorStatusSubscription.SubscriptionType != nil && *sensorStatusSubscription.SubscriptionType != SENSOR_STATUS_SUBSCRIPTION {
		err = errors.New("SubscriptionType attribute shall be present in request body")
		log.Error(err.Error())
		return "", err
	}

	if sensorStatusSubscription.Links != nil {
		err = errors.New("Links attribute should not be present in request body")
		log.Error(err.Error())
		return "", err
	}

	// FIXME FSCOM Check filter values

	registerSensorStatusSubscription(subsIdStr, nil, sensorStatusSubscription)

	sensorStatusSubscription.SubscriptionType = new(SubscriptionType)
	*sensorStatusSubscription.SubscriptionType = SENSOR_STATUS_SUBSCRIPTION
	sensorStatusSubscription.Links = link

	// Store subscription key in redis
	keyName := baseKey + "subscriptions:" + subsIdStr
	log.Info("processSensorStatusSubscription: keyName: ", keyName)
	log.Info("processSensorStatusSubscription: sensorStatusSubscription: ", sensorStatusSubscription)
	jsonResponse, err := json.Marshal(sensorStatusSubscription)
	if err != nil {
		log.Error(err.Error())
		return "", err
	}
	jsonResponse_str := string(jsonResponse)
	err = rc.JSONSetEntry(keyName, ".", jsonResponse_str)
	if err != nil {
		log.Error(err.Error())
		return "", err
	}

	return jsonResponse_str, nil
}

/*
 * registerSensorStatusSubscription to register new sensorStatusSubscription
 * @param {string} subsIdStr contains an Id to uniquely subscription
 * @param {struct} currentSensorStatusSubscription contains the existing SensorStatusSubscription
 * @param {struct} sensorStatusSubscription contains request body send to /subscriptions endpoint
 */
func registerSensorStatusSubscription(subId string, currentSensorStatusSubscription *SensorStatusSubscription, sensorStatusSubscription *SensorStatusSubscription) {
	log.Debug(">>> registerSensorStatusSubscription: subId: ", subId)
	log.Debug(">>> registerSensorStatusSubscription: currentSensorStatusSubscription: ", currentSensorStatusSubscription)
	log.Debug(">>> registerSensorStatusSubscription: sensorStatusSubscription: ", sensorStatusSubscription)

	subsId, _ := strconv.Atoi(subId)
	log.Info("registerSensorStatusSubscription: subsId: ", subsId)
	mutex.Lock()
	defer mutex.Unlock()

	log.Info("registerSensorStatusSubscription: Before subscriptionExpiryMap: ", subscriptionExpiryMap)
	// Replace the current subscription by the new one
	sensorStatusSubscriptionMap[subsId] = sensorStatusSubscription
	if currentSensorStatusSubscription != nil {
		// Update the subscriptionExpiryMap
		// 1. Find the old one if any
		if currentSensorStatusSubscription.ExpiryDeadline != nil {
			if *currentSensorStatusSubscription.ExpiryDeadline != *sensorStatusSubscription.ExpiryDeadline {
				// 1.1 Remove the existing one
				intList := subscriptionExpiryMap[int(currentSensorStatusSubscription.ExpiryDeadline.Seconds)]
				// TODO FSCOM Common code with delSubscription, create a function to remove an expiruDeadline from the subscriptionExpiryMap
				for i, subsIndex := range intList {
					log.Info("registerSensorStatusSubscription: i: ", i)
					log.Info("registerSensorStatusSubscription: subsIndex: ", subsIndex)
					if subsIndex == subsId {
						//
						log.Info("registerSensorStatusSubscription: found index, delete entry")
						// Remove item and update subscriptionExpiryMap
						subscriptionExpiryMap[int(currentSensorStatusSubscription.ExpiryDeadline.Seconds)] = append(intList[:i], intList[i+1:]...)
						break
					}
				} // End of 'for' statement
				log.Info("delSubscription: After removal: subscriptionExpiryMap: ", subscriptionExpiryMap)

				// 1.2 And add the new one
				subscriptionExpiryMap[int(sensorStatusSubscription.ExpiryDeadline.Seconds)] = append(subscriptionExpiryMap[int(sensorStatusSubscription.ExpiryDeadline.Seconds)], subsId)
			}
		} else {
			// 2. Add new expiry if any
			if sensorStatusSubscription.ExpiryDeadline != nil {
				intList := subscriptionExpiryMap[int(sensorStatusSubscription.ExpiryDeadline.Seconds)]
				intList = append(intList, subsId)
				subscriptionExpiryMap[int(sensorStatusSubscription.ExpiryDeadline.Seconds)] = intList
			}
		}
	} else { // First registration
		if sensorStatusSubscription.ExpiryDeadline != nil {
			intList := subscriptionExpiryMap[int(sensorStatusSubscription.ExpiryDeadline.Seconds)]
			intList = append(intList, subsId)
			subscriptionExpiryMap[int(sensorStatusSubscription.ExpiryDeadline.Seconds)] = intList
		}
	}
	log.Info("registerSensorStatusSubscription: After subscriptionExpiryMap: ", subscriptionExpiryMap)
	log.Info("New registration: ", subsId, " type: ", SENS_DISCOVERY)
}

/*
 * isSubscriptionIdRegisteredSensorStatus to verify if subscription is already registered
 * @param {string} subsIdStr contains an Id to uniquely subscription
 * @return {bool} true on success, false otherwise
 */
func isSubscriptionIdRegisteredSensorStatus(subsIdStr string) bool {
	var returnVal bool
	subsId, _ := strconv.Atoi(subsIdStr)
	mutex.Lock()
	defer mutex.Unlock()

	if sensorStatusSubscriptionMap[subsId] != nil {
		returnVal = true
	} else {
		returnVal = false
	}
	return returnVal
}

/*
*  func processSensorDataSubscription(bodyBytes []byte, link *SubscriptionLinks, subsIdStr string, sensorDataSubscription *SensorDataSubscription) (string, error) {
is to create subscription at /subscriptions endpoint
* @param {struct} w HTTP write reference
* @param {struct} r contains the HTTP request
* @see ETSI GS MEC 046 V3.1.1 (2024-04) Clause 7.10.3.4 POST
*/
func processSensorDataSubscription(bodyBytes []byte, link *SubscriptionLinks, subsIdStr string, sensorDataSubscription *SensorDataSubscription) (string, error) {

	err := json.Unmarshal(bodyBytes, sensorDataSubscription)
	if err != nil {
		log.Error(err.Error())
		return "", err
	}

	// Validating mandatory parameters provided in the request body
	if sensorDataSubscription.SubscriptionType != nil && *sensorDataSubscription.SubscriptionType != SENSOR_DATA_SUBSCRIPTION {
		err = errors.New("SubscriptionType attribute shall be present in request body")
		log.Error(err.Error())
		return "", err
	}

	if sensorDataSubscription.Links != nil {
		err = errors.New("Links attribute should not be present in request body")
		log.Error(err.Error())
		return "", err
	}

	// FIXME FSCOM Check filter values

	registerSensorDataSubscription(subsIdStr, nil, sensorDataSubscription)

	sensorDataSubscription.SubscriptionType = new(SubscriptionType)
	*sensorDataSubscription.SubscriptionType = SENSOR_DATA_SUBSCRIPTION
	sensorDataSubscription.Links = link

	// Store subscription key in redis
	keyName := baseKey + "subscriptions:" + subsIdStr
	log.Info("processSensorDataSubscription: keyName: ", keyName)
	log.Info("processSensorDataSubscription: sensorDataSubscription: ", sensorDataSubscription)
	jsonResponse, err := json.Marshal(sensorDataSubscription)
	if err != nil {
		log.Error(err.Error())
		return "", err
	}
	jsonResponse_str := string(jsonResponse)
	err = rc.JSONSetEntry(keyName, ".", jsonResponse_str)
	if err != nil {
		log.Error(err.Error())
		return "", err
	}

	return jsonResponse_str, nil
}

/*
 * registerSensorDataSubscription to register new sensorDataSubscription
 * @param {string} subsIdStr contains an Id to uniquely subscription
 * @param {struct} currentSensorDataSubscription contains the existing SensorDataSubscription
 * @param {struct} sensorDataSubscription contains request body send to /subscriptions endpoint
 */
func registerSensorDataSubscription(subId string, currentSensorDataSubscription *SensorDataSubscription, sensorDataSubscription *SensorDataSubscription) {
	log.Debug(">>> registerSensorDataSubscription: subId: ", subId)
	log.Debug(">>> registerSensorDataSubscription: currentSensorDataSubscription: ", currentSensorDataSubscription)
	log.Debug(">>> registerSensorDataSubscription: sensorDataSubscription: ", sensorDataSubscription)

	subsId, _ := strconv.Atoi(subId)
	log.Info("registerSensorDataSubscription: subsId: ", subsId)
	mutex.Lock()
	defer mutex.Unlock()

	log.Info("registerSensorDataSubscription: Before subscriptionExpiryMap: ", subscriptionExpiryMap)
	// Replace the current subscription by the new one
	sensorDataSubscriptionMap[subsId] = sensorDataSubscription
	if currentSensorDataSubscription != nil {
		// Update the subscriptionExpiryMap
		// 1. Find the old one if any
		if currentSensorDataSubscription.ExpiryDeadline != nil {
			if *currentSensorDataSubscription.ExpiryDeadline != *sensorDataSubscription.ExpiryDeadline {
				// 1.1 Remove the existing one
				intList := subscriptionExpiryMap[int(currentSensorDataSubscription.ExpiryDeadline.Seconds)]
				// TODO FSCOM Common code with delSubscription, create a function to remove an expiruDeadline from the subscriptionExpiryMap
				for i, subsIndex := range intList {
					log.Info("registerSensorDataSubscription: i: ", i)
					log.Info("registerSensorDataSubscription: subsIndex: ", subsIndex)
					if subsIndex == subsId {
						//
						log.Info("registerSensorDataSubscription: found index, delete entry")
						// Remove item and update subscriptionExpiryMap
						subscriptionExpiryMap[int(currentSensorDataSubscription.ExpiryDeadline.Seconds)] = append(intList[:i], intList[i+1:]...)
						break
					}
				} // End of 'for' statement
				log.Info("delSubscription: After removal: subscriptionExpiryMap: ", subscriptionExpiryMap)

				// 1.2 And add the new one
				subscriptionExpiryMap[int(sensorDataSubscription.ExpiryDeadline.Seconds)] = append(subscriptionExpiryMap[int(sensorDataSubscription.ExpiryDeadline.Seconds)], subsId)
			}
		} else {
			// 2. Add new expiry if any
			if sensorDataSubscription.ExpiryDeadline != nil {
				intList := subscriptionExpiryMap[int(sensorDataSubscription.ExpiryDeadline.Seconds)]
				intList = append(intList, subsId)
				subscriptionExpiryMap[int(sensorDataSubscription.ExpiryDeadline.Seconds)] = intList
			}
		}
	} else { // First registration
		if sensorDataSubscription.ExpiryDeadline != nil {
			intList := subscriptionExpiryMap[int(sensorDataSubscription.ExpiryDeadline.Seconds)]
			intList = append(intList, subsId)
			subscriptionExpiryMap[int(sensorDataSubscription.ExpiryDeadline.Seconds)] = intList
		}
	}
	log.Info("registerSensorDataSubscription: After subscriptionExpiryMap: ", subscriptionExpiryMap)
	log.Info("New registration: ", subsId, " type: ", SENS_DISCOVERY)
}

/*
 * isSubscriptionIdRegisteredSensorData to verify if subscription is already registered
 * @param {string} subsIdStr contains an Id to uniquely subscription
 * @return {bool} true on success, false otherwise
 */
func isSubscriptionIdRegisteredSensorData(subsIdStr string) bool {
	var returnVal bool
	subsId, _ := strconv.Atoi(subsIdStr)
	mutex.Lock()
	defer mutex.Unlock()

	if sensorDataSubscriptionMap[subsId] != nil {
		returnVal = true
	} else {
		returnVal = false
	}
	return returnVal
}

func createSubscriptionLinkList(subType string, sensorIdentifiers []string) (*SubscriptionLinkList, error) {
	log.Debug(">>> createSubscriptionLinkList: subType: ", subType)
	log.Debug(">>> createSubscriptionLinkList: sensorIdentifiers: ", sensorIdentifiers)

	log.Debug("createSubscriptionLinkList: sensorDiscoveryEventSubscriptionMap: ", sensorDiscoveryEventSubscriptionMap)
	log.Debug("createSubscriptionLinkList: sensorStatusSubscriptionMap: ", sensorStatusSubscriptionMap)
	log.Debug("createSubscriptionLinkList: sensorDataSubscriptionMap: ", sensorDataSubscriptionMap)

	subscriptionLinkList := new(SubscriptionLinkList)

	links := new(SubscriptionLinkListLinks)
	self := new(LinkType)
	self.Href = hostUrl.String() + basePath + "subscriptions"

	links.Self = self

	//loop through all different types of subscription
	mutex.Lock()
	defer mutex.Unlock()

	if subType == SENS_DISCOVERY {
		for _, discoveryEventSubscription := range sensorDiscoveryEventSubscriptionMap {
			if discoveryEventSubscription != nil {
				var subscriptions SubscriptionLinkListSubscription
				if discoveryEventSubscription.Links != nil && discoveryEventSubscription.Links.Self != nil {
					subscriptions.Href = discoveryEventSubscription.Links.Self.Href
				}
				subscriptions.SubscriptionType = SENS_DISCOVERY
				links.Subscriptions = append(links.Subscriptions, subscriptions)
			}
		} // End of 'for' statement
	} else if subType == SENS_STATUS {
		for _, statusSubscription := range sensorStatusSubscriptionMap {
			if statusSubscription != nil {
				var subscriptions SubscriptionLinkListSubscription
				if statusSubscription.Links != nil && statusSubscription.Links.Self != nil {
					subscriptions.Href = statusSubscription.Links.Self.Href
				}
				subscriptions.SubscriptionType = SENS_STATUS
				links.Subscriptions = append(links.Subscriptions, subscriptions)
			}
		} // End of 'for' statement
	} else if subType == SENS_DATA {
		for _, dataSubscription := range sensorDataSubscriptionMap {
			if dataSubscription != nil {
				var subscriptions SubscriptionLinkListSubscription
				if dataSubscription.Links != nil && dataSubscription.Links.Self != nil {
					subscriptions.Href = dataSubscription.Links.Self.Href
				}
				subscriptions.SubscriptionType = SENS_DATA_NOTIF
				links.Subscriptions = append(links.Subscriptions, subscriptions)
			}
		} // End of 'for' statement
	} else { // Build complete list
		for _, discoveryEventSubscription := range sensorDiscoveryEventSubscriptionMap {
			if discoveryEventSubscription != nil {
				var subscriptions SubscriptionLinkListSubscription
				subscriptions.Href = discoveryEventSubscription.Links.Self.Href
				subscriptions.SubscriptionType = SENS_DISCOVERY
				links.Subscriptions = append(links.Subscriptions, subscriptions)
			}
		} // End of 'for' statement
		for _, statusSubscription := range sensorStatusSubscriptionMap {
			if statusSubscription != nil {
				var subscriptions SubscriptionLinkListSubscription
				subscriptions.Href = statusSubscription.Links.Self.Href
				subscriptions.SubscriptionType = SENS_STATUS
				links.Subscriptions = append(links.Subscriptions, subscriptions)
			}
		} // End of 'for' statement
		for _, dataSubscription := range sensorDataSubscriptionMap {
			if dataSubscription != nil {
				var subscriptions SubscriptionLinkListSubscription
				subscriptions.Href = dataSubscription.Links.Self.Href
				subscriptions.SubscriptionType = SENS_DATA
				links.Subscriptions = append(links.Subscriptions, subscriptions)
			}
		} // End of 'for' statement
	}
	log.Debug("createSubscriptionLinkList: links.Subscriptions: ", links.Subscriptions)

	subscriptionLinkList.Links = links

	return subscriptionLinkList, nil
}

/*
 * checkForExpiredSubscriptions delete those subscriptions whose expiryTime is reached
 */
func checkForExpiredSubscriptions() {
	//log.Debug(">>> checkForExpiredSubscriptions")

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
				var notif = ExpiryNotification{}
				if strings.Contains(subscription, SENS_DISCOVERY) {
					if sensorDiscoveryEventSubscriptionMap[subsId] != nil {
						cbRef = sensorDiscoveryEventSubscriptionMap[subsId].CallbackReference
						notif.NotificationType = SENS_DISCOVERY_NOTIF
					} else {
						continue
					}
				} else if strings.Contains(subscription, SENS_STATUS) {
					if sensorStatusSubscriptionMap[subsId] != nil {
						cbRef = sensorStatusSubscriptionMap[subsId].CallbackReference
						notif.NotificationType = SENS_SATUS_NOTIF
					} else {
						continue
					}
				} else if strings.Contains(subscription, SENS_DATA) {
					if sensorDataSubscriptionMap[subsId] != nil {
						cbRef = sensorDataSubscriptionMap[subsId].CallbackReference
						notif.NotificationType = SENS_DATA_NOTIF
					} else {
						continue
					}
				}

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

	if strings.Contains(subscription, SENS_DISCOVERY) {
		deregisterSensorDiscoveryEventSubscription(subsId, mutexTaken)
	} else if strings.Contains(subscription, SENS_STATUS) {
		deregisterSensorStatusSubscription(subsId, mutexTaken)
	} else if strings.Contains(subscription, SENS_DATA) {
		deregisterSensorDataSubscription(subsId, mutexTaken)
	}

	return nil
}

func deregisterSensorDiscoveryEventSubscription(subsIdStr string, mutexTaken bool) {
	log.Debug(">>> deregisterSensorDiscoveryEventSubscription: subsId: ", subsIdStr)

	subsId, _ := strconv.Atoi(subsIdStr)
	if !mutexTaken {
		mutex.Lock()
		defer mutex.Unlock()
	}
	log.Info("deregisterSensorDiscoveryEventSubscription: Before sensorDiscoveryEventSubscriptionMap", sensorDiscoveryEventSubscriptionMap)
	delete(sensorDiscoveryEventSubscriptionMap, subsId)
	log.Info("deregisterSensorDiscoveryEventSubscription: After sensorDiscoveryEventSubscriptionMap", sensorDiscoveryEventSubscriptionMap)

	log.Info("deregisterSensorDiscoveryEventSubscription: ", subsId, " type: ", SENS_DISCOVERY)
}

func deregisterSensorStatusSubscription(subsIdStr string, mutexTaken bool) {
	log.Debug(">>> deregisterSensorStatusSubscription: subsId: ", subsIdStr)

	subsId, _ := strconv.Atoi(subsIdStr)
	if !mutexTaken {
		mutex.Lock()
		defer mutex.Unlock()
	}
	log.Info("deregisterSensorStatusSubscription: Before sensorStatusSubscriptionMap", sensorStatusSubscriptionMap)
	delete(sensorStatusSubscriptionMap, subsId)
	log.Info("deregisterSensorStatusSubscription: After sensorStatusSubscriptionMap", sensorStatusSubscriptionMap)

	log.Info("deregisterSensorStatusSubscription: ", subsId, " type: ", SENS_STATUS)
}

func deregisterSensorDataSubscription(subsIdStr string, mutexTaken bool) {
	log.Debug(">>> deregisterSensorDataSubscription: subsId: ", subsIdStr)

	subsId, _ := strconv.Atoi(subsIdStr)
	if !mutexTaken {
		mutex.Lock()
		defer mutex.Unlock()
	}
	log.Info("deregisterSensorDataSubscription: Before sensorDataSubscriptionMap", sensorDataSubscriptionMap)
	delete(sensorDataSubscriptionMap, subsId)
	log.Info("deregisterSensorDataSubscription: After sensorDataSubscriptionMap", sensorDataSubscriptionMap)

	log.Info("deregisterSensorDataSubscription: ", subsId, " type: ", SENS_DATA)
}

/*
 * sendTestNotification sends test notification to validate callback URI
 * @param {string} notifyUrl contains the call reference address
 * @param {struct} notification contains the test notification
 * @see ETSI GS MEC 046 V3.1.1 (2024-04) Clause 6
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
		met.ObserveNotification(sandboxName, serviceName, notification.NotificationType, notifyUrl, nil, duration)
		return
	}
	met.ObserveNotification(sandboxName, serviceName, notification.NotificationType, notifyUrl, resp, duration)
	defer resp.Body.Close()
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
		met.ObserveNotification(sandboxName, serviceName, notification.NotificationType, notifyUrl, nil, duration)
		return
	}
	met.ObserveNotification(sandboxName, serviceName, notification.NotificationType, notifyUrl, resp, duration)
	defer resp.Body.Close()
}

/*
 * discoveryNotify sends notification to the call reference address
 * @param {string} notifyUrl contains the call reference address
 * @param {struct} notification contains notification body of type SensorDiscoveryEventNotification
 * @see ETSI GS MEC 046 V3.1.1 (2024-04) Clause 7
 */
func discoveryNotify(map[string]interface{}) {
	log.Debug(">>> sendTestNotification: discoveryNotify")

}

func statusNotify(map[string]interface{}) {
	log.Debug(">>> sendTestNotification: statusNotify")

}

func dataNotify(map[string]interface{}) {
	log.Debug(">>> sendTestNotification: dataNotify")

}

// func convertSensorDiscoveryInfoListFromSbi(s []sbi.SensorDiscoveryInfo) (sensors []SensorDiscoveryInfo) {
// 	sensors = make([]SensorDiscoveryInfo, len(s))
// 	for i, val := range s {
// 		sensors[i] = convertSensorDiscoveryInfoFromSbi(val)
// 	} // End of 'for' statement

// 	return sensors
// }

func convertSensorDiscoveryInfoListFromSbi_with_filter(s []sbi.SensorDiscoveryInfo, filter url.Values) (sensors []SensorDiscoveryInfo) {
	log.Debug(">>> convertSensorDiscoveryInfoListFromSbi_with_filter: s: ", s)
	log.Debug(">>> convertSensorDiscoveryInfoListFromSbi_with_filter: filter: ", filter)
	//log.Debug("sensorDiscoveryLookupGET: q[sensorCharacteristicList][0]: ", q["sensorCharacteristicList"][0])
	//log.Debug("sensorDiscoveryLookupGET: type(q[sensorCharacteristicList][0]): ", reflect.TypeOf(q["sensorCharacteristicList"][0]))
	//q: map[geographicalArea:[[object Object]] sensorCharacteristicList:[[object Object]] sensorPropertyList:[string1,string2] type:[string]]","time":"2025-02-04T08:35:35Z"}
	// /sens/v1/queries/sensor_discovery?type=4&sensorPropertyList=%5B%22con%22%5D&sensorCharacteristicList=%5B%7B%22characteristicName%22%3A%22pi%22%2C%22characteristicUnitOfMeasure%22%3A%22ae_parent%22%7D%5D&geographicalArea=%5B%7B%22shape%22%3A0%2C%22radius%22%3A6%2C%22points%22%3A%5B%7B%22latitude%22%3A0.8008281904610115%2C%22longitude%22%3A6.027456183070403%7D%2C%7B%22latitude%22%3A0.8008281904610115%2C%22longitude%22%3A6.027456183070403%7D%5D%7D%5D' \

	sensors = make([]SensorDiscoveryInfo, 0)
	for _, val := range s {
		log.Debug("convertSensorDiscoveryInfoListFromSbi_with_filter: processing val: ", val)
		if filter["type"][0] != val.SensorType {
			log.Debug("convertSensorDiscoveryInfoListFromSbi_with_filter: ", filter["type"][0], " not found, discard it")
			continue // Discard it
		} else {
			// Apply filter on sensorPropertyList
			if filter["sensorPropertyList"][0] != "saref:any" { // FIXME FSCOM Which values to be used???
				if len(val.SensorPropertyList) == 0 {
					log.Debug("convertSensorDiscoveryInfoListFromSbi_with_filter: ", filter["sensorPropertyList"][0], " not found, discard it")
					continue // Discard it
				} else {
					found := 0
					for _, fu := range filter["sensorPropertyList"] {
						log.Debug("convertSensorDiscoveryInfoListFromSbi_with_filter (sensorPropertyList): processing fu: ", fu)
						l := strings.Split(fu, ",")
						log.Debug("convertSensorDiscoveryInfoListFromSbi_with_filter: l: ", l)
						// TODO FSCOM Apply filter on
					} // End of 'for' statement
					if found != len(val.SensorPropertyList) { // Not all filter items found
						log.Debug("convertSensorDiscoveryInfoListFromSbi_with_filter: Not all filter items found, discard it")
						continue // Discard it
					}
				}
			} // else, continue

			// Apply filter on sensorCharacteristicList
			if _, ok := filter["sensorCharacteristicList"]; !ok { // 'sensorCharacteristicList'filter omitted
				sensors = append(sensors, convertSensorDiscoveryInfoFromSbi(val))
			} else {
				found := 0
				for _, fu := range filter["sensorCharacteristicList"] {
					log.Debug("convertSensorDiscoveryInfoListFromSbi_with_filter (sensorCharacteristicList): processing fu: ", fu)
					l := strings.Split(fu, ",")
					log.Debug("convertSensorDiscoveryInfoListFromSbi_with_filter: l: ", l)
					// TODO FSCOM Apply filter on
				} // End of 'for' statement
				if found == len(filter["sensorCharacteristicList"]) { // All filter iems found
					sensors = append(sensors, convertSensorDiscoveryInfoFromSbi(val))
				} else {
					continue // Skip this item
				}

				// if len(val.SensorPropertyList) != 0 {
				// 	if f, ok := filter["sensorPropertyList"]; ok {
				// 		log.Debug("convertSensorDiscoveryInfoListFromSbi_with_filter: f: ", f)
				// 		found := 0
				// 		for _, fu := range f {
				// 			log.Debug("convertSensorDiscoveryInfoListFromSbi_with_filter: processing fu: ", fu)
				// 			l := strings.Split(fu, ",")
				// 			log.Debug("convertSensorDiscoveryInfoListFromSbi_with_filter: l: ", l)
				// 			for _, s := range l {
				// 				log.Debug("convertSensorDiscoveryInfoListFromSbi_with_filter: processing s: ", s)
				// 				for _, v := range val.SensorPropertyList {
				// 					log.Debug("convertSensorDiscoveryInfoListFromSbi_with_filter: processing v: ", v)
				// 					if v == s {
				// 						found += 1
				// 						log.Debug("convertSensorDiscoveryInfoListFromSbi_with_filter: matching: ", found)
				// 						break // Exit loop on sensor SensorPropertyList
				// 					}
				// 				} // End of 'for' statement
				// 			} // End of 'for' statement
				// 			log.Debug("convertSensorDiscoveryInfoListFromSbi_with_filter: found: ", found, " compared to ", len(l))
				// 			if found == len(l) { // All filter iems found
				// 				sensors = append(sensors, convertSensorDiscoveryInfoFromSbi(val))
				// 			} else {
				// 				break // Exit loop on filter list
				// 			}
				// 		} // End of 'for' statement
				// 		// FIXME FSCOM Add filtering
				// 	} else {
				// 		sensors = append(sensors, convertSensorDiscoveryInfoFromSbi(val))
				// 	}
			}
		}
	} // End of 'for' statement
	log.Debug("convertSensorDiscoveryInfoListFromSbi_with_filter: sensors: ", sensors)

	return sensors
}

func convertSensorDiscoveryInfoFromSbi(s sbi.SensorDiscoveryInfo) (sensor SensorDiscoveryInfo) {
	sensor = SensorDiscoveryInfo{
		SensorIdentifier:   s.SensorIdentifier,
		SensorType:         s.SensorType,
		SensorPropertyList: []string{"SAREF"},
	}
	if len(s.SensorPropertyList) != 0 {
		copy(sensor.SensorPropertyList, s.SensorPropertyList)
	}
	if len(s.SensorCharacteristicList) != 0 {
		sensor.SensorCharacteristicList = make([]SensorCharacteristic, len(s.SensorCharacteristicList))
		for i, val := range s.SensorCharacteristicList {
			sensor.SensorCharacteristicList[i] = SensorCharacteristic{
				CharacteristicName:  val.CharacteristicName,
				CharacteristicValue: val.CharacteristicValue,
				// TODO FSCOM CharacteristicUnitOfMeasure: val.CharacteristicUnitOfMeasure,
			}
		} // End of 'for' statement
	}
	if s.SensorPosition != nil {
		sensor.SensorPosition = &Point{
			Latitude:  s.SensorPosition.Latitude,
			Longitude: s.SensorPosition.Longitude,
		}
	} else { // SensorPosition is mandatory
		sensor.SensorPosition = &Point{
			Latitude:  0,
			Longitude: 0,
		}
	}

	return sensor
}

func convertSensorDataFromSbi(d sbi.SensorData) (data SensorData) {
	data = SensorData{
		SensorIdentifier:  d.SensorIdentifier,
		Data:              d.Data,
		DataFormat:        "",
		DataUnitOfMeasure: "",
	}
	if d.DataTimestamp != nil {
		data.DataTimestamp = &TimeStamp{
			Seconds:     d.DataTimestamp.Seconds,
			NanoSeconds: d.DataTimestamp.NanoSeconds,
		}
	}

	return data
}

func convertSensorStatusInfoFromSbi(s sbi.SensorStatusInfo) (status SensorStatusInfo) {
	status = SensorStatusInfo{
		SensorIdentifier: s.SensorIdentifier,
		SensorStatusType: s.SensorStatusType,
		ErrorInformation: s.ErrorInformation,
	}

	return status
}
