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
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	dkm "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-data-key-mgr"
	log "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-logger"
	mq "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-mq"
	redis "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-redis"
	subs "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-subscriptions"
	uuid "github.com/google/uuid"

	"github.com/gorilla/mux"
)

const moduleName = "meep-app-enablement"
const svcMgmtBasePath = "mec_service_mgmt/v1/"
const appEnablementKey = "app-enablement"
const globalLocationName = "global"
const SER_AVAILABILITY_NOTIF_SUB_TYPE = "SerAvailabilityNotificationSubscription"
const SER_AVAILABILITY_NOTIF_TYPE = "SerAvailabilityNotification"
const APP_STATE_READY = "READY"
const capifEventsBasePath = "capif-events/v1"

// const logModuleAppEnablement = "meep-app-enablement"
const serviceName = "App Enablement Service"

// App Info fields
const fieldState = "state"

// MQ payload fields
const fieldSvcInfo = "svc-info"
const fieldAppId = "app-id"
const fieldChangeType = "change-type"
const fieldLocationName = "mep-name"

var mutex *sync.Mutex
var redisAddr string // = "meep-redis-master.default.svc.cluster.local:6379"
var APP_ENABLEMENT_DB = 0
var rc *redis.Connector
var mqLocal *mq.MsgQueue
var hostUrl *url.URL
var sandboxName string
var mepName string
var basePath string
var baseKey string
var baseKeyAnyMep string
var subMgr *subs.SubscriptionMgr

type ServiceInfoList struct {
	Services                 []ServiceInfo
	ConsumedLocalOnlyPresent bool
	IsLocalPresent           bool
	Filters                  *FilterParameters
}

type FilterParameters struct {
	serInstanceId     []string
	serName           []string
	serCategoryId     string
	consumedLocalOnly bool
	isLocal           bool
	scopeOfLocality   string
}

type StateData struct {
	State ServiceState
	AppId string
}

var livenessTimerList map[string]ServiceLivenessInfo

func Init(sandbox string, mep string, host *url.URL, msgQueue *mq.MsgQueue, redisAddr_ string, globalMutex *sync.Mutex) (err error) {
	redisAddr = redisAddr_
	sandboxName = sandbox
	mepName = mep
	hostUrl = host
	mqLocal = msgQueue
	mutex = globalMutex

	// Set base path & storage key
	if mepName == globalLocationName {
		basePath = "/" + sandboxName + "/" + svcMgmtBasePath
		baseKey = dkm.GetKeyRoot(sandboxName) + appEnablementKey + ":mep-global:"
		baseKeyAnyMep = dkm.GetKeyRoot(sandboxName) + appEnablementKey + ":mep-global:"
	} else {
		basePath = "/" + sandboxName + "/" + mepName + "/" + svcMgmtBasePath
		baseKey = dkm.GetKeyRoot(sandboxName) + appEnablementKey + ":mep:" + mepName + ":"
		baseKeyAnyMep = dkm.GetKeyRoot(sandboxName) + appEnablementKey + ":mep:*:"
	}

	// Connect to Redis DB
	rc, err = redis.NewConnector(redisAddr, APP_ENABLEMENT_DB)
	if err != nil {
		log.Error("Failed connection to Redis DB. Error: ", err)
		return err
	}
	log.Info("Connected to Redis DB")

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

	livenessTimerList = make(map[string]ServiceLivenessInfo)

	// TODO -- Initialize subscriptions from DB

	return nil
}

// Run - Start Service Mgmt
func Run() (err error) {

	// Register Message Queue handler
	handler := mq.MsgHandler{Handler: msgHandler, UserData: nil}
	_, err = mqLocal.RegisterHandler(handler)
	if err != nil {
		log.Error("Failed to listen for sandbox updates: ", err.Error())
		return err
	}

	return nil
}

// Stop - Stop Service Mgmt
func Stop() (err error) {

	if len(livenessTimerList) != 0 {
		livenessTimerList = make(map[string]ServiceLivenessInfo)
	}

	return nil
}

func createLivenessTicker(sInfo ServiceInfo) {
	log.Debug(">>> createLivenessTicker: ", sInfo)

	livenessTimerList[sInfo.SerInstanceId] = ServiceLivenessInfo{
		State:     &ACTIVE_ServiceState,
		TimeStamp: &ServiceLivenessInfoTimeStamp{Seconds: 0, NanoSeconds: 0},
		Interval:  sInfo.LivenessInterval,
	}
}

func updateLivenessTicker(sInfo ServiceInfo) {
	log.Debug(">>> updateLivenessTicker: ", sInfo)

	if sInfo.LivenessInterval != livenessTimerList[sInfo.SerInstanceId].Interval {
		deleteLivenessTicker(sInfo.SerInstanceId)
		createLivenessTicker(sInfo)
	}
}

func deleteLivenessTicker(serInstanceId string) {
	log.Debug(">>> deleteLivenessTicker: ", serInstanceId)

	delete(livenessTimerList, serInstanceId)
}

// Message Queue handler
func msgHandler(msg *mq.Msg, userData interface{}) {
	switch msg.Message {
	case mq.MsgMecSvcUpdate:
		log.Debug("RX MSG: ", mq.PrintMsg(msg))
		sInfoJson := msg.Payload[fieldSvcInfo]
		mep := msg.Payload[fieldLocationName]
		changeType := msg.Payload[fieldChangeType]
		processSvcUpdate(sInfoJson, mep, changeType)
	default:
	}
}

func appServicesPOST(w http.ResponseWriter, r *http.Request) {
	log.Info("appServicesPOST")

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	vars := mux.Vars(r)
	appId := vars["apfId"]

	mutex.Lock()
	defer mutex.Unlock()

	// Get App instance
	appInfo, err := getAppInfo(appId)
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

	// Retrieve request parameters from body
	if r.Body == nil {
		err := errors.New("Request body is missing")
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}
	// NOTE: Set default values for omitted fields
	locality := MEC_HOST_LocalityType
	sInfoPost := ServiceApiDescription{
		VendorSpecificUrnetsimeccapifextserviceInfo: &MecServiceInfoCapifExt{
			ScopeOfLocality:   &locality,
			IsLocal:           true,
			ConsumedLocalOnly: true,
		},
	}
	decoder := json.NewDecoder(r.Body)
	err = decoder.Decode(&sInfoPost)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Check for mandatory properties
	if sInfoPost.ApiId != "" {
		errStr := "Service instance ID must not be present"
		log.Error(errStr)
		errHandlerProblemDetails(w, errStr, http.StatusBadRequest)
		return
	}
	if sInfoPost.ApiName == "" {
		errStr := "Mandatory Service Name parameter not present"
		log.Error(errStr)
		errHandlerProblemDetails(w, errStr, http.StatusBadRequest)
		return
	}
	if len(sInfoPost.AefProfiles) == 0 || len(sInfoPost.AefProfiles[0].Versions) == 0 {
		errStr := "Mandatory Service Version parameter not present"
		log.Error(errStr)
		errHandlerProblemDetails(w, errStr, http.StatusBadRequest)
		return
	}
	// Reject if more than one version is provided in any AEF Profile
	for _, aef := range sInfoPost.AefProfiles {
		if len(aef.Versions) > 1 {
			errStr := fmt.Sprintf("Only one apiVersion is allowed per AEF profile; found %d", len(aef.Versions))
			log.Error(errStr)
			errHandlerProblemDetails(w, errStr, http.StatusBadRequest)
			return
		}
	}
	if sInfoPost.VendorSpecificUrnetsimeccapifextserviceInfo.State == nil {
		errStr := "Mandatory Service State parameter not present"
		log.Error(errStr)
		errHandlerProblemDetails(w, errStr, http.StatusBadRequest)
		return
	}
	if sInfoPost.VendorSpecificUrnetsimeccapifextserviceInfo.Serializer == nil {
		errStr := "Mandatory Serializer parameter not present"
		log.Error(errStr)
		errHandlerProblemDetails(w, errStr, http.StatusBadRequest)
		return
	}
	if sInfoPost.VendorSpecificUrnetsimeccapifextserviceInfo.Category != nil {
		errStr := validateCategoryRef(sInfoPost.VendorSpecificUrnetsimeccapifextserviceInfo.Category)
		if errStr != "" {
			log.Error(errStr)
			errHandlerProblemDetails(w, errStr, http.StatusBadRequest)
			return
		}
	}

	if len(sInfoPost.AefProfiles) == 0 || sInfoPost.AefProfiles[0].VendorSpecificUrnetsimeccapifexttransportInfo == nil {
		errStr := "Mandatory TransportInfo (VendorSpecificUrnetsimeccapifexttransportInfo) not present"
		log.Error(errStr)
		errHandlerProblemDetails(w, errStr, http.StatusBadRequest)
		return
	}
	if len(sInfoPost.AefProfiles) == 0 || sInfoPost.AefProfiles[0].InterfaceDescriptions == nil {
		errStr := "Mandatory AefProfiles (InterfaceDescriptions) not present"
		log.Error(errStr)
		errHandlerProblemDetails(w, errStr, http.StatusBadRequest)
		return
	}
	if err := validateInterfaceDescriptions(sInfoPost.AefProfiles[0].InterfaceDescriptions); err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(sInfoPost.AefProfiles) == 0 || sInfoPost.AefProfiles[0].AefId == "" {
		errStr := "Mandatory AefProfiles (AefId) not present"
		log.Error(errStr)
		errHandlerProblemDetails(w, errStr, http.StatusBadRequest)
		return
	}
	transportInfo := sInfoPost.AefProfiles[0].VendorSpecificUrnetsimeccapifexttransportInfo
	// if sInfoPost.TransportInfo != nil {
	if transportInfo.Name == "" ||
		string(*transportInfo.Type_) == "" ||
		transportInfo.Protocol == "" ||
		transportInfo.Version == "" {
		errStr := "Id, Name, Type, Protocol, Version,  are all mandatory parameters of TransportInfo"
		log.Error(errStr)
		errHandlerProblemDetails(w, errStr, http.StatusBadRequest)
		return
	}
	// }

	if sInfoPost.AefProfiles[0].VendorSpecificUrnetsimeccapifexttransportInfo.Security == nil {
		sInfoPost.AefProfiles[0].VendorSpecificUrnetsimeccapifexttransportInfo.Security = &SecurityInfo{}
	}

	aefProfile := &AefProfile{
		AefId:                 sInfoPost.AefProfiles[0].AefId,
		Versions:              []Version{sInfoPost.AefProfiles[0].Versions[0]},
		InterfaceDescriptions: sInfoPost.AefProfiles[0].InterfaceDescriptions,
		VendorSpecificUrnetsimeccapifexttransportInfo: &MecTransportInfoCapifExt{
			Name:     sInfoPost.AefProfiles[0].VendorSpecificUrnetsimeccapifexttransportInfo.Name,
			Type_:    sInfoPost.AefProfiles[0].VendorSpecificUrnetsimeccapifexttransportInfo.Type_,
			Protocol: sInfoPost.AefProfiles[0].VendorSpecificUrnetsimeccapifexttransportInfo.Protocol,
			Version:  sInfoPost.AefProfiles[0].VendorSpecificUrnetsimeccapifexttransportInfo.Version,
			Security: sInfoPost.AefProfiles[0].VendorSpecificUrnetsimeccapifexttransportInfo.Security,
		},
	}
	dsInfo := &ServiceApiDescription{
		ApiName:     sInfoPost.ApiName,
		ApiId:       uuid.New().String(),
		AefProfiles: []AefProfile{*aefProfile},
		VendorSpecificUrnetsimeccapifextserviceInfo: &MecServiceInfoCapifExt{
			Serializer:        sInfoPost.VendorSpecificUrnetsimeccapifextserviceInfo.Serializer,
			State:             sInfoPost.VendorSpecificUrnetsimeccapifextserviceInfo.State,
			ScopeOfLocality:   sInfoPost.VendorSpecificUrnetsimeccapifextserviceInfo.ScopeOfLocality,
			ConsumedLocalOnly: sInfoPost.VendorSpecificUrnetsimeccapifextserviceInfo.ConsumedLocalOnly,
			IsLocal:           sInfoPost.VendorSpecificUrnetsimeccapifextserviceInfo.IsLocal,
			Category:          sInfoPost.VendorSpecificUrnetsimeccapifextserviceInfo.Category,
		},
	}

	transportInfo_ := TransportInfo{
		Id:       sInfoPost.AefProfiles[0].AefId,
		Name:     sInfoPost.AefProfiles[0].VendorSpecificUrnetsimeccapifexttransportInfo.Name,
		Type_:    sInfoPost.AefProfiles[0].VendorSpecificUrnetsimeccapifexttransportInfo.Type_,
		Protocol: sInfoPost.AefProfiles[0].VendorSpecificUrnetsimeccapifexttransportInfo.Protocol,
		Version:  sInfoPost.AefProfiles[0].VendorSpecificUrnetsimeccapifexttransportInfo.Version,
		Endpoint: ConvertInterfaceDescriptionsToEndpoint(sInfoPost.AefProfiles[0].InterfaceDescriptions),
		Security: sInfoPost.AefProfiles[0].VendorSpecificUrnetsimeccapifexttransportInfo.Security,
	}

	// Create Service
	sInfo := &ServiceInfo{
		SerInstanceId:     dsInfo.ApiId,
		SerName:           dsInfo.ApiName,
		SerCategory:       dsInfo.VendorSpecificUrnetsimeccapifextserviceInfo.Category,
		Version:           dsInfo.AefProfiles[0].Versions[0].APIVersion,
		State:             dsInfo.VendorSpecificUrnetsimeccapifextserviceInfo.State,
		TransportInfo:     &transportInfo_,
		Serializer:        dsInfo.VendorSpecificUrnetsimeccapifextserviceInfo.Serializer,
		ScopeOfLocality:   dsInfo.VendorSpecificUrnetsimeccapifextserviceInfo.ScopeOfLocality,
		ConsumedLocalOnly: dsInfo.VendorSpecificUrnetsimeccapifextserviceInfo.ConsumedLocalOnly,
		IsLocal:           dsInfo.VendorSpecificUrnetsimeccapifextserviceInfo.IsLocal,
		LivenessInterval:  0,
	}
	sInfo.Links = &ServiceInfoLinks{
		Self: &LinkType{
			Href: hostUrl.String() + basePath + "applications/" + appId + "/services/" + sInfo.SerInstanceId,
		},
	}
	if sInfo.LivenessInterval != 0 {
		sInfo.Links.Liveness = &LinkType{
			Href: hostUrl.String() + basePath + "resource_uri_allocated_by_MEC_platform/" + sInfo.SerInstanceId,
		}
	}

	err, retCode := setService(appId, sInfo, AVAILABLE)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), retCode)
		return
	}

	// Set Location Header
	updatedBasePath := strings.Replace(basePath, "mec_service_mgmt/v1", "published-apis/v1", 1)

	// Send response
	w.Header().Set("Location", hostUrl.String()+updatedBasePath+appId+"/service-apis/"+dsInfo.ApiId)
	w.WriteHeader(http.StatusCreated)
	fmt.Fprint(w, convertServiceInfoToJson_1(dsInfo))
}

func appServicesByIdPUT(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	log.Info("appServicesByIdPUT")
	vars := mux.Vars(r)
	svcId := vars["serviceApiId"]
	appId := vars["apfId"]

	mutex.Lock()
	defer mutex.Unlock()

	// Get App instance
	appInfo, err := getAppInfo(appId)
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

	// Get previous service info
	sInfoPrevJson, err := getServiceById(appId, svcId)
	if err != nil {
		log.Error(err.Error())
		w.WriteHeader(http.StatusNotFound)
		return
	}
	sInfoPrev := convertJsonToServiceInfo(sInfoPrevJson)

	// Retrieve request parameters from body
	if r.Body == nil {
		err := errors.New("Request body is missing")
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}
	// NOTE: Set default values for omitted fields
	locality := MEC_HOST_LocalityType
	sInfo := ServiceApiDescription{
		VendorSpecificUrnetsimeccapifextserviceInfo: &MecServiceInfoCapifExt{
			ScopeOfLocality:   &locality,
			IsLocal:           true,
			ConsumedLocalOnly: true,
		},
	}
	decoder := json.NewDecoder(r.Body)
	err = decoder.Decode(&sInfo)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}

	aefProfile := &AefProfile{
		AefId:                 sInfo.AefProfiles[0].AefId,
		Versions:              sInfo.AefProfiles[0].Versions,
		InterfaceDescriptions: sInfo.AefProfiles[0].InterfaceDescriptions,
		VendorSpecificUrnetsimeccapifexttransportInfo: &MecTransportInfoCapifExt{
			Name:     sInfo.AefProfiles[0].VendorSpecificUrnetsimeccapifexttransportInfo.Name,
			Type_:    sInfo.AefProfiles[0].VendorSpecificUrnetsimeccapifexttransportInfo.Type_,
			Protocol: sInfo.AefProfiles[0].VendorSpecificUrnetsimeccapifexttransportInfo.Protocol,
			Version:  sInfo.AefProfiles[0].VendorSpecificUrnetsimeccapifexttransportInfo.Version,
			Security: sInfo.AefProfiles[0].VendorSpecificUrnetsimeccapifexttransportInfo.Security,
		},
	}
	dsInfo := &ServiceApiDescription{
		ApiName:     sInfo.ApiName,
		ApiId:       sInfo.ApiId,
		AefProfiles: []AefProfile{*aefProfile},
		VendorSpecificUrnetsimeccapifextserviceInfo: &MecServiceInfoCapifExt{
			Serializer:        sInfo.VendorSpecificUrnetsimeccapifextserviceInfo.Serializer,
			State:             sInfo.VendorSpecificUrnetsimeccapifextserviceInfo.State,
			ScopeOfLocality:   sInfo.VendorSpecificUrnetsimeccapifextserviceInfo.ScopeOfLocality,
			ConsumedLocalOnly: sInfo.VendorSpecificUrnetsimeccapifextserviceInfo.ConsumedLocalOnly,
			IsLocal:           sInfo.VendorSpecificUrnetsimeccapifextserviceInfo.IsLocal,
			Category:          sInfo.VendorSpecificUrnetsimeccapifextserviceInfo.Category,
		},
	}

	transportInfo_ := TransportInfo{
		Id:       sInfo.AefProfiles[0].AefId,
		Name:     sInfo.AefProfiles[0].VendorSpecificUrnetsimeccapifexttransportInfo.Name,
		Type_:    sInfo.AefProfiles[0].VendorSpecificUrnetsimeccapifexttransportInfo.Type_,
		Protocol: sInfo.AefProfiles[0].VendorSpecificUrnetsimeccapifexttransportInfo.Protocol,
		Version:  sInfo.AefProfiles[0].VendorSpecificUrnetsimeccapifexttransportInfo.Version,
		Endpoint: ConvertInterfaceDescriptionsToEndpoint(sInfo.AefProfiles[0].InterfaceDescriptions),
		Security: sInfo.AefProfiles[0].VendorSpecificUrnetsimeccapifexttransportInfo.Security,
	}

	// Create Service
	_sInfo := ServiceInfo{
		SerInstanceId:     dsInfo.ApiId,
		SerName:           dsInfo.ApiName,
		SerCategory:       dsInfo.VendorSpecificUrnetsimeccapifextserviceInfo.Category,
		Version:           dsInfo.AefProfiles[0].Versions[0].APIVersion,
		State:             dsInfo.VendorSpecificUrnetsimeccapifextserviceInfo.State,
		TransportInfo:     &transportInfo_,
		Serializer:        dsInfo.VendorSpecificUrnetsimeccapifextserviceInfo.Serializer,
		ScopeOfLocality:   dsInfo.VendorSpecificUrnetsimeccapifextserviceInfo.ScopeOfLocality,
		ConsumedLocalOnly: dsInfo.VendorSpecificUrnetsimeccapifextserviceInfo.ConsumedLocalOnly,
		IsLocal:           dsInfo.VendorSpecificUrnetsimeccapifextserviceInfo.IsLocal,
		LivenessInterval:  0,
	}

	// Current implementation only supports state parameter change;
	state := *_sInfo.State
	*_sInfo.State = *sInfoPrev.State
	// isLocal is only set in responses, subscriptions and notifications;
	// Ignore this field while comparing the previous & new service info structs
	_sInfo.IsLocal = sInfoPrev.IsLocal

	// Compare service information as JSON strings
	/* FSCOM: It is not specified that only the ServiceInfo state property may be changed in ETSI GS MEC 011 V3.2.1 (2024-04)
	sInfoJson := convertServiceInfoToJson(&sInfo)
	if sInfoJson != sInfoPrevJson {
		errStr := "Only the ServiceInfo state property may be changed"
		log.Error(errStr)
		errHandlerProblemDetails(w, errStr, http.StatusBadRequest)
		return
	}*/

	// Compare service info states & update DB if necessary
	*_sInfo.State = state
	if *_sInfo.State != *sInfoPrev.State {
		err, retCode := setService(appId, &_sInfo, UPDATE)
		if err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), retCode)
			return
		}
	} else {
		err, retCode := setService(appId, &_sInfo, UPDATE)
		if err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), retCode)
			return
		}
	}

	// Compare LivenessInterval
	if _sInfo.LivenessInterval != sInfoPrev.LivenessInterval {
		if _, ok := livenessTimerList[_sInfo.SerInstanceId]; ok { // An entry already exist
			if _sInfo.LivenessInterval != 0 { // update it
				updateLivenessTicker(_sInfo)
			} else {
				deleteLivenessTicker(_sInfo.SerInstanceId)
			}
		} else { // No entry
			if _sInfo.LivenessInterval != 0 { // Create a new entry
				createLivenessTicker(_sInfo)
			}
		}
	} // else, nothing to do
	_sInfo.LivenessInterval = sInfoPrev.LivenessInterval

	// Send response
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, convertServiceInfoToJson_1(dsInfo))
}

func appServicesByIdPATCH(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	log.Info("appServicesByIdPUT")
	vars := mux.Vars(r)
	svcId := vars["serviceApiId"]
	appId := vars["apfId"]

	mutex.Lock()
	defer mutex.Unlock()

	// Get App instance
	appInfo, err := getAppInfo(appId)
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

	// Get previous service info
	sInfoPrevJson, err := getServiceById(appId, svcId)
	if err != nil {
		log.Error(err.Error())
		w.WriteHeader(http.StatusNotFound)
		return
	}
	sInfoPrev := convertJsonToServiceInfo(sInfoPrevJson)

	// Retrieve request parameters from body
	if r.Body == nil {
		err := errors.New("Request body is missing")
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}
	// NOTE: Set default values for omitted fields
	locality := MEC_HOST_LocalityType
	sInfo := ServiceApiDescriptionPatch{
		VendorSpecificUrnetsimeccapifextserviceInfo: &MecServiceInfoCapifExtPatch{
			ScopeOfLocality:   &locality,
			IsLocal:           true,
			ConsumedLocalOnly: true,
		},
	}
	decoder := json.NewDecoder(r.Body)
	err = decoder.Decode(&sInfo)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}
	aefProfile := &AefProfile{
		AefId:                 sInfo.AefProfiles[0].AefId,
		Versions:              sInfo.AefProfiles[0].Versions,
		InterfaceDescriptions: sInfo.AefProfiles[0].InterfaceDescriptions,
		VendorSpecificUrnetsimeccapifexttransportInfo: &MecTransportInfoCapifExt{
			Name:     sInfo.AefProfiles[0].VendorSpecificUrnetsimeccapifexttransportInfo.Name,
			Type_:    sInfo.AefProfiles[0].VendorSpecificUrnetsimeccapifexttransportInfo.Type_,
			Protocol: sInfo.AefProfiles[0].VendorSpecificUrnetsimeccapifexttransportInfo.Protocol,
			Version:  sInfo.AefProfiles[0].VendorSpecificUrnetsimeccapifexttransportInfo.Version,
			Security: sInfo.AefProfiles[0].VendorSpecificUrnetsimeccapifexttransportInfo.Security,
		},
	}
	dsInfo := &ServiceApiDescriptionPatch{
		AefProfiles: []AefProfile{*aefProfile},
		VendorSpecificUrnetsimeccapifextserviceInfo: &MecServiceInfoCapifExtPatch{
			Serializer:        sInfo.VendorSpecificUrnetsimeccapifextserviceInfo.Serializer,
			State:             sInfo.VendorSpecificUrnetsimeccapifextserviceInfo.State,
			ScopeOfLocality:   sInfo.VendorSpecificUrnetsimeccapifextserviceInfo.ScopeOfLocality,
			ConsumedLocalOnly: sInfo.VendorSpecificUrnetsimeccapifextserviceInfo.ConsumedLocalOnly,
			IsLocal:           sInfo.VendorSpecificUrnetsimeccapifextserviceInfo.IsLocal,
			Category:          sInfo.VendorSpecificUrnetsimeccapifextserviceInfo.Category,
		},
	}

	transportInfo_ := TransportInfo{
		Id:       sInfo.AefProfiles[0].AefId,
		Name:     sInfo.AefProfiles[0].VendorSpecificUrnetsimeccapifexttransportInfo.Name,
		Type_:    sInfo.AefProfiles[0].VendorSpecificUrnetsimeccapifexttransportInfo.Type_,
		Protocol: sInfo.AefProfiles[0].VendorSpecificUrnetsimeccapifexttransportInfo.Protocol,
		Version:  sInfo.AefProfiles[0].VendorSpecificUrnetsimeccapifexttransportInfo.Version,
		Endpoint: ConvertInterfaceDescriptionsToEndpoint(sInfo.AefProfiles[0].InterfaceDescriptions),
		Security: sInfo.AefProfiles[0].VendorSpecificUrnetsimeccapifexttransportInfo.Security,
	}
	// Create Service
	_sInfo := ServiceInfo{
		SerInstanceId:     sInfoPrev.SerInstanceId,
		SerName:           sInfoPrev.SerName,
		Version:           dsInfo.AefProfiles[0].Versions[0].APIVersion,
		SerCategory:       dsInfo.VendorSpecificUrnetsimeccapifextserviceInfo.Category,
		State:             dsInfo.VendorSpecificUrnetsimeccapifextserviceInfo.State,
		TransportInfo:     &transportInfo_,
		Serializer:        dsInfo.VendorSpecificUrnetsimeccapifextserviceInfo.Serializer,
		ScopeOfLocality:   dsInfo.VendorSpecificUrnetsimeccapifextserviceInfo.ScopeOfLocality,
		ConsumedLocalOnly: dsInfo.VendorSpecificUrnetsimeccapifextserviceInfo.ConsumedLocalOnly,
		IsLocal:           dsInfo.VendorSpecificUrnetsimeccapifextserviceInfo.IsLocal,
		LivenessInterval:  0,
	}

	// Current implementation only supports state parameter change;
	_sInfo.IsLocal = sInfoPrev.IsLocal

	// Compare service information as JSON strings
	/* FSCOM: It is not specified that only the ServiceInfo state property may be changed in ETSI GS MEC 011 V3.2.1 (2024-04)
	sInfoJson := convertServiceInfoToJson(&sInfo)
	if sInfoJson != sInfoPrevJson {
		errStr := "Only the ServiceInfo state property may be changed"
		log.Error(errStr)
		errHandlerProblemDetails(w, errStr, http.StatusBadRequest)
		return
	}*/

	// Compare service info states & update DB if necessary
	// *_sInfo.State = state
	if *_sInfo.State != *sInfoPrev.State {
		err, retCode := setService(appId, &_sInfo, UPDATE)
		if err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), retCode)
			return
		}
	} else {
		err, retCode := setService(appId, &_sInfo, UPDATE)
		if err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), retCode)
			return
		}
	}

	// Compare LivenessInterval
	if _sInfo.LivenessInterval != sInfoPrev.LivenessInterval {
		if _, ok := livenessTimerList[_sInfo.SerInstanceId]; ok { // An entry already exist
			if _sInfo.LivenessInterval != 0 { // update it
				updateLivenessTicker(_sInfo)
			} else {
				deleteLivenessTicker(_sInfo.SerInstanceId)
			}
		} else { // No entry
			if _sInfo.LivenessInterval != 0 { // Create a new entry
				createLivenessTicker(_sInfo)
			}
		}
	} // else, nothing to do
	_sInfo.LivenessInterval = sInfoPrev.LivenessInterval

	// Map ServiceInfoList to ServiceApiDescription list
	serviceApiDescriptions := make([]ServiceApiDescription, 0)

	service := _sInfo

	// Create the ServiceApiDescription and populate it with data from ServiceInfo
	apiDesc := ServiceApiDescription{
		ApiName:     service.SerName,       // Map SerName to ApiName
		ApiId:       service.SerInstanceId, // Map SerInstanceId to ApiId
		AefProfiles: dsInfo.AefProfiles,
		VendorSpecificUrnetsimeccapifextserviceInfo: &MecServiceInfoCapifExt{
			Serializer:        service.Serializer,
			State:             service.State,
			ScopeOfLocality:   service.ScopeOfLocality,
			ConsumedLocalOnly: service.ConsumedLocalOnly,
			IsLocal:           service.IsLocal,
			Category:          service.SerCategory,
		},
	}

	// Add the created apiDesc to the serviceApiDescriptions slice
	serviceApiDescriptions = append(serviceApiDescriptions, apiDesc)

	// Prepare & send response
	jsonResponse, err := json.Marshal(serviceApiDescriptions)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(jsonResponse))
}

func appServicesByIdDELETE(w http.ResponseWriter, r *http.Request) {
	log.Info("appServicesByIdDELETE")
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	vars := mux.Vars(r)
	svcId := vars["serviceApiId"]
	appId := vars["apfId"]

	mutex.Lock()
	defer mutex.Unlock()

	// Get App instance
	appInfo, err := getAppInfo(appId)
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

	// Get service info
	sInfoJson, err := getServiceById(appId, svcId)
	if err != nil {
		log.Error(err.Error())
		w.WriteHeader(http.StatusNotFound)
		return
	}
	sInfo := convertJsonToServiceInfo(sInfoJson)

	// Delete service
	err = delServiceById(appId, svcId)
	if err != nil {
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Notify remote listeners (except if global instance)
	changeType := UNAVAILABLE
	if mepName != globalLocationName {
		sendSvcUpdateMsg(sInfoJson, appId, mepName, string(changeType))
	}

	// Send local service availability notifications
	checkSerAvailNotification(sInfo, mepName, changeType)

	w.WriteHeader(http.StatusNoContent)
}

func appServicesGET(w http.ResponseWriter, r *http.Request) {
	log.Info("appServicesGET")
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	vars := mux.Vars(r)
	appId := vars["apfId"]

	mutex.Lock()
	defer mutex.Unlock()

	// Get App instance
	appInfo, err := getAppInfoAnyMep(appId)
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

	getServices(w, r, appId)
}

func appServicesByIdGET(w http.ResponseWriter, r *http.Request) {
	log.Info("appServicesByIdGET")
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	vars := mux.Vars(r)
	svcId := vars["serviceApiId"]
	appId := vars["apfId"]

	mutex.Lock()
	defer mutex.Unlock()

	// Get App instance
	appInfo, err := getAppInfoAnyMep(appId)
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

	getService(w, r, appId, svcId)
}

func servicesByIdGET(w http.ResponseWriter, r *http.Request) {
	log.Info("servicesByIdGET")
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	vars := mux.Vars(r)
	svcId := vars["serviceId"]

	mutex.Lock()
	defer mutex.Unlock()

	getService(w, r, "", svcId)
}

func servicesGET(w http.ResponseWriter, r *http.Request) {
	log.Info("servicesGET")
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	mutex.Lock()
	defer mutex.Unlock()
	// Validate query parameters
	u, _ := url.Parse(r.URL.String())
	q := u.Query()

	// Extract and parse query parameters
	api_invoker_id := q.Get("api-invoker-id")
	if api_invoker_id != "" {
		validParams := []string{"api-invoker-id"}
		err := validateQueryParams(q, validParams)
		if err != nil {
			errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
			return
		}
		getServices(w, r, api_invoker_id)
	} else {
		getServices(w, r, "")
	}
}

func applicationsSubscriptionsPATCH(w http.ResponseWriter, r *http.Request) {
	log.Info("applicationsSubscriptionsPATCH")
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	vars := mux.Vars(r)
	subId := vars["subscriptionId"]
	appId := vars["subscriberId"]

	mutex.Lock()
	defer mutex.Unlock()

	// Get App instance
	appInfo, err := getAppInfo(appId)
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

	// Retrieve the existing subscription
	sub, err := subMgr.GetSubscription(subId)
	if err != nil {
		log.Error("Subscription not found: ", err.Error())
		errHandlerProblemDetails(w, "Subscription not found", http.StatusNotFound)
		return
	}

	// Retrieve the patch request body
	if r.Body == nil {
		err := errors.New("Request body is missing")
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}

	var patchSub EventSubscriptionPatch
	decoder := json.NewDecoder(r.Body)
	err = decoder.Decode(&patchSub)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, "Failed to decode request body", http.StatusInternalServerError)
		return
	}

	// Apply partial updates
	if patchSub.NotificationDestination != "" {
		sub.Cfg.NotifyUrl = patchSub.NotificationDestination
	}

	if patchSub.EventFilters != nil {
		sub.Cfg.EventFilters = convertEventFiltersToStrings(patchSub.EventFilters)
	}

	if patchSub.Events != nil {
		sub.Cfg.CapifEvent = convertEventToStrings(patchSub.Events)
	}

	// Update the subscription in the manager
	err = subMgr.UpdateSubscription(sub)
	if err != nil {
		log.Error("Failed to update subscription: ", err.Error())
		errHandlerProblemDetails(w, "Failed to update subscription", http.StatusInternalServerError)
		return
	}

	// Convert the updated subscription to an EventSubscription struct
	updatedSub := EventSubscription{
		NotificationDestination: sub.Cfg.NotifyUrl,
		EventFilters:            convertStringsToEventFilters(sub.Cfg.EventFilters),
		Events:                  convertStringsToEvents(sub.Cfg.CapifEvent),
	}

	// Convert the updated EventSubscription to JSON format for the response
	jsonSub := convertSerAvailabilityNotifSubToJson_1(&updatedSub)

	// Send the full updated subscription in the response
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, jsonSub)
}

func applicationsSubscriptionsPUT(w http.ResponseWriter, r *http.Request) {
	log.Info("applicationsSubscriptionsPUT")
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	vars := mux.Vars(r)
	subId := vars["subscriptionId"]
	appId := vars["subscriberId"]

	mutex.Lock()
	defer mutex.Unlock()

	// Get App instance
	appInfo, err := getAppInfo(appId)
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
	sub, err := subMgr.GetSubscription(subId)
	if err != nil {
		log.Error("Subscription not found: ", err.Error())
		errHandlerProblemDetails(w, "Subscription not found", http.StatusNotFound)
		return
	}
	// Retrieve the update request
	if r.Body == nil {
		err := errors.New("Request body is missing")
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}
	var updatedSub EventSubscription
	decoder := json.NewDecoder(r.Body)
	err = decoder.Decode(&updatedSub)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, "Failed to decode request body", http.StatusInternalServerError)
		return
	}
	if updatedSub.NotificationDestination == "" {
		log.Error("NotificationDestination is required for PUT")
		errHandlerProblemDetails(w, "NotificationDestination is required", http.StatusBadRequest)
		return
	}

	// Assume `EventFilters` and `Events` are mandatory for `PUT` as well
	if updatedSub.EventFilters == nil || len(updatedSub.EventFilters) == 0 {
		log.Error("EventFilters are required for PUT")
		errHandlerProblemDetails(w, "EventFilters are required", http.StatusBadRequest)
		return
	}

	if updatedSub.Events == nil || len(updatedSub.Events) == 0 {
		log.Error("Events are required for PUT")
		errHandlerProblemDetails(w, "Events are required", http.StatusBadRequest)
		return
	}
	// Update the subscription object
	if updatedSub.NotificationDestination != "" {
		sub.Cfg.NotifyUrl = updatedSub.NotificationDestination
	}

	// Update event filters if provided
	if updatedSub.EventFilters != nil {
		sub.Cfg.EventFilters = convertEventFiltersToStrings(updatedSub.EventFilters)
	}
	if updatedSub.Events != nil {
		sub.Cfg.CapifEvent = convertEventToStrings(updatedSub.Events)
	}
	// Update the subscription in the manager
	err = subMgr.UpdateSubscription(sub)
	if err != nil {
		log.Error("Failed to update subscription: ", err.Error())
		errHandlerProblemDetails(w, "Failed to update subscription", http.StatusInternalServerError)
		return
	}
	jsonSub := convertSerAvailabilityNotifSubToJson_1(&updatedSub)
	// Send success response
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, jsonSub)
}

var HrefUrl string

func applicationsSubscriptionsPOST(w http.ResponseWriter, r *http.Request) {
	log.Info("applicationsSubscriptionsPOST")

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	vars := mux.Vars(r)
	appId := vars["subscriberId"]

	mutex.Lock()
	defer mutex.Unlock()

	// Get App instance
	appInfo, err := getAppInfo(appId)
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

	// Retrieve subscription request
	if r.Body == nil {
		err := errors.New("Request body is missing")
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}
	var serAvailNotifSub EventSubscription
	decoder := json.NewDecoder(r.Body)
	err = decoder.Decode(&serAvailNotifSub)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Validate mandatory properties
	if serAvailNotifSub.NotificationDestination == "" {
		log.Error("Mandatory NotificationDestination parameter not present")
		errHandlerProblemDetails(w, "Mandatory NotificationDestination parameter not present", http.StatusBadRequest)
		return
	}

	// Validate Service filter params
	if serAvailNotifSub.EventFilters != nil {
		nbMutuallyExclusiveParams := 0
		for _, filter := range serAvailNotifSub.EventFilters {
			if len(filter.ApiIds) == 0 {
				log.Error("Mandatory ApiIds(serInstanceIds) parameter not present")
				errHandlerProblemDetails(w, "Mandatory ApiIds(serInstanceIds) parameter not present", http.StatusBadRequest)
				return
			}
			if filter.ApiIds != nil && len(filter.ApiIds) > 0 {
				nbMutuallyExclusiveParams++
			}
		}
		if nbMutuallyExclusiveParams > 1 {
			errStr := "FilteringCriteria attributes serInstanceIds, serNames, serCategories are mutually-exclusive"
			log.Error(errStr)
			errHandlerProblemDetails(w, errStr, http.StatusBadRequest)
			return
		}
	}

	// Get a new subscription ID
	subId := subMgr.GenerateSubscriptionId()

	// Create & store subscription
	subCfg := newSerAvailabilityNotifSubCfg_1(&serAvailNotifSub, subId, appId)
	jsonSub := convertSerAvailabilityNotifSubToJson_1(&serAvailNotifSub)
	_, err = subMgr.CreateSubscription(subCfg, jsonSub)
	if err != nil {
		log.Error("Failed to create subscription")
		errHandlerProblemDetails(w, "Failed to create subscription", http.StatusInternalServerError)
		return
	}

	// Set Location Header
	updatedBasePath := strings.Replace(basePath, "mec_service_mgmt/v1", capifEventsBasePath, 1)
	HrefUrl := hostUrl.String() + updatedBasePath + appId + "/subscriptions/" + subId

	// Send response
	w.Header().Set("Location", HrefUrl)
	w.WriteHeader(http.StatusCreated)
	fmt.Fprint(w, jsonSub)
}

func applicationsSubscriptionGET(w http.ResponseWriter, r *http.Request) {
	log.Info("applicationsSubscriptionGET")

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	vars := mux.Vars(r)
	subId := vars["subscriptionId"]
	appId := vars["appInstanceId"]

	mutex.Lock()
	defer mutex.Unlock()

	// Get App instance info
	appInfo, err := getAppInfo(appId)
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
	// Validate subscription
	if sub.Cfg.AppId != appId || sub.Cfg.Type != SER_AVAILABILITY_NOTIF_SUB_TYPE {
		err = errors.New("Subscription not found")
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusNotFound)
		return
	}

	// Return original marshalled subscription
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, sub.JsonSubOrig)
}

func applicationsSubscriptionDELETE(w http.ResponseWriter, r *http.Request) {
	log.Info("applicationsSubscriptionDELETE")

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	vars := mux.Vars(r)
	subId := vars["subscriptionId"]
	appId := vars["subscriberId"]

	mutex.Lock()
	defer mutex.Unlock()

	// Get App instance info
	appInfo, err := getAppInfo(appId)
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
	if sub.Cfg.AppId != appId {
		err = errors.New("Subscription not found")
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
	log.Info("applicationsSubscriptionsGET")

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	vars := mux.Vars(r)
	appId := vars["appInstanceId"]

	mutex.Lock()
	defer mutex.Unlock()

	// Get App instance info
	appInfo, err := getAppInfo(appId)
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

	// Get subscriptions for App instance
	subList, err := subMgr.GetFilteredSubscriptions(appId, SER_AVAILABILITY_NOTIF_SUB_TYPE)
	if err != nil {
		log.Error("Failed to get subscription list with err: ", err.Error())
		return
	}

	// Create subscription link list
	subscriptionLinkList := &SubscriptionLinkList{
		Links: &SubscriptionLinkListLinks{
			Self: &LinkType{
				Href: hostUrl.String() + basePath + "applications/" + appId + "/subscriptions",
			},
		},
	}

	for _, sub := range subList {
		// Create subscription reference & append it to link list
		subscription := SubscriptionLinkListLinksSubscriptions{
			// In v2.1.1 it should be SubscriptionType, but spec is expecting "rel" as per v1.1.1
			SubscriptionType: sub.Cfg.Type,
			Href:             sub.Cfg.Self,
		}
		subscriptionLinkList.Links.Subscriptions = append(subscriptionLinkList.Links.Subscriptions, subscription)
	}

	// Send response
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, convertSubscriptionLinkListToJson(subscriptionLinkList))
}

func getIndividualMECService(w http.ResponseWriter, r *http.Request) {
	log.Info("getIndividualMECService")

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	vars := mux.Vars(r)
	serInstanceId := vars["serInstanceId"]

	mutex.Lock()
	defer mutex.Unlock()

	if serInstanceId == "" {
		err := errors.New("wrong request parameters")
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}
	log.Info("getIndividualMECService: ", serInstanceId)

	if entry, ok := livenessTimerList[serInstanceId]; !ok {
		err := errors.New("Invalid Service instance ID")
		errHandlerProblemDetails(w, err.Error(), http.StatusNotFound)
		return
	} else {
		entry.TimeStamp = &ServiceLivenessInfoTimeStamp{
			Seconds: int32(time.Now().Unix()),
		}
		fmt.Fprint(w, convertServiceLivenessInfoToJson(&entry))
		livenessTimerList[serInstanceId] = entry
	}

	// Send response
	w.WriteHeader(http.StatusOK)
}

func patchIndividualMECService(w http.ResponseWriter, r *http.Request) {
	log.Info("patchIndividualMECService")

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	vars := mux.Vars(r)
	serInstanceId := vars["serInstanceId"]

	mutex.Lock()
	defer mutex.Unlock()

	if serInstanceId == "" {
		err := errors.New("wrong request parameters")
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}
	log.Info("patchIndividualMECService: ", serInstanceId)

	if entry, ok := livenessTimerList[serInstanceId]; !ok {
		err := errors.New("Invalid Service instance ID")
		errHandlerProblemDetails(w, err.Error(), http.StatusNotFound)
		return
	} else {
		// Retrieve request
		if r.Body == nil {
			err := errors.New("Request body is missing")
			errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
			return
		}
		var serviceLivenessUpdate ServiceLivenessUpdate
		decoder := json.NewDecoder(r.Body)
		err := decoder.Decode(&serviceLivenessUpdate)
		if err != nil {
			log.Error(err.Error())
			errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
			return
		}
		log.Info("patchIndividualMECService: serviceLivenessUpdate: ", serviceLivenessUpdate)
		if *serviceLivenessUpdate.State == ACTIVE_ServiceState {
			entry.State = &ACTIVE_ServiceState
		} else { // ETSI GS MEC 011 V3.2.1 (2024-04) Table 8.1.2.5-1: Attributes of ServiceLivenessUpdate
			err := errors.New("Wrong body content")
			errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
			return
		}
		entry.TimeStamp = &ServiceLivenessInfoTimeStamp{
			Seconds: 0,
		}
		fmt.Fprint(w, convertServiceLivenessInfoToJson(&entry))
		livenessTimerList[serInstanceId] = entry
	}

	// Send response
	w.WriteHeader(http.StatusOK)
}

func transportsGET(w http.ResponseWriter, r *http.Request) {
	log.Info("transportsGET")
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	// Create transport info
	var endpoint OneOfTransportInfoEndpoint
	endpoint.Uris = append(endpoint.Uris, hostUrl.String()+basePath)
	transportType := REST_HTTP_TransportType
	transportInfo := TransportInfo{
		Id:       "sandboxTransport",
		Name:     "REST",
		Type_:    &transportType,
		Protocol: "HTTP",
		Version:  "2.0",
		Endpoint: &endpoint,
	}
	var transportInfoResp []TransportInfo
	transportInfoResp = append(transportInfoResp, transportInfo)

	// Prepare & send response
	jsonResponse, err := json.Marshal(transportInfoResp)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(jsonResponse))
}

// Delete App services subscriptions
func DeleteServiceSubscriptions(appId string) error {
	log.Info("DeleteServiceSubscriptions: ", appId)

	// Get App instance info
	appInfo, err := getAppInfo(appId)
	if err != nil {
		log.Error(err.Error())
		return err
	}

	// Validate App info
	_, _, err = validateAppInfo(appInfo)
	if err != nil {
		log.Error(err.Error())
		return err
	}

	// Delete app support subscriptions
	err = subMgr.DeleteFilteredSubscriptions(appId, SER_AVAILABILITY_NOTIF_SUB_TYPE)
	if err != nil {
		log.Error(err.Error())
		return err
	}
	return nil
}

// Delete App services
func DeleteServices(appId string) error {
	log.Debug(">>> DeleteServices: ", appId)

	// Get App instance info
	appInfo, err := getAppInfo(appId)
	if err != nil {
		log.Error(err.Error())
		return err
	}

	// Validate App info
	_, _, err = validateAppInfo(appInfo)
	if err != nil {
		log.Error(err.Error())
		return err
	}

	// Get Service list
	key := baseKey + "app:" + appId + ":svc:*"
	err = rc.ForEachJSONEntry(key, deleteService, appId)
	if err != nil {
		log.Error(err.Error())
		return err
	}
	return nil
}

func deleteService(key string, sInfoJson string, data interface{}) error {
	log.Debug(">>> DeleteService: key: ", key)
	log.Debug(">>> DeleteService: sInfoJson: ", sInfoJson)

	// Get App instance ID from user data
	appId := data.(string)
	if appId == "" {
		return errors.New("appInstanceId not found")
	}

	// Delete entry
	err := rc.JSONDelEntry(key, ".")
	if err != nil {
		log.Error(err.Error())
		return err
	}

	// Get service information
	sInfo := convertJsonToServiceInfo(sInfoJson)

	// Notify remote listeners (except if global instance)
	changeType := UNAVAILABLE
	if mepName != globalLocationName {
		sendSvcUpdateMsg(sInfoJson, appId, mepName, string(changeType))
	}

	// Send local service availability notifications
	checkSerAvailNotification(sInfo, mepName, changeType)

	return nil
}

func delServiceById(appId string, svcId string) error {
	key := baseKey + "app:" + appId + ":svc:" + svcId
	err := rc.JSONDelEntry(key, ".")
	if err != nil {
		return err
	}

	// Delete Liveness timer is any
	if _, ok := livenessTimerList[svcId]; ok {
		deleteLivenessTicker(svcId)
	}

	return nil
}

func setService(appId string, sInfo *ServiceInfo, changeType CapifEvent) (err error, retCode int) {
	// Create/update service
	sInfoJson := convertServiceInfoToJson(sInfo)
	key := baseKey + "app:" + appId + ":svc:" + sInfo.SerInstanceId
	err = rc.JSONSetEntry(key, ".", sInfoJson)
	if err != nil {
		return err, http.StatusInternalServerError
	}

	// Notify remote listeners (except if global instance)
	if mepName != globalLocationName {
		sendSvcUpdateMsg(sInfoJson, appId, mepName, string(changeType))
	}

	// Send local service availability notifications
	checkSerAvailNotification(sInfo, mepName, changeType)

	// Set Liveness mechanism if required
	if sInfo.LivenessInterval == 0 { // Liveness interval was ommitted
		if _, ok := livenessTimerList[sInfo.SerInstanceId]; ok {
			deleteLivenessTicker(sInfo.SerInstanceId)
		}
	} else { // Liveness interval was set
		if _, ok := livenessTimerList[sInfo.SerInstanceId]; ok { // An entry already exist, update it
			updateLivenessTicker(*sInfo)
		} else { // Create new entry
			createLivenessTicker(*sInfo)
		}
	}

	return nil, http.StatusOK
}

func getServiceById(appId string, svcId string) (string, error) {
	key := baseKey + "app:" + appId + ":svc:" + svcId
	sInfoJson, err := rc.JSONGetEntry(key, ".")
	if err != nil {
		return "", err
	}
	if sInfoJson == "" {
		return "", errors.New("Service info not found")
	}
	return sInfoJson, nil
}

type QueryParam struct {
	Target string `json:"target"`
	Value  string `json:"value"`
}

func parseJSONQueryParam(param string) (QueryParam, error) {
	var qp QueryParam
	err := json.Unmarshal([]byte(param), &qp)
	return qp, err
}
func getServices(w http.ResponseWriter, r *http.Request, appId string) {
	// Validate query parameters
	u, _ := url.Parse(r.URL.String())
	q := u.Query()
	validParams := []string{"api-invoker-id", "vend-spec-etsi-mec-serinstance-id", "api-name", "vend-spec-etsi-mec-sercategory-id", "vend-spec-etsi-mec-consumed-local-only", "vend-spec-etsi-mec-is-local", "vend-spec-etsi-mec-scope-of-locality"}
	err := validateQueryParams(q, validParams)
	if err != nil {
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Extract and parse query parameters
	serInstanceIdParam := q.Get("vend-spec-etsi-mec-serinstance-id")
	var serInstanceId []string
	if serInstanceIdParam != "" {
		parsedParam, err := parseJSONQueryParam(serInstanceIdParam)
		if err != nil {
			errHandlerProblemDetails(w, "Invalid JSON in ser_instance_id", http.StatusBadRequest)
			return
		}
		serInstanceId = append(serInstanceId, parsedParam.Value)
	}

	serName := q["api-name"]

	serCategoryIdParam := q.Get("vend-spec-etsi-mec-sercategory-id")
	var serCategoryId string
	if serCategoryIdParam != "" {
		parsedParam, err := parseJSONQueryParam(serCategoryIdParam)
		if err != nil {
			errHandlerProblemDetails(w, "Invalid JSON in ser_category_id", http.StatusBadRequest)
			return
		}
		serCategoryId = parsedParam.Value
	}

	consumedLocalOnlyParam := q.Get("vend-spec-etsi-mec-consumed-local-only")
	var consumedLocalOnly bool
	var consumedLocalOnlyPresent bool
	if consumedLocalOnlyParam != "" {
		parsedParam, err := parseJSONQueryParam(consumedLocalOnlyParam)
		if err != nil {
			errHandlerProblemDetails(w, "Invalid JSON in consumed_local_only", http.StatusBadRequest)
			return
		}
		consumedLocalOnly, err = strconv.ParseBool(parsedParam.Value)
		consumedLocalOnlyPresent = true
		if err != nil {
			consumedLocalOnly = false
			consumedLocalOnlyPresent = false
		}

	}

	isLocalParam := q.Get("vend-spec-etsi-mec-is-local")
	var isLocal bool
	var isLocalPresent bool
	if isLocalParam != "" {
		parsedParam, err := parseJSONQueryParam(isLocalParam)
		if err != nil {
			errHandlerProblemDetails(w, "Invalid JSON in is_local", http.StatusBadRequest)
			return
		}
		isLocal, err = strconv.ParseBool(parsedParam.Value)
		isLocalPresent = true
		if err != nil {
			isLocal = false
			isLocalPresent = false
		}
	}

	scopeOfLocalityParam := q.Get("vend-spec-etsi-mec-scope-of-locality")
	var scopeOfLocality string
	if scopeOfLocalityParam != "" {
		parsedParam, err := parseJSONQueryParam(scopeOfLocalityParam)
		if err != nil {
			errHandlerProblemDetails(w, "Invalid JSON in scope_of_locality", http.StatusBadRequest)
			return
		}
		scopeOfLocality = parsedParam.Value
	}

	// Make sure only 1 or none of the following are present: ser_instance_id, ser_name, ser_category_id
	err = validateServiceQueryParams(serInstanceId, serName, serCategoryId)
	if err != nil {
		errHandlerProblemDetails(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Retrieve all matching services
	sInfoList := &ServiceInfoList{
		ConsumedLocalOnlyPresent: consumedLocalOnlyPresent,
		IsLocalPresent:           isLocalPresent,
		Filters: &FilterParameters{
			serInstanceId:     serInstanceId,
			serName:           serName,
			serCategoryId:     serCategoryId,
			consumedLocalOnly: consumedLocalOnly,
			isLocal:           isLocal,
			scopeOfLocality:   scopeOfLocality,
		},
		Services: make([]ServiceInfo, 0),
	}

	var key string
	if appId == "" {
		key = baseKeyAnyMep + "app:*:svc:*"
	} else {
		key = baseKeyAnyMep + "app:" + appId + ":svc:*"
	}

	err = rc.ForEachJSONEntry(key, populateServiceInfoList, sInfoList)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Map ServiceInfoList to ServiceApiDescription list
	serviceApiDescriptions := make([]ServiceApiDescription, 0)
	for _, service := range sInfoList.Services {
		aefProfile := AefProfile{
			AefId:                 service.TransportInfo.Id,
			Versions:              []Version{{APIVersion: service.Version}},
			InterfaceDescriptions: ConvertEndpointToInterfaceDescriptions(service.TransportInfo.Endpoint),
			VendorSpecificUrnetsimeccapifexttransportInfo: &MecTransportInfoCapifExt{
				Name:     service.TransportInfo.Name,
				Type_:    service.TransportInfo.Type_,
				Protocol: service.TransportInfo.Protocol,
				Version:  service.TransportInfo.Version,
				Security: service.TransportInfo.Security,
			},
		}
		apiDesc := ServiceApiDescription{
			ApiName:     service.SerName,
			ApiId:       service.SerInstanceId,
			AefProfiles: []AefProfile{aefProfile},
			VendorSpecificUrnetsimeccapifextserviceInfo: &MecServiceInfoCapifExt{
				Serializer:        service.Serializer,
				State:             service.State,
				ScopeOfLocality:   service.ScopeOfLocality,
				ConsumedLocalOnly: service.ConsumedLocalOnly,
				IsLocal:           service.IsLocal,
				Category:          service.SerCategory,
			},
		}
		serviceApiDescriptions = append(serviceApiDescriptions, apiDesc)
	}
	// Store the response in the DiscoveredAPIs struct
	discoveredAPIs := DiscoveredAPIs{
		ServiceAPIDescriptions: serviceApiDescriptions,
	}
	// Prepare & send response
	jsonResponse, err := json.Marshal(discoveredAPIs)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(jsonResponse))
}

func getService(w http.ResponseWriter, r *http.Request, appId string, serviceId string) {
	// Validate input params
	if serviceId == "" {
		errStr := "Invalid Service ID"
		log.Error(errStr)
		errHandlerProblemDetails(w, errStr, http.StatusInternalServerError)
		return
	}

	// Retrieve all matching services
	var sInfoList ServiceInfoList

	var key string
	if appId == "" {
		key = baseKeyAnyMep + "app:*:svc:" + serviceId
	} else {
		key = baseKeyAnyMep + "app:" + appId + ":svc:" + serviceId
	}

	err := rc.ForEachJSONEntry(key, populateServiceInfoList, &sInfoList)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Validate result
	if len(sInfoList.Services) == 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Map ServiceInfoList to ServiceApiDescription list
	serviceApiDescriptions := make([]ServiceApiDescription, 0)
	for _, service := range sInfoList.Services {
		aefProfile := AefProfile{
			AefId:                 service.TransportInfo.Id,
			Versions:              []Version{{APIVersion: service.Version}},
			InterfaceDescriptions: ConvertEndpointToInterfaceDescriptions(service.TransportInfo.Endpoint),
			VendorSpecificUrnetsimeccapifexttransportInfo: &MecTransportInfoCapifExt{
				Name:     service.TransportInfo.Name,
				Type_:    service.TransportInfo.Type_,
				Protocol: service.TransportInfo.Protocol,
				Version:  service.TransportInfo.Version,
				Security: service.TransportInfo.Security,
			},
		}
		apiDesc := ServiceApiDescription{
			ApiName:     service.SerName,
			ApiId:       service.SerInstanceId,
			AefProfiles: []AefProfile{aefProfile},
			VendorSpecificUrnetsimeccapifextserviceInfo: &MecServiceInfoCapifExt{
				Serializer:        service.Serializer,
				State:             service.State,
				ScopeOfLocality:   service.ScopeOfLocality,
				ConsumedLocalOnly: service.ConsumedLocalOnly,
				IsLocal:           service.IsLocal,
				Category:          service.SerCategory,
			},
		}
		serviceApiDescriptions = append(serviceApiDescriptions, apiDesc)
	}

	// Prepare & send response
	jsonResponse, err := json.Marshal(serviceApiDescriptions)
	if err != nil {
		log.Error(err.Error())
		errHandlerProblemDetails(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(jsonResponse))
}

func populateServiceInfoList(key string, jsonInfo string, sInfoList interface{}) error {
	// Get query params & userlist from user data
	data := sInfoList.(*ServiceInfoList)
	if data == nil {
		return errors.New("ServiceInfoList not found")
	}

	// Retrieve user info from DB
	var sInfo ServiceInfo
	err := json.Unmarshal([]byte(jsonInfo), &sInfo)
	if err != nil {
		return err
	}

	// Set IsLocal flag
	if mepName == globalLocationName {
		sInfo.IsLocal = true
	} else {
		// Get service MEP Name
		mep := getLocationNameFromKey(key)

		// Check if service is local
		if *sInfo.ScopeOfLocality == MEC_SYSTEM_LocalityType || (mep != "" && mep == mepName) {
			sInfo.IsLocal = true
		} else {
			sInfo.IsLocal = false
		}
	}

	// Filter out non-local services with "consumedLocalOnly" flag set to "true"
	if !sInfo.IsLocal && sInfo.ConsumedLocalOnly {
		return nil
	}

	// Filter services
	if data.Filters != nil {

		// Service instance ID
		if len(data.Filters.serInstanceId) > 0 {
			found := false
			for _, value := range data.Filters.serInstanceId {
				if sInfo.SerInstanceId == value {
					found = true
					break
				}
			}
			if !found {
				return nil
			}
		}

		// Service name
		if len(data.Filters.serName) > 0 {
			found := false
			for _, value := range data.Filters.serName {
				if sInfo.SerName == value {
					found = true
					break
				}
			}
			if !found {
				return nil
			}
		}

		// Service category
		// NOTE: Compare with either the category name or id, spec is not clear
		if data.Filters.serCategoryId != "" {
			categoryId := data.Filters.serCategoryId
			if sInfo.SerCategory == nil || (categoryId != sInfo.SerCategory.Name && categoryId != sInfo.SerCategory.Id) {
				return nil
			}
		}

		// Scope of Locality
		if data.Filters.scopeOfLocality != "" {
			if data.Filters.scopeOfLocality != string(*sInfo.ScopeOfLocality) {
				return nil
			}
		}

		// Service consumed local only
		if data.ConsumedLocalOnlyPresent {
			if data.Filters.consumedLocalOnly {
				if !sInfo.ConsumedLocalOnly {
					return nil
				}
			} else { //data.Filters.consumedLocalOnly is false
				if sInfo.ConsumedLocalOnly {
					return nil
				}
			}
		}

		// Is local service
		if data.IsLocalPresent {
			if data.Filters.isLocal {
				if !sInfo.IsLocal {
					return nil
				}
			}
		}
	}

	// Add service to list
	data.Services = append(data.Services, sInfo)
	return nil
}

func sendSvcUpdateMsg(sInfoJson, appId, mep, changeType string) {
	// Inform other MEP instances
	// Send MEC Service Update Notification message on local Message Queue
	msg := mqLocal.CreateMsg(mq.MsgMecSvcUpdate, mq.TargetAll, sandboxName)
	msg.Payload[fieldSvcInfo] = sInfoJson
	msg.Payload[fieldAppId] = appId
	msg.Payload[fieldLocationName] = mep
	msg.Payload[fieldChangeType] = changeType
	log.Debug("TX MSG: ", mq.PrintMsg(msg))
	err := mqLocal.SendMsg(msg)
	if err != nil {
		log.Error("Failed to send message. Error: ", err.Error())
	}
}

func processSvcUpdate(sInfoJson, mep, changeType string) {
	// Ignore updates for global MEP instance
	if mepName == globalLocationName {
		log.Warn("Ignoring service update received at global instance")
		return
	}
	// Ignore local MEP updates (already processed)
	if mep == mepName {
		return
	}

	// Unmarshal received service info
	sInfo := convertJsonToServiceInfo(sInfoJson)

	// Check if notifications must be sent
	checkSerAvailNotification(sInfo, mep, CapifEvent(changeType))
}

func checkSerAvailNotification(sInfo *ServiceInfo, mep string, changeType CapifEvent) {
	// Set IsLocal flag
	if *sInfo.ScopeOfLocality == MEC_SYSTEM_LocalityType || (mep != "" && mep == mepName) {
		sInfo.IsLocal = true
	} else {
		sInfo.IsLocal = false
	}

	// Filter out non-local services with "consumedLocalOnly" flag set to "true"
	if !sInfo.IsLocal && sInfo.ConsumedLocalOnly {
		return
	}

	// Get subscriptions with matching type
	subList, err := subMgr.GetFilteredSubscriptions("", SER_AVAILABILITY_NOTIF_SUB_TYPE)
	if err != nil {
		log.Error("Failed to get subscription list with err: ", err.Error())
		return
	}

	// Process service availability notification
	for _, sub := range subList {

		// Unmarshal original JSON subscription
		origSub := convertJsonToSerAvailabilityNotifSub_1(sub.JsonSubOrig)
		if origSub == nil {
			continue
		}

		// Check subscription filter criteria
		if len(origSub.EventFilters) > 0 {
			found := false
			// Service Instance IDs
			for _, eventFilter := range origSub.EventFilters {
				// Check if ApiIds (replacing SerInstanceIds) match
				if len(eventFilter.ApiIds) > 0 {
					for _, apiId := range eventFilter.ApiIds {
						if apiId == sInfo.SerInstanceId { // Compare with the current Service Instance ID (now ApiId)
							found = true
							break
						}
					}
				}
				if !found {
					continue
				}
			}
		}
		// versions := []string{sInfo.Version}
		aefProfile := &AefProfile{
			AefId:                 sInfo.TransportInfo.Id,
			Versions:              []Version{{APIVersion: sInfo.Version}},
			InterfaceDescriptions: ConvertEndpointToInterfaceDescriptions(sInfo.TransportInfo.Endpoint),
			VendorSpecificUrnetsimeccapifexttransportInfo: &MecTransportInfoCapifExt{
				Name:     sInfo.TransportInfo.Name,
				Type_:    sInfo.TransportInfo.Type_,
				Protocol: sInfo.TransportInfo.Protocol,
				Version:  sInfo.TransportInfo.Version,
				Security: sInfo.TransportInfo.Security,
			},
		}
		dsInfo := &ServiceApiDescription{
			ApiName:     sInfo.SerName,
			ApiId:       sInfo.SerInstanceId,
			AefProfiles: []AefProfile{*aefProfile},
			VendorSpecificUrnetsimeccapifextserviceInfo: &MecServiceInfoCapifExt{
				Serializer:        sInfo.Serializer,
				State:             sInfo.State,
				ScopeOfLocality:   sInfo.ScopeOfLocality,
				ConsumedLocalOnly: sInfo.ConsumedLocalOnly,
				IsLocal:           sInfo.IsLocal,
				Category:          sInfo.SerCategory,
			},
		}

		event_detail := &CapifEventDetail{}
		event_detail.ServiceApiDescriptions = append(event_detail.ServiceApiDescriptions, *dsInfo)
		event_detail.ApiIds = append(event_detail.ApiIds, sInfo.SerInstanceId)
		// Create a new EventNotification instance
		notif := &EventNotification{
			SubscriptionId: sub.Cfg.Id,
			EventDetail:    event_detail,
		}
		// Set the event type based on changeType
		var eventType CapifEvent
		switch changeType {
		case "SERVICE_API_AVAILABLE":
			eventType = AVAILABLE
		case "SERVICE_API_UNAVAILABLE":
			eventType = UNAVAILABLE
		case "SERVICE_API_UPDATE":
			eventType = UPDATE
		default:
			continue
		}

		// If eventType is set, append it to the Events slice
		if eventType != "" {
			notif.Events = append(notif.Events, eventType)
		}

		// Send notification
		go func(sub *subs.Subscription) {
			log.Info("Sending Service Availability notification (" + sub.Cfg.Id + ") for " + string(changeType))
			err := subMgr.SendNotification(sub, []byte(convertServiceAvailabilityNotifToJson_1(notif)))
			if err != nil {
				log.Error("Failed to send Service Availability notif with err: ", err.Error())
			}
		}(sub)
	}
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

func validateServiceQueryParams(serInstanceId []string, serName []string, serCategoryId string) error {
	count := 0
	if len(serInstanceId) > 0 {
		count++
	}
	if len(serName) > 0 {
		count++
	}
	if serCategoryId != "" {
		count++
	}
	if count > 1 {
		err := errors.New("Either \"ser_instance_id\" or \"ser_name\" or \"ser_category_id\" or none of them shall be present")
		log.Error(err.Error())
		return err
	}
	return nil
}

func getLocationNameFromKey(key string) string {
	fields := strings.Split(strings.TrimPrefix(key, dkm.GetKeyRoot(sandboxName)+appEnablementKey+":mep:"), ":")
	if len(fields) > 0 {
		return fields[0]
	}
	return ""
}

func getAppInfo(appId string) (map[string]string, error) {
	var appInfo map[string]string

	// Get app instance from local MEP only
	key := baseKey + "app:" + appId + ":info"
	appInfo, err := rc.GetEntry(key)
	if err != nil || len(appInfo) == 0 {
		return nil, errors.New("App Instance not found")
	}
	return appInfo, nil
}

func getAppInfoAnyMep(appId string) (map[string]string, error) {
	var appInfoList []map[string]string

	// Get app instance from any MEP
	keyMatchStr := baseKeyAnyMep + "app:" + appId + ":info"
	err := rc.ForEachEntry(keyMatchStr, populateAppInfo, &appInfoList)
	if err != nil || len(appInfoList) != 1 {
		return nil, errors.New("App Instance not found")
	}
	return appInfoList[0], nil
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

func validateCategoryRef(categoryRef *CategoryRef) string {
	if categoryRef != nil {
		if categoryRef.Href == "" {
			return "CategoryRef mandatory parameter Href missing."
		}
		if categoryRef.Id == "" {
			return "CategoryRef mandatory parameter Id missing."
		}
		if categoryRef.Name == "" {
			return "CategoryRef mandatory parameter Name missing."
		}
		if categoryRef.Version == "" {
			return "CategoryRef mandatory parameter Version missing."
		}
	}
	return ""
}

// FSCOM Unsused
// func newSerAvailabilityNotifSubCfg(sub *SerAvailabilityNotificationSubscription, subId string, appId string) *subs.SubscriptionCfg {
// 	subCfg := &subs.SubscriptionCfg{
// 		Id:                  subId,
// 		AppId:               appId,
// 		Type:                SER_AVAILABILITY_NOTIF_SUB_TYPE,
// 		Self:                sub.Links.Self.Href,
// 		NotifyUrl:           sub.CallbackReference,
// 		ExpiryTime:          nil,
// 		PeriodicInterval:    0,
// 		RequestTestNotif:    false,
// 		RequestWebsocketUri: false,
// 	}
// 	return subCfg
// }

func newSerAvailabilityNotifSubCfg_1(sub *EventSubscription, subId string, appId string) *subs.SubscriptionCfg {
	var eventFilters []string
	for _, filter := range sub.EventFilters {
		eventFilters = append(eventFilters, filter.ApiIds...)
	}
	var capifEvent []string
	capifEvent = append(capifEvent, string(sub.Events[0]))
	subCfg := &subs.SubscriptionCfg{
		Id:                  subId,
		AppId:               appId,
		Type:                SER_AVAILABILITY_NOTIF_SUB_TYPE,
		Self:                "",
		NotifyUrl:           sub.NotificationDestination,
		ExpiryTime:          nil,
		PeriodicInterval:    0,
		RequestTestNotif:    false,
		RequestWebsocketUri: false,
		EventFilters:        eventFilters,
		CapifEvent:          capifEvent,
	}
	return subCfg
}

func errHandlerProblemDetails(w http.ResponseWriter, error string, code int) {
	var pd ProblemDetails
	pd.Detail = error
	pd.Status = int32(code)

	jsonResponse := convertProblemDetailstoJson(&pd)

	w.WriteHeader(code)
	fmt.Fprint(w, jsonResponse)
}

func validateInterfaceDescriptions(descs []InterfaceDescription) error {
	for i, desc := range descs {
		hasAddr := desc.Ipv4Addr != nil || desc.Ipv6Addr != nil
		hasFqdn := desc.Fqdn != nil

		if (hasAddr && hasFqdn) || (!hasAddr && !hasFqdn) {
			return fmt.Errorf("InterfaceDescription[%d]: exactly one of ipv4Addr/ipv6Addr or fqdn must be set", i)
		}
	}
	return nil
}

func ConvertInterfaceDescriptionsToEndpoint(descs []InterfaceDescription) *OneOfTransportInfoEndpoint {
	if len(descs) == 0 {
		return nil
	}

	// 1. Try to construct URI(s) when all required fields are available
	var uris []string
	allHaveHost := true
	for _, desc := range descs {
		var host string
		switch {
		case desc.Ipv4Addr != nil:
			host = *desc.Ipv4Addr
		case desc.Ipv6Addr != nil:
			host = "[" + *desc.Ipv6Addr + "]"
		case desc.Fqdn != nil:
			host = *desc.Fqdn
		default:
			allHaveHost = false
		}

		if host == "" {
			allHaveHost = false
			continue
		}

		uri := "http://" + host
		if desc.Port != nil {
			uri += ":" + fmt.Sprintf("%d", *desc.Port)
		}
		if desc.ApiPrefix != nil {
			uri += *desc.ApiPrefix
		}
		uris = append(uris, uri)
	}

	if allHaveHost && len(uris) > 0 {
		return &OneOfTransportInfoEndpoint{
			EndPointInfoUris: EndPointInfoUris{
				Uris: uris,
			},
		}
	}

	// 2. Try FQDN (without port)
	var fqdns []string
	for _, desc := range descs {
		if desc.Fqdn != nil && desc.Port == nil {
			fqdns = append(fqdns, *desc.Fqdn)
		}
	}
	if len(fqdns) > 0 {
		return &OneOfTransportInfoEndpoint{
			EndPointInfoFqdn: EndPointInfoFqdn{
				Fqdn: fqdns,
			},
		}
	}

	// 3. Fallback to addresses (host + port)
	var addresses []EndPointInfoAddress
	for _, desc := range descs {
		if desc.Port == nil {
			continue
		}

		var host string
		switch {
		case desc.Ipv4Addr != nil:
			host = *desc.Ipv4Addr
		case desc.Ipv6Addr != nil:
			host = *desc.Ipv6Addr
		case desc.Fqdn != nil:
			host = *desc.Fqdn
		}

		if host != "" {
			addresses = append(addresses, EndPointInfoAddress{
				Host: host,
				Port: int32(*desc.Port),
			})
		}
	}

	if len(addresses) > 0 {
		return &OneOfTransportInfoEndpoint{
			EndPointInfoAddresses: EndPointInfoAddresses{
				Addresses: addresses,
			},
		}
	}

	// 4. None matched: treat as bad input
	return nil
}

func ConvertEndpointToInterfaceDescriptions(endpoint *OneOfTransportInfoEndpoint) []InterfaceDescription {
	if endpoint == nil {
		return nil
	}

	var descs []InterfaceDescription

	// 1. uris → try to extract apiPrefix and host/port info
	if len(endpoint.Uris) > 0 {
		for _, uriStr := range endpoint.Uris {
			u, err := url.Parse(uriStr)
			if err != nil || u.Host == "" {
				continue
			}

			host, portStr, _ := net.SplitHostPort(u.Host)
			var port *int
			if portStr != "" {
				p, err := strconv.Atoi(portStr)
				if err == nil {
					port = &p
				}
			} else {
				host = u.Host
			}

			desc := InterfaceDescription{
				ApiPrefix: &u.Path,
				Port:      port,
			}

			if ip := net.ParseIP(host); ip != nil {
				if ip.To4() != nil {
					desc.Ipv4Addr = &host
				} else {
					desc.Ipv6Addr = &host
				}
			} else {
				desc.Fqdn = &host
			}

			descs = append(descs, desc)
		}
		return descs
	}

	// 2. fqdn (no port)
	if len(endpoint.Fqdn) > 0 {
		for _, fqdn := range endpoint.Fqdn {
			desc := InterfaceDescription{
				Fqdn: &fqdn,
			}
			descs = append(descs, desc)
		}
		return descs
	}

	// 3. addresses
	if len(endpoint.Addresses) > 0 {
		for _, addr := range endpoint.Addresses {
			port := int(addr.Port)
			desc := InterfaceDescription{
				Port: &port,
			}
			if ip := net.ParseIP(addr.Host); ip != nil {
				if ip.To4() != nil {
					desc.Ipv4Addr = &addr.Host
				} else {
					desc.Ipv6Addr = &addr.Host
				}
			} else {
				desc.Fqdn = &addr.Host
			}
			descs = append(descs, desc)
		}
		return descs
	}

	return nil
}
