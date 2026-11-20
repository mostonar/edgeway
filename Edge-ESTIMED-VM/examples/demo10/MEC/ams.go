package mec

import (
	model "demo10/Model"
	utils "demo10/utils"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
)

// CreateAMSRegistration creates a MEC Application Mobility Service (AMS) registration
//
// This function creates an AMS registration for a given application instance ID.
// The registration includes device information if provided, which is used to
// associate the device with the application instance for mobility management.
//
// Reference: ETSI GS MEC 021 - Application Mobility Service API
//
// @param url The URL endpoint for AMS registration (e.g., /amsi/v1/app_mobility_services)
// @param app_instance_id The MEC Application Instance ID to register
// @param device_info Optional device information for the registration (can be nil)
// @return *model.RegistrationInfo The registration information returned by the MEC platform
// @return error Returns nil on success, or an error if the registration fails
func CreateAMSRegistration(url string, app_instance_id string, device_info *model.RegistrationInfoDeviceInformation) (*model.RegistrationInfo, error) {
	payload := &model.RegistrationInfo{}
	if device_info != nil {
		payload = &model.RegistrationInfo{
			ServiceConsumerId: &model.RegistrationInfoServiceConsumerId{
				AppInstanceId: app_instance_id,
			},
			DeviceInformation: []model.RegistrationInfoDeviceInformation{*device_info},
		}
	} else {
		payload = &model.RegistrationInfo{
			ServiceConsumerId: &model.RegistrationInfoServiceConsumerId{
				AppInstanceId: app_instance_id,
			},
		}
	}
	resp, err := utils.SendMECRequest("POST", url, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to create AMS registration: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("failed to create AMS registration: received status code %d", resp.StatusCode)
	}

	var registrationInfo model.RegistrationInfo
	if err := json.NewDecoder(resp.Body).Decode(&registrationInfo); err != nil {
		return nil, fmt.Errorf("failed to decode AMS registration response: %v", err)
	}

	return &registrationInfo, nil
}

// CreateAMSSubscription creates a MEC Application Mobility Service (AMS) subscription
//
// This function creates a subscription for mobility procedure notifications.
// The subscription allows the application to receive notifications when mobility
// events occur, such as when the application instance moves to a different MEC host.
//
// Reference: ETSI GS MEC 021 - Application Mobility Service API
//
// @param url The URL endpoint for AMS subscription (e.g., /amsi/v1/subscriptions)
// @param app_instance_id The MEC Application Instance ID to subscribe to
// @param callbackURL The callback URL where mobility notifications will be sent
// @param associateId Optional associate ID (e.g., UE IP address) for filtering notifications
// @return *model.InlineSubscription The subscription information returned by the MEC platform
// @return error Returns nil on success, or an error if the subscription creation fails
func CreateAMSSubscription(url string, app_instance_id string, callbackURL string, associateId *model.AssociateId) (*model.InlineSubscription, error) {
	payload := &model.InlineSubscription{}
	if associateId != nil {
		payload = &model.InlineSubscription{
			SubscriptionType: "MobilityProcedureSubscription",
			FilterCriteria: &model.MobilityProcedureSubscriptionFilterCriteria{
				AppInstanceId: app_instance_id,
				AssociateId:   []model.AssociateId{*associateId},
			},
			CallbackReference: callbackURL,
		}
	} else {
		payload = &model.InlineSubscription{
			SubscriptionType: "MobilityProcedureSubscription",
			FilterCriteria: &model.MobilityProcedureSubscriptionFilterCriteria{
				AppInstanceId: app_instance_id,
			},
			// TODO: Update callback reference URL as needed
			CallbackReference: callbackURL,
		}
	}

	resp, err := utils.SendMECRequest("POST", url, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to create AMS subscription: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("failed to create AMS subscription: received status code %d", resp.StatusCode)
	}

	var subscriptionInfo model.InlineSubscription
	if err := json.NewDecoder(resp.Body).Decode(&subscriptionInfo); err != nil {
		return nil, fmt.Errorf("failed to decode AMS subscription response: %v", err)
	}

	return &subscriptionInfo, nil
}

// AMSNotificationHandler handles incoming AMS mobility procedure notifications
//
// This HTTP handler processes POST requests containing AMS notifications.
// It extracts the target application instance ID from the notification payload,
// which indicates that the application should migrate to a new MEC host.
//
// Reference: ETSI GS MEC 021 - Application Mobility Service API
//
// @param w HTTP response writer
// @param r HTTP request containing the AMS notification
// @return string The target application instance ID extracted from the notification
// @return error Returns nil on success, or an error if the notification cannot be processed
func AMSNotificationHandler(w http.ResponseWriter, r *http.Request) (string, error) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return "", fmt.Errorf("invalid request method")
	}

	var notification map[string]interface{}
	err := json.NewDecoder(r.Body).Decode(&notification)
	if err != nil {
		http.Error(w, "Failed to decode request body", http.StatusBadRequest)
		return "", fmt.Errorf("failed to decode request body: %w", err)
	}
	log.Printf("Received AMS notification: %+v", notification)
	w.WriteHeader(http.StatusNoContent)
	// Extract target app instance ID from the notification
	var targetAppInstanceID string
	if targetAppInfo, ok := notification["targetAppInfo"].(map[string]interface{}); ok {
		if appInstanceID, ok := targetAppInfo["appInstanceId"].(string); ok {
			targetAppInstanceID = appInstanceID
			log.Printf("Target App Instance ID: %s", targetAppInstanceID)
		}
	}
	if targetAppInstanceID == "" {
		log.Println("Target App Instance ID not found in notification")
	}
	return targetAppInstanceID, nil
}

// RegistrationAndSubscriptionHandler creates AMS registrations and subscriptions for multiple app instances
//
// This function iterates through matching application instances and creates:
//   - AMS registration for each app instance (with device info for the first instance)
//   - AMS subscription for each app instance to receive mobility notifications
//
// It collects platform information (AMS service ID, subscription ID, node name) for each
// app instance and returns it in a structured format.
//
// Reference: ETSI GS MEC 021 - Application Mobility Service API
//
// @param matchingAppInstances Array of matching MEC Application Instance IDs
// @param platformURL Base URL of the MEC platform
// @param sandbox_name Name of the MEC sandbox
// @param callbackURL URL where AMS notifications will be sent
// @param currentAppInstanceId Pointer to store the current app instance ID (first instance)
// @param deviceInfo Device information for registration (used for first instance)
// @param associateId Associate ID for subscription filtering (used for first instance)
// @return []map[string]model.PlatformInfo Array of platform information maps keyed by app instance ID
// @return error Returns nil on success, or an error if registration/subscription fails
func RegistrationAndSubscriptionHandler(matchingAppInstances []model.AppInstanceID, platformURL string,
	sandbox_name string, callbackURL string, currentAppInstanceId *string,
	deviceInfo *model.RegistrationInfoDeviceInformation, associateId *model.AssociateId) ([]map[string]model.PlatformInfo, error) {
	var platformDetails []map[string]model.PlatformInfo

	for i, appInstance := range matchingAppInstances {
		log.Printf("Creating AMS registration for App Instance ID: %s, Name: %s", appInstance.ID, appInstance.Name)
		amsURL := fmt.Sprintf("%s/%s/mep1/amsi/v1/app_mobility_services", platformURL, sandbox_name)
		var resp *model.RegistrationInfo
		var err error
		if i == 0 {
			*currentAppInstanceId = appInstance.ID
			resp, err = CreateAMSRegistration(amsURL, appInstance.ID, deviceInfo)
			if err != nil {
				return nil, fmt.Errorf("failed to create AMS registration: %v", err)
			} else {
				log.Printf("AMS registration created successfully: %v", resp)
			}
		} else {
			resp, err = CreateAMSRegistration(amsURL, appInstance.ID, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to create AMS registration: %v", err)
			} else {
				log.Printf("AMS registration created successfully: %v", resp)
			}
		}
		amsSubURL := fmt.Sprintf("%s/%s/mep1/amsi/v1/subscriptions", platformURL, sandbox_name)
		sub_resp := &model.InlineSubscription{}
		if i == 0 {
			sub_resp, err = CreateAMSSubscription(amsSubURL, appInstance.ID, callbackURL, associateId)
			if err != nil {
				return nil, fmt.Errorf("failed to create AMS subscription: %v", err)
			} else {
				log.Printf("AMS subscription created successfully: %v", sub_resp)
			}
		} else {
			sub_resp, err = CreateAMSSubscription(amsSubURL, appInstance.ID, callbackURL, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to create AMS subscription: %v", err)
			} else {
				log.Printf("AMS subscription created successfully: %v", sub_resp)
			}
		}
		// Extract subscription ID from href using regex
		subID := ""
		if sub_resp.Links.Self.Href != "" {
			re := regexp.MustCompile(`subscriptions/(.*)`)
			matches := re.FindStringSubmatch(sub_resp.Links.Self.Href)
			if len(matches) > 1 {
				subID = matches[1]
			}
		}
		platformInfo := model.PlatformInfo{
			AmsServiceID:      resp.AppMobilityServiceId,
			AppInstanceID:     appInstance.ID,
			AmsSubscriptionID: subID,
			NodeName:          appInstance.NodeName,
		}
		platformDetails = append(platformDetails, map[string]model.PlatformInfo{
			appInstance.ID: platformInfo,
		})
	}

	return platformDetails, nil
}

// PutAMSRegistrationInfo updates AMS registration information for mobility handover
//
// This function updates the AMS registration for both the current and target application
// instances during a mobility handover procedure. It updates the context transfer state
// to reflect the mobility status.
//
// Reference: ETSI GS MEC 021 - Application Mobility Service API
//
// @param url The base URL endpoint for AMS registration updates
// @param targetappinstance The target application instance ID for the handover
// @param platformDetails Array of platform information maps containing AMS service IDs
// @param current_app_instance Pointer to the current application instance ID
// @param associateID Associate ID (e.g., UE IP address) for the registration
// @return error Returns nil on success, or an error if the update fails
func PutAMSRegistrationInfo(url string, targetappinstance string, platformDetails []map[string]model.PlatformInfo,
	current_app_instance *string, associateID *model.AssociateId) error {
	if len(platformDetails) == 0 {
		return fmt.Errorf("platformDetails is empty")
	}
	if targetappinstance == "" {
		log.Println("Initializing AMS registration info...")
	}

	// Find the PlatformInfo for the current app instance by iterating through the slice
	var currentPlatformInfo *model.PlatformInfo
	for _, detail := range platformDetails {
		if info, exists := detail[*current_app_instance]; exists {
			currentPlatformInfo = &info
			break // Found it, no need to continue
		}
	}
	payload := model.RegistrationInfo{
		AppMobilityServiceId: currentPlatformInfo.AmsServiceID,
		DeviceInformation: []model.RegistrationInfoDeviceInformation{
			{
				AssociateId:             associateID,
				AppMobilityServiceLevel: &[]model.AppMobilityServiceLevel{"APP_MOBILITY_WITHOUT_CONFIRMATION"}[0],
				ContextTransferState:    &[]model.ContextTransferState{"USER_CONTEXT_TRANSFER_COMPLETED"}[0],
			},
		},
		ServiceConsumerId: &model.RegistrationInfoServiceConsumerId{
			AppInstanceId: *current_app_instance,
		},
	}
	req_url := fmt.Sprintf("%s/%s", url, currentPlatformInfo.AmsServiceID)
	resp, err := utils.SendMECRequest("PUT", req_url, payload)
	if err != nil {
		return fmt.Errorf("failed to update AMS registration info: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to update AMS registration info: received status code %d", resp.StatusCode)
	}
	log.Println("AMS registration info updated successfully")

	// find the PlatformInfo for the target app instance
	var targetPlatformInfo *model.PlatformInfo
	for _, detail := range platformDetails {
		if info, exists := detail[targetappinstance]; exists {
			targetPlatformInfo = &info
			break // Found it, no need to continue
		}
	}
	payload = model.RegistrationInfo{
		AppMobilityServiceId: targetPlatformInfo.AmsServiceID,
		DeviceInformation: []model.RegistrationInfoDeviceInformation{
			{
				AssociateId:             associateID,
				AppMobilityServiceLevel: &[]model.AppMobilityServiceLevel{"APP_MOBILITY_WITHOUT_CONFIRMATION"}[0],
				ContextTransferState:    &[]model.ContextTransferState{"NOT_TRANSFERRED"}[0],
			},
		},
		ServiceConsumerId: &model.RegistrationInfoServiceConsumerId{
			AppInstanceId: targetappinstance,
		},
	}
	req_url = fmt.Sprintf("%s/%s", url, targetPlatformInfo.AmsServiceID)
	resp, err = utils.SendMECRequest("PUT", req_url, payload)
	if err != nil {
		return fmt.Errorf("failed to update AMS registration info for target app instance: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to update AMS registration info for target app instance: received status code %d", resp.StatusCode)
	}

	log.Println("AMS registration info for target app instance updated successfully")

	return nil
}

// PUTAMSSubscriptionInfo updates AMS subscription information for mobility handover
//
// This function updates the AMS subscription for both the current and target application
// instances during a mobility handover. It updates the mobility status filter criteria
// to reflect the current state of the handover procedure.
//
// Reference: ETSI GS MEC 021 - Application Mobility Service API
//
// @param url The base URL endpoint for AMS subscription updates
// @param targetappinstance The target application instance ID for the handover
// @param platformDetails Array of platform information maps containing subscription IDs
// @param current_app_instance Pointer to the current application instance ID
// @param call_back_url The callback URL for receiving mobility notifications
// @param associateid Associate ID (e.g., UE IP address) for subscription filtering
// @return error Returns nil on success, or an error if the update fails
func PUTAMSSubscriptionInfo(url string, targetappinstance string, platformDetails []map[string]model.PlatformInfo,
	current_app_instance *string, call_back_url string, associateid *model.AssociateId) error {
	if len(platformDetails) == 0 {
		return fmt.Errorf("platformDetails is empty")
	}
	if targetappinstance == "" {
		log.Println("Initializing AMS subscription info...")
	}

	// Find the PlatformInfo for the current app instance by iterating through the slice
	var currentPlatformInfo *model.PlatformInfo
	for _, detail := range platformDetails {
		if info, exists := detail[*current_app_instance]; exists {
			currentPlatformInfo = &info
			break // Found it, no need to continue
		}
	}

	payload := model.InlineSubscription{
		SubscriptionType: "MobilityProcedureSubscription",
		Links: &model.MobilityProcedureSubscriptionLinks{
			Self: &model.LinkType{
				Href: fmt.Sprintf("%s/%s", url, currentPlatformInfo.AmsSubscriptionID),
			},
		},
		CallbackReference: call_back_url,
		FilterCriteria: &model.MobilityProcedureSubscriptionFilterCriteria{
			AppInstanceId:  *current_app_instance,
			AssociateId:    []model.AssociateId{*associateid},
			MobilityStatus: []model.MobilityStatus{"INTERHOST_MOVEOUT_COMPLETED"},
		},
	}
	resp, err := utils.SendMECRequest("PUT", fmt.Sprintf("%s/%s", url, currentPlatformInfo.AmsSubscriptionID), payload)
	if err != nil {
		return fmt.Errorf("failed to update AMS subscription info: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to update AMS subscription info: received status code %d", resp.StatusCode)
	}
	log.Println("AMS subscription info updated successfully")

	// find the PlatformInfo for the target app instance
	var targetPlatformInfo *model.PlatformInfo
	for _, detail := range platformDetails {
		if info, exists := detail[targetappinstance]; exists {
			targetPlatformInfo = &info
			break // Found it, no need to continue
		}
	}

	payload = model.InlineSubscription{
		SubscriptionType: "MobilityProcedureSubscription",
		Links: &model.MobilityProcedureSubscriptionLinks{
			Self: &model.LinkType{
				Href: fmt.Sprintf("%s/%s", url, targetPlatformInfo.AmsSubscriptionID),
			},
		},
		CallbackReference: call_back_url,
		FilterCriteria: &model.MobilityProcedureSubscriptionFilterCriteria{
			AppInstanceId:  targetappinstance,
			AssociateId:    []model.AssociateId{*associateid},
			MobilityStatus: []model.MobilityStatus{"INTERHOST_MOVEOUT_TRIGGERED"},
		},
	}
	resp, err = utils.SendMECRequest("PUT", fmt.Sprintf("%s/%s", url, targetPlatformInfo.AmsSubscriptionID), payload)
	if err != nil {
		return fmt.Errorf("failed to update AMS subscription info for target app instance: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to update AMS subscription info for target app instance: received status code %d", resp.StatusCode)
	}

	log.Println("AMS subscription info for target app instance updated successfully")
	return nil
}
