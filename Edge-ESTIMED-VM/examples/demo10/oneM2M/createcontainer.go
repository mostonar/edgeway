package onem2m

import (
	"demo10/utils"
	"fmt"
	"io"
	"log"
	"net/http"
)

// CreateContainer creates a Container resource under the Application Entity
//
// This function creates a Container resource in the oneM2M resource tree.
// Containers are used to store ContentInstance resources (data) and provide
// a logical grouping mechanism for related data.
//
// Reference: oneM2M TS-0001 - Functional Architecture
//
// @param platformURL The base URL of the oneM2M CSE platform
// @param cse_name The name of the Common Services Entity (CSE)
// @param cse_id The structured ID of the CSE
// @param App_Name The resource name for the application
// @param AE_ID The Application Entity ID for authentication
// @param containerName The name of the container to create
// @param remote_cse_id The remote CSE ID for access control
// @return error Returns nil on success, or an error if container creation fails
func CreateContainer(platformURL, cse_name, cse_id, App_Name, AE_ID, containerName, remote_cse_id string) error {
	log.Println("Creating Container...")

	reqBody := fmt.Sprintf(`{
		"m2m:cnt": {
			"rn": "%s",
			"mni": 10,
			"at": ["%s"],
			"lbl": ["Data/Telemetry"],
			"aa": ["lbl"]
		}
	}`, containerName, remote_cse_id)

	url := fmt.Sprintf("%s/~/%s/%s/%s", platformURL, cse_id, cse_name, App_Name)
	log.Println("url (Create Container):", url)
	resp, err := utils.MakeOneM2MRequest("POST", url, reqBody, 3, AE_ID)
	if err != nil {
		return fmt.Errorf("request error: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusCreated {
		log.Println("✓ Container created successfully")
		return nil
	} else if resp.StatusCode == http.StatusConflict {
		log.Println("⚠ Container already exists")
		return nil
	}

	return fmt.Errorf("failed to create container: status %d, body: %s", resp.StatusCode, string(body))
}
