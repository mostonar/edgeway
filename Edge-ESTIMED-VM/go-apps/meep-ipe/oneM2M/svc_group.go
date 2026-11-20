package onem2m

import (
	"fmt"
	"io"
	"log"
	"net/http"

	"meep-ipe/utils"
)

func CreateMecServicesGroup(nodeName string) error {
	reqBody := `{
		"mec:MecServices": {
			"rn": "MecServices",
			"lbl": ["MEC"],
			"cnd": "org.etsi.mec.moduleclass.MecServicesGroup"
		}
	}`

	url := fmt.Sprintf("%s:%s/~/%s/%s/%s", config["CSE-URL"], config["CSE-PORT"],
		config["CSE-ID"], config["CSE-NAME"], nodeName)

	resp, err := utils.MakeOneM2MRequest("POST", url, reqBody, 28, "CAdmin")
	if err != nil {
		return fmt.Errorf("request error: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusCreated {
		log.Println("✓ MEC services group created")
		return nil
	} else if resp.StatusCode == http.StatusConflict {
		log.Println("⚠ Group already exists")
		return nil
	}

	return fmt.Errorf("failed to create MEC services group, status: %d, body: %s", resp.StatusCode, string(body))
}
