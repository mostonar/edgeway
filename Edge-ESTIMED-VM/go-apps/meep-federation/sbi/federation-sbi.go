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

package sbi

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"

	fm "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-federation-mgr"
	log "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-logger"
	mod "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-model"
	mq "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-mq"
	smc "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-service-mgmt-client"
	sam "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-swagger-api-mgr"
)

const moduleName string = "meep-federation-sbi"

type SbiCfg struct {
	ModuleName     string
	SandboxName    string
	FedBasePath    string
	HostUrl        string
	FedBroker      string
	FedTopic       string
	LocationName        string
	RedisAddr      string
	InfluxAddr     string
	Locality       []string
	ScenarioNameCb func(string)
	FedNotify      func(msg string, systemId string)
	CleanUpCb      func()
}

type FedSbi struct {
	moduleName           string
	sandboxName          string
	hostUrl              string
	mepName              string
	fedBasePath          string
	scenarioName         string
	localityEnabled      bool
	locality             map[string]bool
	fedBroker            string
	fedTopic             string
	mqLocal              *mq.MsgQueue
	handlerId            int
	apiMgr               *sam.SwaggerApiMgr
	activeModel          *mod.Model
	federationMgr        *fm.FederationMgr
	messageBrokerStarted bool
	updateScenarioNameCB func(string)
	fedNotify            func(string, string)
	cleanUpCB            func()
	mutex                sync.Mutex
}

type SystemInfo struct { // MEC Federation implementation of sharing MEC Plateform details
	SystemId       string
	SystemName     string
	SystemProvider string
	TransportTypes string
	Protocol       string
	SerializerType string
	Host           string
}

type SystemInfoUpdate struct {
	SystemId       string
	SystemName     string
	SystemProvider string
	Endpoint       EndPointInfo
}

type EndPointInfo struct {
	Uris        []string
	Fqdn        []string
	Addresses   []Addresses
	Alternative string
}

type Addresses struct {
	Host string
	Port int32
}

var localSystemInfo = SystemInfo{}

var sbi *FedSbi

var systemInfopMap = map[string]SystemInfo{} // List of discovered MEC Plateform

// Init - V2XI Service SBI initialization
func Init(cfg SbiCfg) (err error) {

	// Create new SBI instance
	if sbi != nil {
		sbi = nil
	}
	sbi = new(FedSbi)
	sbi.moduleName = cfg.ModuleName
	sbi.sandboxName = cfg.SandboxName
	sbi.fedBasePath = cfg.FedBasePath
	sbi.hostUrl = cfg.HostUrl
	sbi.mepName = cfg.LocationName
	sbi.scenarioName = ""
	sbi.fedBroker = cfg.FedBroker
	sbi.fedTopic = cfg.FedTopic
	sbi.updateScenarioNameCB = cfg.ScenarioNameCb
	sbi.fedNotify = cfg.FedNotify
	sbi.cleanUpCB = cfg.CleanUpCb

	// Fill locality map
	if len(cfg.Locality) > 0 {
		sbi.locality = make(map[string]bool)
		for _, locality := range cfg.Locality {
			sbi.locality[locality] = true
		}
		sbi.localityEnabled = true
	} else {
		sbi.localityEnabled = false
	}

	// Create message queue
	sbi.mqLocal, err = mq.NewMsgQueue(mq.GetLocalName(sbi.sandboxName), moduleName, sbi.sandboxName, cfg.RedisAddr)
	if err != nil {
		log.Error("Failed to create Message Queue with error: ", err)
		return err
	}
	log.Info("Message Queue created")

	// Indicate MEC-FED mepName
	msg := sbi.mqLocal.CreateMsg(mq.MecFedConfig, mq.TargetAll, sbi.sandboxName)
	msg.Payload["URL"] = "http://" + sbi.mepName + "-meep-federation/" + sbi.fedBasePath
	log.Debug("TX MSG: ", mq.PrintMsg(msg))
	err = sbi.mqLocal.SendMsg(msg)
	if err != nil {
		log.Error("Failed to send message. Error: ", err.Error())
		return err
	}

	// Create Swagger API Manager
	sbi.apiMgr, err = sam.NewSwaggerApiMgr(sbi.moduleName, sbi.sandboxName, sbi.mepName, sbi.mqLocal)
	if err != nil {
		log.Error("Failed to create Swagger API Manager. Error: ", err)
		return err
	}
	log.Info("Swagger API Manager created")

	// Create new active scenario model
	modelCfg := mod.ModelCfg{
		Name:      "activeScenario",
		Namespace: sbi.sandboxName,
		Module:    moduleName,
		UpdateCb:  nil,
		DbAddr:    cfg.RedisAddr,
	}
	sbi.activeModel, err = mod.NewModel(modelCfg)
	if err != nil {
		log.Error("Failed to create model: ", err.Error())
		return err
	}

	// Connect to FED Traffic Manager
	sbi.federationMgr, err = fm.NewFederationMgr(sbi.moduleName, sbi.sandboxName, cfg.FedBroker, cfg.FedTopic, cfg.FedNotify)
	if err != nil {
		log.Error("Failed connection to FED Traffic Manager: ", err)
		return err
	}
	sbi.messageBrokerStarted = false
	log.Info("Connected to FED Traffic Manager")

	systemInfopMap = make(map[string]SystemInfo)

	// Initialize service
	processActiveScenarioUpdate()

	return nil
}

// Run - MEEP FED execution
func Run() (err error) {

	// Start Swagger API Manager (provider)
	err = sbi.apiMgr.Start(true, false)
	if err != nil {
		log.Error("Failed to start Swagger API Manager with error: ", err.Error())
		return err
	}
	log.Info("Swagger API Manager started")

	// Add module Swagger APIs
	err = sbi.apiMgr.AddApis()
	if err != nil {
		log.Error("Failed to add Swagger APIs with error: ", err.Error())
		return err
	}
	log.Info("Swagger APIs successfully added")

	// Register Message Queue handler
	handler := mq.MsgHandler{Handler: msgHandler, UserData: nil}
	sbi.handlerId, err = sbi.mqLocal.RegisterHandler(handler)
	if err != nil {
		log.Error("Failed to register message queue handler: ", err.Error())
		return err
	}

	return nil
}

func Stop() (err error) {
	if sbi == nil {
		return
	}

	if sbi.mqLocal != nil {
		sbi.mqLocal.UnregisterHandler(sbi.handlerId)
	}

	if sbi.apiMgr != nil {
		// Remove APIs
		err = sbi.apiMgr.RemoveApis()
		if err != nil {
			log.Error("Failed to remove APIs with err: ", err.Error())
			return err
		}
	}

	systemInfopMap = nil

	// Delete FED Manager
	if sbi.federationMgr != nil {
		err = sbi.federationMgr.DeleteFederationMgr(localSystemInfo.SystemId)
		if err != nil {
			log.Error(err.Error())
			return err
		}
	}

	return nil
}

// Message Queue handler
func msgHandler(msg *mq.Msg, userData interface{}) {
	switch msg.Message {
	case mq.MsgScenarioActivate:
		log.Debug("RX MSG: ", mq.PrintMsg(msg))
		processActiveScenarioUpdate()
	case mq.MsgScenarioUpdate:
		log.Debug("RX MSG: ", mq.PrintMsg(msg))
		processActiveScenarioUpdate()
	case mq.MsgScenarioTerminate:
		log.Debug("RX MSG: ", mq.PrintMsg(msg))
		processActiveScenarioTerminate()
	default:
		log.Trace("Ignoring unsupported message: ", mq.PrintMsg(msg))
	}
}

func processActiveScenarioTerminate() {
	log.Debug("processActiveScenarioTerminate")

	// Sync with active scenario store
	sbi.activeModel.UpdateScenario()

	// Update scenario name
	sbi.scenarioName = ""

	sbi.cleanUpCB()
}

func processActiveScenarioUpdate() {
	sbi.mutex.Lock()
	defer sbi.mutex.Unlock()

	log.Debug("processActiveScenarioUpdate")
	sbi.activeModel.UpdateScenario()

	// Process new scenario
	var scenarioName = sbi.activeModel.GetScenarioName()
	if scenarioName != sbi.scenarioName {
		log.Info("processActiveScenarioUpdate: Entering in then")
		// Update scenario name
		sbi.scenarioName = scenarioName
		sbi.updateScenarioNameCB(sbi.scenarioName)

		log.Info("processActiveScenarioUpdate: sbi.scenarioName: ", sbi.scenarioName)
	}
}

func RegisterSystemInfo(requestData SystemInfo) (responseData SystemInfo, err error) {
	log.Info(">>> RegisterSystemInfo: ", requestData)

	if !sbi.messageBrokerStarted {
		err = sbi.federationMgr.StartFedMessageBrokerServer()
		if err != nil {
			log.Error("Failed to start message broker server: ", err)
			return responseData, err
		}
		sbi.messageBrokerStarted = true
	}

	s := convertSystemInfotoJson(&requestData)
	if s == "" {
		err = errors.New("Failed to convert message")
		log.Error(err)
		return responseData, err
	}
	err = sbi.federationMgr.PublishMessageOnMessageBroker(s, requestData.SystemId) // Remove emtry from brocker
	if err != nil {
		log.Error("Failed to publish message broker server: ", err)
		return responseData, err
	}

	responseData = requestData
	if localSystemInfo.SystemId == "" { // Self registration
		localSystemInfo = requestData
		log.Debug("RegisterSystemInfo: localSystemInfo=", localSystemInfo)
	} // otherwise, this is another MEC platform which is regsitering

	systemInfopMap[responseData.SystemId] = responseData
	log.Debug("RegisterSystemInfo: systemInfopMap=", systemInfopMap)

	return responseData, nil
}

func UpdateRegisteredSystemInfo(systemInfoUpdate SystemInfoUpdate) (err error) {
	log.Info(">>> UpdateRegisteredSystemInfo: ", systemInfoUpdate)

	// Sanity checks
	if len(systemInfoUpdate.Endpoint.Uris) == 0 && len(systemInfoUpdate.Endpoint.Fqdn) == 0 && len(systemInfoUpdate.Endpoint.Addresses) == 0 && systemInfoUpdate.Endpoint.Alternative == "" {
		err = errors.New("Invalid EndPointInfo value")
		return err
	}

	// if len(systemInfoUpdate.Endpoint.Uris) != 0 {
	// 	// FIXME FSCOM What to do with this???
	// } else if len(systemInfoUpdate.Endpoint.Fqdn) != 0 {
	// 	// FIXME FSCOM What to do with this???
	// } else if len(systemInfoUpdate.Endpoint.Addresses) != 0 {
	// 	// FIXME FSCOM What to do with this???
	// } else if systemInfoUpdate.Endpoint.Alternative != "" {
	// 	// FIXME FSCOM What to do with this???
	// }

	return nil
}

func DeregisterSystemInfo(systemId string) (err error) {
	log.Info(">>> DeregisterSystemInfo: ", systemId)

	if _, ok := systemInfopMap[systemId]; !ok {
		err = errors.New("Invalid SysemId: " + systemId)
		log.Error(err)
		return err
	}

	delete(systemInfopMap, systemId)
	log.Info("DeregisterSystemInfo: systemInfopMap: ", systemInfopMap)

	if sbi.messageBrokerStarted {
		err = sbi.federationMgr.DeleteFederationMgr(systemId)
		// Defer error processing
		if len(systemInfopMap) == 0 {
			log.Info("DeregisterSystemInfo: Call StopFedMessageBrokerServer")
			sbi.federationMgr.StopFedMessageBrokerServer()
			sbi.messageBrokerStarted = false
		}
	}

	if err != nil {
		log.Error(err)
		return err
	}

	return nil
}

func GetMecServices(host string) (svcs []smc.ServiceInfo, err error) {
	log.Info(">>> GetMecServices: ", host)

	// Create App Info client
	srvMgmtClientCfg := smc.NewConfiguration()
	srvMgmtClientCfg.BasePath = host + "/mec_service_mgmt/v1"
	svcMgmtClient := smc.NewAPIClient(srvMgmtClientCfg)
	if svcMgmtClient == nil {
		err = errors.New("Failed to create App Enablement Service Management REST API client")
		log.Info(err.Error())
		return svcs, err
	}
	log.Info("GetMecServices: Create App Enablement Service Management REST API client")

	svcs, _, err = svcMgmtClient.MecServiceMgmtApi.ServicesGET(context.TODO(), nil)
	if err != nil {
		log.Info(err.Error())
		return svcs, err
	}
	log.Info("GetMecServices: svcs: ", svcs)

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

func convertSystemInfotoJson(systemInfo *SystemInfo) string {
	jsonInfo, err := json.Marshal(*systemInfo)
	if err != nil {
		log.Error(err.Error())
		return ""
	}
	return string(jsonInfo)
}
