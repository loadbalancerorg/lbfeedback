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
	"strings"

	"github.com/sirupsen/logrus"
)

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
	}
	if responseObject != nil {
		// Remove fields that we want to hide from the object
		responseObject.Request = nil
		responseObject.ID = nil
		reformatted, err := json.MarshalIndent(responseObject, "", "    ")
		if err != nil {
			println("Error: Failed to format response.")
		} else {
			println(
				"JSON response from the Feedback Agent:\n" +
					string(reformatted) + "\n",
			)
		}
	}
	resultMsg := ""
	if responseObject == nil || !responseObject.Success {
		resultMsg = "could not be completed"
	} else {
		resultMsg = "was successful"
	}
	println("The operation " + resultMsg + ".")
	return
}

func CLIHandleAgentAction(actionName string, actionType string, argv []string) (
	responseObject *APIResponse, responseJSON string, err error) {
	// Define the set of flags available for all actions to
	// process from the CLI. Note that it is the responsibility of
	// the API to validate that the correct parameters have been supplied.
	apiArgs := flag.NewFlagSet("", flag.ContinueOnError)
	apiArgs.Usage = func() {}
	argType := apiArgs.String("type", "", "")
	argTargetName := apiArgs.String("name", "", "")
	argCommandList := apiArgs.String("command-list", "", "")

	// Fields for FeedbackResponder API requests.
	argProtocolName := apiArgs.String("protocol", "", "")
	argIPAddress := apiArgs.String("ip", "", "")
	argListenPort := apiArgs.String("port", "", "")
	argRequestTimeout := apiArgs.Int("request-timeout", 0, "")
	argResponseTimeout := apiArgs.Int("response-timeout", 0, "")
	argThresholdString := apiArgs.String("threshold-mode", "", "")
	argResponderThreshold := apiArgs.Int("threshold-max", -1, "")
	argCommandInterval := apiArgs.Int("command-interval", -1, "")

	// Fields for monitor and source API requests.
	argMonitorName := apiArgs.String("monitor", "", "")
	argSourceSignificance := apiArgs.Float64("significance", 1.0, "")
	argSourceMaxValue := apiArgs.Int64("max-value", -1, "")
	argMetricType := apiArgs.String("metric-type", "", "")

	// Fields for MetricParams configuration. Note that all
	// of these are String values within metric.go.
	argSampleTime := apiArgs.String("sampling-ms", "", "")
	argScriptName := apiArgs.String("script-name", "", "")
	argDiskPath := apiArgs.String("disk-path", "", "")
	argShapingEnabledString := apiArgs.String("smart-shape", "", "")
	argLogStateChangesString := apiArgs.String("log-state-changes", "", "")

	// $$ TO DO: Define help for actions.
	err = apiArgs.Parse(argv)
	if err != nil && !errors.Is(err, flag.ErrHelp) {
		err = errors.New("one or more parameters are invalid; " +
			"use the 'help' command for syntax")
		return
	}

	// If no action type was specified, a -type flag can be
	// set instead; handle this situation.
	if actionType == "" && argType != nil && *argType != "" {
		actionType = *argType
	}

	// Process parameters where required.
	argCommandInterval = PointerHandleIntValue(argCommandInterval)
	argResponderThreshold = PointerHandleIntValue(argResponderThreshold)
	argCommandList = PointerHandleStringValue(argCommandList)
	argSourceMaxValue = PointerHandleInt64Value(argSourceMaxValue)
	argShapingEnabled := PointerHandleBoolString(argShapingEnabledString)
	argLogStateChanges := PointerHandleBoolString(argLogStateChangesString)

	// Process any metric parameters specified on the CLI.
	params := MetricParams{}
	if argSampleTime != nil && *argSampleTime != "" {
		params[ParamKeySampleTime] = *argSampleTime
	}
	if argScriptName != nil && *argScriptName != "" {
		params[ParamKeyScriptName] = *argScriptName
	}
	if argDiskPath != nil && *argDiskPath != "" {
		params[ParamKeyDiskPath] = *argDiskPath
	}

	// Set fields into the new API request; the API will be responsible
	// for determining the validity of options for a request.
	request := APIRequest{
		Action:             actionName,
		Type:               actionType,
		TargetName:         *argTargetName,
		ProtocolName:       argProtocolName,
		ListenIPAddress:    argIPAddress,
		ListenPort:         argListenPort,
		RequestTimeout:     argRequestTimeout,
		ResponseTimeout:    argResponseTimeout,
		CommandList:        argCommandList,
		ThresholdMode:      argThresholdString,
		ThresholdScore:     argResponderThreshold,
		CommandInterval:    argCommandInterval,
		SourceMonitorName:  argMonitorName,
		SourceSignificance: argSourceSignificance,
		SourceMaxValue:     argSourceMaxValue,
		MetricType:         argMetricType,
		MetricInterval:     argCommandInterval,
		MetricParams:       &params,
		SmartShape:         argShapingEnabled,
		LogStateChanges:    argLogStateChanges,
	}

	// Workaround for * being expanded into a glob in bash
	if *request.ListenIPAddress == "any" {
		asterisk := "*"
		request.ListenIPAddress = &asterisk
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
