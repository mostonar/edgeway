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

package sbi

import (
	"encoding/json"
	"errors"
	"sync"

	log "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-logger"
	mod "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-model"
	mq "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-mq"
	tm "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-sss-mgr"
	sam "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-swagger-api-mgr"
)

const moduleName string = "meep-sss-sbi"

type IotPlatformInfo struct {
	Host     string
	Port     int
	Path     string
	CseName  string
	Id       string
	Protocol string
}

type Point struct {
	Latitude  float64
	Longitude float64
}

type SensorCharacteristic struct {
	CharacteristicName          string
	CharacteristicValue         string
	CharacteristicUnitOfMeasure *string
}

type SensorDiscoveryInfo struct {
	SensorIdentifier         string
	SensorType               string
	SensorPropertyList       []string
	SensorCharacteristicList []SensorCharacteristic
	SensorPosition           *Point
}

type SensorStatusInfo struct {
	SensorIdentifier string
	SensorStatusType string
	ErrorInformation string
}

type SensorData struct {
	SensorIdentifier  string
	Data              string
	DataFormat        string
	DataUnitOfMeasure string
	DataTimestamp     *TimeStamp
}

type SensorDiscoveryEventSubscription struct {
	SubscriptionType string
	SensorInfoList   []string
	GeographicalArea []AreaInfo
}

type AreaInfo struct {
	Shape  string
	Points []Point
	Radius int32
}

type TimeStamp struct {
	Seconds     int32
	NanoSeconds int32
}

type SbiCfg struct {
	ModuleName      string
	SandboxName     string
	LocationName         string
	RedisAddr       string
	InfluxAddr      string
	Locality        []string
	Protocol        string
	Host            string
	Port            int
	HostId          string
	Name            string
	DiscoveryNotify func(map[string]interface{})
	StatusNotify    func(map[string]interface{})
	DataNotify      func(map[string]interface{})
	ScenarioNameCb  func(string)
	CleanUpCb       func()
}

type SssSbi struct {
	moduleName           string
	sandboxName          string
	mepName              string
	scenarioName         string
	localityEnabled      bool
	locality             map[string]bool
	mqLocal              *mq.MsgQueue
	handlerId            int
	apiMgr               *sam.SwaggerApiMgr
	activeModel          *mod.Model
	sssMgr               *tm.SssMgr
	protocol             string
	host                 string
	port                 int
	hostId               string
	name                 string
	discoveryNotify      func(map[string]interface{})
	statusNotify         func(map[string]interface{})
	dataNotify           func(map[string]interface{})
	updateScenarioNameCB func(string)
	cleanUpCB            func()
	mutex                sync.Mutex
}

var sbi *SssSbi

// Init - SSS Service SBI initialization
func Init(cfg SbiCfg) (err error) {

	// Create new SBI instance
	if sbi != nil {
		sbi = nil
	}
	sbi = new(SssSbi)
	sbi.moduleName = cfg.ModuleName
	sbi.sandboxName = cfg.SandboxName
	sbi.mepName = cfg.LocationName
	sbi.scenarioName = ""
	sbi.updateScenarioNameCB = cfg.ScenarioNameCb
	sbi.cleanUpCB = cfg.CleanUpCb
	sbi.protocol = cfg.Protocol
	sbi.host = cfg.Host
	sbi.port = cfg.Port
	sbi.hostId = cfg.HostId
	sbi.name = cfg.Name
	sbi.discoveryNotify = cfg.DiscoveryNotify
	sbi.statusNotify = cfg.StatusNotify
	sbi.dataNotify = cfg.DataNotify

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

	// Connect to SSS Manager
	if sbi.port != 0 {
		sbi.sssMgr, err = tm.NewSssMgr(sbi.moduleName, sbi.sandboxName, sbi.protocol, sbi.host, sbi.port, sbi.hostId, sbi.name, sbi.discoveryNotify, sbi.statusNotify, sbi.dataNotify)
		if err != nil {
			log.Error("Failed connection to SSS Manager: ", err)
			return err
		}
		log.Info("Connected to SSS Manager")
	} else {
		sbi.sssMgr = nil
		log.Warn("No IoTPlatform registered")
	}

	// Initialize service
	processActiveScenarioUpdate()

	return nil
}

// Run - MEEP SSS execution
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

	// Delete SSS Manager
	if sbi.sssMgr != nil {
		err = sbi.sssMgr.DeleteSssMgr()
		if err != nil {
			log.Error(err.Error())
			return err
		}
	}

	return nil
}

func UpdatePlatformList(iotPlatformInfoList []IotPlatformInfo) (err error) { // FIXME FSCOM To be refined to support multiple IoT platform registered
	// Connect to SSS Manager
	if sbi.sssMgr != nil {
		log.Warn("UpdatePlatformList: IoTPlatform already registered")
		return nil
	}

	sbi.sssMgr, err = tm.NewSssMgr(sbi.moduleName, sbi.sandboxName, iotPlatformInfoList[0].Protocol, iotPlatformInfoList[0].Host, iotPlatformInfoList[0].Port, iotPlatformInfoList[0].Id, iotPlatformInfoList[0].CseName, sbi.discoveryNotify, sbi.statusNotify, sbi.dataNotify)
	if err != nil {
		log.Error("UpdatePlatformList: Failed connection to SSS Manager: ", err)
		return err
	}
	log.Info("UpdatePlatformList: Connected to SSS Manager")

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

		// err := initializeIotMessageDistribution()
		// if err != nil {
		// 	log.Error("Failed to initialize V2X message distribution: ", err)
		// 	return
		// }
	}
}

func SensorDiscoveryInfoAll() (sensors []SensorDiscoveryInfo, err error) {
	log.Info("sbi.SensorDiscoveryInfoAll")

	if sbi.sssMgr == nil {
		err := errors.New("No IoT platform registered")
		return nil, err
	}

	s, err := sbi.sssMgr.SensorDiscoveryInfoAll()
	if err != nil {
		return nil, err
	}
	//log.Info("sbi.SensorDiscoveryInfoAll: s: ", s)

	sensors = convertSensorDiscoveryInfoListFromSssMgr(s)
	log.Info("sbi.SensorDiscoveryInfoAll: sensors: ", sensors)

	return sensors, nil
}

func GetSensorStatus(sensorIdentifier string) (status SensorStatusInfo, err error) {
	log.Info("sbi.GetSensorStatus")

	if sbi.sssMgr == nil {
		err := errors.New("No IoT platform registered")
		return status, err
	}

	s, err := sbi.sssMgr.GetSensor(sensorIdentifier)
	if err != nil {
		return status, err
	}
	log.Info("sbi.GetSensorStatus: s: ", s)

	sensor := convertSensorDiscoveryInfoFromSssMgr(s)
	log.Info("sbi.GetSensorStatus: sensors: ", sensor)

	// FIXME FSCOM Check status of the sensor???
	status = SensorStatusInfo{
		SensorIdentifier: sensor.SensorIdentifier,
		SensorStatusType: "ONLINE",
		ErrorInformation: "",
	}

	return status, nil
}

func GetSensorData(sensorIdentifier string) (data SensorData, err error) {
	log.Info("sbi.GetSensorData")

	if sbi.sssMgr == nil {
		err := errors.New("No IoT platform registered")
		return data, err
	}

	s, err := sbi.sssMgr.GetSensor(sensorIdentifier)
	if err != nil {
		return data, err
	}
	log.Info("sbi.GetSensorData: s: ", s)
	log.Info("sbi.GetSensorData: s.SensorPropertyList: ", s.SensorPropertyList)
	log.Info("sbi.GetSensorData: s.SensorCharacteristicList: ", s.SensorCharacteristicList)
	log.Info("sbi.GetSensorData: s.Flex: ", s.Flex)

	sensor := convertSensorDiscoveryInfoFromSssMgr(s)
	log.Info("sbi.GetSensorData: sensors: ", sensor)

	data = SensorData{ // FIXME FSCOM Review content of SensorData. What is the mapping oneM2M?
		SensorIdentifier: sensor.SensorIdentifier,
		//Data:              string(sensor.SensorCharacteristicList),
		DataFormat:        "",
		DataUnitOfMeasure: "",
		DataTimestamp:     nil,
	}
	j, err := json.Marshal(sensor.SensorCharacteristicList)
	if err != nil {
		log.Warn("sbi.GetSensorData: j: ", j)
		return data, err
	}
	data.Data = string(j)

	return data, nil
}

func ProcessSensorDiscoveryEventSubscription(sensorDiscoveryEventSubscription SensorDiscoveryEventSubscription) (err error) {
	log.Info("sbi.processSensorDiscoveryEventSubscription: ", sensorDiscoveryEventSubscription)

	if sbi.sssMgr == nil {
		err = errors.New("No IoT platform registered")
		return err
	}

	sub := tm.SensorDiscoveryEventSubscription{
		SubscriptionType: sensorDiscoveryEventSubscription.SubscriptionType,
		SensorInfoList:   sensorDiscoveryEventSubscription.SensorInfoList,
	}
	// TODO FSCOM Add GeographicalArea field & create a convert function
	err = sbi.sssMgr.ProcessSensorDiscoveryEventSubscription(sub)
	if err != nil {
		return err
	}

	return nil
}

func convertSensorDiscoveryInfoListFromSssMgr(s []tm.SensorDiscoveryInfo) (sensors []SensorDiscoveryInfo) {
	sensors = make([]SensorDiscoveryInfo, len(s))
	for i, val := range s {
		sensors[i] = convertSensorDiscoveryInfoFromSssMgr(val)
	} // End of 'for' statement

	return sensors
}

func convertSensorDiscoveryInfoFromSssMgr(s tm.SensorDiscoveryInfo) (sensor SensorDiscoveryInfo) {
	log.Info(">>> sbi.convertSensorDiscoveryInfoFromSssMgr: ", s)

	sensor = SensorDiscoveryInfo{
		SensorIdentifier: s.SensorIdentifier,
		SensorType:       s.SensorType,
	}
	log.Info("sbi.convertSensorDiscoveryInfoFromSssMgr: len(s.SensorPropertyList): ", len(s.SensorPropertyList))
	if len(s.SensorPropertyList) != 0 {
		sensor.SensorPropertyList = make([]string, len(s.SensorPropertyList))
		copy(sensor.SensorPropertyList, s.SensorPropertyList)
	}
	log.Info("sbi.convertSensorDiscoveryInfoFromSssMgr: len(s.SensorCharacteristicList): ", len(s.SensorCharacteristicList))
	if len(s.SensorCharacteristicList) != 0 {
		sensor.SensorCharacteristicList = make([]SensorCharacteristic, len(s.SensorCharacteristicList))
		for i, val := range s.SensorCharacteristicList {
			sensor.SensorCharacteristicList[i] = SensorCharacteristic{
				CharacteristicName:          val.CharacteristicName,
				CharacteristicValue:         val.CharacteristicValue,
				CharacteristicUnitOfMeasure: val.CharacteristicUnitOfMeasure,
			}
		} // End of 'for' statement
	}
	log.Info("sbi.convertSensorDiscoveryInfoFromSssMgr: len(s.Flex): ", len(s.Flex))
	if len(s.Flex) != 0 {
		sensor.SensorCharacteristicList = make([]SensorCharacteristic, len(s.Flex))
		i := 0
		for key, val := range s.Flex {
			sensor.SensorCharacteristicList[i] = SensorCharacteristic{
				CharacteristicName:          key,
				CharacteristicValue:         val.(string),
				CharacteristicUnitOfMeasure: nil,
			}
			i += 1
		} // End of 'for' statement
	}
	if s.SensorPosition != nil {
		sensor.SensorPosition = &Point{
			Latitude:  s.SensorPosition.Latitude,
			Longitude: s.SensorPosition.Longitude,
		}
	}

	log.Info("<<< sbi.convertSensorDiscoveryInfoFromSssMgr: ", sensor)
	return sensor
}
