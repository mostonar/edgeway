/*
 * Copyright (c) 2022  The AdvantEDGE Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package helm

import (
	"errors"
	"os"
	"os/exec"
	"strings"

	log "github.com/InterDigitalInc/AdvantEDGE/go-packages/meep-logger"
)

func installCharts(charts []Chart, sandboxName string) error {
	err := ensureReleases(charts, sandboxName)
	if err != nil {
		return err
	}

	errChan := make(chan error, len(charts))
	for _, chart := range charts {
		go func(c Chart) {
			errChan <- install(c)
		}(chart)
	}

	var installErr error
	for i := 0; i < len(charts); i++ {
		if err := <-errChan; err != nil {
			installErr = err
		}
	}

	if installErr != nil {
		log.Info("Cleaning installed releases")
		cleanReleases(charts, sandboxName)
		return installErr
	}

	return nil
}

func ensureReleases(charts []Chart, sandboxName string) error {
	// ensure that releases do not already exist
	releases, _ := GetReleasesName(sandboxName)
	for _, c := range charts {
		for _, r := range releases {
			if c.ReleaseName == r.Name {
				err := errors.New("Release [" + c.ReleaseName + "] already exists")
				log.Error(err)
				return err
			}
		}
	}
	return nil
}

func install(chart Chart) error {
	log.Debug("Installing chart: " + chart.ReleaseName)
	var cmd *exec.Cmd
	if strings.Trim(chart.ValuesFile, " ") == "" {
		codecovLocation := strings.TrimSpace(os.Getenv("MEEP_CODECOV_LOCATION")) + chart.ReleaseName
		codecovEnabled := strings.TrimSpace(os.Getenv("MEEP_CODECOV"))
		/*onboardedappEnabled := strings.TrimSpace(os.Getenv("MEEP_ONBOARDEDAPP"))
		onboardedappLocation := strings.TrimSpace(os.Getenv("MEEP_ONBOARDEDAPP_LOCATION")) + chart.ReleaseName*/
		cmd = exec.Command("helm", "install", chart.ReleaseName,
			"--namespace", chart.Namespace, "--create-namespace",
			"--set", "nameOverride="+chart.Name,
			"--set", "fullnameOverride="+chart.Name,
			chart.Location, "--replace", "--disable-openapi-validation",
			"--set", "codecov.enabled="+codecovEnabled,
			"--set", "codecov.location="+codecovLocation,
			"--set", "image.env.MEEP_CODECOV="+codecovEnabled) /*,
		"--set", "onboardedapp.enabled="+onboardedappEnabled,
		"--set", "onboardedapp.location="+onboardedappLocation)*/
	} else {
		cmd = exec.Command("helm", "install", chart.ReleaseName,
			"--namespace", chart.Namespace, "--create-namespace",
			"--set", "nameOverride="+chart.Name,
			"--set", "fullnameOverride="+chart.Name,
			"-f", chart.ValuesFile,
			chart.Location, "--replace")
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Error("Failed to install Release [" + chart.ReleaseName + "] at " + chart.Location)
		log.Error("Error(", err.Error(), "): ", string(out))
		return err
	}
	return nil
}

func cleanReleases(charts []Chart, sandboxName string) {
	var toClean []Chart
	var cnt int
	releases, _ := GetReleasesName(sandboxName)

	for _, c := range charts {
		for _, r := range releases {
			if c.ReleaseName == r.Name {
				toClean = append(toClean, c)
				cnt++
			}
		}
	}

	if cnt > 0 {
		_ = deleteReleases(toClean)
	}
}
