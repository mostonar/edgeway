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

package cmd

import (
	"fmt"

	"sync"
	"time"

	"github.com/InterDigitalInc/AdvantEDGE/go-apps/meepctl/utils"
	"github.com/spf13/cobra"
)

type DeleteData struct {
	coreApps []string
	depApps  []string
}

const deleteDesc = `Delete containers from the K8s cluster

AdvantEDGE is composed of a collection of micro-services (a.k.a the groups).

Delete command removes a group of containers from the K8s cluster.`

const deleteExample = `  # Delete dependency containers
  meepctl delete dep
  # Delete a single dependency
  meepctl delete dep meep-couchdb
  # Delete only AdvantEDGE core containers
  meepctl delete core`

// deleteCmd represents the delete command
var deleteCmd = &cobra.Command{
	Use:     "delete <group> [app]",
	Short:   "Delete containers from the K8s cluster",
	Long:    deleteDesc,
	Example: deleteExample,
	Args:    cobra.RangeArgs(1, 2),
	Run:     deleteRun,
}

var deleteData DeleteData

func init() {
	// Get targets from repo config file
	deleteData.coreApps = utils.GetTargets("repo.core.go-apps", "deploy")
	deleteData.depApps = utils.GetTargets("repo.dep", "deploy")

	// Configure the list of valid arguments
	deleteCmd.ValidArgs = []string{"dep", "core"}

	// Add list of arguments to Example usage
	deleteCmd.Example += "\n\nValid Targets:"
	for _, arg := range deleteCmd.ValidArgs {
		deleteCmd.Example += "\n  * " + arg
	}

	// Add command
	rootCmd.AddCommand(deleteCmd)
}

func deleteRun(cmd *cobra.Command, args []string) {
	if !utils.ConfigValidate("") {
		fmt.Println("Fix configuration issues")
		return
	}

	group := args[0]
	var targetApp string
	if len(args) > 1 {
		targetApp = args[1]
	}

	v, _ := cmd.Flags().GetBool("verbose")
	t, _ := cmd.Flags().GetBool("time")
	if v {
		fmt.Println("Delete called")
		fmt.Println("[arg]  group:", group)
		if targetApp != "" {
			fmt.Println("[arg]  app:", targetApp)
		}
		fmt.Println("[flag] verbose:", v)
		fmt.Println("[flag] time:", t)
	}

	start := time.Now()
	switch group {
	case "core":
		if targetApp != "" {
			out := k8sDelete(targetApp, cmd)
			fmt.Print(out)
		} else {
			deleteApps(deleteData.coreApps, cmd)
		}
	case "dep":
		if targetApp != "" {
			deleteSingleDepApp(targetApp, cmd)
		} else {
			deleteApps(deleteData.depApps, cmd)
		}
	}

	elapsed := time.Since(start)
	if t {
		fmt.Println("Took ", elapsed.Round(time.Millisecond).String())
	}
}

func deleteApps(apps []string, cobraCmd *cobra.Command) {
	var wg sync.WaitGroup
	var printMutex sync.Mutex

	for _, app := range apps {
		wg.Add(1)
		go func(appName string) {
			defer wg.Done()
			output := k8sDelete(appName, cobraCmd)

			if output != "" {
				printMutex.Lock()
				fmt.Print(output)
				printMutex.Unlock()
			}
		}(app)
	}
	wg.Wait()
}

// Delete a single dep app
func deleteSingleDepApp(app string, cobraCmd *cobra.Command) {
	// Validate the app is in the dep list
	found := false
	for _, depApp := range deleteData.depApps {
		if depApp == app {
			found = true
			break
		}
	}
	if !found {
		fmt.Println("Error: '" + app + "' is not a valid dep target")
		fmt.Println("Valid dep targets:")
		for _, a := range deleteData.depApps {
			fmt.Println("  * " + a)
		}
		return
	}
	out := k8sDelete(app, cobraCmd)
	fmt.Print(out)
}

func k8sDelete(component string, cobraCmd *cobra.Command) string {
	var out string
	// If release exist
	exist, outRel, _ := utils.IsHelmRelease(component, cobraCmd)
	out += outRel
	if exist {
		switch component {
		case "meep-prometheus":
			// PVCs are created by StatefulSet and shouldn't be deleted manually if we want to retain data
		case "meep-thanos":
			// PVCs are kept by helm resource-policy, so we don't manually delete them
		case "meep-thanos-archive":
			// PVCs are kept by helm resource-policy, so we don't manually delete them
		}
		// Delete
		outDel, err := utils.HelmDelete(component, cobraCmd)
		out += outDel
		if err != nil {
			out += fmt.Sprintf("Helm delete failed with Error: %v\n", err)
		}
	}
	return out
}
