package mec

import (
	"fmt"
	// "io"
	"log"
	"net/http"

	utils "meep-ipe/utils"
	// onem2m "meep-ipe/oneM2M"
)

// PostRegisteredDevice sends a minimal POST /registered_devices request,
// populating only the required DeviceInfo fields:
//   - deviceAuthenticationInfo
//   - deviceId
//   - enabled

func PostRegisteredDevice(deviceInfo *DeviceInfo) (*http.Response, error) {
	// body := RegisteredDevicesBody{DeviceInfo: deviceInfo}

	url := fmt.Sprintf("%s/%s/%s/iots/v1/registered_devices", config["sandboxUrl"].(string), config["sandboxName"].(string),
		config["mecplatform"].(string))

	log.Println("Register Device URL : ", url)

	res, err := utils.SendMECRequest("POST", url, deviceInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	return res, nil
}

// func PostRegisterDeviceOnIoTPlatform(w http.ResponseWriter, r *http.Request) (*http.Response, error) {
// 	log.Println("PostRegisterDeviceOnIoTPlatform called")

// 	body, err := io.ReadAll(r.Body)
// 	if err != nil {
// 		log.Printf("Error reading request body: %v", err)
// 		http.Error(w, "Failed to read request body", http.StatusBadRequest)
// 		return nil, err
// 	}
// 	defer r.Body.Close()

// 	resp, err := onem2m.CreateAE(body)
// 	if err != nil {
// 		log.Printf("Error creating AE on IoT Platform: %v", err)
// 		http.Error(w, "Failed to create AE on IoT Platform", http.StatusInternalServerError)
// 		return nil, err
// 	}

// 	log.Printf("Response from IoT Platform: status %d, body: %s", resp.StatusCode, string(resp.Body))
// 	return resp, nil

// }