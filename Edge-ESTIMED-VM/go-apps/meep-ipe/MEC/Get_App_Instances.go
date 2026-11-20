package mec

import (
	"encoding/json"
	"fmt"
	"io"
	utils "meep-ipe/utils"
	"net/http"
)

// GetAppInstances retrieves all MEC Application Instances from the platform
//
// This function queries the MEC platform's application management API to obtain
// a list of all available application instances, including their IDs, names, and node names.
//
// Reference: ETSI GS MEC 011 - MEC Application Lifecycle Management API
//
// @param url The URL endpoint for querying application instances
//
//	(e.g., /sandbox-ctrl/v1/applications)
//
// @return []mec.AppInstanceID Array of application instance information
// @return error Returns nil on success, or an error if the request fails
func GetAppInstances(url string) ([]AppInstanceID, error) {
	resp, err := utils.SendMECRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get MEC app instances: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get MEC App Instance ID: received status code %d", resp.StatusCode)
	}

	// From the response body, extract all App Instance IDs
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}
	//log.Println("Response body:", string(body))

	var allAppInstances []AppInstanceID
	if err := json.Unmarshal(body, &allAppInstances); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response body: %v", err)
	}
	return allAppInstances, nil
}
