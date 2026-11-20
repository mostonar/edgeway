package onem2m

import (
	"fmt"
	"io"
	"log"
	"net/http"

	"meep-ipe/utils"
)

func DeleteNode(nodeName string) error {
	log.Println("Deleting Node...")

	url := fmt.Sprintf("%s:%s/~/%s/%s/%s", config["CSE-URL"], config["CSE-PORT"],
		config["CSE-ID"], config["CSE-NAME"], nodeName)
	log.Println("url (Delete Node):", url)

	resp, err := utils.MakeOneM2MRequest("DELETE", url, "", 0, "CAdmin")
	if err != nil {
		return fmt.Errorf("request error: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusOK {
		log.Println("✓ Node deleted successfully")
		return nil
	} else if resp.StatusCode == http.StatusNotFound {
		log.Println("⚠ Node does not exist")
		return nil
	}

	return fmt.Errorf("failed to delete node: status %d, body: %s", resp.StatusCode, string(body))
}