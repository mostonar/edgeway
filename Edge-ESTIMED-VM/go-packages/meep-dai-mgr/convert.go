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

package meepdaimgr

import (
	"encoding/json"
	"math"

	log "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-logger"
)

func convertPolygonToJson(area *Polygon) string {
	if area == nil { // This is not an error
		return "nil"
	}
	jsonInfo, err := json.Marshal(*area)
	if err != nil {
		log.Error(err.Error())
		return "nil"
	}
	return string(jsonInfo)
}

func convertJsonToPolygon(jsonInfo string) *Polygon {
	var obj Polygon
	if jsonInfo != "nil" { // nil string is processed as an empty array
		err := json.Unmarshal([]byte(jsonInfo), &obj)
		if err != nil {
			log.Error(err.Error())
			return nil
		}
		return &obj
	}

	return nil
}

func convertCivicAddressElementToJson(civicAddressElement *CivicAddressElement) string {
	if civicAddressElement == nil { // This is not an error
		return "nil"
	}
	jsonInfo, err := json.Marshal(*civicAddressElement)
	if err != nil {
		log.Error(err.Error())
		return "nil"
	}

	return string(jsonInfo)
}

func convertJsonToCivicAddressElement(jsonInfo string) *CivicAddressElement {
	var obj CivicAddressElement
	if jsonInfo != "nil" { // nil string is processed as an empty array
		err := json.Unmarshal([]byte(jsonInfo), &obj)
		if err != nil {
			log.Error(err.Error())
			return nil
		}
		return &obj
	}

	return nil
}

func convertApplicationListToJson(applicationList *ApplicationList) string {
	if applicationList == nil { // This is not an error
		return "nil"
	}
	jsonInfo, err := json.Marshal(*applicationList)
	if err != nil {
		log.Error(err.Error())
		return "nil"
	}

	return string(jsonInfo)
}

func convertJsonToApplicationList(jsonInfo string) *ApplicationList {
	var obj ApplicationList
	if jsonInfo != "nil" { // nil string is processed as an empty array
		err := json.Unmarshal([]byte(jsonInfo), &obj)
		if err != nil {
			log.Error(err.Error())
			return nil
		}
		return &obj
	}

	return nil
}

func NilToEmptyString(s *string) string {
	if s == nil {
		return "nil"
	}

	return *s
}

func EmptyToNilString(s string) *string {
	if s == "nil" {
		return nil
	}

	r := new(string)
	*r = s

	return r
}

func NilToEmptyUri(s *Uri) Uri {
	if s == nil {
		return "nil"
	}

	return *s
}

func EmptyToNilUInt32(s int) *uint32 {
	if s == math.MaxUint32 {
		return nil
	}

	u := new(uint32)
	*u = uint32(s)
	return u
}

func NilToEmptyUInt32(s *uint32) uint32 {
	if s == nil {
		return math.MaxUint32
	}

	return *s
}

func EmptyToNilUri(s *Uri) *Uri {
	if s != nil && *s == "nil" {
		return nil
	}

	return s
}
