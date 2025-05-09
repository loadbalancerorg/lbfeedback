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
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
)

// Constants to define the flag names used by the CLI.

const (
	FlagType               = "type"
	FlagName               = "name"
	FlagCommandList        = "command-list"
	FlagProtocol           = "protocol"
	FlagIP                 = "ip"
	FlagPort               = "port"
	FlagRequestTimeout     = "request-timeout"
	FlagResponseTimeout    = "response-timeout"
	FlagThresholdMode      = "threshold-mode"
	FlagThresholdMax       = "threshold-max"
	FlagCommandInterval    = "command-interval"
	FlagMonitorName        = "monitor"
	FlagSourceSignificance = "significance"
	FlagSourceMaxValue     = "max-value"
	FlagMetricType         = "metric-type"
	FlagMetricInterval     = "interval-ms"
	FlagSampleTime         = "sampling-ms"
	FlagScriptName         = "script-name"
	FlagDiskPath           = "disk-path"
	FlagShapingEnabled     = "smart-shape"
	FlagLogState           = "log-state-changes"
)

// List of all flag names for use in processing the arguments.

var FlagList = []string{
	FlagType,
	FlagName,
	FlagCommandList,
	FlagProtocol,
	FlagIP,
	FlagPort,
	FlagRequestTimeout,
	FlagResponseTimeout,
	FlagThresholdMode,
	FlagThresholdMax,
	FlagCommandInterval,
	FlagMonitorName,
	FlagSourceSignificance,
	FlagSourceMaxValue,
	FlagMetricType,
	FlagMetricInterval,
	FlagSampleTime,
	FlagScriptName,
	FlagDiskPath,
	FlagShapingEnabled,
	FlagLogState,
}

// RunClientCLI delivers the client CLI personality of the Feedback Agent.
func RunClientCLI() (status int) {
	// Print the CLI masthead.
	fmt.Println(ShellBanner)
	// Suppress any log message output where we are calling
	// agent functions for loading the configuration.
	logrus.SetOutput(io.Discard)
	// Check minimum parameters have been provided.
	argc := len(os.Args)
	if argc < 2 {
		fmt.Println("Error: No command specified; terminating.")
		PlatformPrintRunInstructions()
		fmt.Println(
			"For a brief summary of CLI control and configuration syntax, \n" +
				"  use the 'help' command.",
		)
		status = ExitStatusError
		return
	}
	if os.Args[1] == "help" {
		PlatformPrintHelpMessage()
		status = ExitStatusNormal
		return
	}
	// Get the actionName and remaining arguments.
	actionName := os.Args[1]
	actionType := ""
	var actionArgs []string
	// -- Process arguments/flags.
	// Assume an unadorned third argument is the type field
	// unless it is prefixed with "-" as a flag.
	if argc >= 3 {
		actionArgs = os.Args[2:]
		actionArgs[0] = strings.TrimSpace(actionArgs[0])
		if !strings.HasPrefix(actionArgs[0], "-") {
			actionType = actionArgs[0]
			if len(actionArgs) >= 2 {
				actionArgs = actionArgs[1:]
			} else {
				actionArgs = make([]string, 0)
			}
		}
	}
	// Handle the specified action.
	responseObject, _, err := CLIHandleAgentAction(actionName, actionType, actionArgs)
	// Print any errors that occur.
	if err != nil {
		println("Error: " + err.Error() + ".")
		status = ExitStatusError
		return
	}
	// If there is a valid response object, pretty print it.
	if responseObject != nil {
		// Remove fields that we want to hide from the object
		responseObject.Request = nil
		responseObject.ID = nil
		// Marshal back again to JSON from the model object to pretty-print it.
		prettyPrintedJSON, err := json.MarshalIndent(responseObject, "", "    ")
		if err != nil {
			println("Error: Failed to format response: " + err.Error())
		} else {
			println(
				"JSON response from the Feedback Agent:\n\n" +
					string(prettyPrintedJSON) + "\n",
			)
			if responseObject.Message != "" {
				println(responseObject.Message)
			}
			if responseObject.Output != "" {
				println(responseObject.Output)
			}
		}
	}
	resultMsg := "The operation "
	if responseObject == nil || !responseObject.Success {
		resultMsg += "could not be completed."
	} else {
		resultMsg += "was successful."
	}
	println(resultMsg)
	return
}

func CLIHandleAgentAction(actionName string, actionType string, argv []string) (
	responseObject *APIResponse, responseJSON string, err error) {
	// Parse the CLI arguments into a Feedback Agent request.
	request, err := ParseArgumentsToRequest(actionName, actionType, argv)
	if err != nil {
		return
	}
	// $ TO DO: Allow user to specify the API IP, port and key as flags,
	// or alternatively the config dir and/or the config filename.
	configDir := DefaultConfigDir
	configFile := ConfigFileName
	// If this binary was built in local path mode, use that local path.
	if LocalPathMode {
		configDir, _ = os.Getwd()
	}
	// Attempt to load the API access settings from the config file.
	// ip, port, key, err := LoadAPIConfigFromFile(configDir, configFile)
	config, err := LoadAPIConfigFromFile(configDir, configFile)
	if err != nil {
		return
	}
	// Set the API key in the new request and build the URL.
	request.APIKey = config.Key
	apiURL := "https://" + config.IPAddress + ":" + config.Port
	// Marshal the request into JSON to send to the agent API.
	reqBodyJSON, err := json.MarshalIndent(request, "", "    ")
	if err != nil {
		return
	}
	// Create a custom transport object with certificate validation
	// checking disabled. Really, we should at some point implement
	// a method for setting a custom CA which is shared between the
	// agent and the client, but this will have to suffice for now.
	customTransport := http.DefaultTransport.(*http.Transport).Clone()
	customTransport.TLSClientConfig = &tls.Config{
		InsecureSkipVerify: true,
	}
	client := &http.Client{
		Transport: customTransport,
	}
	// Send the marshalled JSON to the API via HTTP.
	httpResponse, err := client.Post(
		apiURL,
		"application/json",
		bytes.NewBuffer(reqBodyJSON),
	)
	// Handle any resulting errors.
	if err != nil {
		err = errors.New(
			err.Error() + "\nThe CLI Client failed to establish " +
				"an HTTP connection to the Agent." +
				"\nPlease check that the Agent is running and able to " +
				"accept API requests",
		)
		return
	}
	// Read the contents of the response.
	responseBytes, err := io.ReadAll(httpResponse.Body)
	if err != nil {
		return
	}
	responseJSON = string(responseBytes)
	responseObject, err = UnmarshalAPIResponse(responseJSON)
	return
}

func ParseArgumentsToRequest(actionName string, actionType string, argv []string) (request APIRequest, err error) {
	// Define the set of flags available for all actions to
	// parse from the input arguments. Note that it is the responsibility of
	// the API to validate that the correct parameters have been supplied.
	apiArgs := flag.NewFlagSet("", flag.ContinueOnError)
	apiArgs.Usage = func() {}
	// Initialise the argMap, which provides a map of flag names and their
	// resulting value pointers that will be set on parsing by the 'flag'
	// package.
	argMap := make(map[string]*string)
	foundMap := make(map[string]bool)
	for _, argKey := range FlagList {
		argMap[argKey] = apiArgs.String(argKey, "", "")
	}
	// Parse the incoming command line parameters.
	err = apiArgs.Parse(argv)
	// Exit if any parameters were invalid.
	if err != nil && !errors.Is(err, flag.ErrHelp) {
		err = errors.New("one or more parameters are invalid; " +
			"use the 'help' command for syntax")
		return
	}
	// Visit all flags and mark which ones were set.
	apiArgs.Visit(func(f *flag.Flag) {
		foundMap[f.Name] = true
	})
	// Create the destination objects for the new request.
	params := MetricParams{}
	request = APIRequest{
		Action:       actionName,
		Type:         actionType,
		MetricParams: &params,
	}
	// Iterate through all the flags and process their values, if specified.
	for argKey, argString := range argMap {
		strVal := strings.TrimSpace(*argString)
		// Skip this key (leaving the JSON field as nil) if it wasn't specified
		// or its trimmed value consists of an empty string.
		if !foundMap[argKey] || strVal == "" {
			continue
		}
		// Convert the argument string into our range of possible value types.
		intVal, _ := strconv.Atoi(strVal)
		int64Val := int64(intVal)
		floatVal, _ := strconv.ParseFloat(strVal, 64)
		boolVal, _ := strconv.ParseBool(strVal)

		// Map this CLI flag into the JSON request object field based on the flag key.
		switch argKey {
		case FlagType:
			request.Type = strVal
		case FlagName:
			request.TargetName = strVal
		case FlagCommandList:
			request.CommandList = &strVal
		case FlagProtocol:
			request.ProtocolName = &strVal
		case FlagIP:
			if strVal == "any" {
				request.ListenIPAddress = StringAddr("*")
			} else {
				request.ListenIPAddress = &strVal
			}
		case FlagPort:
			request.ListenPort = &strVal
		case FlagRequestTimeout:
			request.RequestTimeout = &intVal
		case FlagResponseTimeout:
			request.ResponseTimeout = &intVal
		case FlagThresholdMode:
			request.ThresholdMode = &strVal
		case FlagThresholdMax:
			request.ThresholdScore = &intVal
		case FlagCommandInterval:
			request.CommandInterval = &intVal
		case FlagMonitorName:
			request.SourceMonitorName = &strVal
		case FlagSourceSignificance:
			request.SourceSignificance = &floatVal
		case FlagSourceMaxValue:
			request.SourceMaxValue = &int64Val
		case FlagMetricType:
			request.MetricType = &strVal
		case FlagMetricInterval:
			request.MetricInterval = &intVal
		case FlagSampleTime:
			params[ParamKeySampleTime] = strconv.Itoa(intVal)
		case FlagScriptName:
			params[ParamKeyScriptName] = strVal
		case FlagDiskPath:
			params[ParamKeyDiskPath] = strVal
		case FlagShapingEnabled:
			request.SmartShape = &boolVal
		case FlagLogState:
			request.LogStateChanges = &boolVal
		}
	}
	return
}

// UnmarshalAPIResponse parses a JSON request string into an [APIRequest].
func UnmarshalAPIResponse(responseJSON string) (response *APIResponse, err error) {
	// Attempt to unmarshal the request into the target object.
	response = &APIResponse{}
	err = json.Unmarshal([]byte(responseJSON), response)
	return
}

// LoadAPIConfigFromFile attempts to load the API access details from the JSON config.
func LoadAPIConfigFromFile(dir string, file string) (config APIConfig, err error) {
	// Try to load a config from the location.
	agentConfig := FeedbackAgent{}
	agentConfig.InitialiseServiceMaps()
	_, err = agentConfig.LoadAgentConfig(dir, file)
	if err != nil {
		err = errors.New(
			"unable to load agent config for API credentials:\n" +
				err.Error(),
		)
		return
	}
	api, err := agentConfig.GetResponderByName("api")
	if err != nil {
		err = errors.New("failed to obtain API config: " + err.Error())
	}
	config = APIConfig{
		IPAddress: api.ListenIPAddress,
		Port:      api.ListenPort,
		Key:       agentConfig.APIKey,
	}
	return
}

// -------------------------------------------------------------------
// END OF FILE
// -------------------------------------------------------------------
