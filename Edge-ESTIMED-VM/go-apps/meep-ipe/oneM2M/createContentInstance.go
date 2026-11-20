package onem2m

import (
	"meep-ipe/utils"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

// CreateContentInstance creates a ContentInstance resource in the specified container
//
// This function creates a ContentInstance resource containing the provided data.
// ContentInstances represent actual data values stored in a Container and are
// the leaf nodes in the oneM2M resource tree.
//
// Reference: oneM2M TS-0001 - Functional Architecture
//
// @param platformURL The base URL of the oneM2M CSE platform
// @param cse_name The name of the Common Services Entity (CSE)
// @param App_Name The resource name for the application
// @param AE_ID The Application Entity ID for authentication
// @param containerName The name of the container where the content will be stored
// @param remote_cse_id The remote CSE ID for access control
// @param data The JSON data string to store in the ContentInstance
// @return error Returns nil on success, or an error if ContentInstance creation fails
func CreateContentInstance(platformURL, cse_name, App_Name, AE_ID, containerName, remote_cse_id, data string) error {
	log.Printf("Creating ContentInstance with data: %s", data)

	// Escape JSON quotes in the content field
	escapedData := strings.ReplaceAll(data, `"`, `\"`)

	reqBody := fmt.Sprintf(`{
		"m2m:cin": {
			"con": "%s",
			"lbl": ["telemetry"],
			"at": ["%s"],
			"aa": ["lbl", "con"]
		}
	}`, escapedData, remote_cse_id)

	url := fmt.Sprintf("%s/%s/%s/%s", platformURL, cse_name, App_Name, containerName)

	resp, err := utils.MakeOneM2MRequest("POST", url, reqBody, 4, AE_ID)
	if err != nil {
		return fmt.Errorf("request error: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusCreated {
		log.Println("✓ ContentInstance created successfully")
		return nil
	}

	return fmt.Errorf("failed to create content instance: status %d, body: %s", resp.StatusCode, string(body))
}
