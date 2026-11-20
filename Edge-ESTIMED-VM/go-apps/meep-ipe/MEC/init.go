package mec

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	onem2m "meep-ipe/oneM2M"
	utils "meep-ipe/utils"

	"github.com/google/uuid"
)

var config map[string]interface{}

// scopeOfLocality returns the scope the IPE service registers with, from
// MEEP_SCOPE_OF_LOCALITY like the other MEC services (e.g. MEC_HOST to be
// discoverable from its own MEP only). Defaults to MEC_SYSTEM when unset or
// not a valid LocalityType.
func scopeOfLocality() *LocalityType {
	l := MEC_SYSTEM_LocalityType
	switch v := LocalityType(strings.TrimSpace(os.Getenv("MEEP_SCOPE_OF_LOCALITY"))); v {
	case MEC_SYSTEM_LocalityType, MEC_HOST_LocalityType, NFVI_POP_LocalityType,
		ZONE_LocalityType, ZONE_GROUP_LocalityType, NFVI_NODE_LocalityType:
		l = v
	case "":
	default:
		log.Printf("Invalid MEEP_SCOPE_OF_LOCALITY %q, using %s", v, l)
	}
	log.Printf("MEEP_SCOPE_OF_LOCALITY: %s", l)
	return &l
}

func InitMEC(cfg map[string]interface{}) {
	config = cfg
	var ipe_Endpoint string
	// Initialize MEC platform settings or configurations here
	// mec_pltf_instanciated := true

	// Step # 1: (check if the config contains necessary keys and values)
	// app_id, ok := config["appid"].(string)
	// if !ok || app_id == "" {
	// 	mec_pltf_instanciated = false
	// }
	if !config["mec_pltf_instanciated"].(bool) {
		// Send POST request to MEC Sandbox controller to get the appid
		url := fmt.Sprintf("%s/%s/sandbox-ctrl/v1/applications", config["sandboxUrl"], config["sandboxName"])
		log.Println("url: ", url)
		payload := ApplicationInfo{
			Name:     config["appName"].(string),
			NodeName: config["mecplatform"].(string),
		}
		resp, err := utils.SendMECRequest("POST", url, payload)
		if err != nil {
			log.Println("Error occurred while sending request:", err)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode == 201 {
			log.Println("Successfully obtained appid from MEC Sandbox controller.")
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				log.Println("Error occurred while reading response body:", err)
				return
			}

			var responseData ApplicationInfo
			err = json.Unmarshal(body, &responseData)
			if err != nil {
				log.Println("Error occurred while unmarshalling response body:", err)
				return
			}

			config["appid"] = responseData.Id
		}
	}

	// Wait for the MEC platform to be ready before sending the confirm ready request
	time.Sleep(2 * time.Second)

	// Step # 2: (Send Confirm ready request to MEC Platform)
	url := fmt.Sprintf("%s/%s/%s/mec_app_support/v2/applications/%s/confirm_ready", config["sandboxUrl"],
		config["sandboxName"], config["mecplatform"], config["appid"])

	payload := AppReadyConfirmation{
		Indication: "READY",
	}
	resp, err := utils.SendMECRequest("POST", url, payload)
	if err != nil {
		log.Println("Error occurred while sending request:", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == 204 {
		log.Println("Successfully sent confirm ready request to MEC Platform.")
	} else {
		log.Println("Failed to send confirm ready request to MEC Platform. Status code:", resp.StatusCode)
		return
	}

	if config["mec_pltf_instanciated"].(bool) {
		ipe_Endpoint = fmt.Sprintf("https://%s/%s/%s/meep_ipe/v1",
			config["serverIp"],
			config["sandboxName"],
			config["mecplatform"],
		)
	} else {
		if config["https"].(bool) {
			if config["basePath"] != "" {
				ipe_Endpoint = fmt.Sprintf("https://%s:%s%s/meep_ipe/v1",
					config["serverIp"],
					config["ipePort"],
					config["basePath"],
				)
			} else {
				ipe_Endpoint = fmt.Sprintf("https://%s:%s/meep_ipe/v1",
					config["serverIp"],
					config["ipePort"],
				)
			}
		} else {
			if config["basePath"] != "" {
				ipe_Endpoint = fmt.Sprintf("http://%s:%s%s/meep_ipe/v1",
					config["serverIp"],
					config["ipePort"],
					config["basePath"],
				)
			} else {
				ipe_Endpoint = fmt.Sprintf("http://%s:%s/meep_ipe/v1",
					config["serverIp"],
					config["ipePort"],
				)
			}
		}
	}
	// Step # 3: (Register with MEC platform application registry if instantiated by MEC)
	if config["mec_pltf_instanciated"].(bool) {
		url = fmt.Sprintf("%s/%s/%s/mec_app_support/v2/registrations", config["sandboxUrl"],
			config["sandboxName"], config["mecplatform"])
		req := AppInfo{
			AppName:       config["appName"].(string),
			AppProvider:   "MEC IPE",
			AppInstanceId: config["appid"].(string),
			Endpoint: &OneOfAppInfoEndpoint{
				EndPointInfoUris: EndPointInfoUris{
					// FIXME: xFlow, Need to properly set the endpoint logic, for now, just set the endpoint to the MEC IPE service
					Uris: []string{
						ipe_Endpoint,
					},
				},
			},
		}
		resp, err = utils.SendMECRequest("POST", url, req)
		if err != nil {
			log.Println("Error occurred while sending request:", err)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode == 201 {
			log.Println("Successfully registered with MEC platform application registry.")
		} else {
			log.Println("Failed to register with MEC platform application registry. Status code:", resp.StatusCode)
			return
		}
	}

	// Step # 4: (Register with MEC platform as MEC IPE service)

	// Declare MEC IPE as a service to the MEC platform (MEC IPE service registration)
	url = fmt.Sprintf("%s/%s/%s/mec_service_mgmt/v1/applications/%s/services", config["sandboxUrl"],
		config["sandboxName"], config["mecplatform"], config["appid"])
	// var endpoint *OneOfTransportInfoEndpoint
	// if config["mec_pltf_instanciated"].(bool) {
	// 	endpoint = &OneOfTransportInfoEndpoint{
	// 		EndPointInfoUris: EndPointInfoUris{
	// 			Uris: []string{
	// 				ipe_Endpoint,
	// 			},
	// 		},
	// 	}
	// } else {
	// 	endpoint = &OneOfTransportInfoEndpoint{
	// 		EndPointInfoUris: EndPointInfoUris{
	// 			Uris: []string{
	// 				fmt.Sprintf("%s:%s/meep_ipe/v1",
	// 					config["ipeUrl"],
	// 					config["ipePort"],
	// 				),
	// 			},
	// 		},
	// 	}
	// }
	payload_svc := ServiceInfo{
		SerName: "MEC IPE Service",
		Version: "1.0",
		State:   &ACTIVE_ServiceState,
		SerCategory: &CategoryRef{
			Href:    "catalogueHref",
			Name:    "MEC IPE",
			Version: "v1.0",
			Id:      "meep_ipe",
		},
		Serializer: func() *SerializerType {
			s := JSON
			return &s
		}(),
		ScopeOfLocality: func() *LocalityType {
			l := MEC_HOST_LocalityType
			return &l
		}(),
		ConsumedLocalOnly: true,
		TransportInfo: &TransportInfo{
			Id: "transportId",
			Type_: func() *TransportType {
				t := REST_HTTP
				return &t
			}(),
			Name:        "MEC IPE Transport",
			Description: "Transport for MEC IPE service",
			Version:     "1.1",
			Protocol:    "HTTP",
			Endpoint: &OneOfTransportInfoEndpoint{
				EndPointInfoUris: EndPointInfoUris{
					Uris: []string{
						ipe_Endpoint,
					},
				},
			},
		},
	}
	log.Println("Svc URL : ", url)
	resp, err = utils.SendMECRequest("POST", url, payload_svc)
	if err != nil {
		log.Println("Error occurred while sending request:", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == 201 {
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Println("Error occurred while reading response body:", err)
			return
		}
		var responseData ServiceInfo
		err = json.Unmarshal(respBody, &responseData)
		if err != nil {
			log.Println("Error occurred while unmarshalling response body:", err)
			return
		}
		config["serInstanceId"] = responseData.SerInstanceId
		log.Println("Successfully registered MEC IPE service with MEC platform.")
	} else {
		log.Println("Failed to register MEC IPE service with MEC platform. Status code:", resp.StatusCode)
		return
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println("Error occurred while reading response body:", err)
		return
	}
	log.Println("Response Body: ", string(body))

	// Step # 5: (Subscription to availability and termination of MEC services)
	// Service Availability Notification Subscription
	url = fmt.Sprintf("%s/%s/%s/mec_service_mgmt/v1/applications/%s/subscriptions", config["sandboxUrl"],
		config["sandboxName"], config["mecplatform"], config["appid"])
	payload_sub := SerAvailabilityNotificationSubscription{
		SubscriptionType:  "SerAvailabilityNotificationSubscription",
		CallbackReference: fmt.Sprintf("%s/service_availability_notification", ipe_Endpoint),
		FilteringCriteria: &SerAvailabilityNotificationSubscriptionFilteringCriteria{
			IsLocal: true,
		},
	}
	resp, err = utils.SendMECRequest("POST", url, payload_sub)
	if err != nil {
		log.Println("Error occurred while sending request:", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == 201 {
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Println("Error occurred while reading response body:", err)
			return
		}
		var responseData SerAvailabilityNotificationSubscription
		err = json.Unmarshal(respBody, &responseData)
		if err != nil {
			log.Println("Error occurred while unmarshalling response body:", err)
			return
		}
		sub_id := responseData.Links.Self.Href
		// Remove everything before the last slash to get the subscription ID
		sub_id = sub_id[strings.LastIndex(sub_id, "/")+1:]
		config["SerAvailabilityNotificationSubscriptionId"] = sub_id
		log.Println("Successfully subscribed to availability and termination of MEC services.")
	} else {
		log.Println("Failed to subscribe to availability and termination of MEC services. Status code:", resp.StatusCode)
		return
	}
	body, err = io.ReadAll(resp.Body)
	if err != nil {
		log.Println("Error occurred while reading response body:", err)
		return
	}
	log.Println("Subscription Response Body: ", string(body))

	// Service Termination Notification Subscription
	url = fmt.Sprintf("%s/%s/%s/mec_app_support/v2/applications/%s/subscriptions", config["sandboxUrl"],
		config["sandboxName"], config["mecplatform"], config["appid"])
	callbackURL := fmt.Sprintf("%s/meep_ipe/v1/service_termination_notification",
		ipe_Endpoint,
	)
	if config["basePath"] != "" {
		callbackURL = fmt.Sprintf("%s/service_termination_notification",
			ipe_Endpoint,
		)
	}
	payload_sub_term := AppTerminationNotificationSubscription{
		SubscriptionType:  "AppTerminationNotificationSubscription",
		CallbackReference: callbackURL,
		AppInstanceId:     config["appid"].(string),
	}
	resp, err = utils.SendMECRequest("POST", url, payload_sub_term)
	if err != nil {
		log.Println("Error occurred while sending request:", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == 201 {
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Println("Error occurred while reading response body:", err)
			return
		}
		var responseData AppTerminationNotificationSubscription
		err = json.Unmarshal(respBody, &responseData)
		if err != nil {
			log.Println("Error occurred while unmarshalling response body:", err)
			return
		}
		sub_id := responseData.Links.Self.Href
		// Remove everything before the last slash to get the subscription ID
		sub_id = sub_id[strings.LastIndex(sub_id, "/")+1:]
		config["AppTerminationNotificationSubscriptionId"] = sub_id
		log.Println("Successfully subscribed to termination of MEC services.")
	} else {
		log.Println("Failed to subscribe to termination of MEC services. Status code:", resp.StatusCode)
		return
	}
	body, err = io.ReadAll(resp.Body)
	if err != nil {
		log.Println("Error occurred while reading response body:", err)
		return
	}
	log.Println("Termination Subscription Response Body: ", string(body))

	// ToDo: Logic to handle available services, link between MEC and OneM2m worlds.

	// Step # 6: Register with MEC IoT service (MEC 033)
	url = fmt.Sprintf("%s/%s/%s/iots/v1/registered_iot_platforms", config["sandboxUrl"],
		config["sandboxName"], config["mecplatform"])
	callbackURL = fmt.Sprintf("%s/mep_iot_notify",
		ipe_Endpoint,
	)
	config["IotPlatformId"] = uuid.NewString()
	iot_platform_info := &IotPlatformInfo{
		IotPlatformId: config["IotPlatformId"].(string),
		UserTransportInfo: []MbTransportInfo{
			{
				Id:          "transportId",
				Name:        "MEC IPE Transport",
				Description: "Transport for MEC IPE service",
				Type_: func() *TransportType {
					t := MB_TOPIC_BASED
					return &t
				}(),
				Protocol: "MQTT",
				Version:  "2",
				Endpoint: &EndPointInfo{
					Addresses: []Addresses{{}},
				},
				Security: &SecurityInfo{
					// Define security information as per your requirements
				},
				ImplSpecificInfo: &ImplSpecificInfo{},
			},
		},
		CustomServicesTransportInfo: []TransportInfo{
			{
				Id:          "customTransportId",
				Name:        "meep_ipe",
				Description: "Custom transport for MEC IPE service",
				Type_: func() *TransportType {
					t := REST_HTTP
					return &t
				}(),
				Protocol: "REST_HTTP",
				Version:  "5",
				Endpoint: &OneOfTransportInfoEndpoint{
					EndPointInfoUris: EndPointInfoUris{
						Uris: []string{
							callbackURL,
						},
					},
				},
				Security: &SecurityInfo{
					// Define security information as per your requirements
				},
				ImplSpecificInfo: "",
			},
		},
		Enabled: true,
	}
	resp, err = utils.SendMECRequest("POST", url, iot_platform_info)
	if err != nil {
		log.Println("Error occurred while sending request:", err)
		return
	}
	// defer resp.Body.Close()
	if resp.StatusCode == 201 {
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Println("Error occurred while reading response body:", err)
			return
		}
		var responseData IotPlatformInfo
		err = json.Unmarshal(respBody, &responseData)
		if err != nil {
			log.Println("Error occurred while unmarshalling response body:", err)
			return
		}
		config["iotPlatformId"] = responseData.IotPlatformId
		log.Println("Successfully registered with MEC IoT service. IoT Platform ID:", responseData.IotPlatformId)
		log.Println("IoT Registration Response Body: ", string(respBody))
		log.Println("Successfully registered with MEC IoT service.")
	} else {
		log.Println("Failed to register with MEC IoT service. Status code:", resp.StatusCode)
		return
	}

	// Step # 7: Create a Node on the oneM2M platform to represent the MEC IPE service
	// nodeName := "mec_ipe_node"
	nodeID := uuid.NewString()
	
	err = onem2m.CreateNode(onem2m.NodeName(), nodeID)
	if err != nil {
		log.Printf("Failed to create a Node on the oneM2M platform: %v", err)
		return
	}
	log.Println("Successfully created a Node on the oneM2M platform to represent the MEC IPE service.")

	// Step # 8: Create a MEC services group on the oneM2M platform to represent the MEC IPE service
	err = onem2m.CreateMecServicesGroup(onem2m.NodeName())
	if err != nil {
		log.Printf("Failed to create a MEC services group on the oneM2M platform: %v", err)
		return
	}
	log.Println("Successfully created a MEC services group on the oneM2M platform to represent the MEC IPE service.")

	// Step # 9: Create a flex container for each MEC service and publish it onto the CSE
	// This step is done after retrieving the list of available MEC services from the MEC platform
	// GET MEC services from MEC platform
	url = fmt.Sprintf("%s/%s/%s/mec_service_mgmt/v1/services", config["sandboxUrl"],
		config["sandboxName"], config["mecplatform"])

	resp, err = utils.SendMECRequest("GET", url, nil)
	if err != nil {
		log.Println("Error occurred while sending request:", err)
		return
	}
	defer resp.Body.Close()
	var responseData []ServiceInfo
	if resp.StatusCode == 200 {
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Println("Error occurred while reading response body:", err)
			return
		}
		err = json.Unmarshal(respBody, &responseData)
		if err != nil {
			log.Println("Error occurred while unmarshalling response body:", err)
			return
		}
		// config["services"] = responseData
		log.Println("Successfully retrieved MEC services from MEC platform.")
	} else {
		log.Println("Failed to retrieve MEC services from MEC platform. Status code:", resp.StatusCode)
		return
	}

	// Publish services onto CSE as a flex container for each service
	// -> Create a flex container for each service and publish it onto the CSE
	url = fmt.Sprintf("%s:%s/~/%s/%s", config["CSE-URL"], config["CSE-PORT"], config["CSE-ID"], config["CSE-NAME"])
	for _, service := range responseData {
		// Containers live under this IPE's node, so the service name alone is unique
		containerName := service.SerName
		// Sent Get request to service endpoint
		fullUrl, err := fetchAndAppendHref(fmt.Sprintf("%suser-api/", service.TransportInfo.Endpoint.Uris[0]))
		if err != nil {
			log.Printf("Failed to fetch and append href for service %s: %v", service.SerName, err)
			continue
		}

		log.Printf("Full URL for service %s: %s", service.SerName, fullUrl)
		err = onem2m.CreateContainer(containerName, fullUrl)
		if err != nil {
			log.Printf("Failed to create a flex container for service %s: %v", service.SerName, err)
			continue
		}
		log.Printf("Successfully created flex container for service %s", service.SerName)
	}

	// Create a subscription to CSE for new node creation and deletion events
	callbackURL = fmt.Sprintf("%s/oneM2M_node_creation", ipe_Endpoint)
	// For testing purposes, we can use a public ngrok URL as the callback URL. In production, this should be replaced with the actual MEC IPE endpoint.
	// callbackURL = "https://stowaway-booth-gradually.ngrok-free.dev/meep_ipe/v1/oneM2M_node_creation"
	err = onem2m.CreateNodeSubscription(callbackURL)
	if err != nil {
		log.Printf("Failed to create a subscription for new node creation and deletion events: %v", err)
		return
	}
	log.Println("Successfully created a subscription for new node creation and deletion events.")

}
