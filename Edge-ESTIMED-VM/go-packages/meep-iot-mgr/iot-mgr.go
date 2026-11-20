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

package iotmgr

import (
	"errors"
	"sync"
	"time"

	log "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-logger"
	sssmgr "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-sss-mgr"
)

// IOT Manager
type IotMgr struct {
	name          string
	namespace     string
	mutex         sync.Mutex
	refreshTicker *time.Ticker
	originator    string
}

type IotPlatformInfo struct {
	IotPlatformId               string
	UserTransportInfo           []MbTransportInfo
	CustomServicesTransportInfo []TransportInfo
	Enabled                     bool
	oneM2M                      *sssmgr.SssMgr
}

type MbTransportInfo struct {
	Id               string
	Name             string
	Description      string
	Type_            *string
	Protocol         string
	Version          string
	Endpoint         *EndPointInfo
	Security         *SecurityInfo
	ImplSpecificInfo *ImplSpecificInfo
}

type TransportInfo struct {
	Id               string
	Name             string
	Description      string
	Type_            *string
	Protocol         string
	Version          string
	Endpoint         *EndPointInfo
	Security         *SecurityInfo
	ImplSpecificInfo string
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

type SecurityInfo struct {
	OAuth2Info *OAuth2Info
	Extensions string
}

type OAuth2Info struct {
	GrantTypes    []string
	TokenEndpoint string
}

type ImplSpecificInfo struct {
	EventTopics    []string
	UplinkTopics   []string
	DownlinkTopics []string
}

type TrafficRuleDescriptor struct {
	TrafficRuleId string
	FilterType    string
	Priority      int32
	TrafficFilter []TrafficFilter
	Action        string
	DstInterface  *InterfaceDescriptor
}

type InterfaceDescriptor struct {
	InterfaceType string
	TunnelInfo    *TunnelInfo
	SrcMACAddress string
	DstMACAddress string
	DstIPAddress  string
}

type TunnelInfo struct {
	TunnelType         string
	TunnelDstAddress   string
	TunnelSrcAddress   string
	TunnelSpecificData string
}

type TrafficFilter struct {
	SrcAddress       []string
	DstAddress       []string
	SrcPort          []string
	DstPort          []string
	Protocol         []string
	Tag              []string
	Uri              []string
	PacketLabel      []string
	SrcTunnelAddress []string
	TgtTunnelAddress []string
	SrcTunnelPort    []string
	DstTunnelPort    []string
	QCI              int32
	DSCP             int32
	TC               int32
}

type KeyValuePair struct {
	Key   string
	Value string
}

type DownlinkInfo struct {
	DownlinkTopic string
	DevicePort    int32
}

type DeviceSpecificMessageFormats struct {
	EventMsgFormat  *EventMsg
	UplinkMsgFormat *UplinkMsg
}

type EventMsg struct {
	EventTopic            string
	SelectedSerializer    *string
	IncludeDeviceAddr     bool
	IncludeDeviceMetadata bool
	IncludePei            bool
	IncludeSupi           bool
	IncludeImei           bool
	IncludeImsi           bool
	IncludeIccid          bool
	IncludeDeviceId       bool
}

type UplinkMsg struct {
	UplinkTopic           string
	SelectedSerializer    *string
	IncludeDevicePort     bool
	IncludeDeviceAddr     bool
	IncludeDeviceMetadata bool
	IncludePei            bool
	IncludeSupi           bool
	IncludeImei           bool
	IncludeImsi           bool
	IncludeIccid          bool
	IncludeDeviceId       bool
}

type DeviceInfo struct {
	DeviceAuthenticationInfo     string
	DeviceMetadata               []KeyValuePair
	Gpsi                         string
	Pei                          string
	Supi                         string
	Msisdn                       string
	Imei                         string
	Imsi                         string
	Iccid                        string
	DeviceId                     string
	RequestedMecTrafficRule      []TrafficRuleDescriptor
	RequestedIotPlatformId       string
	RequestedUserTransportId     string
	DeviceSpecificMessageFormats *DeviceSpecificMessageFormats
	DownlinkInfo                 *DownlinkInfo
	ClientCertificate            string
	Enabled                      bool
}

var registeredIotPlatformsMap = map[string]IotPlatformInfo{}              // List of discovered IOT Plateform
var devicesMap = map[string]DeviceInfo{}                                  // Map device by deviceId
var devicesPerPlatformMap = map[string][]string{}                         // Map deviceIds per platform
var platformPerUserTransportIdMap = map[string][]string{}                 // Map userTransportId per platform
var registeredIotPlatformsAEMap = map[string]sssmgr.SensorDiscoveryInfo{} // Map AE created per IOT Plateform

// Timer to refresh devices list for all IoT platform
const refreshTickerExpeary = 60 // In seconds

// Enable profiling
const profiling = false

var profilingTimers map[string]time.Time

const (
	headerAccept      = "application/json"
	headerContentType = "application/json"
)

// NewIotMgr - Creates and initializes a new IOT Traffic Manager
func NewIotMgr(name string, namespace string, originator string) (tm *IotMgr, err error) {
	if name == "" {
		err = errors.New("Missing connector name")
		return nil, err
	}

	// Create new Traffic Manager
	tm = new(IotMgr)
	tm.name = name
	tm.originator = originator
	if namespace != "" {
		tm.namespace = namespace
	} else {
		tm.namespace = "default"
	}

	tm.init()

	return tm, nil
}

// Profiling init
func (tm *IotMgr) init() {
	if profiling {
		profilingTimers = make(map[string]time.Time)
	}

	registeredIotPlatformsMap = make(map[string]IotPlatformInfo, 0)
	devicesMap = make(map[string]DeviceInfo, 0)
	devicesPerPlatformMap = make(map[string][]string, 0)
	platformPerUserTransportIdMap = make(map[string][]string, 0)
	registeredIotPlatformsAEMap = make(map[string]sssmgr.SensorDiscoveryInfo, 0)
	tm.refreshTicker = nil
}

// DeleteIotMgr -
func (tm *IotMgr) DeleteIotMgr() (err error) {
	registeredIotPlatformsMap = nil
	devicesMap = nil
	devicesPerPlatformMap = nil
	platformPerUserTransportIdMap = nil
	registeredIotPlatformsAEMap = nil
	tm.refreshTicker = nil
	return nil
}

func (tm *IotMgr) startRefreshTicker() {
	log.Debug(">>> iotmgr.startRefreshTicker: ", time.Duration(refreshTickerExpeary)*time.Second)

	tm.refreshTicker = time.NewTicker(time.Duration(refreshTickerExpeary) * time.Second)
	go func() {
		if tm.refreshTicker != nil {
			for range tm.refreshTicker.C {
				log.Debug("iotmgr.startRefreshTicker: Fired")
				log.Debug("iotmgr.startRefreshTicker: registeredIotPlatformsMap: ", registeredIotPlatformsMap)
				for _, v := range registeredIotPlatformsMap {
					if v.oneM2M != nil {
						tm.mutex.Lock()
						err := tm.populateDevicesPerIotPlatforms(v)
						tm.mutex.Unlock()
						if err != nil {
							log.Error(err)
						}
					} else {
						log.Debug("iotmgr.startRefreshTicker: Nothing to do")
					}
				} // End of 'for' statement
			} // End of 'for' statement
			log.Debug("iotmgr.startRefreshTicker: Leaving time loop")
		}
	}()
}

func (tm *IotMgr) stopRefreshTicker() {
	if tm.refreshTicker != nil {
		// Refresh the list of devices
		tm.refreshTicker.Stop()
		tm.refreshTicker = nil
		log.Debug("Refresh loop stopped")
	}
}

func (tm *IotMgr) RegisterIotPlatformInfo(iotPlatformInfo IotPlatformInfo) (err error) {
	if profiling {
		profilingTimers["RegisterIotPlatformInfo"] = time.Now()
	}

	log.Info(">>> RegisterIotPlatformInfo: iotPlatformId: ", iotPlatformInfo)

	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	if iotPlatformInfo.Enabled {
		iotPlatformInfo.oneM2M = nil
		log.Debug("RegisterIotPlatformInfo: CustomServicesTransportInfo: ", iotPlatformInfo.CustomServicesTransportInfo)
		if len(iotPlatformInfo.CustomServicesTransportInfo) == 0 ||
			iotPlatformInfo.CustomServicesTransportInfo[0].Endpoint == nil ||
			(len(iotPlatformInfo.CustomServicesTransportInfo[0].Endpoint.Addresses) == 0 &&
				len(iotPlatformInfo.CustomServicesTransportInfo[0].Endpoint.Uris) == 0 &&
				len(iotPlatformInfo.CustomServicesTransportInfo[0].Endpoint.Fqdn) == 0) {
			log.Error("RegisterIotPlatformInfo: Cannot use provided CustomServicesTransportInfo")
		} else {
			log.Debug("RegisterIotPlatformInfo: CustomServicesTransportInfo[0].Endpoint: ", iotPlatformInfo.CustomServicesTransportInfo[0].Endpoint)
			log.Debug("RegisterIotPlatformInfo: CustomServicesTransportInfo[0].Endpoint.Addresses: ", iotPlatformInfo.CustomServicesTransportInfo[0].Endpoint.Addresses)
			log.Debug("RegisterIotPlatformInfo: CustomServicesTransportInfo[0].Endpoint.Uris: ", iotPlatformInfo.CustomServicesTransportInfo[0].Endpoint.Uris)
			log.Debug("RegisterIotPlatformInfo: CustomServicesTransportInfo[0].Protocol: ", iotPlatformInfo.CustomServicesTransportInfo[0].Protocol)
			log.Debug("RegisterIotPlatformInfo: CustomServicesTransportInfo[0].Name: ", iotPlatformInfo.CustomServicesTransportInfo[0].Name)
			var pltf *sssmgr.SssMgr = nil
			var err error
			if len(iotPlatformInfo.CustomServicesTransportInfo[0].Endpoint.Addresses) != 0 {
				log.Debug("RegisterIotPlatformInfo: Using CustomServicesTransportInfo[0].Endpoint.Addresses: ", iotPlatformInfo.CustomServicesTransportInfo[0].Endpoint.Addresses)
				for _, addr := range iotPlatformInfo.CustomServicesTransportInfo[0].Endpoint.Addresses {
					pltf, err = sssmgr.NewSssMgr(tm.name, tm.namespace, iotPlatformInfo.CustomServicesTransportInfo[0].Protocol, addr.Host, int(addr.Port), iotPlatformInfo.IotPlatformId, iotPlatformInfo.CustomServicesTransportInfo[0].Name, nil, nil, nil)
					if err == nil {
						break
					}
				}
				if err != nil {
					log.Error("RegisterIotPlatformInfo: ", err)
					iotPlatformInfo.oneM2M = nil
					return err
				}
			} else if len(iotPlatformInfo.CustomServicesTransportInfo[0].Endpoint.Uris) != 0 {
				log.Debug("RegisterIotPlatformInfo: Using CustomServicesTransportInfo[0].Endpoint.Uris: ", iotPlatformInfo.CustomServicesTransportInfo[0].Endpoint.Uris)
				for _, uri := range iotPlatformInfo.CustomServicesTransportInfo[0].Endpoint.Uris {
					pltf, err = sssmgr.NewSssMgr_with_uri(tm.name, tm.namespace, iotPlatformInfo.CustomServicesTransportInfo[0].Protocol, uri, iotPlatformInfo.IotPlatformId, iotPlatformInfo.CustomServicesTransportInfo[0].Name, nil, nil, nil)
					if err == nil {
						break
					}
				}
				if err != nil {
					log.Error("RegisterIotPlatformInfo: ", err)
					iotPlatformInfo.oneM2M = nil
					return err
				}
			} else if len(iotPlatformInfo.CustomServicesTransportInfo[0].Endpoint.Fqdn) != 0 {
				log.Debug("RegisterIotPlatformInfo: Using CustomServicesTransportInfo[0].Endpoint.Fqdn: ", iotPlatformInfo.CustomServicesTransportInfo[0].Endpoint.Fqdn)
				for _, fqdn := range iotPlatformInfo.CustomServicesTransportInfo[0].Endpoint.Fqdn {
					pltf, err = sssmgr.NewSssMgr(tm.name, tm.namespace, iotPlatformInfo.CustomServicesTransportInfo[0].Protocol, fqdn, 0, iotPlatformInfo.IotPlatformId, iotPlatformInfo.CustomServicesTransportInfo[0].Name, nil, nil, nil)
					if err == nil {
						break
					}
				}
				if err != nil {
					log.Error("RegisterIotPlatformInfo: ", err)
					iotPlatformInfo.oneM2M = nil
					return err
				}
			} else {
				log.Error("RegisterIotPlatformInfo: Cannot use provided CustomServicesTransportInfo")
				return errors.New("Cannot use provided CustomServicesTransportInfo")
			}
			// No error, create the IoT platform
			log.Info("RegisterIotPlatformInfo: IoT pltf created")
			iotPlatformInfo.oneM2M = pltf
			// Create the an AE for this ETSI MEC platform
			var ae = sssmgr.SensorDiscoveryInfo{
				SensorIdentifier: iotPlatformInfo.IotPlatformId,
				SensorType:       "AE",
				SensorPosition:   nil,
				IotPlatformId:    iotPlatformInfo.IotPlatformId,
			}
			ae, err = pltf.OneM2M_create(ae, "", tm.originator)
			if err != nil {
				log.Error("RegisterIotPlatformInfo: Failed to create new AE sensor, plateform registration rejected")
				pltf.DeleteSssMgr()
				iotPlatformInfo.oneM2M = nil
				return err
			} else {
				registeredIotPlatformsMap[iotPlatformInfo.IotPlatformId] = iotPlatformInfo
				registeredIotPlatformsAEMap[iotPlatformInfo.IotPlatformId] = ae
				if tm.refreshTicker == nil {
					log.Info("RegisterIotPlatformInfo: Start RefreshTicker")
					tm.startRefreshTicker()
				}
				log.Info("RegisterIotPlatformInfo: iotPlatformId: ", registeredIotPlatformsMap[iotPlatformInfo.IotPlatformId])
			}
		}
	}

	if profiling {
		now := time.Now()
		log.Debug("RegisterIotPlatformInfo: ", now.Sub(profilingTimers["RegisterIotPlatformInfo"]))
	}
	return nil
}

func (tm *IotMgr) DeregisterIotPlatformInfo(iotPlatformId string) (err error) {
	if profiling {
		profilingTimers["DeregisterIotPlatformInfo"] = time.Now()
	}

	log.Info(">>> DeregisterIotPlatformInfo: iotPlatformId: ", iotPlatformId)

	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	// Remove the list of the devices for this IoT platform
	if val, ok := devicesPerPlatformMap[iotPlatformId]; ok {
		// Free resources from devicesMap map
		for _, dev := range val {
			delete(devicesMap, dev)
		} // End of 'for' statement
		delete(devicesPerPlatformMap, iotPlatformId)
		log.Info("DeregisterIotPlatformInfo: platformPerUserTransportIdMap (before): ", platformPerUserTransportIdMap)
		for _, rule := range platformPerUserTransportIdMap {
			for idx, pltf := range rule {
				if pltf == iotPlatformId {
					rule = append(rule[:idx], rule[idx+1:]...)
				}
			} // End of 'for' statement
		} // End of 'for' statement
		log.Info("DeregisterIotPlatformInfo: platformPerUserTransportIdMap (after): ", platformPerUserTransportIdMap)
	}
	if pltf, ok := registeredIotPlatformsMap[iotPlatformId]; ok {
		if pltf.oneM2M != nil {
			if val, ok := registeredIotPlatformsAEMap[iotPlatformId]; ok {
				_ = pltf.oneM2M.OneM2M_delete(val, val.IotPlatformId)
				delete(registeredIotPlatformsAEMap, iotPlatformId)
			}
			_ = pltf.oneM2M.DeleteSssMgr()
			pltf.oneM2M = nil
			log.Info("RegisterIotPlatformInfo: IoT pltf removed")
		}
		delete(registeredIotPlatformsMap, iotPlatformId)
		if len(registeredIotPlatformsMap) == 0 {
			if tm.refreshTicker != nil {
				log.Info("RegisterIotPlatformInfo: Stop RefreshTicker")
				tm.stopRefreshTicker()
				tm.refreshTicker = nil
			}

		}
	}

	if profiling {
		now := time.Now()
		log.Debug("DeregisterIotPlatformInfo: ", now.Sub(profilingTimers["DeregisterIotPlatformInfo"]))
	}
	return nil
}

func (tm *IotMgr) UpdateIotPlatform(platform IotPlatformInfo) error {
	if profiling {
		profilingTimers["UpdateIotPlatform"] = time.Now()
	}
	log.Info(">>> UpdateIotPlatform: ", platform)

	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	if _, ok := registeredIotPlatformsMap[platform.IotPlatformId]; !ok {
		return errors.New("IoT platform not found")
	}

	// Keep the original oneM2M reference if it exists
	platform.oneM2M = registeredIotPlatformsMap[platform.IotPlatformId].oneM2M
	registeredIotPlatformsMap[platform.IotPlatformId] = platform

	if profiling {
		now := time.Now()
		log.Debug("UpdateIotPlatform: ", now.Sub(profilingTimers["UpdateIotPlatform"]))
	}
	return nil
}

func (tm *IotMgr) GetDevices() (devices []DeviceInfo, err error) {
	if profiling {
		profilingTimers["GetDevices"] = time.Now()
	}

	log.Info(">>> GetDevices")

	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	devices = make([]DeviceInfo, 0)
	if len(registeredIotPlatformsMap) == 0 {
		return devices, nil
	}

	for _, v := range devicesMap { // Process all devices
		log.Info("GetDevices: adding device: ", v)
		devices = append(devices, v)
	} // End of 'for' statement
	log.Info("GetDevices: devices: ", devices)

	if profiling {
		now := time.Now()
		log.Debug("GetDevices: ", now.Sub(profilingTimers["GetDevices"]))
	}

	return devices, nil
}

func (tm *IotMgr) GetDevice(deviceId string) (device DeviceInfo, err error) {
	if profiling {
		profilingTimers["GetDevice"] = time.Now()
	}

	log.Info(">>> GetDevice: deviceId: ", deviceId)

	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	if val, ok := devicesMap[deviceId]; !ok {
		err = errors.New("Wrong Device identifier")
		return device, err
	} else {
		device = val
	}

	if profiling {
		now := time.Now()
		log.Debug("GetDevice: ", now.Sub(profilingTimers["GetDevice"]))
	}
	log.Info("GetDevice: device: ", device)

	return device, nil
}

func (tm *IotMgr) CreateDevice(device DeviceInfo) (deviceResp DeviceInfo, err error) {
	if profiling {
		profilingTimers["CreateDevice"] = time.Now()
	}

	log.Info(">>> CreateDevice: ", device)

	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	if len(device.RequestedIotPlatformId) != 0 {
		deviceResp, err = tm.createDeviceWithIotPlatformId(device, device.RequestedIotPlatformId)
	} else {
		deviceResp, err = tm.createDeviceWithRequestedUserTransportId(device, device.RequestedUserTransportId)
	}
	if err != nil {
		log.Error(err.Error())
		return deviceResp, err
	}
	log.Info("CreateDevice: deviceResp: ", deviceResp)

	if profiling {
		now := time.Now()
		log.Debug("CreateDevice: ", now.Sub(profilingTimers["CreateDevice"]))
	}

	return deviceResp, nil
}

func (tm *IotMgr) UpdateDevice(device DeviceInfo) (deviceResp DeviceInfo, err error) {
	if profiling {
		profilingTimers["UpdateDevice"] = time.Now()
	}
	log.Info(">>> UpdateDevice: ", device)

	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	oldDevice, ok := devicesMap[device.DeviceId]
	if !ok {
		return deviceResp, errors.New("Device not found")
	}

	// Handle RequestedIotPlatformId change
	if oldDevice.RequestedIotPlatformId != device.RequestedIotPlatformId {
		if oldDevice.RequestedIotPlatformId != "" {
			if list, ok := devicesPerPlatformMap[oldDevice.RequestedIotPlatformId]; ok {
				for i, devId := range list {
					if devId == device.DeviceId {
						devicesPerPlatformMap[oldDevice.RequestedIotPlatformId] = append(list[:i], list[i+1:]...)
						break
					}
				}
			}
		}
		if device.RequestedIotPlatformId != "" {
			devicesPerPlatformMap[device.RequestedIotPlatformId] = append(devicesPerPlatformMap[device.RequestedIotPlatformId], device.DeviceId)
		}
	}

	devicesMap[device.DeviceId] = device

	deviceResp = device
	if profiling {
		now := time.Now()
		log.Debug("UpdateDevice: ", now.Sub(profilingTimers["UpdateDevice"]))
	}
	return deviceResp, nil
}

func (tm *IotMgr) DeleteDevice(deviceId string) (err error) {
	if profiling {
		profilingTimers["DeleteDevice"] = time.Now()
	}

	log.Info(">>> DeleteDevice: device: ", deviceId)

	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	if _, ok := devicesMap[deviceId]; !ok {
		err = errors.New("Invalid device identifier")
		log.Error(err.Error())
		return err
	}

	device := devicesMap[deviceId]
	// Remove the list of the devices for this IoT platform
	if val, ok := devicesPerPlatformMap[device.RequestedIotPlatformId]; ok {
		// Free resource from devicesMap map
		log.Info("DeleteDevice: devicesPerPlatformMap (before): ", devicesPerPlatformMap)
		for idx, devId := range val {
			if devId == device.DeviceId {
				val = append(val[:idx], val[idx+1:]...)
				break
			}
		} // End of 'for' statement
	}
	log.Info("DeleteDevice: devicesPerPlatformMap (after): ", devicesPerPlatformMap)
	// Free resource from devicesMap map
	log.Info("DeleteDevice: devicesMap (before): ", devicesMap)
	delete(devicesMap, device.DeviceId)
	log.Info("DeleteDevice: devicesMap (after): ", devicesMap)

	if profiling {
		now := time.Now()
		log.Debug("DeleteDevice: ", now.Sub(profilingTimers["DeleteDevice"]))
	}

	return nil
}

func (tm *IotMgr) createDeviceWithIotPlatformId(device DeviceInfo, requestedIotPlatformId string) (deviceResp DeviceInfo, err error) {
	log.Info(">>> createDeviceWithIotPlatformId: ", device)

	// Sanity checks
	if _, ok := registeredIotPlatformsMap[requestedIotPlatformId]; !ok {
		err = errors.New("Invalid IotPlatform identifier")
		return deviceResp, err
	}
	if _, ok := devicesMap[device.DeviceId]; ok {
		err = errors.New("Device already exist")
		return deviceResp, err
	}
	if _, ok := registeredIotPlatformsAEMap[requestedIotPlatformId]; !ok {
		err = errors.New("Devaice cannot be created without oneM2M AE parent")
		return deviceResp, err
	}

	// FIXME FSCOM How to manage these fields from DeviceInfo
	if registeredIotPlatformsMap[requestedIotPlatformId].oneM2M != nil && device.Enabled {
		log.Info("createDeviceWithIotPlatformId: Create device on IoT platform", device)
		var sensor = sssmgr.SensorDiscoveryInfo{
			SensorIdentifier: device.DeviceId,
			SensorType:       "FLX", // FIXME FSCOM How to retrieve this info
			SensorPosition:   nil,
			IotPlatformId:    requestedIotPlatformId,
		}
		sensor.Flex = make(map[string]interface{}, 0)
		sensor.Flex["type"] = "mec:iotDev"
		sensor.Flex["cnd"] = "org.onem2m.common.mec.device.iotDevice"
		sensor.Flex["devId"] = device.DeviceId
		sensor.Flex["rIPId"] = requestedIotPlatformId
		sensor.Flex["enabd"] = device.Enabled
		if device.Gpsi != "" {
			sensor.Flex["gpsi"] = device.Gpsi
		}
		if device.Pei != "" {
			sensor.Flex["pei"] = device.Pei
		}
		if device.Supi != "" {
			sensor.Flex["supi"] = device.Supi
		}
		if device.Msisdn != "" {
			sensor.Flex["msisdn"] = device.Msisdn
		}
		if device.Imei != "" {
			sensor.Flex["imei"] = device.Imei
		}
		if device.Imsi != "" {
			sensor.Flex["imsi"] = device.Imsi
		}
		if device.Iccid != "" {
			sensor.Flex["iccid"] = device.Iccid
		}

		if len(device.DeviceMetadata) != 0 {
			sensor.SensorCharacteristicList = make([]sssmgr.SensorCharacteristic, len(device.DeviceMetadata))
			for i, c := range device.DeviceMetadata {
				sensor.SensorCharacteristicList[i] = sssmgr.SensorCharacteristic{CharacteristicName: c.Key, CharacteristicValue: c.Value}
			} // End of 'for' statement
		}
		log.Info("createDeviceWithIotPlatformId: Call OneM2M_create: ", sensor)
		log.Info("createDeviceWithIotPlatformId: registeredIotPlatformsMap: ", registeredIotPlatformsMap)
		sensor, err := registeredIotPlatformsMap[requestedIotPlatformId].oneM2M.OneM2M_create(sensor, registeredIotPlatformsAEMap[requestedIotPlatformId].SensorIdentifier, tm.originator)
		if err != nil {
			return deviceResp, err
		}
	}

	devicesMap[device.DeviceId] = device
	devicesPerPlatformMap[device.DeviceId] = append(devicesPerPlatformMap[device.DeviceId], requestedIotPlatformId)
	platformPerUserTransportIdMap[requestedIotPlatformId] = append(platformPerUserTransportIdMap[requestedIotPlatformId], device.RequestedUserTransportId)

	deviceResp = device
	log.Debug("createDeviceWithIotPlatformId: deviceResp: ", deviceResp)

	return deviceResp, nil
}

func (tm *IotMgr) createDeviceWithRequestedUserTransportId(device DeviceInfo, requestedUserTransportId string) (deviceResp DeviceInfo, err error) {
	log.Info(">>> createDeviceWithRequestedUserTransportId: ", device)

	if val, ok := platformPerUserTransportIdMap[requestedUserTransportId]; ok {
		deviceResp, err = tm.createDeviceWithIotPlatformId(device, val[0])
	} else {
		err = errors.New("Invalid requestedUserTransportId")
	}
	if err != nil {
		log.Error("createDeviceWithIotPlatformId: ", err.Error())
		return deviceResp, err
	}
	// FIXME FSCOM How to manage these fields from DeviceInfo
	// 	DeviceAuthenticationInfo string
	// 	Gpsi                     string
	// 	Pei                      string
	// 	Supi                     string
	// 	Msisdn                   string
	// 	Imei                     string
	// 	Imsi                     string
	// 	Iccid                    string
	// 	RequestedMecTrafficRule  []TrafficRuleDescriptor
	// 	//DeviceSpecificMessageFormats *DeviceSpecificMessageFormats
	// 	//DownlinkInfo *DownlinkInfo
	// 	ClientCertificate string
	// }
	log.Info("createDeviceWithIotPlatformId: deviceResp: ", deviceResp)

	return deviceResp, nil
}

func (tm *IotMgr) resetMaps(iotPlatformId string) {
	log.Info(">>> resetMaps: ", iotPlatformId)

	// Free resources from devicesMap map
	// Remove all devices for this IoT platform
	log.Info("resetMaps: devicesMap (before): ", devicesMap)
	for _, deviceId := range devicesPerPlatformMap[iotPlatformId] {
		delete(devicesMap, deviceId)
	} // End of 'for' statement
	log.Info("resetMaps: devicesMap (after): ", devicesMap)

	// Remove all devices for this IoT platform
	log.Info("resetMaps: devicesPerPlatformMap (before): ", devicesPerPlatformMap)
	delete(devicesPerPlatformMap, iotPlatformId)
	log.Info("resetMaps: devicesPerPlatformMap (after): ", devicesPerPlatformMap)

	log.Info("resetMaps: platformPerUserTransportIdMap (before): ", platformPerUserTransportIdMap)
	for _, rule := range platformPerUserTransportIdMap {
		for idx, pltf := range rule {
			if pltf == iotPlatformId {
				rule = append(rule[:idx], rule[idx+1:]...)
			}
		} // End of 'for' statement
	} // End of 'for' statement
	log.Info("resetMaps: platformPerUserTransportIdMap (after): ", platformPerUserTransportIdMap)
}

func (tm *IotMgr) populateDevicesPerIotPlatforms(iotPlatformInfo IotPlatformInfo) (err error) {
	log.Info(">>> populateDevicesPerIotPlatforms: ", iotPlatformInfo)

	if iotPlatformInfo.oneM2M == nil {
		log.Info("populateDevicesPerIotPlatforms: Nothing to do")
		return nil
	}

	// Reset maps
	tm.resetMaps(iotPlatformInfo.IotPlatformId)

	sensors, err := iotPlatformInfo.oneM2M.SensorDiscoveryInfoAll()
	if err != nil {
		log.Error("populateDevicesPerIotPlatforms: ", err)
		return err
	}
	log.Info("populateDevicesPerIotPlatforms: sensors: ", sensors)

	for _, sensor := range sensors {
		var deviceInfo = DeviceInfo{
			DeviceId:               sensor.SensorIdentifier,
			RequestedIotPlatformId: sensor.IotPlatformId,
			Enabled:                true,
		}
		deviceInfo.DeviceMetadata = make([]KeyValuePair, len(sensor.SensorCharacteristicList))
		for i, c := range sensor.SensorCharacteristicList {
			deviceInfo.DeviceMetadata[i] = KeyValuePair{Key: c.CharacteristicName, Value: c.CharacteristicValue}
		} // End of 'for' statement
		log.Info("populateDevicesPerIotPlatforms: len(sensor.Flex): ", len(sensor.Flex))
		if len(sensor.Flex) != 0 {
			for k, v := range sensor.Flex { // Process all flex attributes
				log.Info("populateDevicesPerIotPlatforms: key: ", k)
				log.Info("populateDevicesPerIotPlatforms: value: ", v)
				deviceInfo.DeviceMetadata = append(deviceInfo.DeviceMetadata, KeyValuePair{Key: k, Value: v.(string)})
			} // End of 'for' statement
		}

		log.Info("populateDevicesPerIotPlatforms: add deviceInfo: ", deviceInfo)
		devicesMap[deviceInfo.DeviceId] = deviceInfo
		devicesPerPlatformMap[iotPlatformInfo.IotPlatformId] = append(devicesPerPlatformMap[iotPlatformInfo.IotPlatformId], deviceInfo.DeviceId)
	} // End of 'for' statement
	log.Info("populateDevicesPerIotPlatforms: devicesMap: ", devicesMap)
	log.Info("populateDevicesPerIotPlatforms: devicesPerPlatformMap: ", devicesPerPlatformMap)

	return nil
}
