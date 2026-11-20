package onem2m

import (
	"fmt"
	"net/http"
	"strconv"
)

// oneM2M Response Status Codes (TS-0004) treated as success for create operations
const (
	RscCreated                        = 2001
	RscConflict                       = 4105
	RscOriginatorHasAlreadyRegistered = 4117
)

// responseRSC extracts the oneM2M Response Status Code from the X-M2M-RSC header
func responseRSC(resp *http.Response) (int, error) {
	value := resp.Header.Get("X-M2M-RSC")
	if value == "" {
		return 0, fmt.Errorf("missing X-M2M-RSC header (HTTP status %d)", resp.StatusCode)
	}
	rsc, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid X-M2M-RSC header %q: %v", value, err)
	}
	return rsc, nil
}

// isCreateSuccess reports whether an RSC is considered a successful create
func isCreateSuccess(rsc int) bool {
	return rsc == RscCreated || rsc == RscConflict || rsc == RscOriginatorHasAlreadyRegistered
}
