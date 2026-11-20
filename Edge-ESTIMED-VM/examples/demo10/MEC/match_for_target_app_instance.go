package mec

import (
	model "demo10/Model"
	"fmt"
	"regexp"
)

// MatchForTargetAppInstance matches application instances by CSE name using regex
//
// This function filters the list of all application instances to find those
// whose names match the provided CSE name pattern using regular expression matching.
//
// @param allAppInstances Array of all available MEC Application Instance IDs
// @param cse_name The CSE name pattern (regex) to match against
// @return []model.AppInstanceID Array of matching application instances
// @return error Returns nil on success, or an error if no matches are found or regex is invalid
func MatchForTargetAppInstance(allAppInstances []model.AppInstanceID, cse_name string) ([]model.AppInstanceID, error) {
	// Compile regex for cse_name once
	reCSE, err := regexp.Compile(cse_name)
	if err != nil {
		return nil, fmt.Errorf("invalid regex for cse_name: %v", err)
	}
	var matchingAppInstances []model.AppInstanceID
	for _, appInstance := range allAppInstances {
		// If name matches cse_name using compiled regex, store it in our array
		if reCSE.MatchString(appInstance.Name) {
			matchingAppInstance := model.AppInstanceID{
				ID:       appInstance.ID,
				Name:     appInstance.Name,
				NodeName: appInstance.NodeName,
			}
			matchingAppInstances = append(matchingAppInstances, matchingAppInstance)
		}
	}

	if len(matchingAppInstances) == 0 {
		return nil, fmt.Errorf("MEC App Instance with name %s not found", cse_name)
	}
	return matchingAppInstances, nil
}
