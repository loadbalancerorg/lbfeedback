// api_receiver.go
// Feedback Agent API Implementation
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
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// ReceiveAPIRequest handles an incoming JSON API request received by this
// FeedbackAgent via a FeedbackResponder service.
func (agent *FeedbackAgent) ReceiveAPIRequest(requestJSON string) (
	responseJSON string, err error, quitAfterResponding bool) {
	// Unmarshal into an empty request
	request, err := UnmarshalAPIRequest(requestJSON)
	// Get a response object for this request (with or without an error).
	response, quitAfterResponding := agent.ProcessAPIRequest(request, err)
	// Marshal the response object into the JSON response.
	output, err := json.MarshalIndent(response, "", "    ")
	if err == nil {
		responseJSON = string(output) + "\n"
	} else {
		logrus.Error("Failed to marshal JSON API response.")
	}
	return
}

// UnmarshalAPIRequest unmarshals a JSON request string into an APIRequest.
func UnmarshalAPIRequest(requestJSON string) (request *APIRequest, err error) {
	// Attempt to unmarshal the request into the target object.
	request = &APIRequest{}
	err = json.Unmarshal([]byte(requestJSON), request)
	return
}

// ValidateAPIRequest performs basic initial sanity checks of an API request.
func (agent *FeedbackAgent) ValidateAPIRequest(request *APIRequest) (errID string,
	errMsg string) {
	if request == nil {
		errID = "bad-json"
		errMsg = "could not read JSON"
	} else if (request.Type == "monitor" || request.Type == "responder") &&
		request.TargetName == "" {
		errID = "missing-target"
		errMsg = "no target service name specified"
	} else if request.APIKey == "" || request.APIKey != agent.APIKey {
		//logrus.Debug("api key mismatch: request = '" + request.APIKey + "', agent = '" + agent.APIKey + "'")
		errID = "bad-api-key"
		errMsg = "invalid or missing API key"
	}
	return
}

// ProcessAPIRequest processes an incoming API request and performs the required actions.
func (agent *FeedbackAgent) ProcessAPIRequest(request *APIRequest, parseErr error) (
	response *APIResponse, quitAfterResponding bool) {
	// -- Perform required initialisation and validation.
	// Build boilerplate for the API response.
	response = &APIResponse{
		APIName: AppIdentifier,
		Version: VersionString,
		Tag:     RandomHexBytes(4),
	}
	// Copy mirrored fields (for client reference) into the response.
	if request != nil {
		response.ID = &request.ID
		request.Action = strings.TrimSpace(request.Action)
		response.Request = request
	} else {
		// Invalid request (nil reference, from JSON unmarshal failure)
		response.Error = "empty-request"
		response.Message = "no API request specified"
		return
	}
	if parseErr != nil {
		// Parsing/syntax error
		response.Error = "json-syntax"
		response.Message = "JSON syntax error: " + parseErr.Error()
		return
	}
	request.Type = strings.TrimSpace(request.Type)
	request.Action = strings.TrimSpace(request.Action)
	request.TargetName = strings.TrimSpace(request.TargetName)
	response.Error, response.Message = agent.ValidateAPIRequest(request)
	if response.Error != "" {
		return
	}
	// -- The main API command tree.
	// This default error will be overridden by nil or another error
	// if a matching part of the tree is reached.
	desc := BuildAPIDescription(request)
	unknownType, suppressLog, quitAfterResponding, err :=
		agent.apiActionTree(request, response)
	// Generate errors for an unknown service type.
	if unknownType {
		err = errors.New("invalid action type '" + request.Type + "'")
	}
	// Handle any unsaved changes after the API tree.
	if agent.unsavedChanges {
		saveSuccess, saveErr := agent.SaveAgentConfigToPaths()
		if saveSuccess {
			logrus.Info("Agent configuration successfully saved.")
		} else {
			logrus.Error("Failed to save agent configuration.")
		}
		err = errors.Join(err, saveErr)
	}
	apiLogHead := "API request #" + response.Tag + " "
	// Handle any errors that have occurred.
	if err != nil {
		response.Error = "api-error"
		response.Message += "failed: " + desc + ": " + err.Error()
		if !suppressLog {
			logrus.Error(apiLogHead + response.Message)
		}
	} else {
		// The request was successful if no errors occurred.
		response.Success = true
		response.Message += "succeeded: " + desc
		if !suppressLog {
			logrus.Info(apiLogHead + response.Message)
		}
	}
	// Hide API key in confirmation of request to the client
	response.Request.APIKey = ""
	return
}

func (agent *FeedbackAgent) apiActionTree(request *APIRequest, response *APIResponse) (
	unknownType bool, suppressLog bool, quitAfterResponding bool, err error) {
	switch request.Action {
	// Service actions
	case "add", "edit", "delete", "start", "restart", "stop":
		request.TargetName = strings.TrimSpace(request.TargetName)
		if request.TargetName == "" {
			err = errors.New("no target name specified")
			return
		}
		switch request.Type {
		case "monitor":
			err = agent.APIHandleMonitorRequest(request)
		case "responder":
			err = agent.APIHandleResponderRequest(request)
		case "source":
			err = agent.APIHandleSourceRequest(request)
		case "agent":
			switch request.Action {
			case "restart":
				err = agent.RestartAllServices()
			case "stop":
				quitAfterResponding = true
			default:
				unknownType = true
			}
		default:
			unknownType = true
		}
	case "status":
		response.ServiceStatus = agent.GetServiceStatusArray()
		suppressLog = true
	case "get":
		switch request.Type {
		case "config":
			config := agent.APIHandleGetConfig()
			response.AgentConfig = &config
			suppressLog = true
		case "feedback":
			response.Output, err =
				agent.APIHandleGetFeedback(request)
			suppressLog = true
		case "sources":
			response.FeedbackSources, err =
				agent.APIHandleGetSources(request)
			suppressLog = true
		default:
			unknownType = true
		}
	case "set":
		switch request.Type {
		case "commands", "cmd":
			err = agent.APIHandleSetCommands(request, true)
		case "threshold":
			err = agent.APIHandleSetThreshold(request)
		default:
			unknownType = true
		}
	case "send":
		switch request.Type {
		case "online":
			err = agent.APIHandleSetOnlineState(request.TargetName,
				true, HAPEnumNone)
		case "offline":
			err = agent.APIHandleSetOnlineState(request.TargetName,
				false, HAPEnumNone)
		default:
			unknownType = true
		}
	case "force":
		switch request.Type {
		case "halt", "maint":
			err = agent.APIHandleSetOnlineState(request.TargetName,
				false, HAPEnumMaintenance)
		case "drain":
			err = agent.APIHandleSetOnlineState(request.TargetName,
				false, HAPEnumDrain)
		case "online":
			err = agent.APIHandleSetOnlineState(request.TargetName,
				true, HAPDefaultOnline)
		case "save-config":
			agent.unsavedChanges = true
		default:
			unknownType = true
		}
	default:
		err = errors.New("invalid action specified")
	}
	return
}

func (agent *FeedbackAgent) APIHandleGetConfig() (config FeedbackAgent) {
	// Shallow-copy the fields from the agent first to avoid overwriting them.
	config = *agent
	// Hide the API key from the response for security
	config.APIKey = ""
	// Remove duplicated service name and version
	config.ServiceName = ""
	config.Version = ""
	return
}

// GetServiceStatusArray builds an array of the service status.
func (agent *FeedbackAgent) GetServiceStatusArray() (array []APIServiceStatus) {
	// Report status of responders
	for name, responder := range agent.Responders {
		array = AppendToStatusArray(array, "responder", name,
			ServiceRunningToString(responder.runState))
	}
	// Report status of monitors
	for name, monitor := range agent.Monitors {
		array = AppendToStatusArray(array, "monitor", name,
			ServiceRunningToString(monitor.runState))
	}
	return
}

// AppendToStatusArray appends an item to a service status array.
func AppendToStatusArray(array []APIServiceStatus, serviceType string,
	name string, state string) []APIServiceStatus {
	return append(array, APIServiceStatus{
		ServiceType:   serviceType,
		ServiceName:   name,
		ServiceStatus: state,
	})
}

// ServiceRunningToString converts a boolean running state to a descriptive string.
func ServiceRunningToString(running bool) string {
	if running {
		return "running"
	} else {
		return "stopped"
	}
}

// GetAgentStatusString outputs the current agent status as a descriptive string.
func (agent *FeedbackAgent) GetAgentStatusString() (status string) {
	if agent.isStarting {
		status = "starting"
	} else if agent.isRunning {
		status = "running"
	} else {
		status = "stopped"
	}
	return
}

// BuildAPIDescription makes a log line description of an API request.
func BuildAPIDescription(request *APIRequest) (desc string) {
	desc = "(no action)"
	if request.Action != "" {
		desc = "action '" + request.Action + "'"
	}
	if request.Type != "" {
		desc += ", type '" + request.Type + "'"
	}
	if request.TargetName != "" {
		desc += ", target name '" + request.TargetName + "'"
	}
	return
}

// ----------------------------------------
// API action handlers
// ----------------------------------------

func (agent *FeedbackAgent) APIAddMonitor(request *APIRequest) (err error) {
	metricType := ""
	if request.MetricType != nil {
		metricType = *request.MetricType
	} else {
		err = errors.New("system metric type not specified")
		return
	}
	interval := 0
	if request.MetricInterval != nil {
		interval = *request.MetricInterval
	}
	params := MetricParams{}
	if request.MetricParams != nil {
		params = *request.MetricParams
	}
	shaping := false
	if request.SmartShape != nil {
		shaping = *request.SmartShape
	}
	// Try to add this as a new [SystemMonitor].
	err = agent.AddMonitor(
		request.TargetName,
		metricType,
		interval,
		params,
		shaping,
	)
	if err != nil {
		return
	}
	// Attempt to start the new monitor.
	err = agent.StartMonitorByName(request.TargetName)
	// If this failed, remove the new monitor and concatenate the errors.
	if err != nil {
		deleteErr := agent.DeleteMonitorByName(request.TargetName)
		err = errors.Join(err, deleteErr)
		return
	}
	agent.unsavedChanges = true
	return
}
func (agent *FeedbackAgent) APIAddResponder(request *APIRequest) (err error) {
	protocolName := ""
	if request.ProtocolName != nil {
		protocolName = *request.ProtocolName
	} else {
		err = errors.New("protocol not specified")
		return
	}
	ipAddress := ""
	if request.ListenIPAddress != nil {
		ipAddress = *request.ListenIPAddress
	} else {
		err = errors.New("IP address not specified")
		return
	}
	listenPort := ""
	if request.ListenPort != nil {
		listenPort = *request.ListenPort
	} else {
		err = errors.New("listen port not specified")
		return
	}
	hapThreshold := 0
	if request.ThresholdScore != nil {
		hapThreshold = *request.ThresholdScore
	}
	thresholdMode := ""
	if request.ThresholdMode != nil {
		thresholdMode = *request.ThresholdMode
	}
	hapCommands := ""
	if request.CommandList != nil {
		hapCommands = *request.CommandList
	}
	logStateChanges := false
	if request.LogStateChanges != nil {
		logStateChanges = *request.LogStateChanges
	}
	// Try to add this as a new [FeedbackResponder]. The AddResponder() function will
	// look for and find the object for the [SystemMonitor] if it exists.
	err = agent.AddResponder(
		request.TargetName,
		*request.FeedbackSources,
		protocolName,
		ipAddress,
		listenPort,
		hapCommands,
		thresholdMode,
		hapThreshold,
		logStateChanges,
	)
	// If we couldn't add the responder (e.g. because the monitor doesn't exist),
	// fail out to an error.
	if err != nil {
		return
	}
	// Attempt to start the new responder.
	err = agent.StartResponderByName(request.TargetName)
	// If this failed, remove the new responder and concatenate the errors.
	if err != nil {
		deleteErr := agent.DeleteResponderByName(request.TargetName)
		err = errors.Join(err, deleteErr)
		return
	}
	agent.unsavedChanges = true
	return
}

func (agent *FeedbackAgent) APIEditMonitor(request *APIRequest) (err error) {
	name := request.TargetName
	// Fetch the monitor this request refers to (if any, otherwise error).
	oldMonitor, err := agent.GetMonitorByName(name)
	if err != nil {
		return
	}
	changed := false
	valid := false

	// Copy the old monitor so that we can apply the changes to it.
	newMonitor := oldMonitor.Copy()

	// Handle any changes to the metric type.
	if request.MetricType != nil {
		metricType, _ := StandardiseNameIdentifier(*request.MetricType)
		if metricType != "" {
			valid = true
			if metricType != newMonitor.MetricType {
				newMonitor.MetricType = metricType
				changed = true
			}
		}
	}

	if request.MetricInterval != nil {
		valid = true
		if *request.MetricInterval != oldMonitor.Interval {
			newMonitor.Interval = *request.MetricInterval
			changed = true
		}
	}

	// Process metric parameter key/value pairs if required.
	if request.MetricParams != nil {
		for key, value := range *request.MetricParams {
			key = strings.ToLower(strings.TrimSpace(key))
			value = strings.TrimSpace(value)
			if key != "" && value != "" {
				valid = true
				newMonitor.Params[key] = value
				changed = true
			}
		}
	}

	if request.SmartShape != nil {
		valid = true
		if *request.SmartShape != oldMonitor.SmartShape {
			newMonitor.SmartShape = *request.SmartShape
			changed = true
		}
	}
	if !changed {
		if !valid {
			err = errors.New("no valid fields to change specified")
		}
		return
	}
	// Attempt to initialise the new monitor to validate it, else error.
	err = newMonitor.Initialise()
	if err != nil {
		return
	}
	// This is valid, so replace it in the list of monitors.
	agent.Monitors[request.TargetName] = &newMonitor
	// Preserve the current run state during the swap.
	wasRunning := oldMonitor.IsRunning()
	if wasRunning {
		err = oldMonitor.Stop()
		if err != nil {
			return
		}
		err = newMonitor.Start()
		if err != nil {
			return
		}
	}
	// Search and swap this monitor for any Responders using it.
	for _, responder := range agent.Responders {
		_, exists := responder.FeedbackSources[name]
		if exists {
			responder.FeedbackSources[name].Monitor = &newMonitor
		}
	}
	agent.unsavedChanges = true
	return
}

func (agent *FeedbackAgent) APIEditResponder(request *APIRequest) (err error) {
	// Fetch the responder that this pertains to (otherwise, return the error).
	oldResponder, err := agent.GetResponderByName(request.TargetName)
	if err != nil {
		return
	}
	// Copy the old responder so that we can apply the changes to it.
	newResponder := oldResponder.Copy()
	// Process JSON pointer fields (to determine if they were set or not).
	if request.FeedbackSources != nil {
		newResponder.FeedbackSources = *request.FeedbackSources
	}
	if request.ProtocolName != nil {
		if request.TargetName == "api" {
			err = errors.New("API responders do not have a configurable protocol")
			return
		}
		newResponder.ProtocolName = *request.ProtocolName
	}
	if request.ListenIPAddress != nil {
		newResponder.ListenIPAddress = *request.ListenIPAddress
	}
	if request.ListenPort != nil {
		newResponder.ListenPort = *request.ListenPort
	}
	if request.RequestTimeout != nil {
		newResponder.RequestTimeout = time.Duration(*request.RequestTimeout)
	}
	if request.ResponseTimeout != nil {
		newResponder.ResponseTimeout = time.Duration(*request.ResponseTimeout)
	}
	if request.ThresholdMode != nil {
		newResponder.ThresholdModeName = *request.ThresholdMode
	}
	if request.ThresholdScore != nil {
		newResponder.ThresholdScore = *request.ThresholdScore
	}
	if request.FeedbackSources != nil {
		newResponder.FeedbackSources = *request.FeedbackSources
	}
	if request.LogStateChanges != nil {
		newResponder.LogStateChanges = *request.LogStateChanges
	}
	// Attempt to initialise the new responder to validate it, else error.
	err = newResponder.Initialise()
	if err != nil {
		return
	}
	// This is valid, so replace it in the list of monitors.
	agent.Responders[request.TargetName] = &newResponder
	// Preserve the current run state during the swap.
	wasRunning := oldResponder.IsRunning()
	if wasRunning {
		err = oldResponder.Stop()
		if err != nil {
			return
		}
		err = newResponder.Start()
	}
	if err != nil {
		return
	}
	agent.unsavedChanges = true
	return
}

func (agent *FeedbackAgent) APIDeleteResponder(request *APIRequest) (
	err error) {
	if request.TargetName == "api" {
		err = errors.New("cannot delete the API Responder")
		return
	}
	err = agent.DeleteResponderByName(request.TargetName)
	return
}

func (agent *FeedbackAgent) APIDeleteMonitor(request *APIRequest) (
	err error) {
	name := request.TargetName
	// Fetch the monitor that this pertains to (otherwise, an error occurs).
	mon, err := agent.GetMonitorByName(name)
	if err != nil {
		return
	}
	// Search for any responders attached to this monitor;
	// fail if any currently in use.
	for _, responder := range agent.Responders {
		_, exists := responder.FeedbackSources[name]
		if exists {
			err = errors.New("cannot delete monitor '" + name +
				"': currently in use by responder '" +
				responder.ResponderName)
			return
		}
	}
	// Not currently in use; go ahead and delete it, stopping it first
	// if it's running.
	if mon.IsRunning() {
		err = mon.Stop()
		if err != nil {
			return
		}
	}
	err = agent.DeleteMonitorByName(name)
	if err != nil {
		return
	}
	agent.unsavedChanges = true
	return
}

func (agent *FeedbackAgent) APIHandleResponderRequest(request *APIRequest) (
	err error) {
	if request.Action == "add" {
		err = agent.APIAddResponder(request)
		return
	}
	res, err := agent.GetResponderByName(request.TargetName)
	if err != nil {
		return
	}
	switch request.Action {
	case "edit":
		err = agent.APIEditResponder(request)
	case "delete":
		err = agent.APIDeleteResponder(request)
	case "start":
		err = res.Start()
	case "stop":
		err = res.Stop()
	case "restart":
		err = res.Restart()
	default:
		err = errors.New("unknown action '" + request.Action + "'")
	}
	return
}

func (agent *FeedbackAgent) APIHandleMonitorRequest(request *APIRequest) (
	err error) {
	if request.Action == "add" {
		err = agent.APIAddMonitor(request)
		return
	}
	mon, err := agent.GetMonitorByName(request.TargetName)
	if err != nil {
		return
	}
	switch request.Action {
	case "edit":

		err = agent.APIEditMonitor(request)
	case "delete":
		err = agent.APIDeleteMonitor(request)
	case "start":
		err = mon.Start()
	case "stop":
		err = mon.Stop()
	case "restart":
		err = mon.Restart()
	default:
		err = errors.New("unknown action '" + request.Action + "'")
	}
	return
}

func (agent *FeedbackAgent) APIHandleSourceRequest(request *APIRequest) (
	err error) {
	res, err := agent.GetResponderByName(request.TargetName)
	if err != nil {
		return
	}
	if request.SourceMonitorName == nil {
		err = errors.New("no source monitor specified")
		return
	}
	switch request.Action {
	case "add":
		err = res.AddFeedbackSource(*request.SourceMonitorName,
			request.SourceSignificance, request.SourceMaxValue,
			request.ThresholdScore)
	case "edit":
		err = res.EditFeedbackSource(*request.SourceMonitorName,
			request.SourceSignificance, request.SourceMaxValue,
			request.ThresholdScore)
	case "delete":
		err = res.DeleteFeedbackSource(*request.SourceMonitorName)
	default:
		err = errors.New("unknown action '" + request.Action + "'")
		return
	}
	agent.unsavedChanges = true
	return
}

func (agent *FeedbackAgent) APIHandleGetSources(request *APIRequest) (
	sources map[string]*FeedbackSource, err error) {
	res, err := agent.GetResponderByName(request.TargetName)
	if err != nil {
		return
	}
	sources = res.FeedbackSources
	return
}

func (agent *FeedbackAgent) APIHandleGetFeedback(request *APIRequest) (
	feedback string, err error) {
	res, err := agent.GetResponderByName(request.TargetName)
	if err == nil {
		feedback, _ = res.GetResponse("")
		feedback = strings.ReplaceAll(feedback, "\n", "")
	}
	return
}

func (agent *FeedbackAgent) APIHandleSetOnlineState(name string,
	isOnline bool, commandMask int) (err error) {
	name = strings.TrimSpace(name)
	targets := make(map[string]*FeedbackResponder)
	if name == "" {
		targets = agent.Responders
	} else {
		var res *FeedbackResponder
		res, err = agent.GetResponderByName(name)
		if err != nil {
			return
		}
		targets[name] = res
	}
	for _, res := range targets {
		res.SetCommandState(isOnline, true, commandMask)
	}
	return
}

// APIHandleSetThreshold processes an API request to set a value or state
// for applying a threshold to the Feedback Agent output.
func (agent *FeedbackAgent) APIHandleSetThreshold(request *APIRequest) (
	err error) {
	// Fetch the responder from the parent agent object.
	res, err := agent.GetResponderByName(request.TargetName)
	if err != nil {
		return
	}
	changed := false
	// Process a change to the threshold score, if provided.
	if request.ThresholdScore != nil {
		err = res.ConfigureThresholdValue(*request.ThresholdScore)
		if err != nil {
			return
		}
		changed = true
	}
	// Process a change to whether the threshold is enabled, if provided
	// or triggered by the above code.
	if request.ThresholdMode != nil {
		err = res.ConfigureThresholdMode(*request.ThresholdMode)
		if err != nil {
			return
		}
		changed = true
	}
	// If no change occurred but this was a request to change something
	// about the threshold, then we raise an error. Otherwise, flag
	// to the agent to resave the config file.
	if !changed {
		err = errors.New("no threshold parameters specified")
	} else {
		agent.unsavedChanges = true
	}
	return
}

func (agent *FeedbackAgent) APIHandleSetCommands(request *APIRequest,
	replace bool) (err error) {
	res, err := agent.GetResponderByName(request.TargetName)
	if err != nil {
		return
	}
	if request.CommandList != nil {
		err = res.ConfigureCommands(*request.CommandList, replace, false)
		if err != nil {
			return
		}
	}
	if request.CommandInterval != nil {
		err = res.ConfigureInterval(*request.CommandInterval)
		if err != nil {
			return
		}
	}
	agent.unsavedChanges = true
	return
}

func (agent *FeedbackAgent) APIHandleSetInterval(request *APIRequest) (
	err error) {
	res, err := agent.GetResponderByName(request.TargetName)
	if err != nil {
		return
	}
	if request.CommandInterval == nil {
		err = errors.New("invalid command interval specified (use 0 for disabled)")
		return
	}
	err = res.ConfigureInterval(*request.CommandInterval)
	if err != nil {
		return
	}
	agent.unsavedChanges = true
	return
}

// -------------------------------------------------------------------
// END OF FILE
// -------------------------------------------------------------------
