package mec

import (
	"fmt"
	"io"
	"log"
	"regexp"

	utils "meep-ipe/utils"
)

// hrefPattern matches the href attribute value inside an <a> tag,
// e.g. <a href="Application%20Mobility%20Service%20REST%20API">
var hrefPattern = regexp.MustCompile(`<a\s+href="([^"]+)"`)

// fetchAndAppendHref sends a GET to endpoint, extracts the first href
// from the response body as-is (still URL-encoded), and appends it
// directly to endpoint to build the full resource URL.
func fetchAndAppendHref(endpoint string) (string, error) {
	resp, err := utils.SendMECRequest("GET", endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("failed to GET %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	matches := hrefPattern.FindSubmatch(body)
	if len(matches) < 2 {
		return "", fmt.Errorf("no href found in response from %s: %s", endpoint, string(body))
	}

	href := string(matches[1])
	fullURL := endpoint + href

	log.Printf("fetchAndAppendHref: endpoint=%s href=%s -> fullURL=%s", endpoint, href, fullURL)

	return fullURL, nil
}
