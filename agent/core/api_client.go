// api_client.go
// API Client Functions for the CLI Shell Interface
//
// Project:     Loadbalancer.org Feedback Agent v5
// Author:      Nicholas Turnbull
//              <nicholas.turnbull@loadbalancer.org>
//
// Copyright (C) 2024 Loadbalancer.org Ltd
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
)

// Delivers the client CLI personality of the Feedback Agent.
func RunClientCLI() (status int) {
	// Print the CLI masthead.
	fmt.Println(ShellBanner)
	fmt.Println("Entering CLI Client mode.")
	// Check minimum parameters have been provided.
	argc := len(os.Args)
	if argc < 2 {
		fmt.Println("Error: No command specified; terminating.")
		fmt.Println("To run the Agent (either interactively or from a startup script), \n" +
			"  use the 'run-agent' command.")
		fmt.Println("For CLI control and configuration syntax, " +
			"please consult the README file.")
		status = ExitStatusParamError
		return
	}
	// Get the command and remaining arguments.
	command := os.Args[1]
	argv := os.Args[2:]
	responseObject := &APIResponse{}
	var err error
	// Run the specified command, or output an error if it does not exist.
	switch command {
	case "action":
		if argc >= 3 {
			// If we have the minimum arguments, handle the action
			responseObject, _, err =
				CLIHandleAgentAction(argv[0], argv[1:])
		} else {
			// Otherwise we weren't given an action to perform
			err = errors.New("no action type specified")
		}
	default:
		// Command not recognised
		err = errors.New("not recognised")
	}
	if err != nil {
		println("Error: command '" + command + "': " + err.Error() + ".")
		status = ExitStatusParamError
	}
	if responseObject != nil {
		// Remove unwanted fields from the object
		responseObject.Request = nil
		responseObject.ID = nil
		remarshal, err := json.MarshalIndent(responseObject, "", "    ")
		if err != nil {
			println("Error: Failed to format response.")
		} else {
			println("JSON reply from the Feedback Agent:\n" +
				string(remarshal))
		}
	}
	if responseObject.Success {
		println("Operation completed successfully.")
	} else {
		println("An error occurred during the the operation.")
	}
	return
}

func CLIHandleAgentAction(action string, argv []string) (responseObject *APIResponse,
	responseJSON string, err error) {
	// Check that the supplied command is a valid API action.
	switch action {
	case "status", "add", "edit", "delete", "start", "stop",
		"restart", "save-config", "get-config", "get-feedback",
		"haproxy-enable", "haproxy-disable", "haproxy-up",
		"haproxy-down", "haproxy-clear", "haproxy-set-threshold":
		// Valid command; no action
	case action:
		// Unrecognised command
		err = errors.New("action '" + action + "' not recognised")
		return
	}

	// Define the set of flags available for all actions to
	// process from the CLI. Note that it is the API's responsibility
	// to validate that the correct parameters have been supplied.
	act := flag.NewFlagSet("action", flag.ContinueOnError)
	actService := act.String("service", "", "")
	actName := act.String("name", "", "")
	actSourceMonitorName := act.String("monitor", "", "")
	actProtocolName := act.String("protocol", "", "")
	actListenIPAddress := act.String("ip", "", "")
	actListenPort := act.String("port", "", "")
	actRequestTimeout := act.Int("request-timeout", 0, "")
	actResponseTimeout := act.Int("response-timeout", 0, "")
	actHAProxyCommands := act.Bool("send-commands", false, "")
	actHAProxyThreshold := act.Int("threshold-value", 0, "")
	actMetricType := act.String("metric-type", "", "")
	actInterval := act.Int("interval-ms", 0, "")

	// $$ TO DO: Define help for actions (catch error)
	_ = act.Parse(argv)

	// Set fields into the new API request; the API will be responsible
	// for determining the validity of options for a request.
	request := APIRequest{
		Action:            action,
		Service:           *actService,
		Name:              *actName,
		SourceMonitorName: actSourceMonitorName,
		ProtocolName:      actProtocolName,
		ListenIPAddress:   actListenIPAddress,
		ListenPort:        actListenPort,
		RequestTimeout:    actRequestTimeout,
		ResponseTimeout:   actResponseTimeout,
		HAProxyCommands:   actHAProxyCommands,
		HAProxyThreshold:  actHAProxyThreshold,
		MetricType:        actMetricType,
		Interval:          actInterval,
	}

	// Workaround for * being expanded into a glob in bash
	if *request.ListenIPAddress == "any" {
		// Needed to get around the conversion to *string
		asterisk := "*"
		request.ListenIPAddress = &asterisk
	}

	// $ TO DO: Allow user to specify the API IP, port and key as flags,
	// or alternatively the config dir and/or the config filename.
	configDir := ConfigDir
	configFile := ConfigFileName
	// If this binary was built in local path mode, use that local path.
	if LocalPathMode {
		configDir, _ = os.Getwd()
	}
	// Attempt to load the API access settings from the config file.
	ip, port, key, err := LoadAPIConfigFromFile(configDir, configFile)
	if err != nil {
		return
	}
	// Set the API key in the new request and build the URL.
	request.APIKey = key
	apiURL := "http://" + ip + ":" + port
	// Marshal the request into JSON to send to the agent API.
	reqBodyJSON, err := json.MarshalIndent(request, "", "    ")
	if err != nil {
		return
	}
	// Send the marshalled JSON to the API via HTTP.
	httpResponse, err := http.Post(apiURL, "application/json",
		bytes.NewBuffer(reqBodyJSON))
	if err != nil {
		err = errors.New("Error: " + err.Error() + "\nThe CLI Client " +
			"failed to establish an HTTP connection to the agent." +
			"\nPlease check that the Agent is running and able to " +
			"accept API requests")
		return
	}
	// Read the contents of the response.
	responseBytes, err := io.ReadAll(httpResponse.Body)
	if err != nil {
		return
	}
	responseJSON = string(responseBytes)
	responseObject, err = UnmarshalAPIResponse(responseJSON)
	if err != nil {
		return
	}
	return
}

// Unmarshals a JSON request string into an [APIRequest].
func UnmarshalAPIResponse(responseJSON string) (response *APIResponse, err error) {
	// Attempt to unmarshal the request into the target object.
	response = &APIResponse{}
	err = json.Unmarshal([]byte(responseJSON), response)
	return
}

// Attempts to load the API access details from the JSON config.
func LoadAPIConfigFromFile(dir string, file string) (ip string, port string,
	key string, err error) {
	// Try to load a config from the location.
	agentConfig := FeedbackAgent{}
	agentConfig.InitialiseServiceMaps()
	_, err = agentConfig.LoadAgentConfig(dir, file)
	if err != nil {
		err = errors.New("unable to load agent config for API credentials:\n" +
			err.Error())
		return
	}
	api, err := agentConfig.GetResponderByName("api")
	if err != nil {
		err = errors.New("failed to obtain API config: " + err.Error())
	}
	ip = api.ListenIPAddress
	port = api.ListenPort
	key = agentConfig.APIKey
	return
}

// -------------------------------------------------------------------
// END OF FILE
// -------------------------------------------------------------------
