/*
 * Copyright (c) 2024  The AdvantEDGE Authors
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

package server

import (
	"encoding/json"

	log "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-logger"
)

func convertServiceInfoToJson(obj *ServiceInfo) string {
	jsonInfo, err := json.Marshal(*obj)
	if err != nil {
		log.Error(err.Error())
		return ""
	}
	return string(jsonInfo)
}

func convertServiceInfoToJson_1(obj *ServiceApiDescription) string {
	jsonInfo, err := json.Marshal(*obj)
	if err != nil {
		log.Error(err.Error())
		return ""
	}
	return string(jsonInfo)
}

// FSCOM Unsused
// func convertServiceInfoToJson_2(obj *ServiceApiDescriptionPatch) string {
// 	jsonInfo, err := json.Marshal(*obj)
// 	if err != nil {
// 		log.Error(err.Error())
// 		return ""
// 	}
// 	return string(jsonInfo)
// }

func convertJsonToServiceInfo(jsonInfo string) *ServiceInfo {
	var obj ServiceInfo
	err := json.Unmarshal([]byte(jsonInfo), &obj)
	if err != nil {
		log.Error(err.Error())
		return nil
	}
	return &obj
}

// FSCOM Unsused
// func convertSerAvailabilityNotifSubToJson(obj *SerAvailabilityNotificationSubscription) string {
// 	jsonInfo, err := json.Marshal(*obj)
// 	if err != nil {
// 		log.Error(err.Error())
// 		return ""
// 	}
// 	return string(jsonInfo)
// }

func convertSerAvailabilityNotifSubToJson_1(obj *EventSubscription) string {
	jsonInfo, err := json.Marshal(*obj)
	if err != nil {
		log.Error(err.Error())
		return ""
	}
	return string(jsonInfo)
}

// FSCOM Unsused
// func convertJsonToSerAvailabilityNotifSub(jsonData string) *SerAvailabilityNotificationSubscription {
// 	var obj SerAvailabilityNotificationSubscription
// 	err := json.Unmarshal([]byte(jsonData), &obj)
// 	if err != nil {
// 		log.Error(err.Error())
// 		return nil
// 	}
// 	return &obj
// }

// Convert []string back to []CapifEventFilter
func convertStringsToEventFilters(stringFilters []string) []CapifEventFilter {
	var eventFilters []CapifEventFilter
	for _, filter := range stringFilters {
		eventFilters = append(eventFilters, CapifEventFilter{
			ApiIds: []string{filter}, // Assuming each string corresponds to an ApiId in the CapifEventFilter
		})
	}
	return eventFilters
}

// Convert []string back to []CapifEvent
func convertStringsToEvents(stringEvents []string) []CapifEvent {
	var events []CapifEvent
	for _, event := range stringEvents {
		events = append(events, CapifEvent(event)) // Assuming the string directly maps to CapifEvent
	}
	return events
}

func convertEventFiltersToStrings(filters []CapifEventFilter) []string {
	var stringFilters []string
	for _, filter := range filters {
		stringFilters = append(stringFilters, filter.ApiIds...)
	}
	return stringFilters
}

func convertEventToStrings(events []CapifEvent) []string {
	var stringEvents []string
	if len(events) > 0 {
		// Directly convert CapifEvent to string and append
		stringEvents = append(stringEvents, string(events[0]))
	}
	return stringEvents
}

func convertJsonToSerAvailabilityNotifSub_1(jsonData string) *EventSubscription {
	var obj EventSubscription
	err := json.Unmarshal([]byte(jsonData), &obj)
	if err != nil {
		log.Error(err.Error())
		return nil
	}
	return &obj
}

// FSCOM Unsused
// func convertServiceAvailabilityNotifToJson(obj *ServiceAvailabilityNotification) string {
// 	jsonInfo, err := json.Marshal(*obj)
// 	if err != nil {
// 		log.Error(err.Error())
// 		return ""
// 	}
// 	return string(jsonInfo)
// }

func convertServiceAvailabilityNotifToJson_1(obj *EventNotification) string {
	jsonInfo, err := json.Marshal(*obj)
	if err != nil {
		log.Error(err.Error())
		return ""
	}
	return string(jsonInfo)
}

func convertProblemDetailsToJson(obj *ProblemDetails) string {
	jsonInfo, err := json.Marshal(*obj)
	if err != nil {
		log.Error(err.Error())
		return ""
	}
	return string(jsonInfo)
}

func convertSubscriptionLinkListToJson(obj *SubscriptionLinkList) string {
	jsonInfo, err := json.Marshal(*obj)
	if err != nil {
		log.Error(err.Error())
		return ""
	}
	return string(jsonInfo)
}

func convertServiceLivenessInfoToJson(obj *ServiceLivenessInfo) string {
	jsonInfo, err := json.Marshal(*obj)
	if err != nil {
		log.Error(err.Error())
		return ""
	}
	return string(jsonInfo)
}

// func convertMecServiceMgmtApiSubscriptionLinkListToJson(obj *MecServiceMgmtApiSubscriptionLinkList) string {
// 	jsonInfo, err := json.Marshal(*obj)
// 	if err != nil {
// 		log.Error(err.Error())
// 		return ""
// 	}
// 	return string(jsonInfo)
// }

func convertProblemDetailstoJson(probdetails *ProblemDetails) string {
	jsonInfo, err := json.Marshal(*probdetails)
	if err != nil {
		log.Error(err.Error())
		return ""
	}
	return string(jsonInfo)
}
