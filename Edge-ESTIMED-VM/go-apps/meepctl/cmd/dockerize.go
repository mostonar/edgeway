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
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/InterDigitalInc/AdvantEDGE/go-apps/meepctl/utils"

	"github.com/roymx/viper"
	"github.com/spf13/cobra"
)

type DockerizeData struct {
	registry      string
	gitdir        string
	coreGoApps    []string
	sandboxGoApps []string
	noCache       bool
}

const dockerizeDesc = `Dockerize core components

AdvantEDGE is composed of a collection of micro-services.

Dockerize command genrates AdvantEDGE Docker images and stores them in
the local Docker registry.
Multiple targets can be specified (e.g. meepctl dockerize <target1> <target2>...)`

const dockerizeExample = `  # Dockerize all components
  meepctl dockerize all
  # Dockerize meep-platform-ctrl component only
  meepctl dockerize meep-platform-ctrl`

// dockerizeCmd represents the dockerize command
var dockerizeCmd = &cobra.Command{
	Use:       "dockerize <targets>",
	Short:     "Dockerize core components",
	Long:      dockerizeDesc,
	Example:   dockerizeExample,
	Args:      cobra.OnlyValidArgs,
	ValidArgs: nil,
	Run:       dockerizeRun,
}

var dockerizeData DockerizeData

func init() {
	// Get targets from repo config file
	dockerizeData.coreGoApps = utils.GetTargets("repo.core.go-apps", "dockerize")
	dockerizeData.sandboxGoApps = utils.GetTargets("repo.sandbox.go-apps", "dockerize")

	// Create the list of valid arguments
	baseArgs := []string{"all"}
	configArgs := append(dockerizeData.coreGoApps, dockerizeData.sandboxGoApps...)
	sort.Strings(configArgs)
	dockerizeCmd.ValidArgs = append(baseArgs, configArgs...)

	// Add list of arguments to Example usage
	dockerizeCmd.Example += "\n\n" + utils.FormatHeader("Valid Targets:")
	for _, arg := range dockerizeCmd.ValidArgs {
		dockerizeCmd.Example += "\n  * " + utils.FormatStep(arg)
	}

	// Set dockerize-specific flags
	dockerizeCmd.Flags().StringP("registry", "r", "", "Override registry from config file")
	dockerizeCmd.Flags().BoolVar(&dockerizeData.noCache, "no-cache", false, "Build Docker images without using the cache")

	// Add command
	rootCmd.AddCommand(dockerizeCmd)
}

func dockerizeRun(cmd *cobra.Command, args []string) {
	if !utils.ConfigValidate("") {
		fmt.Println("Fix configuration issues")
		return
	}

	targets := args
	if len(targets) == 0 {
		fmt.Println(utils.FormatError("Error: Need to specify at least one target"))
		_ = cmd.Usage()
		return
	}

	dockerizeData.registry, _ = cmd.Flags().GetString("registry")
	v, _ := cmd.Flags().GetBool("verbose")
	t, _ := cmd.Flags().GetBool("time")

	if v {
		fmt.Println("Dockerize called")
		fmt.Println("[arg]  targets:", targets)
		fmt.Println("[flag] registry:", dockerizeData.registry)
		fmt.Println("[flag] verbose:", v)
		fmt.Println("[flag] time:", t)
	}

	start := time.Now()

	// Retrieve registry from config file if not already set
	if dockerizeData.registry == "" {
		dockerizeData.registry = viper.GetString("meep.registry")
	}
	dockerizeData.registry = strings.TrimSuffix(dockerizeData.registry, "/")
	fmt.Println("Using docker registry:", dockerizeData.registry)

	// Get config
	dockerizeData.gitdir = strings.TrimSuffix(viper.GetString("meep.gitdir"), "/")

	// Dockerize microservices
	for _, target := range targets {
		if target == "all" {
			dockerizeAll(cmd)
		} else {
			dockerizeTarget(target, cmd)
		}
	}

	elapsed := time.Since(start)
	if t {
		fmt.Println("Took ", elapsed.Round(time.Millisecond).String())
	}
}

func dockerizeAll(cobraCmd *cobra.Command) {
	for _, target := range dockerizeData.coreGoApps {
		dockerize(target, "repo.core.go-apps.", cobraCmd)
		fmt.Println("")
	}
	for _, target := range dockerizeData.sandboxGoApps {
		dockerize(target, "repo.sandbox.go-apps.", cobraCmd)
		fmt.Println("")
	}
}

func dockerizeTarget(targetName string, cobraCmd *cobra.Command) {
	for _, target := range dockerizeData.coreGoApps {
		if target == targetName {
			dockerize(target, "repo.core.go-apps.", cobraCmd)
			return
		}
	}
	for _, target := range dockerizeData.sandboxGoApps {
		if target == targetName {
			dockerize(target, "repo.sandbox.go-apps.", cobraCmd)
			return
		}
	}
	fmt.Println(utils.FormatError("Error: Unsupported target: " + targetName))
}

func dockerize(targetName string, repo string, cobraCmd *cobra.Command) {
	srcdir := dockerizeData.gitdir + "/" + utils.RepoCfg.GetString(repo+targetName+".src")
	bindir := dockerizeData.gitdir + "/" + utils.RepoCfg.GetString(repo+targetName+".bin")
	fmt.Println(utils.FormatHeader("-- " + targetName + " --"))

	// Check if bindir exists, create it if it doesn't
	if _, err := os.Stat(bindir); os.IsNotExist(err) {
		fmt.Println(utils.FormatStep("   + creating bin directory: " + bindir))
		err := os.MkdirAll(bindir, 0755)
		if err != nil {
			fmt.Println(utils.FormatError("Error: Failed to create bin directory: " + err.Error()))
			os.Exit(1)
		}
	}
	// Copy Dockerfile
	fmt.Println(utils.FormatStep("   + copy docker file"))
	cmd := exec.Command("cp", "Dockerfile", bindir)
	cmd.Dir = srcdir
	out, err := utils.ExecuteCmd(cmd, cobraCmd)
	if err != nil {
		fmt.Println(utils.FormatError("Error: " + err.Error()))
		fmt.Println(out)
		os.Exit(1)
	}

	// copy service api files locally
	fmt.Println(utils.FormatStep("   + copy api files"))
	err = dockerizeCopyApi(repo+targetName+".api", bindir+"/api", cobraCmd)
	if err != nil {
		return
	}
	err = dockerizeCopyApi(repo+targetName+".user-api", bindir+"/user-api", cobraCmd)
	if err != nil {
		return
	}

	// copy container data locally
	fmt.Println(utils.FormatStep("   + copy docker data"))
	err = dockerizeCopyData(repo+targetName+".docker-data", bindir+"/data", cobraCmd)
	if err != nil {
		return
	}

	// Obtain checksum of bin folder contents to add as a label in docker image
	cmd = exec.Command("/bin/sh", "-c", "find "+bindir+" -type f -print0 | xargs -0 sha256sum | sort | sha256sum")
	output, _ := utils.ExecuteCmd(cmd, cobraCmd)
	checksum := strings.Split(output, " ")

	// dockerize & push to private meep docker registry
	buildArgs := []string{"build", "--rm", "--label", "MeepVersion=" + checksum[0]}
	if verbose, _ := cobraCmd.Flags().GetBool("verbose"); !verbose {
		// Line-based output that RunWithProgress can summarize
		buildArgs = append(buildArgs, "--progress=plain")
	}
	if dockerizeData.noCache {
		buildArgs = append(buildArgs, "--no-cache")
	}
	buildArgs = append(buildArgs, "-t")

	if dockerizeData.registry != "" {
		tag := dockerizeData.registry + "/" + targetName
		buildArgs = append(buildArgs, tag, bindir)
		cmd := exec.Command("docker", buildArgs...)
		err = utils.RunWithProgress(cmd, "   + dockerize "+targetName, cobraCmd)
		if err != nil {
			// Don't push: it would publish the previous, stale image under this tag
			fmt.Println(utils.FormatError("Error: Failed to dockerize " + tag + " with error: " + err.Error()))
			return
		}
		cmd = exec.Command("docker", "push", tag)
		err = utils.RunWithProgress(cmd, "   + push "+tag, cobraCmd)
		if err != nil {
			fmt.Println(utils.FormatError("Error: Failed to push " + tag + " with error: " + err.Error()))
			os.Exit(1)
		}
	} else {
		buildArgs = append(buildArgs, targetName, bindir)
		cmd := exec.Command("docker", buildArgs...)
		if err := utils.RunWithProgress(cmd, "   + dockerize "+targetName, cobraCmd); err != nil {
			fmt.Println(utils.FormatError("Error: Failed to dockerize " + targetName + " with error: " + err.Error()))
		}
	}
}

type dockerizeApi struct {
	Name string `json:"name"`
	File string `json:"file"`
}

func dockerizeCopyApi(key string, dstDir string, cobraCmd *cobra.Command) (err error) {
	verbose, _ := cobraCmd.Flags().GetBool("verbose")

	// Create dest dir
	cmd := exec.Command("rm", "-rf", dstDir)
	_, _ = utils.ExecuteCmd(cmd, cobraCmd)
	cmd = exec.Command("mkdir", "-p", dstDir)
	_, _ = utils.ExecuteCmd(cmd, cobraCmd)

	// Get key map data
	var apiList []dockerizeApi
	err = utils.RepoCfg.UnmarshalKey(key, &apiList)
	if err != nil {
		fmt.Println(utils.FormatError("Error: Failed to unmarshal api @ " + key))
		return err
	}

	// Copy API files
	for _, api := range apiList {
		dstApi := dstDir + "/" + api.Name
		srcApi := dockerizeData.gitdir + "/" + api.File
		if _, err = os.Stat(srcApi); !os.IsNotExist(err) {
			if verbose {
				fmt.Println("    Using: " + srcApi + " --> " + dstApi)
			}
			cmd := exec.Command("rm", "-f", dstApi)
			_, _ = utils.ExecuteCmd(cmd, cobraCmd)
			cmd = exec.Command("cp", srcApi, dstApi)
			_, err = utils.ExecuteCmd(cmd, cobraCmd)
			if err != nil {
				fmt.Println(utils.FormatError("Error: Failed to copy data: " + srcApi + " --> " + dstApi))
				return err
			}
		} else {
			fmt.Println(utils.FormatError("Error: Source data not found: " + srcApi + " --> " + dstApi))
			return err
		}
	}
	return nil
}

func dockerizeCopyData(key string, dstDir string, cobraCmd *cobra.Command) (err error) {
	verbose, _ := cobraCmd.Flags().GetBool("verbose")

	// Create dest dir
	cmd := exec.Command("rm", "-rf", dstDir)
	_, _ = utils.ExecuteCmd(cmd, cobraCmd)
	cmd = exec.Command("mkdir", "-p", dstDir)
	_, _ = utils.ExecuteCmd(cmd, cobraCmd)

	// Get key map data
	data := utils.RepoCfg.GetStringMapString(key)
	for k, v := range data {
		dstDataDir := dstDir + "/" + k
		srcDataDir := dockerizeData.gitdir + "/" + v
		if _, err = os.Stat(srcDataDir); !os.IsNotExist(err) {
			if verbose {
				fmt.Println("    Using: " + srcDataDir + " --> " + dstDataDir)
			}
			cmd := exec.Command("rm", "-rf", dstDataDir)
			_, _ = utils.ExecuteCmd(cmd, cobraCmd)
			cmd = exec.Command("cp", "-r", srcDataDir, dstDataDir)
			_, err = utils.ExecuteCmd(cmd, cobraCmd)
			if err != nil {
				fmt.Println(utils.FormatError("Error: Failed to copy data: " + srcDataDir + " --> " + dstDataDir))
				return err
			}
		} else {
			fmt.Println(utils.FormatError("Error: Source data not found: " + srcDataDir + " --> " + dstDataDir))
			return err
		}
	}
	return nil
}
