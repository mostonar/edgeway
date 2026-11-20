package mec

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
)

func SensorRegisterPostHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("Received Sensor Register POST request:", r.URL.String())

	// Get the request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("Error reading request body: %v", err)
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Extract DeviceAuthenticationInfo (string), DeviceId (string), from the request body
	var deviceInfo DeviceInfo
	err = json.Unmarshal(body, &deviceInfo)
	if err != nil {
		log.Printf("Error unmarshalling request body: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	// Sanity check for required fields
	if deviceInfo.DeviceAuthenticationInfo == "" || deviceInfo.DeviceId == "" {
		log.Printf("Missing required fields in request body: %+v", deviceInfo)
		http.Error(w, "Missing required fields: DeviceAuthenticationInfo and DeviceId are required", http.StatusBadRequest)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if !deviceInfo.Enabled {
		log.Printf("Device is not enabled: %+v", deviceInfo)
		http.Error(w, "Device is not enabled", http.StatusBadRequest)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if len(deviceInfo.RequestedMecTrafficRule) == 0 {
		if deviceInfo.RequestedIotPlatformId == "" || deviceInfo.RequestedUserTransportId == "" {
			deviceInfo.RequestedIotPlatformId = config["IotPlatformId"].(string)
			deviceInfo.RequestedUserTransportId = "REST_HTTP"
		}
	}
	// Call the PostRegisteredDevice function to register the device
	resp, err := PostRegisteredDevice(&deviceInfo)
	if err != nil {
		log.Printf("Error registering device: %v", err)
		http.Error(w, "Failed to register device", http.StatusInternalServerError)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	// Read the response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Error reading response body: %v", err)
		http.Error(w, "Failed to read response body", http.StatusInternalServerError)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	log.Printf("Device registration response: %s", string(respBody))

	// Set the response headers
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	w.Write(respBody)
}
