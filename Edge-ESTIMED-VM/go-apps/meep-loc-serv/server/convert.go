/*
 * Copyright (c) 2022  The AdvantEDGE Authors
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

func convertJsonToUserInfo(jsonInfo string) *UserInfo {

	if jsonInfo == "" {
		return nil
	}

	var userInfo UserInfo
	err := json.Unmarshal([]byte(jsonInfo), &userInfo)
	if err != nil {
		log.Error(err.Error())
		return nil
	}
	return &userInfo
}

func convertJsonToZoneInfo(jsonInfo string) *ZoneInfo {

	if jsonInfo == "" {
		return nil
	}

	var zoneInfo ZoneInfo
	err := json.Unmarshal([]byte(jsonInfo), &zoneInfo)
	if err != nil {
		log.Error(err.Error())
		return nil
	}
	return &zoneInfo
}

func convertJsonToAccessPointInfo(jsonInfo string) *AccessPointInfo {

	if jsonInfo == "" {
		return nil
	}

	var apInfo AccessPointInfo
	err := json.Unmarshal([]byte(jsonInfo), &apInfo)
	if err != nil {
		log.Error(err.Error())
		return nil
	}
	return &apInfo
}

func convertUserInfoToJson(userInfo *UserInfo) string {

	jsonInfo, err := json.Marshal(*userInfo)
	if err != nil {
		log.Error(err.Error())
		return ""
	}

	return string(jsonInfo)
}

func convertZoneInfoToJson(zoneInfo *ZoneInfo) string {

	jsonInfo, err := json.Marshal(*zoneInfo)
	if err != nil {
		log.Error(err.Error())
		return ""
	}

	return string(jsonInfo)
}

func convertAccessPointInfoToJson(apInfo *AccessPointInfo) string {

	jsonInfo, err := json.Marshal(*apInfo)
	if err != nil {
		log.Error(err.Error())
		return ""
	}

	return string(jsonInfo)
}

func convertZoneStatusSubscriptionToJson(zonalSubs *ZoneStatusSubscription) string {

	jsonInfo, err := json.Marshal(*zonalSubs)
	if err != nil {
		log.Error(err.Error())
		return ""
	}

	return string(jsonInfo)
}

func convertJsonToZoneStatusSubscription(jsonInfo string) *ZoneStatusSubscription {

	if jsonInfo == "" {
		return nil
	}

	var zonal ZoneStatusSubscription
	err := json.Unmarshal([]byte(jsonInfo), &zonal)
	if err != nil {
		log.Error(err.Error())
		return nil
	}
	return &zonal
}

func convertZonalSubscriptionToJson1(zonalSubs *ZoneLocationEventSubscription) string {

	jsonInfo, err := json.Marshal(*zonalSubs)
	if err != nil {
		log.Error(err.Error())
		return ""
	}

	return string(jsonInfo)
}

func convertJsonToZonalSubscription_1(jsonInfo string) *ZoneLocationEventSubscription {

	if jsonInfo == "" {
		return nil
	}

	var zonal ZoneLocationEventSubscription
	err := json.Unmarshal([]byte(jsonInfo), &zonal)
	if err != nil {
		log.Error(err.Error())
		return nil
	}
	return &zonal
}

func convertUserSubscriptionToJson1(userSubs *UserLocationEventSubscription) string {

	jsonInfo, err := json.Marshal(*userSubs)
	if err != nil {
		log.Error(err.Error())
		return ""
	}

	return string(jsonInfo)
}

func convertJsonToUserSubscription1(jsonInfo string) *UserLocationEventSubscription {

	if jsonInfo == "" {
		return nil
	}

	var user UserLocationEventSubscription
	err := json.Unmarshal([]byte(jsonInfo), &user)
	if err != nil {
		log.Error(err.Error())
		return nil
	}
	return &user
}

func convertPeriodicSubscriptionToJson1(periodicSubs *UserLocationPeriodicSubscription) string {

	jsonInfo, err := json.Marshal(*periodicSubs)
	if err != nil {
		log.Error(err.Error())
		return ""
	}

	return string(jsonInfo)
}

/*
func convertJsonToPeriodicSubscription(jsonInfo string) *PeriodicNotificationSubscription {

        if jsonInfo == "" {
                return nil
        }

        var periodic PeriodicNotificationSubscription
        err := json.Unmarshal([]byte(jsonInfo), &periodic)
        if err != nil {
                log.Error(err.Error())
                return nil
        }
        return &periodic
}
*/

func convertAreaSubscriptionToJson(AreaSubs *UserAreaSubscription) string {

	jsonInfo, err := json.Marshal(*AreaSubs)
	if err != nil {
		log.Error(err.Error())
		return ""
	}

	return string(jsonInfo)
}

func convertDistanceSubscriptionToJson1(distanceSubs *UserDistanceSubscription) string {

	jsonInfo, err := json.Marshal(*distanceSubs)
	if err != nil {
		log.Error(err.Error())
		return ""
	}

	return string(jsonInfo)
}

func convertStringToOperationStatus(opStatus string) OperationStatus {

	switch opStatus {
	case "Serviceable":
		return SERVICEABLE
	case "Unserviceable":
		return UNSERVICEABLE
	default:
		return UNKNOWN
	}
}

func convertStringToConnectionType(conType string) ConnectionType {

	switch conType {
	case "5G NR":
		return _5GNR_ConnectionType
	case "LTE":
		return LTE_ConnectionType
	case "Wifi":
		return WIFI_ConnectionType
	case "Wimax":
		return WIMAX_ConnectionType
	default:
		return UNKNOWN_ConnectionType
	}
}

func convertProblemDetailstoJson(probdetails *ProblemDetails) string {
	jsonInfo, err := json.Marshal(*probdetails)
	if err != nil {
		log.Error(err.Error())
		return ""
	}
	return string(jsonInfo)
}
