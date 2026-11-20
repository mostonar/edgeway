package mec

import (
	"log"
	"fmt"

	utils "meep-ipe/utils"
)

func TerminateServiceAvailabilityNotificationHandler() {
	log.Println("Deregistering MEC Service Availability Notification Handler")
	
	url := fmt.Sprintf("%s/%s/%s/mec_service_mgmt/v1/applications/%s/subscriptions/%s", config["sandboxUrl"], 
			config["sandboxName"], config["mecplatform"], config["appid"], config["SerAvailabilityNotificationSubscriptionId"])
	
	log.Printf("Deregistering MEC Service Availability Notification Handler with URL: %s", url)

	res, err := utils.SendMECRequest("DELETE", url, nil)
	if err != nil {
		log.Println("Error occurred while sending request:", err)
		return
	}
	defer res.Body.Close()
	if res.StatusCode != 204 {
		log.Printf("Failed to deregister MEC Service Availability Notification Handler. Status code: %d", res.StatusCode)
		return
	}
	log.Println("MEC Service Availability Notification Handler deregistered successfully")
}

func TerminateServiceTerminationNotificationHandler() {
	log.Println("Deregistering MEC Service Termination Notification Handler")
	
	url := fmt.Sprintf("%s/%s/%s/mec_app_support/v2/applications/%s/subscriptions/%s", config["sandboxUrl"], 
			config["sandboxName"], config["mecplatform"], config["appid"], config["AppTerminationNotificationSubscriptionId"])
	
	log.Printf("Deregistering MEC Service Termination Notification Handler with URL: %s", url)

	res, err := utils.SendMECRequest("DELETE", url, nil)
	if err != nil {
		log.Println("Error occurred while sending request:", err)
		return
	}
	defer res.Body.Close()
	if res.StatusCode != 204 {
		log.Printf("Failed to deregister MEC Service Termination Notification Handler. Status code: %d", res.StatusCode)
		return
	}

	log.Println("MEC Service Termination Notification Handler deregistered successfully")
}

func DeleteIotPltf() {
	log.Println("Deleting IoT Platform from MEC")
	
	url := fmt.Sprintf("%s/%s/%s/iots/v1/registered_iot_platforms/%s", config["sandboxUrl"], 
			config["sandboxName"], config["mecplatform"], config["iotPlatformId"])
	
	log.Printf("Deleting IoT Platform from MEC with URL: %s", url)

	res, err := utils.SendMECRequest("DELETE", url, nil)
	if err != nil {
		log.Println("Error occurred while sending request:", err)
		return
	}

	defer res.Body.Close()
	if res.StatusCode != 204 {
		log.Printf("Failed to delete IoT Platform from MEC. Status code: %d", res.StatusCode)
		return
	}
	log.Println("IoT Platform deleted successfully")
}

func DeregisterMecService()	{
	log.Println("Deregistering MEC Service")
	
	url := fmt.Sprintf("%s/%s/%s/mec_service_mgmt/v1/applications/%s/services/%s", config["sandboxUrl"], 
			config["sandboxName"], config["mecplatform"], config["appid"], config["serInstanceId"])
	
	log.Printf("Deregistering MEC Service with URL: %s", url)

	res, err := utils.SendMECRequest("DELETE", url, nil)
	if err != nil {
		log.Println("Error occurred while sending request:", err)
		return
	}
	defer res.Body.Close()
	if res.StatusCode != 204 {
		log.Printf("Failed to deregister MEC Service. Status code: %d", res.StatusCode)
		return
	}
	log.Println("MEC Service deregistered successfully")
}


func DeregisterApplicationInstance(){
	if config["mec_pltf_instanciated"].(bool) {
		log.Println("Deregistering MEC Application Instance")
		
		url := fmt.Sprintf("%s/%s/%s/mec_app_support/v2/registrations/%s", config["sandboxUrl"], 
				config["sandboxName"], config["mecplatform"], config["appid"])
		
		log.Printf("Deregistering MEC Application Instance with URL: %s", url)
		
		res, err := utils.SendMECRequest("DELETE", url, nil)
		if err != nil {
			log.Println("Error occurred while sending request:", err)
			return
		}
		defer res.Body.Close()
		if res.StatusCode != 204 {
			log.Printf("Failed to deregister MEC Application Instance. Status code: %d", res.StatusCode)
			return
		}
		log.Println("MEC Application Instance deregistered successfully")
	}else {
		log.Println("MEC Application Instance was not instantiated by MEC, skipping deregistration of MEC application instance")
	}
}
func ConfirmTermination() {
	if !config["mec_pltf_instanciated"].(bool) {
		log.Println("MEC Application Instance was not instantiated by MEC, skipping confirmation of termination")
		return
	}
	log.Println("Confirming Termination to MEC")

	url := fmt.Sprintf("%s/%s/%s/mec_app_support/v2/applications/%s/confirm_termination", config["sandboxUrl"], 
			config["sandboxName"], config["mecplatform"], config["appid"])
	
	log.Printf("Confirming termination with URL: %s", url)

	payload := AppTerminationConfirmation{
	OperationAction: []OperationActionType{TERMINATING_OperationActionType},
	}

	res, err := utils.SendMECRequest("POST", url, payload)
	if err != nil {
		log.Println("Error occurred while sending request:", err)
		return
	}
	defer res.Body.Close()
	if res.StatusCode != 204 {
		log.Printf("Failed to confirm termination. Status code: %d", res.StatusCode)
		return
	}
	log.Println("Termination confirmed successfully")
}