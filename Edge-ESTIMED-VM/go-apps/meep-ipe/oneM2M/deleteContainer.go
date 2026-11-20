package onem2m

import (
	"fmt"
	"io"
	"log"
	"meep-ipe/utils"
	"net/http"
)

func DeleteContainer(containerName string) error {
	log.Println("Deleting Container...")

	url := fmt.Sprintf("%s:%s/~/%s/%s/%s/MecServices/%s", config["CSE-URL"], config["CSE-PORT"],
		config["CSE-ID"], config["CSE-NAME"], NodeName(), containerName)
	log.Println("url (Delete Container):", url)

	resp, err := utils.MakeOneM2MRequest("DELETE", url, "", 0, "CAdmin")
	if err != nil {
		return fmt.Errorf("request error: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusOK {
		log.Println("✓ Container deleted successfully")
		return nil
	} else if resp.StatusCode == http.StatusNotFound {
		log.Println("⚠ Container does not exist")
		return nil
	}

	return fmt.Errorf("failed to delete container: status %d, body: %s", resp.StatusCode, string(body))
}
