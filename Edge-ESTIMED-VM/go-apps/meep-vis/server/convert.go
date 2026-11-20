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
	//"fmt"

	log "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-logger"
)

func convertPredictedQostoJson(predictedQos *PredictedQos) string {
	jsonInfo, err := json.Marshal(*predictedQos)
	if err != nil {
		log.Error(err.Error())
		return ""
	}
	return string(jsonInfo)
}

func convertProblemDetailstoJson(probdetails *ProblemDetails) string {
	jsonInfo, err := json.Marshal(*probdetails)
	if err != nil {
		log.Error(err.Error())
		return ""
	}
	return string(jsonInfo)
}

func convertProvChgUuUniSubscriptionToJson(provChgUuUniSubscription *ProvChgUuUniSubscription) string {
	jsonInfo, err := json.Marshal(*provChgUuUniSubscription)
	if err != nil {
		log.Error(err.Error())
		return ""
	}
	return string(jsonInfo)
}

func convertProvChgUuMbmsSubscriptionToJson(provChgUuMbmsSubscription *ProvChgUuMbmsSubscription) string {
	jsonInfo, err := json.Marshal(*provChgUuMbmsSubscription)
	if err != nil {
		log.Error(err.Error())
		return ""
	}
	return string(jsonInfo)
}

func convertProvChgPc5SubscriptionToJson(provChgPc5Subscription *ProvChgPc5Subscription) string {
	jsonInfo, err := json.Marshal(*provChgPc5Subscription)
	if err != nil {
		log.Error(err.Error())
		return ""
	}
	return string(jsonInfo)
}

func convertV2xMsgSubscriptionToJson(v2xMsgSubscription *V2xMsgSubscription) string {
	jsonInfo, err := json.Marshal(*v2xMsgSubscription)
	if err != nil {
		log.Error(err.Error())
		return ""
	}
	return string(jsonInfo)
}

func convertPredQosSubscriptionToJson(predQosSubscription *PredQosSubscription) string {
	jsonInfo, err := json.Marshal(*predQosSubscription)
	if err != nil {
		log.Error(err.Error())
		return ""
	}
	return string(jsonInfo)
}

// func convertV2xMsgPublicationToJson(v2xMsgPublication *V2xMsgPublication) string {
// 	jsonInfo, err := json.Marshal(*v2xMsgPublication)
// 	if err != nil {
// 		log.Error(err.Error())
// 		return ""
// 	}
// 	return string(jsonInfo)
// }
