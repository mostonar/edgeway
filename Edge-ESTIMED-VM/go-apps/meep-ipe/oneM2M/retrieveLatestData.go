package onem2m

import (
	"meep-ipe/utils"
	"fmt"
	"io"
	"log"
	"net/http"
)

// RetrieveLatestData retrieves the latest ContentInstance from the specified container
//
// This function queries the oneM2M platform to retrieve the most recent
// ContentInstance stored in the container using the 'la' (latest) virtual
// resource identifier.
//
// Reference: oneM2M TS-0001 - Functional Architecture
//
// @param platformURL The base URL of the oneM2M CSE platform
// @param cse_name The name of the Common Services Entity (CSE)
// @param App_Name The resource name for the application
// @param AE_ID The Application Entity ID for authentication
// @param containerName The name of the container to query
// @return string The JSON content of the latest ContentInstance
// @return error Returns nil on success, or an error if retrieval fails
func RetrieveLatestData(platformURL, cse_name, App_Name, AE_ID, containerName string) (string, error) {
	// Use 'la' (latest) to get the most recent ContentInstance
	// ToDo: Fix variable name cse_id to cse_name // check this file
	url := fmt.Sprintf("%s/%s/%s/%s/la", platformURL, cse_name, App_Name, containerName)

	resp, err := utils.MakeOneM2MRequest("GET", url, "", 0, AE_ID)
	if err != nil {
		return "", fmt.Errorf("error retrieving data: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusOK {
		log.Println("✓ Latest data retrieved successfully")
		return string(body), nil
	}

	return "", fmt.Errorf("failed to retrieve data: status %d", resp.StatusCode)
}
