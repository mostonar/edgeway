package main

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"strings"

	server "meep-ipe/Server"	
	// mec "meep-ipe/MEC"
)

func main() {
	// Read the YAML file
	configData, err := os.ReadFile("config.yaml")
	if err != nil {
		fmt.Printf("Error reading YAML file: %v\n", err)
		return
	}

	// Parse the YAML data
	var config map[string]interface{}
	err = yaml.Unmarshal(configData, &config)
	if err != nil {
		fmt.Printf("Error parsing YAML: %v\n", err)
		return
	}
	// Sanity check for required fields
	if _, ok := config["basePath"]; !ok {
		fmt.Println("Error: 'basePath' not found in config.yaml")
		return
	}
	if _, ok := config["ipePort"]; !ok {
		fmt.Println("Error: 'ipePort' not found in config.yaml")
		return
	}
	// In case https is false, url needs to be http, otherwise https
	ipeUrl, ok := config["ipeUrl"].(string)
	if !ok {
		fmt.Printf("config[ipeUrl] is missing or not a string")
	}

	config["serverIp"] = config["ipeUrl"].(string)

	httpsFlag, ok := config["https"].(bool)
	if !ok {
		fmt.Printf("config[https] is missing or not a bool")
	}

	if !httpsFlag {
		ipeUrl = strings.Join([]string{"http://", config["ipeUrl"].(string)}, "")
		config["ipeUrl"] = ipeUrl
	} else {
		ipeUrl = strings.Join([]string{"https://", config["ipeUrl"].(string)}, "")
		config["ipeUrl"] = ipeUrl
	}
	
	// server.StartServer(config["basePath"].(string), int(config["ipePort"].(int)))
	server.StartServer(config)
	// mec.InitMEC(config)
}