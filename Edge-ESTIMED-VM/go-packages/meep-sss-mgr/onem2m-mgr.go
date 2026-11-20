/*
 * Copyright (c) 2024-2026  The AdvantEDGE Authors
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

package sssmgr

import (
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	log "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-logger"

	uuid "github.com/google/uuid"
)

// Sensors-Sharing Service Manager
type SssMgr struct {
	name                 string
	namespace            string
	bindingProtocol      string
	protocol             SssMgrBindingProtocol
	host                 string
	port                 int
	scheme               string
	path                 string
	cse_name             string
	hostId               string
	mutex                sync.Mutex
	refreshTicker        *time.Ticker
	sss_discovery_notify func(map[string]interface{})
	sss_status_notify    func(map[string]interface{})
	sss_data_notify      func(map[string]interface{})
}

type IotPlatformInfo struct {
	Address       string
	Port          int
	Scheme        string
	Name          string
	IotPlatformId string
	Aei           string
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
	IotPlatformId            string
	Aei                      string
	Flex                     map[string]interface{}
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

var registeredIotPlatformsMap = map[string]IotPlatformInfo{} // List of discovered IOT Plateform
var sensorsMap = map[string]SensorDiscoveryInfo{}            // Map sensors by sensorIdentifier
var sensorsPerPlatformMap = map[string][]string{}            // Map dsensorIdentifiers per platform
var subscriptionListPerSubId map[string]SensorDiscoveryInfo

// stateMu guards the package-level maps. Each IoT platform has its own SssMgr,
// and those instances run at the same time.
var stateMu sync.RWMutex

// Timer to refresh devices list for all IoT platform
const refreshTickerExpeary = 60 // In seconds

// Enable profiling
const profiling = false

var profilingTimers map[string]time.Time

//var protocol SssMgrBindingProtocol

// NewSssMgr - Creates and initializes a new SSS Traffic Manager
func NewSssMgr(name string, namespace string, bindingProtocol string, host string, port int, hostId string, cse_name string, sss_discovery_notify func(map[string]interface{}), sss_status_notify func(map[string]interface{}), sss_data_notify func(map[string]interface{})) (tm *SssMgr, err error) {
	if name == "" {
		err = errors.New("Missing connector name")
		return nil, err
	} else if host == "" {
		err = errors.New("Missing host address")
		return nil, err
	} else if hostId == "" {
		err = errors.New("Missing host identifier")
		return nil, err
	} else if cse_name == "" {
		err = errors.New("Missing cse_name")
		return nil, err
	}

	// Create new Traffic Manager
	tm = new(SssMgr)
	tm.name = name
	if namespace != "" {
		tm.namespace = namespace
	} else {
		tm.namespace = "default"
	}

	tm.bindingProtocol = bindingProtocol
	tm.host = host
	tm.port = port
	tm.cse_name = cse_name
	tm.hostId = hostId
	tm.protocol = nil
	if tm.bindingProtocol == "MQTT" {
		if tm.host == "" {
			err := errors.New("Host not set for MQTTP protocol")
			log.Error(err.Error())
			return nil, err
		}
		if tm.port == 0 {
			tm.port = 1883
		}
		tm.protocol = NewSssMgrMqtt()
	} else if tm.bindingProtocol == "REST_HTTP" {
		if tm.port == 0 {
			tm.port = 80
		}
		tm.protocol = NewSssMgrHttp()
	} else {
		err := errors.New("Binding protocol not set")
		log.Error(err.Error())
		return nil, err
	}

	err = tm.protocol.init(tm, tm.notify)
	if err != nil {
		log.Error(err.Error())
		return nil, err
	}

	tm.sss_discovery_notify = sss_discovery_notify
	tm.sss_status_notify = sss_status_notify
	tm.sss_data_notify = sss_data_notify

	tm.init()

	return tm, nil
}

func NewSssMgr_with_uri(name string, namespace string, bindingProtocol string, uri string, hostId string, cse_name string, sss_discovery_notify func(map[string]interface{}), sss_status_notify func(map[string]interface{}), sss_data_notify func(map[string]interface{})) (tm *SssMgr, err error) {
	if name == "" {
		err = errors.New("Missing connector name")
		return nil, err
	} else if uri == "" {
		err = errors.New("Missing URI")
		return nil, err
	} else if hostId == "" {
		err = errors.New("Missing host identifier")
		return nil, err
	} else if cse_name == "" {
		err = errors.New("Missing cse_name")
		return nil, err
	}

	u, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}
	log.Debug("NewSssMgr_with_uri: uri: ", u)
	log.Debug("NewSssMgr_with_uri: host: ", u.Host)
	log.Debug("NewSssMgr_with_uri: port: ", u.Port())
	log.Debug("NewSssMgr_with_uri: path: ", u.Path)
	log.Debug("NewSssMgr_with_uri: query: ", u.Query())
	log.Debug("NewSssMgr_with_uri: scheme: ", u.Scheme)
	log.Debug("NewSssMgr_with_uri: raw: ", u.RawQuery)

	// Create new Traffic Manager
	tm = new(SssMgr)
	tm.name = name
	if namespace != "" {
		tm.namespace = namespace
	} else {
		tm.namespace = "default"
	}

	tm.bindingProtocol = bindingProtocol
	tm.host = u.Hostname()
	tm.port, err = strconv.Atoi(u.Port())
	tm.scheme = u.Scheme
	tm.path = u.Path
	if err != nil {
		tm.port = 443
		log.Debug("NewSssMgr_with_uri: Force port to ", tm.port)
	}
	tm.path = u.Path
	tm.cse_name = cse_name
	tm.hostId = hostId
	tm.protocol = nil
	if tm.bindingProtocol == "MQTT" {
		if tm.host == "" {
			err := errors.New("Host not set for MQTTP protocol")
			log.Error(err.Error())
			return nil, err
		}
		if tm.port == 0 {
			tm.port = 1883
		}
		tm.protocol = NewSssMgrMqtt()
	} else if tm.bindingProtocol == "REST_HTTP" {
		if tm.port == 0 {
			if u.Scheme == "https" {
				tm.port = 443
			} else {
				tm.port = 80
			}
		}
		tm.protocol = NewSssMgrHttp()
	} else {
		err := errors.New("Binding protocol not set")
		log.Error(err.Error())
		return nil, err
	}

	err = tm.protocol.init(tm, tm.notify)
	if err != nil {
		log.Error(err.Error())
		return nil, err
	}

	tm.sss_discovery_notify = sss_discovery_notify
	tm.sss_status_notify = sss_status_notify
	tm.sss_data_notify = sss_data_notify

	tm.init()

	return tm, nil
}

func platformById(id string) (IotPlatformInfo, bool) {
	stateMu.RLock()
	defer stateMu.RUnlock()
	if registeredIotPlatformsMap == nil {
		return IotPlatformInfo{}, false
	}
	p, ok := registeredIotPlatformsMap[id]
	return p, ok
}

func setPlatformAei(id string, aei string) {
	stateMu.Lock()
	defer stateMu.Unlock()
	if registeredIotPlatformsMap == nil {
		return
	}
	entry, ok := registeredIotPlatformsMap[id]
	if !ok {
		return
	}
	entry.Aei = aei
	registeredIotPlatformsMap[id] = entry
}

// Profiling init
func (tm *SssMgr) init() {
	if profiling {
		if profilingTimers == nil {
			profilingTimers = make(map[string]time.Time)
		}
	}

	// Add this platform. Do not replace the maps: a second registration must
	// leave the first platform's entry in place.
	stateMu.Lock()
	if registeredIotPlatformsMap == nil {
		registeredIotPlatformsMap = make(map[string]IotPlatformInfo)
	}
	if sensorsMap == nil {
		sensorsMap = make(map[string]SensorDiscoveryInfo)
	}
	if sensorsPerPlatformMap == nil {
		sensorsPerPlatformMap = make(map[string][]string)
	}
	if subscriptionListPerSubId == nil {
		subscriptionListPerSubId = make(map[string]SensorDiscoveryInfo)
	}
	registeredIotPlatformsMap[tm.hostId] = IotPlatformInfo{
		Address:       tm.host,
		Port:          tm.port,
		Scheme:        tm.scheme,
		Name:          tm.cse_name,
		IotPlatformId: tm.hostId,
	}
	stateMu.Unlock()

	tm.refreshTicker = nil
	tm.startRefreshTicker()
}

// DeleteSssMgr -
func (tm *SssMgr) DeleteSssMgr() (err error) {
	tm.stopRefreshTicker()

	if tm.protocol != nil {
		tm.protocol.uninit()
		tm.protocol = nil
	}

	stateMu.Lock()
	delete(registeredIotPlatformsMap, tm.hostId)
	if ids, ok := sensorsPerPlatformMap[tm.hostId]; ok {
		for _, id := range ids {
			delete(sensorsMap, id)
		}
		delete(sensorsPerPlatformMap, tm.hostId)
	}
	for subId, sensor := range subscriptionListPerSubId {
		if sensor.IotPlatformId == tm.hostId {
			delete(subscriptionListPerSubId, subId)
		}
	}
	if len(registeredIotPlatformsMap) == 0 {
		registeredIotPlatformsMap = nil
		sensorsMap = nil
		sensorsPerPlatformMap = nil
		subscriptionListPerSubId = nil
	}
	stateMu.Unlock()

	return nil
}

func (tm *SssMgr) startRefreshTicker() {
	log.Debug(">>> sssmgr.startRefreshTicker: ", time.Duration(refreshTickerExpeary)*time.Second)
	tm.refreshTicker = time.NewTicker(time.Duration(refreshTickerExpeary) * time.Second)
	ticker := tm.refreshTicker
	go func() {
		log.Debug("sssmgr.startRefreshTicker: Fired")
		for range ticker.C {
			// Refresh the list of devices
			tm.mutex.Lock()
			err := tm.populateDevicesPerIotPlatforms()
			tm.mutex.Unlock()
			if err != nil {
				log.Error(err)
			}

			continue // Infinite loop till stopRefreshTicker is call
		} // End of 'for' statement
		log.Debug("sssmgr.startRefreshTicker: Leaving time loop")
	}()
}

func (tm *SssMgr) stopRefreshTicker() {
	if tm.refreshTicker != nil {
		tm.refreshTicker.Stop()
		tm.refreshTicker = nil
		log.Debug("Refresh loop stopped")
	}
}

func (tm *SssMgr) SensorDiscoveryInfoAll() (sensors []SensorDiscoveryInfo, err error) {
	if profiling {
		profilingTimers["SensorDiscoveryInfoAll"] = time.Now()
	}

	log.Info(">>> sssmgr.SensorDiscoveryInfoAll")

	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	sensors = make([]SensorDiscoveryInfo, 0)
	stateMu.RLock()
	if len(registeredIotPlatformsMap) == 0 {
		stateMu.RUnlock()
		return sensors, nil
	}

	for _, v := range sensorsMap {
		if v.IotPlatformId != "" && v.IotPlatformId != tm.hostId {
			continue
		}
		log.Info("sssmgr.SensorDiscoveryInfoAll: adding sensor: ", v)
		sensors = append(sensors, v)
	}
	stateMu.RUnlock() // End of 'for' statement
	log.Info("sssmgr.SensorDiscoveryInfoAll: sensors: ", sensors)

	if profiling {
		now := time.Now()
		log.Debug("sssmgr.SensorDiscoveryInfoAll: ", now.Sub(profilingTimers["SensorDiscoveryInfoAll"]))
	}

	return sensors, nil
}

func (tm *SssMgr) GetSensor(sensorIdentifier string) (sensor SensorDiscoveryInfo, err error) {
	if profiling {
		profilingTimers["GetSensor"] = time.Now()
	}

	log.Info(">>> GetSensor: sensorIdentifier: ", sensorIdentifier)

	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	stateMu.RLock()
	val, ok := sensorsMap[sensorIdentifier]
	stateMu.RUnlock()
	if !ok || (val.IotPlatformId != "" && val.IotPlatformId != tm.hostId) {
		err = errors.New("Wrong Device identifier")
		return sensor, err
	}
	sensor = val

	if profiling {
		now := time.Now()
		log.Debug("GetSensor: ", now.Sub(profilingTimers["GetSensor"]))
	}
	log.Info("GetSensor: sensor: ", sensor)

	return sensor, nil
}

/*
 * func populateDevicesPerIotPlatforms IoT devices for all registered Iot platform
 * @return {struct} nil on success, error otherwise
 */
func (tm *SssMgr) populateDevicesPerIotPlatforms() error {

	if profiling {
		profilingTimers["populateDevicesPerIotPlatforms"] = time.Now()
	}

	stateMu.RLock()
	iotPlatform, ok := registeredIotPlatformsMap[tm.hostId]
	stateMu.RUnlock()
	if !ok {
		return nil
	}

	// Refresh only this manager's platform. Each SssMgr is bound to one CSE.
	log.Debug("populateDevicesPerIotPlatforms: processing: ", iotPlatform.Address)
	err := tm.populateSensors(iotPlatform, "3")
	if err != nil {
		log.Error("populateDevicesPerIotPlatforms: ", err)
	}
	err = tm.populateSensors(iotPlatform, "28")
	if err != nil {
		log.Error("populateDevicesPerIotPlatforms: ", err)
	}
	log.Debug("populateDevicesPerIotPlatforms: After loop")

	if profiling {
		now := time.Now()
		log.Debug("populateDevicesPerIotPlatforms: ", now.Sub(profilingTimers["populateDevicesPerIotPlatforms"]))
	}

	return nil
}

/*
 * func PopulateDevices IoT devices for the specified Iot platform
 * @param {string} iotPlatformId contains the IoT platform identifier
 * @return {struct} nil on success, error otherwise
 */
func (tm *SssMgr) populateSensors(iotPlatformInfo IotPlatformInfo, type_ string) error {
	if profiling {
		profilingTimers["populateSensors"] = time.Now()
	}

	log.Info(">>> populateSensors: iotPlatformInfo=", iotPlatformInfo)

	// 1. Get the list of the AE
	originator := iotPlatformInfo.Aei
	if originator == "" {
		originator = "Admin"
	}

	// Build the context
	var ctx = SssMgrBindingProtocolContext{
		host:   iotPlatformInfo.Address,
		port:   iotPlatformInfo.Port,
		scheme: iotPlatformInfo.Scheme,
		path:   tm.path,
		name:   iotPlatformInfo.Name,
		hostId: iotPlatformInfo.IotPlatformId,
		to:     iotPlatformInfo.Name,
		from:   originator,
		op:     2, // RETRIEVE
		ty:     -1,
		rqi:    uuid.New().String(),
		rvi:    []string{"5"}, // FIXME FSCOM How to get it
		code:   200,
	}
	// Build the queries
	queries := map[string]string{}
	queries["fu"] = "1" // Filter usage
	queries["ty"] = type_
	ctx.queries = queries

	err, resp := tm.protocol.send(ctx)
	if err != nil {
		log.Error("populateSensors: ", err.Error())
		return err
	}
	log.Debug("populateSensors: resp: ", resp)
	log.Debug("populateSensors: TypeOf(resp): ", reflect.TypeOf(resp))
	oneM2M_uril, ok := resp.(map[string]interface{})
	if !ok || oneM2M_uril == nil {
		err = errors.New("unexpected discovery response")
		log.Error("populateSensors: ", err.Error())
		return err
	}
	log.Debug("populateSensors: oneM2M_uril: ", oneM2M_uril)
	log.Debug("populateSensors: TypeOf(oneM2M_uril): ", reflect.TypeOf(oneM2M_uril))
	log.Debug("populateSensors: len(oneM2M_uril): ", len(oneM2M_uril))
	// Loop for each element and build the sensor list
	uril, ok := oneM2M_uril["m2m:uril"]
	if !ok || uril == nil {
		log.Debug("populateSensors: No m2m:uril in response")
		return nil
	}
	urilList, ok := uril.([]interface{})
	if !ok {
		log.Debug("populateSensors: m2m:uril is not a list")
		return nil
	}
	for _, v := range urilList {
		log.Debug("populateSensors: Processing key: v: ", v)
		log.Debug("populateSensors: Processing key: TypeOf(v): ", reflect.TypeOf(v))
		s := v.(string)
		if s == "laboai-cse-in/acpCreateACPs" || s == "laboai-cse-in/CAdmin" {
			// FIXME FSCOM Bug in MQTT DISCOVERY request which does not provide the same response that HTTP DISCOVERY with the same filter criteria
			continue // Discard it
		}
		ctx.to = s
		ctx.queries["fu"] = "2"
		err, resp := tm.protocol.send(ctx)
		if err != nil {
			log.Error("populateSensors: ", err.Error())
			continue
		}
		log.Debug("populateSensors: resp: ", resp)
		log.Debug("populateSensors: type(resp): ", reflect.TypeOf(resp))
		item, ok := resp.(map[string]interface{})
		if !ok || item == nil {
			continue
		}
		// oneM2M_cin := resp.(map[string]interface{})["m2m:cnt"].(map[string]interface{})
		// log.Debug("populateSensors: type(oneM2M_cin): ", reflect.TypeOf(oneM2M_cin))
		// log.Debug("populateSensors: len(oneM2M_cin): ", len(oneM2M_cin))
		// log.Debug("populateSensors: oneM2M_cin: ", oneM2M_cin)
		var sensor = SensorDiscoveryInfo{
			IotPlatformId: iotPlatformInfo.IotPlatformId,
		}
		if type_ == "28" {
			sensor.SensorType = "FLX"
			// TODO To be removed sensor.Flex = map[string]interface{}{} // FIXME FSCOM How to create flex container map from list of attributes recieved? Q&D: extract all attributes != attributes[AE/CNT/CIN]???
			// ==> Use cnd which contains the link to download the onthology or the reference to the oneM2M standard
		}
		sensor, err = tm.oneM2M_deserialize(sensor, item)
		if err != nil {
			log.Warn("populateSensors: ", err.Error())
			continue
		}

		stateMu.Lock()
		if sensorsMap == nil {
			sensorsMap = make(map[string]SensorDiscoveryInfo)
		}
		if sensorsPerPlatformMap == nil {
			sensorsPerPlatformMap = make(map[string][]string)
		}
		delete(sensorsMap, sensor.SensorIdentifier)
		sensorsMap[sensor.SensorIdentifier] = sensor
		log.Info("populateSensors: Before sensorsPerPlatformMap: ", sensorsPerPlatformMap)
		if len(sensorsPerPlatformMap[sensor.IotPlatformId]) == 0 {
			sensorsPerPlatformMap[sensor.IotPlatformId] = append(sensorsPerPlatformMap[sensor.IotPlatformId], sensor.SensorIdentifier)
		} else { // Replace or add a new element
			found := -1
			for i, v := range sensorsPerPlatformMap[sensor.IotPlatformId] {
				if v == sensor.SensorIdentifier {
					found = i
					break
				}
			} // End of 'for' statement
			log.Info("populateSensors: found idx: ", found)
			if found != -1 { // Replace
				sensorsPerPlatformMap[sensor.IotPlatformId] = append(sensorsPerPlatformMap[sensor.IotPlatformId][:found], sensorsPerPlatformMap[sensor.IotPlatformId][found+1:]...)
			}
			// Add
			sensorsPerPlatformMap[sensor.IotPlatformId] = append(sensorsPerPlatformMap[sensor.IotPlatformId], sensor.SensorIdentifier)
		}
		log.Info("populateSensors: After sensorsPerPlatformMap: ", sensorsPerPlatformMap)
		stateMu.Unlock()
	} // End of 'for' statement

	log.Info("populateSensors: sensorsMap: ", sensorsMap)
	log.Info("populateSensors: sensorsPerPlatformMap: ", sensorsPerPlatformMap)

	if profiling {
		now := time.Now()
		log.Debug("populateSensors: ", now.Sub(profilingTimers["populateSensors"]))
	}

	return nil
}

func (tm *SssMgr) ProcessSensorDiscoveryEventSubscription(sensorDiscoveryEventSubscription SensorDiscoveryEventSubscription) (err error) {
	log.Info("sbi.processSensorDiscoveryEventSubscription: ", sensorDiscoveryEventSubscription)

	return nil
}

/****
 * OneM2M_create - MEC IPE implemntation
 * FIXME To be reworked
 ****/

func (tm *SssMgr) OneM2M_create(sensor SensorDiscoveryInfo, path string, originator string) (sensorResp SensorDiscoveryInfo, err error) {

	if profiling {
		profilingTimers["OneM2M_create"] = time.Now()
	}

	log.Info(">>> OneM2M_create: sensor=", sensor)

	if sensor.IotPlatformId == "" {
		err = errors.New("IotPlatformId fiels shall be set")
		log.Error("OneM2M_create: ", err.Error())
		return sensorResp, err
	}

	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	// Create the initial payload dictionary
	var bodyMap = map[string]map[string]interface{}{}
	// Initialize the entry
	if sensor.SensorType == "AE" { // FIXME FSCOM Clarify how to map Deviceinfo with oneM2M AE/CNT/fexContainer
		bodyMap["m2m:ae"] = make(map[string]interface{}, 0)
		bodyMap["m2m:ae"]["api"] = "Norg.etsi." + sensor.IotPlatformId + "." + sensor.SensorIdentifier
		bodyMap["m2m:ae"]["rn"] = sensor.SensorIdentifier
		bodyMap["m2m:ae"]["rr"] = false
		bodyMap["m2m:ae"]["srv"] = []string{"5"}
		// Add metadata
		if len(sensor.SensorCharacteristicList) != 0 {
			for _, val := range sensor.SensorCharacteristicList {
				log.Debug("OneM2M_create: Adding AE metadata: ", val)
				bodyMap["m2m:ae"][val.CharacteristicName] = val.CharacteristicValue
			} // End of 'for' statement
		}
	} else if sensor.SensorType == "CNT" {
		bodyMap["m2m:cnt"] = make(map[string]interface{}, 0)
		bodyMap["m2m:cnt"]["mbs"] = 10000
		bodyMap["m2m:cnt"]["mni"] = 10
		bodyMap["m2m:cnt"]["rn"] = sensor.SensorIdentifier
		// Add metadata
		if len(sensor.SensorCharacteristicList) != 0 {
			for _, val := range sensor.SensorCharacteristicList {
				log.Debug("OneM2M_create: Adding CNT metadata: ", val)
				if val.CharacteristicName == "lbl" || val.CharacteristicName == "label" {
					bodyMap["m2m:cnt"][val.CharacteristicName] = []string{val.CharacteristicValue}
				} else {
					bodyMap["m2m:cnt"][val.CharacteristicName] = val.CharacteristicValue
				}
			} // End of 'for' statement
		}
	} else if sensor.SensorType == "CIN" {
		bodyMap["m2m:cin"] = make(map[string]interface{}, 0)
		bodyMap["m2m:cin"]["cnf"] = "text/plain:0"
		bodyMap["m2m:cin"]["rn"] = sensor.SensorIdentifier
		// Add metadata
		if len(sensor.SensorCharacteristicList) != 0 {
			for _, val := range sensor.SensorCharacteristicList {
				log.Debug("OneM2M_create: Adding CIN metadata: ", val)
				bodyMap["m2m:cin"][val.CharacteristicName] = val.CharacteristicValue
			} // End of 'for' statement
		}
	} else if sensor.SensorType == "FLX" {
		// Sanity checks
		if sensor.Flex == nil {
			err = errors.New("flex parameter shall be present")
			log.Error("OneM2M_create: ", err.Error())
			return sensorResp, err
		} else if _, ok := sensor.Flex["type"]; !ok {
			err = errors.New("'type' entry is required")
			log.Error("OneM2M_create: ", err.Error())
			return sensorResp, err
		} else if _, ok := sensor.Flex["type"].(string); !ok {
			err = errors.New("'type' entry is required")
			log.Error("OneM2M_create: ", err.Error())
			return sensorResp, err
		} else if _, ok := sensor.Flex["cnd"]; !ok {
			err = errors.New("'cnd' entry is required")
			log.Error("OneM2M_create: ", err.Error())
			return sensorResp, err
		} else if _, ok := sensor.Flex["cnd"].(string); !ok {
			err = errors.New("'cnd' entry is required")
			log.Error("OneM2M_create: ", err.Error())
			return sensorResp, err
		}
		k := sensor.Flex["type"].(string)
		bodyMap[k] = make(map[string]interface{}, 0)
		bodyMap[k]["rn"] = sensor.SensorIdentifier
		for i, v := range sensor.Flex {
			if i == "type" {
				continue // skip it
			}
			bodyMap[k][i] = v
		} // End of 'for' statement
	} else {
		err = errors.New("OneM2M_create: Invalid type")
		log.Error("OneM2M_create: ", err.Error())
		return sensorResp, err
	}

	pltf, found := platformById(sensor.IotPlatformId)
	if !found {
		err = errors.New("IoT platform not registered: " + sensor.IotPlatformId)
		log.Error("OneM2M_create: ", err.Error())
		return sensorResp, err
	}

	// Send it and get the result
	var ctx = SssMgrBindingProtocolContext{
		host:   pltf.Address,
		port:   pltf.Port,
		scheme: pltf.Scheme,
		path:   tm.path,
		name:   pltf.Name,
		hostId: sensor.IotPlatformId,
		to:     pltf.Name,
		from:   originator,
		op:     1, // CREATE
		rqi:    uuid.New().String(),
		rvi:    []string{"5"}, // FIXME FSCOM How to get it
		body:   bodyMap,
		code:   201,
	}
	if path != "" {
		ctx.to = path
	}
	if sensor.SensorType == "AE" {
		ctx.ty = 2
	} else if sensor.SensorType == "CNT" {
		ctx.ty = 3
	} else if sensor.SensorType == "CIN" {
		ctx.ty = 4
	} else if sensor.SensorType == "FLX" {
		ctx.ty = 28
	} else {
		err = errors.New("OneM2M_create: Invalid type")
		log.Error("send: ", err.Error())
		return sensorResp, err
	}

	err, resp := tm.protocol.send(ctx)
	if err != nil {
		if sensor.SensorType == "AE" && (strings.Contains(err.Error(), "409") || strings.Contains(err.Error(), "403") || strings.Contains(err.Error(), "already registered")) {
			log.Warn("OneM2M_create: AE already exists, reusing originator as Aei: ", originator)
			sensorResp.SensorIdentifier = sensor.SensorIdentifier
			sensorResp.SensorType = sensor.SensorType
			sensorResp.IotPlatformId = sensor.IotPlatformId
			sensorResp.Aei = originator

			setPlatformAei(sensor.IotPlatformId, originator)

			return sensorResp, nil
		}
		log.Error("OneM2M_create: ", err.Error())
		return sensorResp, err
	}
	log.Debug("OneM2M_create: resp: ", resp)
	log.Debug("OneM2M_create: TypeOf(resp): ", reflect.TypeOf(resp))
	if _, ok := resp.(map[string]interface{}); !ok {
		err = errors.New("Interface not available")
		log.Error("OneM2M_create: ", err.Error())
		return sensorResp, err
	}
	if val, ok := resp.(map[string]interface{})["m2m:dbg"]; ok {
		err = errors.New(val.(string))
		log.Error("OneM2M_create: ", err.Error())
		return sensorResp, err
	}

	// Add additional entries
	sensorResp.SensorIdentifier = sensor.SensorIdentifier
	sensorResp.SensorType = sensor.SensorType
	sensorResp.IotPlatformId = sensor.IotPlatformId
	sensorResp.SensorPosition = sensor.SensorPosition
	sensorResp.Flex = sensor.Flex
	sensorResp, err = tm.oneM2M_deserialize(sensorResp, resp.(map[string]interface{}))
	if err != nil {
		log.Error("OneM2M_create: ", err.Error())
		return sensorResp, err
	}
	log.Debug("OneM2M_create: sensorResp: ", sensorResp)

	if sensor.SensorType == "AE" && sensorResp.Aei != "" {
		setPlatformAei(sensor.IotPlatformId, sensorResp.Aei)
	}

	if profiling {
		now := time.Now()
		log.Debug("OneM2M_create: ", now.Sub(profilingTimers["OneM2M_create"]))
	}

	return sensorResp, nil
}

func (tm *SssMgr) OneM2M_discovery(type_ string, iotPlatformId string, originator string) (sensorResp []SensorDiscoveryInfo, err error) {

	if profiling {
		profilingTimers["OneM2M_discovery"] = time.Now()
	}

	log.Info(">>> OneM2OneM2M_discovery_get: path: ", type_)
	log.Info(">>> OneM2M_discovery: iotPlatformId: ", iotPlatformId)

	if iotPlatformId == "" {
		err = errors.New("IotPlatformId fiels shall be set")
		log.Error("OneM2M_discovery: ", err.Error())
		return nil, err
	}

	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	pltf, found := platformById(iotPlatformId)
	if !found {
		err = errors.New("IoT platform not registered: " + iotPlatformId)
		log.Error("OneM2M_discovery: ", err.Error())
		return nil, err
	}

	// 1. Get the list of the AE
	// Build the context
	var ctx = SssMgrBindingProtocolContext{
		host:   pltf.Address,
		port:   pltf.Port,
		scheme: pltf.Scheme,
		path:   tm.path,
		name:   pltf.Name,
		hostId: iotPlatformId,
		to:     pltf.Name,
		from:   originator,
		op:     2, // RETRIEVE
		ty:     -1,
		rqi:    uuid.New().String(),
		rvi:    []string{"5"}, // FIXME FSCOM How to get it
		code:   200,
	}
	// Build the queries parameters
	queries := map[string]string{}
	queries["fu"] = "1" // Filter usage
	if type_ == "AE" {
		ctx.ty = 2
		queries["ty"] = "2"
	} else if type_ == "CNT" {
		ctx.ty = 3
		queries["ty"] = "3"
	} else if type_ == "CIN" {
		ctx.ty = 4
		queries["ty"] = "4"
	} else {
		err = errors.New("OneM2M_discovery: Invalid type")
		log.Error("OneM2M_discovery: ", err.Error())
		return nil, err
	}
	ctx.queries = queries

	err, resp := tm.protocol.send(ctx)
	if err != nil {
		log.Error("OneM2M_discovery: ", err.Error())
		return nil, err
	}
	log.Debug("OneM2M_discovery: resp: ", resp)
	log.Debug("OneM2M_discovery: TypeOf(resp): ", reflect.TypeOf(resp))
	oneM2M_uril, ok := resp.(map[string]interface{})
	if !ok || oneM2M_uril == nil {
		err = errors.New("unexpected discovery response")
		log.Error("OneM2M_discovery: ", err.Error())
		return nil, err
	}
	log.Debug("OneM2M_discovery: oneM2M_uril: ", oneM2M_uril)
	log.Debug("OneM2M_discovery: TypeOf(oneM2M_uril): ", reflect.TypeOf(oneM2M_uril))
	log.Debug("OneM2M_discovery: len(oneM2M_uril): ", len(oneM2M_uril))
	sensorResp = make([]SensorDiscoveryInfo, len(oneM2M_uril))
	// Loop for each CIN and build the sensor list
	sensorResp = make([]SensorDiscoveryInfo, 0)
	urilList, ok := oneM2M_uril["m2m:uril"].([]interface{})
	if !ok {
		return sensorResp, nil
	}
	for _, v := range urilList {
		log.Debug("OneM2M_discovery: Processing key: v: ", v)
		log.Debug("OneM2M_discovery: Processing key: TypeOf(v): ", reflect.TypeOf(v))
		s := v.(string)
		if s == "laboai-cse-in/acpCreateACPs" || s == "laboai-cse-in/CAdmin" {
			// FIXME FSCOM Bug in MQTT DISCOVERY request which does not provide the same response that HTTP DISCOVERY with the same filter criteria
			continue // Discard it
		}
		r, err := tm.OneM2M_get(v.(string), iotPlatformId, originator)
		if err != nil {
			log.Error("OneM2M_discovery: ", err.Error())
			return nil, err
		}
		sensorResp = append(sensorResp, r)
	} // End of 'for' statement
	log.Debug("OneM2M_discovery: len(sensorResp): ", len(sensorResp))
	log.Debug("OneM2M_discovery: sensorResp: ", sensorResp)

	if profiling {
		now := time.Now()
		log.Debug("OneM2M_discovery: ", now.Sub(profilingTimers["OneM2M_discovery"]))
	}

	return sensorResp, nil
}

func (tm *SssMgr) OneM2M_get(path string, iotPlatformId string, originator string) (sensorResp SensorDiscoveryInfo, err error) {

	if profiling {
		profilingTimers["OneM2M_get"] = time.Now()
	}

	log.Info(">>> OneM2M_get: path: ", path)
	log.Info(">>> OneM2M_get: iotPlatformId: ", iotPlatformId)

	if path == "" || iotPlatformId == "" {
		err = errors.New("OneM2M_get: Invalid parameters")
		log.Error("OneM2M_get: ", err.Error())
		return sensorResp, err
	}

	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	pltf, found := platformById(iotPlatformId)
	if !found {
		err = errors.New("IoT platform not registered: " + iotPlatformId)
		log.Error("OneM2M_get: ", err.Error())
		return sensorResp, err
	}

	// 1. Get the list of the AE
	// Build the context
	var ctx = SssMgrBindingProtocolContext{
		host:   pltf.Address,
		port:   pltf.Port,
		scheme: pltf.Scheme,
		path:   tm.path,
		name:   pltf.Name,
		hostId: iotPlatformId,
		to:     path,
		from:   originator,
		op:     2, // RETRIEVE
		ty:     -1,
		rqi:    uuid.New().String(),
		rvi:    []string{"5"}, // FIXME FSCOM How to get it
		code:   200,
	}

	err, resp := tm.protocol.send(ctx)
	if err != nil {
		log.Error("OneM2M_get: ", err.Error())
		return sensorResp, err
	}
	log.Debug("OneM2M_get: resp: ", resp)
	log.Debug("OneM2M_get: TypeOf(resp): ", reflect.TypeOf(resp))
	if _, ok := resp.(map[string]interface{}); !ok {
		log.Error("OneM2M_get: Interface not available")
		return sensorResp, err
	}

	// Add additional entries
	sensorResp.IotPlatformId = iotPlatformId
	sensorResp, err = tm.oneM2M_deserialize(sensorResp, resp.(map[string]interface{}))

	if err != nil {
		log.Error("OneM2M_get: ", err.Error())
		return sensorResp, err
	}
	log.Debug("OneM2M_get: sensorResp: ", sensorResp)

	if profiling {
		now := time.Now()
		log.Debug("OneM2M_get: ", now.Sub(profilingTimers["OneM2M_get"]))
	}

	return sensorResp, nil
}

func (tm *SssMgr) OneM2M_subscribe(iotPlatformId string, path string, originator string) (subscription string, err error) {
	if profiling {
		profilingTimers["OneM2M_subscribe"] = time.Now()
	}

	log.Info(">>> OneM2M_subscribe: path: ", path)
	log.Info(">>> OneM2M_subscribe: iotPlatformId: ", iotPlatformId)

	if path == "" || iotPlatformId == "" {
		err = errors.New("OneM2M_subscribe: Invalid parameters")
		log.Error("OneM2M_subscribe: ", err.Error())
		return "", err
	}

	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	pltf, found := platformById(iotPlatformId)
	if !found {
		err = errors.New("IoT platform not registered: " + iotPlatformId)
		log.Error("OneM2M_subscribe: ", err.Error())
		return "", err
	}

	// 1. Get the list of the AE
	// Build the context
	var ctx = SssMgrBindingProtocolContext{
		host:   pltf.Address,
		port:   pltf.Port,
		scheme: pltf.Scheme,
		path:   tm.path,
		name:   pltf.Name,
		hostId: iotPlatformId,
		to:     path,
		from:   originator,
		op:     1, // SUBSCRIBE
		ty:     23,
		rqi:    uuid.New().String(),
		rvi:    []string{"5"}, // FIXME FSCOM How to get it
		code:   200,
	}
	var bodyMap = map[string]map[string]interface{}{}
	bodyMap["m2m:sub"] = make(map[string]interface{}, 0)
	net := make(map[string][]int)
	net["net"] = []int{1, 2, 3, 4}
	bodyMap["m2m:sub"]["enc"] = net
	mqttListener := os.Getenv("MEEP_SSS_MGR_MQTT_LISTENER")
	if mqttListener == "" {
		mqttListener = "mqtt://172.29.10.56:1883" // Fallback to legacy
	}
	httpListener := os.Getenv("MEEP_SSS_MGR_HTTP_LISTENER")
	if httpListener == "" {
		httpListener = "http://172.29.10.52:31122/" // Fallback to legacy
	}

	if tm.bindingProtocol == "MQTT" {
		bodyMap["m2m:sub"]["nu"] = []string{mqttListener}
	} else {
		bodyMap["m2m:sub"]["nu"] = []string{httpListener}
	}
	bodyMap["m2m:sub"]["rn"] = uuid.New().String()
	ctx.body = bodyMap

	err, resp := tm.protocol.send(ctx)
	if err != nil {
		log.Error("OneM2M_subscribe: ", err.Error())
		return "", err
	}
	log.Debug("OneM2M_subscribe: resp: ", resp)
	log.Debug("OneM2M_subscribe: TypeOf(resp): ", reflect.TypeOf(resp))
	if _, ok := resp.(map[string]interface{}); !ok {
		log.Error("OneM2M_subscribe: Interface not available")
		return "", err
	}

	if _, ok := resp.(map[string]interface{})["m2m:sub"].(map[string]interface{})["ri"]; !ok {
		err = errors.New("Subsciption identifier not present")
		log.Error("OneM2M_subscribe: ", err.Error())
		return "", err
	}
	subId := resp.(map[string]interface{})["m2m:sub"].(map[string]interface{})["ri"].(string)
	log.Debug("OneM2M_subscribe: subId: ", subId)

	// Add additional entries
	var v = SensorDiscoveryInfo{
		SensorIdentifier: subId,
		SensorType:       "SUB",
		IotPlatformId:    iotPlatformId,
	}
	v, err = tm.oneM2M_deserialize(v, resp.(map[string]interface{}))
	if err != nil {
		log.Error("OneM2M_subscribe: ", err.Error())
		return "", err
	}
	log.Debug("OneM2M_subscribe: v: ", v)

	stateMu.Lock()
	if subscriptionListPerSubId == nil {
		subscriptionListPerSubId = make(map[string]SensorDiscoveryInfo)
	}
	subscriptionListPerSubId[subId] = v
	stateMu.Unlock()

	//map[m2m:sub:map[ct:20250327T065259,502192 enc:map[net:[1 2 3 4]] et:20300326T065259,314830 lt:20250327T065259,502192 nct:1 nu:[http://172.29.10.52:31122] pi:C7feaadbb0400 ri:sub415201783625924167 rn:e31e93f4-8514-4c83-8854-364eee1ad305 ty:23]]

	if profiling {
		now := time.Now()
		log.Debug("OneM2M_subscribe: ", now.Sub(profilingTimers["OneM2M_subscribe"]))
	}

	return subId, nil
}

func (tm *SssMgr) OneM2M_delete(sensor SensorDiscoveryInfo, originator string) (err error) {

	if profiling {
		profilingTimers["OneM2M_delete"] = time.Now()
	}

	log.Info(">>> OneM2M_delete: sensor=", sensor)

	if sensor.SensorIdentifier == "" {
		err = errors.New("OneM2M_delete: Cannot find \"ri\" value")
		log.Error("OneM2M_delete: ", err.Error())
		return err
	}

	if sensor.IotPlatformId == "" {
		err = errors.New("IotPlatformId fiels shall be set")
		log.Error("OneM2M_delete: ", err.Error())
		return err
	}

	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	pltf, found := platformById(sensor.IotPlatformId)
	if !found {
		err = errors.New("IoT platform not registered: " + sensor.IotPlatformId)
		log.Error("OneM2M_delete: ", err.Error())
		return err
	}

	// Send it and get the result
	var ctx = SssMgrBindingProtocolContext{
		host:   pltf.Address,
		port:   pltf.Port,
		scheme: pltf.Scheme,
		path:   tm.path,
		name:   pltf.Name,
		hostId: sensor.IotPlatformId,
		to:     sensor.SensorIdentifier,
		from:   originator,
		op:     4, // DELETE
		ty:     -1,
		rqi:    uuid.New().String(),
		rvi:    []string{"5"}, // FIXME FSCOM How to get it
		code:   200,
	}
	err, _ = tm.protocol.send(ctx)
	if err != nil {
		log.Error("OneM2M_delete: ", err.Error())
		return err
	}

	if profiling {
		now := time.Now()
		log.Debug("OneM2M_delete: ", now.Sub(profilingTimers["OneM2M_delete"]))
	}

	return nil
}

func (tm *SssMgr) OneM2M_delete_subscription(subId string, originator string) (err error) {

	if profiling {
		profilingTimers["OneM2M_delete_subscription"] = time.Now()
	}

	log.Info(">>> OneM2M_delete_subscription: sensor=", subId)

	if subId == "" {
		err = errors.New("subId fiels shall be set")
		log.Error("OneM2M_delete_subscription: ", err.Error())
		return err
	}

	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	stateMu.RLock()
	sub, ok := subscriptionListPerSubId[subId]
	stateMu.RUnlock()
	if !ok {
		err = errors.New("Unkmown subscription identifier")
		log.Error("OneM2M_delete_subscription: ", err.Error())
		return err
	}
	pltf, found := platformById(sub.IotPlatformId)
	if !found {
		err = errors.New("IoT platform not registered: " + sub.IotPlatformId)
		log.Error("OneM2M_delete_subscription: ", err.Error())
		return err
	}

	// Send it and get the result
	var ctx = SssMgrBindingProtocolContext{
		host:   pltf.Address,
		port:   pltf.Port,
		scheme: pltf.Scheme,
		path:   tm.path,
		name:   pltf.Name,
		hostId: sub.IotPlatformId,
		to:     sub.SensorIdentifier,
		from:   originator,
		op:     4, // DELETE
		ty:     -1,
		rqi:    uuid.New().String(),
		rvi:    []string{"5"}, // FIXME FSCOM How to get it
		code:   200,
	}
	err, _ = tm.protocol.send(ctx)
	if err != nil {
		log.Error("OneM2M_delete_subscription: ", err.Error())
		return err
	}

	stateMu.Lock()
	delete(subscriptionListPerSubId, subId)
	log.Info("OneM2M_delete_subscription: New subscriptionListPerSubId: ", subscriptionListPerSubId)
	stateMu.Unlock()

	if profiling {
		now := time.Now()
		log.Debug("OneM2M_delete_subscription: ", now.Sub(profilingTimers["OneM2M_delete_subscription"]))
	}

	return nil
}

func (tm *SssMgr) notify(sub map[string]interface{}) {
	log.Debug(">>> notify: sub: ", sub)

	// Sanity checks
	if _, ok := sub["nev"]; !ok {
		log.Warn("notify: nev entry is missing")
		return
	}
	if _, ok := sub["nev"].(map[string]interface{}); !ok {
		log.Warn("notify: nev entry has unexpected type")
		return
	}
	if _, ok := sub["nev"].(map[string]interface{}); !ok {
		log.Warn("notify: nev entry has an unexpected type")
		return
	}
	if _, ok := sub["sur"]; !ok {
		log.Warn("notify: sur entry is missing")
		return
	}

	// Process subscription
	s := strings.Split(sub["sur"].(string), "/")
	log.Debug("notify: s: ", s)
	log.Debug("notify: CSE-ID: ", s[1])
	log.Debug("notify: subURL: ", strings.Join(s[2:], "/"))

	rep := sub["nev"].(map[string]interface{})
	log.Debug("notify: rep: ", rep["rep"].(map[string]interface{}))
	if _, ok := rep["rep"]; !ok {
		log.Warn("notify: rep entry is missing")
		return
	}
	if _, ok := rep["rep"].(map[string]interface{}); !ok {
		log.Warn("notify: rep entry has unexpected type")
		return
	}
	r := rep["rep"].(map[string]interface{})
	log.Debug("notify: r: ", r)
	// FIXME FSCOM How to mange the 3 levels of subscriptions???
	if v, ok := r["m2m:ae"]; ok { // Discovery notification
		log.Debug("notify: AE level")
		if tm.sss_discovery_notify != nil {
			go func() {
				tm.sss_discovery_notify(v.(map[string]interface{}))
			}()
		}
		return
	} else if v, ok := r["m2m:cnt"]; ok { // Status notification
		log.Debug("notify: CNT level")
		if tm.sss_status_notify != nil {
			go func() {
				tm.sss_status_notify(v.(map[string]interface{}))
			}()
		}
		return
	} else if v, ok := r["m2m:cin"]; ok { // Data notification
		if tm.sss_data_notify != nil {
			go func() {
				tm.sss_data_notify(v.(map[string]interface{}))
			}()
		}
		log.Debug("notify: CIN level")
		return
	} else {
		log.Debug("notify: Unexpected level")
		return
	}
}

func (tm *SssMgr) oneM2M_deserialize(sensor SensorDiscoveryInfo, response map[string]interface{}) (sensorResp SensorDiscoveryInfo, err error) {
	log.Debug(">>> oneM2M_deserialize: sensor: ", sensor)
	log.Debug(">>> oneM2M_deserialize: response: ", response)

	sensorResp = sensor // Same data structure

	if sensor.SensorType == "FLX" { // Extract flex specific attributes first
		// // Sanity checks
		if len(response) != 1 {
			err = errors.New("Wrong 'flex' parameter value")
			log.Error("oneM2M_deserialize: ", err.Error())
			return sensorResp, err
		}
		keys := make([]string, 0, len(response))
		for k := range response {
			keys = append(keys, k)
		}
		s := keys[0] // Resource type. E.g. cod:color
		log.Debug("oneM2M_deserialize: Extract flex specific attributes first")
		sensorResp.Flex = make(map[string]interface{})
		log.Debug("oneM2M_deserialize: s=", s)
		log.Debug("oneM2M_deserialize: response[s]: ", response[s])
		for k, v := range response[s].(map[string]interface{}) {
			log.Debug("oneM2M_deserialize: Processing k= ", k, ", v= ", v)
			if e, ok := response[s].(map[string]interface{})[k]; ok {
				log.Debug("oneM2M_deserialize: ", k, " is a flex attribute: ", e)
				sensorResp.Flex[k] = interface_to_string(e)
			}
		} // End of 'for' statement
		log.Debug("oneM2M_deserialize: After s=", s)
		sensorResp.Flex["type"] = s
		sensorResp.SensorIdentifier = sensorResp.Flex["ri"].(string)
		log.Debug("oneM2M_deserialize: response: ", response)
		log.Debug("oneM2M_deserialize: sensorResp.Flex: ", sensorResp.Flex)
	} else {
		for i, m := range response {
			log.Debug("==> ", i, " value is ", m)
			if _, ok := m.(map[string]interface{}); !ok {
				// Skip it
				log.Warn("oneM2M_deserialize: m is not map[string]interface{}")
				continue
			}
			// m is a map[string]interface.
			// loop over keys and values in the map.
			for k, v := range m.(map[string]interface{}) {
				log.Debug(k, " value is ", v)
				log.Debug("oneM2M_deserialize: type(v): ", reflect.TypeOf(v))

				if k == "ri" {
					if item, ok := v.(string); ok {
						sensorResp.SensorIdentifier = item
					} else {
						log.Error("oneM2M_deserialize: Failed to process ", k)
					}
				} else if k == "aei" {
					if item, ok := v.(string); ok {
						sensorResp.Aei = item
					} else {
						log.Error("oneM2M_deserialize: Failed to process ", k)
					}
				} else if k == "ty" {
					if item, ok := v.(float64); ok {
						switch item {
						case 2:
							sensorResp.SensorType = "AE"
						case 3:
							sensorResp.SensorType = "CNT"
						case 4:
							sensorResp.SensorType = "CIN"
						case 28:
							sensorResp.SensorType = "FLX"
						default:
							sensorResp.SensorType = strconv.FormatFloat(item, 'f', -1, 64)
						}
					} else {
						log.Error("oneM2M_deserialize: Failed to process ", k)
					}
				} else {
					sensorResp.SensorCharacteristicList = append(
						sensorResp.SensorCharacteristicList,
						SensorCharacteristic{
							CharacteristicName:  k,
							CharacteristicValue: interface_to_string(v),
						})
					// FIXME FSCOM To be removed
					// if item, ok := v.(string); ok {
					// 	sensorResp.SensorCharacteristicList = append(
					// 		sensorResp.SensorCharacteristicList,
					// 		SensorCharacteristic{
					// 			CharacteristicName:  k,
					// 			CharacteristicValue: string(item),
					// 		})
					// } else if item, ok := v.(float64); ok {
					// 	sensorResp.SensorCharacteristicList = append(
					// 		sensorResp.SensorCharacteristicList,
					// 		SensorCharacteristic{
					// 			CharacteristicName:  k,
					// 			CharacteristicValue: strconv.FormatFloat(item, 'f', -1, 64),
					// 		})
					// } else if item, ok := v.(int64); ok {
					// 	sensorResp.SensorCharacteristicList = append(
					// 		sensorResp.SensorCharacteristicList,
					// 		SensorCharacteristic{
					// 			CharacteristicName:  k,
					// 			CharacteristicValue: strconv.FormatInt(item, 10),
					// 		})
					// } else if item, ok := v.(bool); ok {
					// 	sensorResp.SensorCharacteristicList = append(
					// 		sensorResp.SensorCharacteristicList,
					// 		SensorCharacteristic{
					// 			CharacteristicName:  k,
					// 			CharacteristicValue: strconv.FormatBool(item),
					// 		})
					// } else if item, ok := v.([]string); ok {
					// 	sensorResp.SensorCharacteristicList = append(
					// 		sensorResp.SensorCharacteristicList,
					// 		SensorCharacteristic{
					// 			CharacteristicName:  k,
					// 			CharacteristicValue: strings.Join(item, ","),
					// 		})
					// } else if item, ok := v.([]int64); ok {
					// 	log.Warn("oneM2M_deserialize: Failed to convert list of int64 into string: ", item)
					// } else if item, ok := v.(map[string]interface{}); ok {
					// 	v, err := json.Marshal(item)
					// 	if err == nil {
					// 		sensorResp.SensorCharacteristicList = append(
					// 			sensorResp.SensorCharacteristicList,
					// 			SensorCharacteristic{
					// 				CharacteristicName:  k,
					// 				CharacteristicValue: string(v),
					// 			})
					// 	} else {
					// 		log.Warn("oneM2M_deserialize: ", err.Error())
					// 	}
					// } else if item, ok := v.([]interface{}); ok {
					// 	log.Debug("oneM2M_deserialize: Got []interface {} for ", k)
					// 	log.Debug("oneM2M_deserialize: ValueOf ", reflect.ValueOf(item))
					// 	v, err := json.Marshal(item)
					// 	if err == nil {
					// 		sensorResp.SensorCharacteristicList = append(
					// 			sensorResp.SensorCharacteristicList,
					// 			SensorCharacteristic{
					// 				CharacteristicName:  k,
					// 				CharacteristicValue: string(v),
					// 			})
					// 	} else {
					// 		log.Warn("oneM2M_deserialize: ", err.Error())
					// 	}
					// } else {
					// 	log.Warn("oneM2M_deserialize: Failed to process: ", k)
					// }
				}
			} // End of 'for' loop

		} // End of 'for' loop
	}
	log.Debug("oneM2M_deserialize: sensorResp: ", sensorResp)

	return sensorResp, nil
}

func interface_to_string(v interface{}) (s string) {
	if item, ok := v.(string); ok {
		return string(item)
	} else if item, ok := v.(float64); ok {
		return strconv.FormatFloat(item, 'f', -1, 64)
	} else if item, ok := v.(int64); ok {
		return strconv.FormatInt(item, 10)
	} else if item, ok := v.(bool); ok {
		return strconv.FormatBool(item)
	} else if item, ok := v.([]string); ok {
		return strings.Join(item, ",")
	} else if item, ok := v.([]int64); ok {
		log.Debug("interface_to_string: Got []interface {} for ", v)
		log.Debug("interface_to_string: ValueOf ", reflect.ValueOf(item))
		val, err := json.Marshal(item)
		if err == nil {
			return string(val)
		} else {
			log.Warn("interface_to_string: ", err.Error())
		}
	} else if item, ok := v.(map[string]interface{}); ok {
		val, err := json.Marshal(item)
		if err == nil {
			return string(val)
		} else {
			log.Warn("interface_to_string: ", err.Error())
		}
	} else if item, ok := v.([]interface{}); ok {
		log.Debug("interface_to_string: Got []interface {} for ", v)
		log.Debug("interface_to_string: ValueOf ", reflect.ValueOf(item))
		val, err := json.Marshal(item)
		if err == nil {
			return string(val)
		} else {
			log.Warn("interface_to_string: ", err.Error())
		}
	} else {
		log.Warn("interface_to_string: Failed to process: ", v)
	}

	return ""
}
