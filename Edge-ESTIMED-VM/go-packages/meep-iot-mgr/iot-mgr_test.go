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
 * distributed under the License is distributed on ance "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package iotmgr

import (
	"fmt"
	"testing"

	log "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-logger"
)

const tmName = "meep-iot"
const tmNamespace = "sandboxtest"

func TestNewIotMgr(t *testing.T) {
	fmt.Println("--- ", t.Name())
	log.MeepTextLogInit(t.Name())

	// Invalid Connector
	fmt.Println("Invalid IOT Asset Manager")
	tm, err := NewIotMgr("", tmNamespace)
	if err == nil || tm != nil {
		t.Fatalf("DB connection should have failed")
	}

	// Valid Connector
	fmt.Println("Create valid IOT Asset Manager")
	tm, err = NewIotMgr(tmName, tmNamespace)
	if err != nil || tm == nil {
		t.Fatalf("Failed to create IOT Asset Manager")
	}

	// Cleanup
	err = tm.DeleteIotMgr()
	if err != nil {
		t.Fatalf("Failed to cleanup IOT Asset Manager")
	}
}

func TestRegisterIotPlatformInfo(t *testing.T) {
	fmt.Println("--- ", t.Name())
	log.MeepTextLogInit(t.Name())

	// Valid Connector
	fmt.Println("Create valid IOT Asset Manager")
	tm, err := NewIotMgr(tmName, tmNamespace)
	if err != nil || tm == nil {
		t.Fatalf("Failed to create IOT Asset Manager")
	}

	// Set a valid platform
	var adresses = []Addresses{}
	adresses = append(adresses, Addresses{
		Host: "172.29.10.56",
		Port: 1883,
	})
	var endpoint = EndPointInfo{
		Addresses: adresses,
	}
	var userTransportInfo = []MbTransportInfo{}
	userTransportInfo = append(userTransportInfo, MbTransportInfo{
		Id:          "d5673793-c55c-4969-b5bc-2121f84b9f8d",
		Name:        "laboai-acme-ic-cse",
		Description: "MQTT",
		Protocol:    "MQTT",
		Version:     "3",
		Endpoint:    &endpoint,
	})
	var adresses_1 = []Addresses{}
	adresses_1 = append(adresses_1, Addresses{
		Host: "172.29.10.20",
		Port: 31110,
	})
	var customServicesTransportInfo = []TransportInfo{}
	var endPointInfo_1 = EndPointInfo{
		Addresses: adresses_1,
	}
	customServicesTransportInfo = append(customServicesTransportInfo, TransportInfo{
		Id:          "2ddb713c-2b41-4ded-a7ad-a5a047c5df13",
		Name:        "laboai-acme-ic-cse",
		Description: "ACME oneM2M CSE",
		Protocol:    "REST_HTTP",
		Version:     "5",
		Endpoint:    &endPointInfo_1,
	})
	var iotPlatformInfo = IotPlatformInfo{
		IotPlatformId:               "523f2df1-8927-429f-906c-56ba92d13762",
		UserTransportInfo:           userTransportInfo,
		CustomServicesTransportInfo: customServicesTransportInfo,
		Enabled:                     true,
	}
	fmt.Println("Create an IotPlatformInfo: ", iotPlatformInfo)
	err = tm.RegisterIotPlatformInfo(iotPlatformInfo)
	if err != nil {
		t.Fatalf("Failed to register new IoT platform")
	}

	// Cleanup
	err = tm.DeregisterIotPlatformInfo(iotPlatformInfo.IotPlatformId)
	if err != nil {
		t.Fatalf("Failed to register new IoT platform")
	}
	err = tm.DeleteIotMgr()
	if err != nil {
		t.Fatalf("Failed to cleanup IOT Asset Manager")
	}

	// t.Fatalf("DONE")
}

func TestCreateDeviceInfo(t *testing.T) {
	fmt.Println("--- ", t.Name())
	log.MeepTextLogInit(t.Name())

	// Valid Connector
	fmt.Println("Create valid IOT Asset Manager")
	tm, err := NewIotMgr(tmName, tmNamespace)
	if err != nil || tm == nil {
		t.Fatalf("Failed to create IOT Asset Manager")
	}

	// Set a valid platform
	fmt.Println("Create an IotPlatformInfo")
	iotPlatformInfo, err := registerIotPltf(tm)
	if err != nil {
		t.Fatalf("Failed to register new IoT platform")
	}

	// Create a new basic device
	fmt.Println("Create a new device")
	requestedIotPlatformId := iotPlatformInfo.IotPlatformId
	var device = DeviceInfo{
		RequestedIotPlatformId: requestedIotPlatformId,
		DeviceId:               requestedIotPlatformId,
		Enabled:                true,
	}
	device, err = tm.CreateDevice(device)
	if err != nil {
		t.Fatalf("Failed to create a new device")
	}

	// Cleanup
	_ = tm.DeleteDevice(device.DeviceId)
	err = tm.DeregisterIotPlatformInfo(iotPlatformInfo.IotPlatformId)
	if err != nil {
		t.Fatalf("Failed to register new IoT platform")
	}
	err = tm.DeleteIotMgr()
	if err != nil {
		t.Fatalf("Failed to cleanup IOT Asset Manager")
	}

	// t.Fatalf("DONE")
}

func TestCreateDeviceInfoFail(t *testing.T) {
	fmt.Println("--- ", t.Name())
	log.MeepTextLogInit(t.Name())

	// Valid Connector
	fmt.Println("Create valid IOT Asset Manager")
	tm, err := NewIotMgr(tmName, tmNamespace)
	if err != nil || tm == nil {
		t.Fatalf("Failed to create IOT Asset Manager")
	}

	// Set a valid platform
	fmt.Println("Create an IotPlatformInfo")
	iotPlatformInfo, err := registerIotPltf(tm)
	if err != nil {
		t.Fatalf("Failed to register new IoT platform")
	}

	// Create an invalid new basic device
	fmt.Println("Create an IotPlatformInfo: ", iotPlatformInfo)
	requestedIotPlatformId := iotPlatformInfo.IotPlatformId
	var device = DeviceInfo{
		RequestedIotPlatformId: "12345", // Invalid IotPlatformId
		DeviceId:               requestedIotPlatformId,
		Enabled:                true,
	}
	device, err = tm.CreateDevice(device)
	if err == nil {
		t.Fatalf("Creation of an invalid new device shall fail")
	}

	// Cleanup
	err = tm.DeregisterIotPlatformInfo(iotPlatformInfo.IotPlatformId)
	if err != nil {
		t.Fatalf("Failed to register new IoT platform")
	}
	err = tm.DeleteIotMgr()
	if err != nil {
		t.Fatalf("Failed to cleanup IOT Asset Manager")
	}

	// t.Fatalf("DONE")
}

func TestDeleteDeviceInfo(t *testing.T) {
	fmt.Println("--- ", t.Name())
	log.MeepTextLogInit(t.Name())

	// Valid Connector
	fmt.Println("Create valid IOT Asset Manager")
	tm, err := NewIotMgr(tmName, tmNamespace)
	if err != nil || tm == nil {
		t.Fatalf("Failed to create IOT Asset Manager")
	}

	// Set a valid platform
	fmt.Println("Create an IotPlatformInfo")
	iotPlatformInfo, err := registerIotPltf(tm)
	if err != nil {
		t.Fatalf("Failed to register new IoT platform")
	}

	// Create a new basic device
	fmt.Println("Create a new device")
	requestedIotPlatformId := iotPlatformInfo.IotPlatformId
	var device = DeviceInfo{
		RequestedIotPlatformId: requestedIotPlatformId,
		DeviceId:               requestedIotPlatformId,
		Enabled:                true,
	}
	device, err = tm.CreateDevice(device)
	if err != nil {
		t.Fatalf("Failed to create a new device")
	}

	err = tm.DeleteDevice(device.DeviceId)
	if err != nil {
		t.Fatalf("Failed to delete a device")
	}

	// Cleanup
	err = tm.DeregisterIotPlatformInfo(iotPlatformInfo.IotPlatformId)
	if err != nil {
		t.Fatalf("Failed to register new IoT platform")
	}
	err = tm.DeleteIotMgr()
	if err != nil {
		t.Fatalf("Failed to cleanup IOT Asset Manager")
	}

	// t.Fatalf("DONE")
}

func TestDeleteDeviceInfoFail(t *testing.T) {
	fmt.Println("--- ", t.Name())
	log.MeepTextLogInit(t.Name())

	// Valid Connector
	fmt.Println("Create valid IOT Asset Manager")
	tm, err := NewIotMgr(tmName, tmNamespace)
	if err != nil || tm == nil {
		t.Fatalf("Failed to create IOT Asset Manager")
	}

	// Set a valid platform
	fmt.Println("Create an IotPlatformInfo")
	iotPlatformInfo, err := registerIotPltf(tm)
	if err != nil {
		t.Fatalf("Failed to register new IoT platform")
	}

	err = tm.DeleteDevice("12345")
	if err == nil {
		t.Fatalf("Deletion of an invalide device shall fail")
	}

	// Cleanup
	err = tm.DeregisterIotPlatformInfo(iotPlatformInfo.IotPlatformId)
	if err != nil {
		t.Fatalf("Failed to register new IoT platform")
	}
	err = tm.DeleteIotMgr()
	if err != nil {
		t.Fatalf("Failed to cleanup IOT Asset Manager")
	}

	// t.Fatalf("DONE")
}

func TestGetDeviceInfo(t *testing.T) {
	fmt.Println("--- ", t.Name())
	log.MeepTextLogInit(t.Name())

	// Valid Connector
	fmt.Println("Create valid IOT Asset Manager")
	tm, err := NewIotMgr(tmName, tmNamespace)
	if err != nil || tm == nil {
		t.Fatalf("Failed to create IOT Asset Manager")
	}

	// Set a valid platform and create a device
	fmt.Println("Create an IotPlatformInfo")
	iotPlatformInfo, device, err := registerIotPltfAndCreateDevice(tm)
	if err != nil {
		t.Fatalf("Failed to register new IoT platform or to create a new device")
	}

	// Get a device
	fmt.Println("Get a device")
	device_rsp, err := tm.GetDevice(device.DeviceId)
	if err != nil {
		t.Fatalf("GetDevice failed")
	}

	// Check the response
	if !validate_device_info(device, device_rsp) {
		t.Fatalf("Devices mismatch")
	}

	fmt.Println("Get a devices")
	devices_rsp, err := tm.GetDevices()
	if err != nil {
		t.Fatalf("GetDevices failed")
	}
	if len(devices_rsp) != 1 {
		t.Fatalf("GetDevices: Unexpected response")
	}

	// Check the response
	if !validate_device_info(device, devices_rsp[0]) {
		t.Fatalf("Devices mismatch")
	}

	// Cleanup
	_ = tm.DeleteDevice(device.DeviceId)

	_ = tm.DeleteDevice(device.DeviceId)
	err = tm.DeregisterIotPlatformInfo(iotPlatformInfo.IotPlatformId)
	if err != nil {
		t.Fatalf("Failed to register new IoT platform")
	}
	err = tm.DeleteIotMgr()
	if err != nil {
		t.Fatalf("Failed to cleanup IOT Asset Manager")
	}

	// t.Fatalf("DONE")
}

func TestGetDeviceInfoFail(t *testing.T) {
	fmt.Println("--- ", t.Name())
	log.MeepTextLogInit(t.Name())

	// Valid Connector
	fmt.Println("Create valid IOT Asset Manager")
	tm, err := NewIotMgr(tmName, tmNamespace)
	if err != nil || tm == nil {
		t.Fatalf("Failed to create IOT Asset Manager")
	}

	// Get a device
	fmt.Println("Get a device")
	_, err = tm.GetDevice("12345")
	if err == nil {
		t.Fatalf("GetDevice shall failed")
	}

	// Set a valid platform and create a device
	fmt.Println("Create an IotPlatformInfo")
	iotPlatformInfo, device, err := registerIotPltfAndCreateDevice(tm)
	if err != nil {
		t.Fatalf("Failed to register new IoT platform or to create a new device")
	}

	// Get a device
	fmt.Println("Get a device")
	_, err = tm.GetDevice("12345")
	if err == nil {
		t.Fatalf("GetDevice shall failed")
	}

	// Get a device
	fmt.Println("Get a device")
	device_rsp, err := tm.GetDevice(device.DeviceId)
	if err != nil {
		t.Fatalf("Failed to create a new device")
	}

	// Check the response
	if !validate_device_info(device, device_rsp) {
		t.Fatalf("Devices mismatch")
	}

	// Cleanup
	_ = tm.DeleteDevice(device.DeviceId)

	_ = tm.DeleteDevice(device.DeviceId)
	err = tm.DeregisterIotPlatformInfo(iotPlatformInfo.IotPlatformId)
	if err != nil {
		t.Fatalf("Failed to register new IoT platform")
	}
	err = tm.DeleteIotMgr()
	if err != nil {
		t.Fatalf("Failed to cleanup IOT Asset Manager")
	}

	// t.Fatalf("DONE")
}

func registerIotPltf(tm *IotMgr) (iotPlatformInfo IotPlatformInfo, err error) {

	// Set a valid platform
	var adresses = []Addresses{}
	adresses = append(adresses, Addresses{
		Host: "172.29.10.56",
		Port: 1883,
	})
	var endpoint = EndPointInfo{
		Addresses: adresses,
	}
	var userTransportInfo = []MbTransportInfo{}
	userTransportInfo = append(userTransportInfo, MbTransportInfo{
		Id:          "d5673793-c55c-4969-b5bc-2121f84b9f8d",
		Name:        "laboai-acme-ic-cse",
		Description: "MQTT",
		Protocol:    "MQTT",
		Version:     "2",
		Endpoint:    &endpoint,
	})
	var adresses_1 = []Addresses{}
	adresses_1 = append(adresses_1, Addresses{
		Host: "172.29.10.20",
		Port: 31110,
	})
	var customServicesTransportInfo = []TransportInfo{}
	var endPointInfo_1 = EndPointInfo{
		Addresses: adresses_1,
	}
	customServicesTransportInfo = append(customServicesTransportInfo, TransportInfo{
		Id:          "2ddb713c-2b41-4ded-a7ad-a5a047c5df13",
		Name:        "laboai-acme-ic-cse",
		Description: "ACME oneM2M CSE",
		Protocol:    "REST_HTTP",
		Version:     "5",
		Endpoint:    &endPointInfo_1,
	})
	iotPlatformInfo = IotPlatformInfo{
		IotPlatformId:               "523f2df1-8927-429f-906c-56ba92d13762",
		UserTransportInfo:           userTransportInfo,
		CustomServicesTransportInfo: customServicesTransportInfo,
		Enabled:                     true,
	}
	err = tm.RegisterIotPlatformInfo(iotPlatformInfo)
	if err != nil {
		return iotPlatformInfo, err
	}

	return iotPlatformInfo, nil
}

func registerIotPltfAndCreateDevice(tm *IotMgr) (iotPlatformInfo IotPlatformInfo, device DeviceInfo, err error) {

	// Set a valid platform
	var adresses = []Addresses{}
	adresses = append(adresses, Addresses{
		Host: "172.29.10.56",
		Port: 1883,
	})
	var endpoint = EndPointInfo{
		Addresses: adresses,
	}
	var userTransportInfo = []MbTransportInfo{}
	userTransportInfo = append(userTransportInfo, MbTransportInfo{
		Id:          "d5673793-c55c-4969-b5bc-2121f84b9f8d",
		Name:        "laboai-acme-ic-cse",
		Description: "MQTT",
		Protocol:    "MQTT",
		Version:     "2",
		Endpoint:    &endpoint,
	})
	var adresses_1 = []Addresses{}
	adresses_1 = append(adresses_1, Addresses{
		Host: "172.29.10.20",
		Port: 31110,
	})
	var customServicesTransportInfo = []TransportInfo{}
	var endPointInfo_1 = EndPointInfo{
		Addresses: adresses_1,
	}
	customServicesTransportInfo = append(customServicesTransportInfo, TransportInfo{
		Id:          "2ddb713c-2b41-4ded-a7ad-a5a047c5df13",
		Name:        "laboai-acme-ic-cse",
		Description: "ACME oneM2M CSE",
		Protocol:    "REST_HTTP",
		Version:     "5",
		Endpoint:    &endPointInfo_1,
	})
	iotPlatformInfo = IotPlatformInfo{
		IotPlatformId:               "523f2df1-8927-429f-906c-56ba92d13762",
		UserTransportInfo:           userTransportInfo,
		CustomServicesTransportInfo: customServicesTransportInfo,
		Enabled:                     true,
	}
	err = tm.RegisterIotPlatformInfo(iotPlatformInfo)
	if err != nil {
		return iotPlatformInfo, device, err
	}

	// Create a new basic device
	fmt.Println("Create a new device")
	requestedIotPlatformId := iotPlatformInfo.IotPlatformId
	device = DeviceInfo{
		RequestedIotPlatformId: requestedIotPlatformId,
		DeviceId:               requestedIotPlatformId,
		Enabled:                true,
	}
	device, err = tm.CreateDevice(device)
	if err != nil {
		return iotPlatformInfo, device, err
	}

	return iotPlatformInfo, device, nil
}

func validate_device_info(expected_device DeviceInfo, received_deviceResp DeviceInfo) bool {
	if expected_device.DeviceId != received_deviceResp.DeviceId {
		fmt.Println("received_deviceResp.DeviceId != DeviceId")
		return false
	}
	if expected_device.Enabled != received_deviceResp.Enabled {
		fmt.Println("received_deviceResp.Enabled != Enabled")
		return false
	}
	if expected_device.DeviceAuthenticationInfo != received_deviceResp.DeviceAuthenticationInfo {
		fmt.Println("received_deviceResp.DeviceAuthenticationInfo != DeviceAuthenticationInfo")
		return false
	}
	if expected_device.Gpsi != received_deviceResp.Gpsi {
		fmt.Println("received_deviceResp.Gpsi != Gpsi")
		return false
	}
	if expected_device.Pei != received_deviceResp.Pei {
		fmt.Println("received_deviceResp.Pei != Pei")
		return false
	}
	if expected_device.Supi != received_deviceResp.Supi {
		fmt.Println("received_deviceResp.Supi != Supi")
		return false
	}
	if expected_device.Msisdn != received_deviceResp.Msisdn {
		fmt.Println("received_deviceResp.Msisdn != Msisdn")
		return false
	}
	if expected_device.Imei != received_deviceResp.Imei {
		fmt.Println("received_deviceResp.Imei != Imei")
		return false
	}
	if expected_device.Imsi != received_deviceResp.Imsi {
		fmt.Println("received_deviceResp.Imsi != Imsi")
		return false
	}
	if expected_device.Iccid != received_deviceResp.Iccid {
		fmt.Println("received_deviceResp.Iccid != Iccid")
		return false
	}
	if expected_device.RequestedIotPlatformId != received_deviceResp.RequestedIotPlatformId {
		fmt.Println("received_deviceResp.RequestedIotPlatformId != RequestedIotPlatformId")
		return false
	}
	if expected_device.RequestedUserTransportId != received_deviceResp.RequestedUserTransportId {
		fmt.Println("received_deviceResp.RequestedUserTransportId != RequestedUserTransportId")
		return false
	}
	if expected_device.ClientCertificate != received_deviceResp.ClientCertificate {
		fmt.Println("received_deviceResp.ClientCertificate != ClientCertificate")
		return false
	}
	if len(expected_device.DeviceMetadata) != len(received_deviceResp.DeviceMetadata) {
		fmt.Println("received_deviceResp.DeviceMetadata != DeviceMetadata")
		return false
	} else {
		for i, val := range expected_device.DeviceMetadata {
			if val.Key != received_deviceResp.DeviceMetadata[i].Key {
				fmt.Println("item #", i, ":received_deviceResp.DeviceMetadata.Key != DeviceMetadata.Key")
				return false
			}
			if val.Value != received_deviceResp.DeviceMetadata[i].Value {
				fmt.Println("item #", i, ":received_deviceResp.DeviceMetadata.Value != DeviceMetadata.Value")
				return false

			}
		} // End of 'for' statement
	}
	if len(expected_device.RequestedMecTrafficRule) != len(received_deviceResp.RequestedMecTrafficRule) {
		fmt.Println("received_deviceResp.RequestedMecTrafficRule != RequestedMecTrafficRule")
		return false
	} else {
		// for i, val := range expected_device.RequestedMecTrafficRule {
		// } // End of 'for' statement
		// TODO To be continued
		//RequestedMecTrafficRule  []TrafficRuleDescriptor
	}
	// TODO To be continued

	return true
}
