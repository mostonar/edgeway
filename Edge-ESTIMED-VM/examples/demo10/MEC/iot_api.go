package mec

import (
	utils "demo10/utils"
	"encoding/json"
	"fmt"
	"net/http"
)

// GETIotPlatformInfo retrieves IoT platform information from the MEC platform
//
// This function queries the MEC platform's IoT API to obtain information about
// registered IoT platforms, including their endpoints and connection details.
//
// Reference: ETSI GS MEC 040 - MEC IoT API
//
// @param url The URL endpoint for querying registered IoT platforms
//            (e.g., /iots/v1/registered_iot_platforms)
// @return []map[string]interface{} Array of IoT platform information maps
// @return error Returns nil on success, or an error if the request fails
func GETIotPlatformInfo(url string) ([]map[string]interface{}, error) {
	resp, err := utils.SendMECRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get IoT platform info: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get IoT platform info: received status code %d", resp.StatusCode)
	}

	var platformInfo []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&platformInfo); err != nil {
		return nil, fmt.Errorf("failed to decode IoT platform info: %v", err)
	}
	return platformInfo, nil
}
