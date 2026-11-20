package onem2m

import (
	"meep-ipe/utils"
	"fmt"
	"io"
	"log"
	"net/http"
)

// RetrieveContainer retrieves container details from the oneM2M platform
//
// This function queries the oneM2M platform to retrieve information about
// a specific Container resource, verifying its existence and accessibility.
//
// Reference: oneM2M TS-0001 - Functional Architecture
//
// @param platformURL The base URL of the oneM2M CSE platform
// @param cse_name The name of the Common Services Entity (CSE)
// @param cse_id The structured ID of the CSE
// @param App_Name The resource name for the application
// @param AE_ID The Application Entity ID for authentication
// @param containerName The name of the container to retrieve
// @return error Returns nil on success, or an error if retrieval fails
func RetrieveContainer(platformURL, cse_name, cse_id, App_Name, AE_ID, containerName string) error {
	url := fmt.Sprintf("%s/~%s/%s/%s/%s", platformURL, cse_id, cse_name, App_Name, containerName)

	resp, err := utils.MakeOneM2MRequest("GET", url, "", 0, AE_ID)
	if err != nil {
		return fmt.Errorf("error retrieving container: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusOK {
		log.Println("✓ Container exists and is accessible")
		return nil
	}

	return fmt.Errorf("container not found: status %d, body: %s", resp.StatusCode, string(body))
}
