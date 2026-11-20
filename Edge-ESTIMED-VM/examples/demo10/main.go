package main

import (
	mec "demo10/MEC"
	model "demo10/Model"
	onem2m "demo10/oneM2M"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// MN-CSE platform details
var platformURL = ""                             // IoT platform URL obtained from MEC platform
var sandboxURL = "https://mec-platform.etsi.org" // MEC platform base URL

var cse_id = ""             // CSE Structured ID
var cse_name = "mep-cse-mn" // CSE Name (for some API calls)
var AE_ID = "CSmartCar"     // Application Entity ID
var App_Name = "SmartCar"   // Application Resource Name
var App_Id = "N-Smart_Car_Application"
var appVersion = "1.0.0"
var containerName = "SmartCarContainer"
var remote_cse_id = "/laboai-id-in"

// MEC platform details
var sandbox_name = "sbxykqjr17" // Namespace allocated to the user upon login
var DeviceId = "10.100.0.1"     // This is the IP of UE moving on map in Sandbox
var ams_pltf = "mep1"

// Endpoints for recieving notifications from MEC platform
var server_port = "9876"
var ams_notification_endpoint = "/ams/notify"
var server_ip = "0.0.0.0"
var ams_callback_address = "172.29.10.52"

var notificationChan = make(chan string, 10)            // Channel for receiving AMS notifications
var currentAppInstanceId = ""                           // Current MEC Application Instance ID
var platformDetails = []map[string]model.PlatformInfo{} // Platform information for each app instance
var associateId = &model.AssociateId{
	Type_: &[]model.AssociateIdType{"UE_IPv4_ADDRESS"}[0],
	Value: DeviceId,
}
var deviceInfo = &model.RegistrationInfoDeviceInformation{
	AssociateId:             associateId,
	AppMobilityServiceLevel: &[]model.AppMobilityServiceLevel{"APP_MOBILITY_WITHOUT_CONFIRMATION"}[0],
	ContextTransferState:    &[]model.ContextTransferState{"NOT_TRANSFERRED"}[0],
}

// initApp initializes the application and connects to the oneM2M platform
//
// This function performs the following operations:
//   - Starts an HTTP server to listen for AMS notifications
//   - Retrieves MEC Application Instance IDs from the MEC platform
//   - Matches target app instances based on CSE name
//   - Creates AMS registrations and subscriptions for each matching app instance
//   - Fetches IoT platform information from MEC
//   - Initializes the oneM2M application on the IoT platform
//
// Returns an error if any initialization step fails.
//
// @return error Returns nil on success, or an error describing the failure
func initApp() error {
	// Start HTTP server to listen for sensor connections
	log.Println("Starting HTTP server on port 9876 to listen for sensor connections...")
	server := &http.Server{Addr: fmt.Sprintf("%s:%s", server_ip, server_port)}
	// Set up HTTP routes
	http.HandleFunc(ams_notification_endpoint, func(w http.ResponseWriter, r *http.Request) {
		id, err := mec.AMSNotificationHandler(w, r)
		if err != nil {
			log.Printf("Error handling AMS notification: %v", err)
		} else if id != "" {
			notificationChan <- id
		}
	})

	// Start server in a goroutine so it doesn't block
	go func() {
		log.Println("Server goroutine starting...")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Server error: %v", err)
			log.Printf("ERROR: Could not start server: %v", err)
		}
	}()

	// Give the server a moment to start and check if it's running
	time.Sleep(1 * time.Second)

	// log.Println("✓ HTTP server started and listening on 0.0.0.0:9876")

	log.Printf("Initializing %s (version %s)", App_Id, appVersion)
	log.Println("Obtaining MEC App Instance ID of mn-cse")

	// Fetch MEC App Instance ID to find the correct App Instance for mn-cse
	// Construct MEC App Instance URL
	url := fmt.Sprintf("%s/%s/sandbox-ctrl/v1/applications", sandboxURL, sandbox_name)

	// Get all MEC App Instance ID
	allAppInstances, err := mec.GetAppInstances(url)
	if err != nil {
		return fmt.Errorf("failed to get MEC App Instance IDs: %v", err)
	}
	log.Printf("Total MEC App Instances retrieved: %d", len(allAppInstances))
	log.Printf("MEC App Instances: %+v", allAppInstances)

	// Array to store matching app instances
	matchingAppInstances, err := mec.MatchForTargetAppInstance(allAppInstances, cse_name)
	if err != nil {
		return fmt.Errorf("failed to match MEC App Instances: %v", err)
	}

	// Create AMS registration and subscription for each matching App Instance
	callbackURL := fmt.Sprintf("http://%s:%s%s", ams_callback_address /*server_ip*/, server_port, ams_notification_endpoint) // FSCOM FIXME: Use server_ip for local testing due to dockerized golanf execution environment
	platformDetails, err = mec.RegistrationAndSubscriptionHandler(matchingAppInstances, sandboxURL,
		sandbox_name, callbackURL, &currentAppInstanceId, deviceInfo, associateId)
	if err != nil {
		return fmt.Errorf("failed to create AMS registration and subscription: %v", err)
	}

	// Request IoT API for IoT platform urls
	log.Println("Fetching IoT platform information from MEC...")

	// Current App Instance ID must be set from previous step
	var current_pltf_details model.PlatformInfo
	for _, pltf := range platformDetails {
		if platformInfo, exists := pltf[currentAppInstanceId]; exists {
			current_pltf_details = platformInfo
			break
		}
	}
	log.Printf("Current platform details: %v", current_pltf_details)

	iot_url := fmt.Sprintf("%s/%s/%s/iots/v1/registered_iot_platforms", sandboxURL, sandbox_name, current_pltf_details.NodeName)
	iot_platforms, err := mec.GETIotPlatformInfo(iot_url)
	if err != nil {
		return fmt.Errorf("failed to get IoT platform info: %v", err)
	}

	platformURL = iot_platforms[0]["customServicesTransportInfo"].([]interface{})[0].(map[string]interface{})["endpoint"].(map[string]interface{})["uris"].([]interface{})[0].(string)
	log.Printf("IoT platform URL: %s", platformURL)
	cse_id = iot_platforms[0]["customServicesTransportInfo"].([]interface{})[0].(map[string]interface{})["id"].(string)
	log.Printf("CSE ID: %s", cse_id)
	cse_name = iot_platforms[0]["customServicesTransportInfo"].([]interface{})[0].(map[string]interface{})["name"].(string)
	log.Printf("CSE Name: %s", cse_name)

	// Initialize oneM2M application
	err = onem2m.InitOneM2MApp(platformURL, cse_name, cse_id, App_Name, App_Id, AE_ID, containerName, remote_cse_id)
	if err != nil {
		return fmt.Errorf("failed to initialize oneM2M application: %v", err)
	}
	// Create subscription for notification server
	// err = onem2m.CreateSubscription(platformURL, cse_name, App_Name, containerName)
	// if err != nil {
	// 	return fmt.Errorf("failed to create oneM2M subscription: %v", err)
	// }
	return nil
}

// main is the entry point of the application
//
// This function:
//   - Sets up graceful shutdown handling for SIGINT and SIGTERM signals
//   - Initializes the application by calling initApp()
//   - Starts a goroutine to handle AMS mobility notifications
//   - Continuously sends telemetry data (temperature, speed, fuel) to the oneM2M platform
//
// The application runs until terminated by a system signal, at which point it:
//   - Retrieves the latest data from the oneM2M platform
//   - Deregisters the Application Entity from the platform
//   - Exits gracefully
func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	// Handle graceful shutdown on system kill signals
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		latestData, err := onem2m.RetrieveLatestData(platformURL, cse_name, App_Name, AE_ID, containerName)
		if err != nil {
			log.Printf("Failed to retrieve latest data: %v", err)
		} else {
			log.Printf("Latest data: %s", latestData)
		}
		if err := onem2m.DeregisterAE(platformURL, cse_name, cse_id, App_Name, AE_ID); err != nil {
			log.Printf("Failed to deregister AE: %v", err)
		}
		log.Println("\nReceived shutdown signal, exiting...")
		os.Exit(0)
	}()

	// Initialize the application
	if err := initApp(); err != nil {
		log.Fatalf("Failed to initialize application: %v", err)
		os.Exit(1)
	}

	// Start goroutine to handle received target app instance IDs
	go func() {
		for targetAppInstanceID := range notificationChan {
			// Get platform details for the received ID
			var targetPltfDetails model.PlatformInfo
			for _, pltf := range platformDetails {
				if platformInfo, exists := pltf[targetAppInstanceID]; exists {
					targetPltfDetails = platformInfo
					break
				}
			}
			log.Printf("Target platform details: %v", targetPltfDetails)

			// Connect to new IoT platform
			iot_url := fmt.Sprintf("%s/%s/%s/iots/v1/registered_iot_platforms", sandboxURL, sandbox_name, targetPltfDetails.NodeName)
			iot_platforms, err := mec.GETIotPlatformInfo(iot_url)
			if err != nil {
				log.Printf("Failed to get IoT platform info for target app instance %s: %v", targetAppInstanceID, err)
				continue
			}
			// Temporarily store old platform URL
			old_platformURL := platformURL
			old_cse_id := cse_id
			old_cse_name := cse_name

			platformURL = iot_platforms[0]["customServicesTransportInfo"].([]interface{})[0].(map[string]interface{})["endpoint"].(map[string]interface{})["uris"].([]interface{})[0].(string)
			log.Printf("New IoT platform URL after mobility: %s", platformURL)

			cse_name = iot_platforms[0]["customServicesTransportInfo"].([]interface{})[0].(map[string]interface{})["name"].(string)
			cse_id = iot_platforms[0]["customServicesTransportInfo"].([]interface{})[0].(map[string]interface{})["id"].(string)

			// Re-initialize oneM2M application on the new platform
			err = onem2m.InitOneM2MApp(platformURL, cse_name, cse_id, App_Name, App_Id, AE_ID, containerName, remote_cse_id)
			if err != nil {
				log.Printf("Failed to re-initialize oneM2M application after mobility: %v", err)
				continue
			} else {
				log.Printf("Successfully re-initialized oneM2M application on new platform after mobility")
			}

			// Disconnect from previous platform
			log.Printf("Disconnecting from previous platform: %s", old_platformURL)
			if err := onem2m.DeregisterAE(old_platformURL, old_cse_name, old_cse_id, App_Name, AE_ID); err != nil {
				log.Printf("Failed to deregister AE from old platform: %v", err)
			} else {
				log.Printf("Successfully deregistered AE from old platform")
			}

			// Update AMS registration to reflect new platform connection
			log.Printf("Updating AMS registration to reflect new platform connection...")

			// PUT request to AMS for currentAppInstanceId to switch
			ams_url := fmt.Sprintf("%s/%s/%s/amsi/v1/app_mobility_services", sandboxURL, sandbox_name, ams_pltf)
			err = mec.PutAMSRegistrationInfo(ams_url, targetAppInstanceID, platformDetails, &currentAppInstanceId, associateId)
			if err != nil {
				log.Printf("Failed to update AMS registration info for target app instance %s: %v", targetAppInstanceID, err)
				continue
			} else {
				log.Printf("Successfully updated AMS registration info for target app instance %s", targetAppInstanceID)
			}

			// Update AMS subscription to reflect new platform connection
			log.Printf("Updating AMS subscription to reflect new platform connection...")
			ams_url = fmt.Sprintf("%s/%s/%s/amsi/v1/subscriptions", sandboxURL, sandbox_name, ams_pltf)
			callbackURL := fmt.Sprintf("http://%s:%s%s", ams_callback_address /*server_ip*/, server_port, ams_notification_endpoint) // FSCOM FIXME: Use server_ip for local testing due to dockerized golanf execution environment
			err = mec.PUTAMSSubscriptionInfo(ams_url, targetAppInstanceID, platformDetails, &currentAppInstanceId, callbackURL, associateId)
			if err != nil {
				log.Printf("Failed to update AMS subscription info for target app instance %s: %v", targetAppInstanceID, err)
				continue
			} else {
				log.Printf("Successfully updated AMS subscription info for target app instance %s", targetAppInstanceID)
			}
			currentAppInstanceId = targetAppInstanceID
		}
	}()

	// Example: Send telemetry data
	speed := []int{60, 62, 58, 65, 68}
	temperature := []float64{25.5, 26.0, 24.8, 25.2, 25.9}
	fuel := []int{75, 74, 73, 72, 71}
	// For only terminates on system kill signal
	for {
		telemetryData := fmt.Sprintf(`{"temperature": %f, "speed": %d, "fuel": %d}`,
			temperature[rand.Intn(len(temperature))],
			speed[rand.Intn(len(speed))],
			fuel[rand.Intn(len(fuel))])
		if err := onem2m.CreateContentInstance(platformURL, cse_name, App_Name, AE_ID, containerName, remote_cse_id, telemetryData); err != nil {
			log.Printf("Failed to send telemetry: %v", err)
		}
		time.Sleep(5 * time.Second)
	}
}
