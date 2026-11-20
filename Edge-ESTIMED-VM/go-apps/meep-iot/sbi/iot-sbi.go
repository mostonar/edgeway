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
	"sync"

	tm "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-iot-mgr"
	log "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-logger"
	mod "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-model"
	mq "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-mq"
	sam "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-swagger-api-mgr"
)

const moduleName string = "meep-iot-sbi"

// var redisAddr string = "meep-redis-master.default.svc.cluster.local:6379"
// var influxAddr string = "http://meep-influxdb.default.svc.cluster.local:8086"

type SbiCfg struct {
	ModuleName     string
	SandboxName    string
	IotBroker      string
	IotTopic       string
	LocationName   string
	Originator     string
	RedisAddr      string
	InfluxAddr     string
	Locality       []string
	IotNotify      func(string, string)
	ScenarioNameCb func(string)
	CleanUpCb      func()
}

type IotSbi struct {
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
	iotMgr               *tm.IotMgr
	updateScenarioNameCB func(string)
	cleanUpCB            func()
	mutex                sync.Mutex
}

var sbi *IotSbi

type IotPlatformInfo struct {
	IotPlatformId               string
	UserTransportInfo           []MbTransportInfo
	CustomServicesTransportInfo []TransportInfo
	Enabled                     bool
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

// Init - IOT Service SBI initialization
func Init(cfg SbiCfg) (err error) {

	// Create new SBI instance
	if sbi != nil {
		sbi = nil
	}
	sbi = new(IotSbi)
	sbi.moduleName = cfg.ModuleName
	sbi.sandboxName = cfg.SandboxName
	sbi.mepName = cfg.LocationName
	sbi.scenarioName = ""
	sbi.updateScenarioNameCB = cfg.ScenarioNameCb
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

	// Connect to IOT Manager
	var mgrName string
	if sbi.mepName != "" {
		mgrName = sbi.mepName + "-" + sbi.moduleName
	} else {
		mgrName = sbi.moduleName
	}
	sbi.iotMgr, err = tm.NewIotMgr(mgrName, sbi.sandboxName, cfg.Originator)
	if err != nil {
		log.Error("Failed connection to IOT Manager: ", err)
		return err
	}
	log.Info("Connected to IOT Manager")

	// Initialize service
	processActiveScenarioUpdate()

	return nil
}

// Run - MEEP IOT execution
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

	// Delete IOT Manager
	if sbi.iotMgr != nil {
		err = sbi.iotMgr.DeleteIotMgr()
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

		// err := initializeIotMessageDistribution()
		// if err != nil {
		// 	log.Error("Failed to initialize V2X message distribution: ", err)
		// 	return
		// }
	}
}

func RegisterIotPlatformInfo(iotPlatformInfo IotPlatformInfo) (responseData IotPlatformInfo, err error) {
	log.Info(">>> RegisterIotPlatformInfo: ", iotPlatformInfo)

	// Populate the list of the devices for this IoT platform
	pltf := convertIotPlatformInfoToIotMgr(iotPlatformInfo)
	err = sbi.iotMgr.RegisterIotPlatformInfo(pltf)
	if err != nil {
		return iotPlatformInfo, err
	}

	log.Info("<<< RegisterIotPlatformInfo: ", iotPlatformInfo)
	return iotPlatformInfo, nil
}

func UpdateIotPlatform(platformInfo IotPlatformInfo) (IotPlatformInfo, error) {
	log.Info(">>> sbi.UpdateIotPlatform: ", platformInfo)

	p := convertIotPlatformInfoToIotMgr(platformInfo)
	err := sbi.iotMgr.UpdateIotPlatform(p)
	if err != nil {
		return platformInfo, err
	}

	return platformInfo, nil
}

func DeregisterIotPlatformInfo(iotPlatformId string) (err error) {
	log.Info(">>> DeregisterIotPlatformInfo: ", iotPlatformId)

	err = sbi.iotMgr.DeregisterIotPlatformInfo(iotPlatformId)
	if err != nil {
		return err
	}

	return nil
}

func GetDevices() (devices []DeviceInfo, err error) {
	log.Info(">>> sbi.GetDevices")

	dev, err := sbi.iotMgr.GetDevices()
	if err != nil {
		return nil, err
	}
	log.Info("sbi.GetDevices: dev: ", dev)

	devices, err = convertDeviceInfosFromIotMgr(dev)
	if err != nil {
		return nil, err
	}
	log.Info("sbi.GetDevices: devices: ", devices)

	return devices, nil
}

func GetDevice(deviceId string) (device DeviceInfo, err error) {
	log.Info(">>> sbi.GetDevice: ", deviceId)

	d, err := sbi.iotMgr.GetDevice(deviceId)
	if err != nil {
		return device, err
	}
	device = convertDeviceInfoFromIotMgr(d)

	return device, nil
}

func CreateDevice(device DeviceInfo) (deviceResp DeviceInfo, err error) {
	log.Info(">>> sbi.CreateDevice: ", device)

	d := convertDeviceInfoToIotMgr(device)
	dev, err := sbi.iotMgr.CreateDevice(d)
	if err != nil {
		return deviceResp, err
	}
	deviceResp = convertDeviceInfoFromIotMgr(dev)

	return deviceResp, nil
}

func UpdateDevice(device DeviceInfo) (deviceResp DeviceInfo, err error) {
	log.Info(">>> sbi.UpdateDevice: ", device)

	d := convertDeviceInfoToIotMgr(device)
	dev, err := sbi.iotMgr.UpdateDevice(d)
	if err != nil {
		return deviceResp, err
	}
	deviceResp = convertDeviceInfoFromIotMgr(dev)

	return deviceResp, nil
}

func DeleteDevice(deviceId string) (err error) {
	log.Info(">>> sbi.DeleteDevice: ", deviceId)

	err = sbi.iotMgr.DeleteDevice(deviceId)

	return err
}

func convertIotPlatformInfoToIotMgr(val IotPlatformInfo) (item tm.IotPlatformInfo) {
	item.IotPlatformId = val.IotPlatformId
	item.Enabled = val.Enabled
	for _, userTransportInfo := range val.UserTransportInfo {
		v := tm.MbTransportInfo{
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
			e := tm.EndPointInfo{
				Uris:        userTransportInfo.Endpoint.Uris,
				Fqdn:        userTransportInfo.Endpoint.Fqdn,
				Alternative: userTransportInfo.Endpoint.Alternative,
			}
			if len(userTransportInfo.Endpoint.Addresses) != 0 {
				for _, a := range userTransportInfo.Endpoint.Addresses {
					e.Addresses = append(e.Addresses, tm.Addresses{Host: a.Host, Port: a.Port})
				}
			}
			v.Endpoint = &e
		}
		if userTransportInfo.Security != nil {
			e := tm.SecurityInfo{}
			if userTransportInfo.Security.OAuth2Info != nil {
				e.OAuth2Info = &tm.OAuth2Info{
					GrantTypes:    userTransportInfo.Security.OAuth2Info.GrantTypes,
					TokenEndpoint: userTransportInfo.Security.OAuth2Info.TokenEndpoint,
				}
			}
			e.Extensions = userTransportInfo.Security.Extensions
		}
		if userTransportInfo.ImplSpecificInfo != nil {
			v.ImplSpecificInfo = &tm.ImplSpecificInfo{
				EventTopics:    userTransportInfo.ImplSpecificInfo.EventTopics,
				UplinkTopics:   userTransportInfo.ImplSpecificInfo.UplinkTopics,
				DownlinkTopics: userTransportInfo.ImplSpecificInfo.DownlinkTopics,
			}
		}
		item.UserTransportInfo = append(item.UserTransportInfo, v)
	}
	if val.CustomServicesTransportInfo != nil && len(val.CustomServicesTransportInfo) != 0 {
		item.CustomServicesTransportInfo = make([]tm.TransportInfo, 0)
		for _, customServicesTransportInfo := range val.CustomServicesTransportInfo {
			v := tm.TransportInfo{
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
				e := tm.EndPointInfo{
					Uris:        customServicesTransportInfo.Endpoint.Uris,
					Fqdn:        customServicesTransportInfo.Endpoint.Fqdn,
					Alternative: customServicesTransportInfo.Endpoint.Alternative,
				}
				if len(customServicesTransportInfo.Endpoint.Addresses) != 0 {
					for _, a := range customServicesTransportInfo.Endpoint.Addresses {
						e.Addresses = append(e.Addresses, tm.Addresses{Host: a.Host, Port: a.Port})
					}
				}
				v.Endpoint = &e
			}
			item.CustomServicesTransportInfo = append(item.CustomServicesTransportInfo, v)
		}
	}

	return item
}

func convertDeviceInfoFromIotMgr(dev tm.DeviceInfo) (device DeviceInfo) {
	log.Debug(">>> convertDeviceInfoFromIotMgr")

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
				s := *dev.DeviceSpecificMessageFormats.EventMsgFormat.SelectedSerializer
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
				s := *dev.DeviceSpecificMessageFormats.UplinkMsgFormat.SelectedSerializer
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

	log.Debug("convertDeviceInfoFromIotMgr: device: ", device)

	return device
}

func convertDeviceInfosFromIotMgr(devicesList []tm.DeviceInfo) (devices []DeviceInfo, err error) {
	log.Debug(">>> convertDeviceInfosFromIotMgr")

	devices = make([]DeviceInfo, len(devicesList))
	for idx, item := range devicesList { // FIXME FSCOM Add Filter
		devices[idx] = convertDeviceInfoFromIotMgr(item)
	} // End of 'for' statement
	log.Debug("convertDeviceInfosFromIotMgr: devices: ", devices)

	return devices, nil
}

func convertDeviceInfoToIotMgr(dev DeviceInfo) (device tm.DeviceInfo) {
	log.Debug(">>> convertDeviceInfoToIotMgr")

	device = tm.DeviceInfo{
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
		device.DeviceMetadata = make([]tm.KeyValuePair, len(dev.DeviceMetadata))
		for i, k := range dev.DeviceMetadata {
			device.DeviceMetadata[i] = tm.KeyValuePair{Key: k.Key, Value: k.Value}
		} // End of 'for' statement
	}
	if len(dev.RequestedMecTrafficRule) != 0 {
		device.RequestedMecTrafficRule = make([]tm.TrafficRuleDescriptor, len(dev.RequestedMecTrafficRule))
		for i, v := range dev.RequestedMecTrafficRule {
			device.RequestedMecTrafficRule[i] = tm.TrafficRuleDescriptor{
				TrafficRuleId: v.TrafficRuleId,
				FilterType:    v.FilterType,
				Priority:      v.Priority,
				Action:        v.Action,
			}
			if len(dev.RequestedMecTrafficRule[i].TrafficFilter) != 0 {
				device.RequestedMecTrafficRule[i].TrafficFilter = make([]tm.TrafficFilter, len(dev.RequestedMecTrafficRule[i].TrafficFilter))
				for j, u := range dev.RequestedMecTrafficRule[i].TrafficFilter {
					device.RequestedMecTrafficRule[i].TrafficFilter[j] = tm.TrafficFilter{
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
				device.RequestedMecTrafficRule[i].DstInterface = &tm.InterfaceDescriptor{
					InterfaceType: v.DstInterface.InterfaceType,
					SrcMACAddress: v.DstInterface.SrcMACAddress,
					DstMACAddress: v.DstInterface.DstMACAddress,
					DstIPAddress:  v.DstInterface.DstIPAddress,
				}
				if v.DstInterface.TunnelInfo != nil {
					device.RequestedMecTrafficRule[i].DstInterface.TunnelInfo = &tm.TunnelInfo{
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
		var deviceSpecificMessageFormats = tm.DeviceSpecificMessageFormats{}
		if dev.DeviceSpecificMessageFormats.EventMsgFormat != nil {
			deviceSpecificMessageFormats.EventMsgFormat = &tm.EventMsg{
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
				s := *dev.DeviceSpecificMessageFormats.EventMsgFormat.SelectedSerializer
				deviceSpecificMessageFormats.EventMsgFormat.SelectedSerializer = &s
			}
		}
		if dev.DeviceSpecificMessageFormats.UplinkMsgFormat != nil {
			deviceSpecificMessageFormats.UplinkMsgFormat = &tm.UplinkMsg{
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
				s := *dev.DeviceSpecificMessageFormats.UplinkMsgFormat.SelectedSerializer
				deviceSpecificMessageFormats.UplinkMsgFormat.SelectedSerializer = &s
			}
		}
		device.DeviceSpecificMessageFormats = &deviceSpecificMessageFormats
	}
	if dev.DownlinkInfo != nil {
		device.DownlinkInfo = &tm.DownlinkInfo{
			DownlinkTopic: dev.DownlinkInfo.DownlinkTopic,
			DevicePort:    dev.DownlinkInfo.DevicePort,
		}
	}
	log.Debug("convertDeviceInfoToIotMgr: device: ", device)

	return device
}
