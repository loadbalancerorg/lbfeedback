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

// Handles an incoming JSON API request received by this [FeedbackAgent]
// via a [FeedbackResponder] service.
func (agent *FeedbackAgent) ReceiveAPIRequest(requestJSON string) (
	responseJSON string, err error, quitAfterResponding bool) {
	// Unmarshal into an empty request
	request, err := UnmarshalAPIRequest(requestJSON)
	// Get a response object for this request (with or without an error).
	response, quitAfterResponding := agent.ProcessAPIRequest(request, err)
	// Marshal the response object into the JSON response.
	output, err := json.MarshalIndent(response, "", "    ")
	if err == nil {
		responseJSON = string(output)
	} else {
		logrus.Error("Failed to marshal JSON API response.")
	}
	return
}

// Unmarshals a JSON request string into an [APIRequest].
func UnmarshalAPIRequest(requestJSON string) (request *APIRequest, err error) {
	// Attempt to unmarshal the request into the target object.
	request = &APIRequest{}
	err = json.Unmarshal([]byte(requestJSON), request)
	return
}

// Performs basic initial sanity checks of an API request.
func (agent *FeedbackAgent) ValidateAPIRequest(request *APIRequest) (errID string, errMsg string) {
	if request == nil {
		errID = "bad-json"
		errMsg = "could not read JSON"
	} else if (request.Service == "monitor" || request.Service == "responder") &&
		request.Name == "" {
		errID = "missing-target"
		errMsg = "no target service name specified"
	} else if request.APIKey == "" || request.APIKey != agent.APIKey {
		errID = "bad-api-key"
		errMsg = "invalid or missing API key"
	}
	return
}

// Processes an incoming API request and performs the required actions.
func (agent *FeedbackAgent) ProcessAPIRequest(request *APIRequest, parseErr error) (
	response *APIResponse, quitAfterResponding bool) {
	// -- Perform required initialisation and validation.
	// Build boilerplate for the API response.
	response = &APIResponse{
		APIName: APIName,
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
	response.Error, response.Message = agent.ValidateAPIRequest(request)
	if response.Error != "" {
		return
	}
	// -- The main API command tree.
	// This default error will be overriden by nil or another error
	// if a matching part of the tree is reached.
	unknownType := false
	suppressLog := false
	desc := BuildAPIDescription(request)
	var err error
	switch request.Action {
	// status: Returns the status of all services as a list.
	case "status":
		response.ServiceStatus = agent.GetServiceStatusArray()
		suppressLog = true
	// add: Creates a service within the Feedback Agent.
	case "add":
		switch request.Service {
		// monitor: Creates a monitor with the given parameters.
		case "monitor":
			err = agent.APIHandleAddMonitor(request)
		// responder: Creates a responder with the given parameters.
		case "responder":
			err = agent.APIHandleAddResponder(request)
		default:
			unknownType = true
		}
	// edit: Modifies a service within the Feedback Agent.
	case "edit":
		switch request.Service {
		case "monitor":
			err = agent.APIHandleModifyMonitor(request)
		case "responder":
			err = agent.APIHandleModifyResponder(request)
		default:
			unknownType = true
		}
	// delete: Deletes a service within the Feedback Agent.
	case "delete":
		switch request.Service {
		case "monitor":
			err = agent.APIHandleDeleteMonitor(request)
		case "responder":
			err = agent.APIHandleDeleteResponder(request)
		default:
			unknownType = true
		}
	case "start":
		switch request.Service {
		case "monitor":
			err = agent.APIHandleStartMonitor(request)
		case "responder":
			err = agent.APIHandleStartResponder(request)
		default:
			unknownType = true
		}
	case "stop":
		switch request.Service {
		case "monitor":
			err = agent.APIHandleStopMonitor(request)
		case "responder":
			err = agent.APIHandleStopResponder(request)
		case "agent":
			err = agent.StopAllServices()
			quitAfterResponding = true
		default:
			unknownType = true
		}
	case "restart":
		switch request.Service {
		case "monitor":
			err = agent.APIHandleRestartMonitor(request)
		case "responder":
			err = agent.APIHandleRestartResponder(request)
		case "agent":
			err = agent.RestartAllServices()
		default:
			unknownType = true
		}
	// save-config: Force saves the agent config to the config file.
	case "save-config":
		agent.unsavedChanges = true
	// get-config: Gets the entire configuration of the agent.
	case "get-config":
		response.AgentConfig = agent
		suppressLog = true
	case "get-feedback":
		response.Output, err = agent.APIHandleGetFeedback(request)
		suppressLog = true
	case "haproxy-enable":
		err = agent.APIHandleEnableCommands(request)
	case "haproxy-disable":
		err = agent.APIHandleDisableCommands(request)
	case "haproxy-up":
		err = agent.APIHandleManualUp(request)
	case "haproxy-down":
		err = agent.APIHandleManualDown(request)
	case "haproxy-clear":
		err = agent.APIHandleManualClear(request)
	case "haproxy-set-threshold":
		err = agent.APIHandleSetThreshold(request)
	default:
		err = errors.New("invalid action specified")
	}
	// Generate errors for an unknown service type.
	if unknownType {
		err = errors.New("invalid service '" + request.Service + "'")
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
		response.Message += "error: " + desc + ": " + err.Error()
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

// Builds an array of the service status.
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

// Appends an item to the service status array.
func AppendToStatusArray(array []APIServiceStatus, serviceType string,
	name string, state string) []APIServiceStatus {
	return append(array, APIServiceStatus{
		ServiceType:   serviceType,
		ServiceName:   name,
		ServiceStatus: state,
	})
}

// Converts a boolean running state to a descriptive string.
func ServiceRunningToString(running bool) string {
	if running {
		return "running"
	} else {
		return "stopped"
	}
}

// Outputs the current agent status as a descriptive string.
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

// Makes a log line description of an API request.
func BuildAPIDescription(request *APIRequest) (desc string) {
	desc = "(no action)"
	if request.Action != "" {
		desc = request.Action
	}
	if request.Service != "" {
		desc += " " + request.Service
	}
	if request.Name != "" {
		desc += " " + request.Name
	}
	return
}

// ----------------------------------------
// API action handlers
// ----------------------------------------

func (agent *FeedbackAgent) APIHandleAddMonitor(request *APIRequest) (err error) {
	metricType := ""
	if request.MetricType != nil {
		metricType = *request.MetricType
	} else {
		err = errors.New("system metric type not specified")
		return
	}
	interval := 0
	if request.Interval != nil {
		interval = *request.Interval
	} else {
		err = errors.New("interval not specified")
		return
	}
	params := MetricParams{}
	if request.Params != nil {
		params = *request.Params
	}
	// Try to add this as a new [SystemMonitor].
	err = agent.AddMonitor(
		request.Name,
		metricType,
		interval,
		params,
		nil,
	)
	if err != nil {
		return
	}
	// Attempt to start the new monitor.
	err = agent.StartMonitorByName(request.Name)
	// If this failed, remove the new monitor and concatenate the errors.
	if err != nil {
		deleteErr := agent.DeleteMonitorByName(request.Name)
		err = errors.Join(err, deleteErr)
		return
	}
	agent.unsavedChanges = true
	return
}

func (agent *FeedbackAgent) APIHandleAddResponder(request *APIRequest) (err error) {
	sourceMonitorName := ""
	if request.SourceMonitorName != nil {
		sourceMonitorName = *request.SourceMonitorName
	} else {
		err = errors.New("source monitor not specified")
		return
	}
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
	hapCommands := false
	if request.HAProxyCommands != nil {
		hapCommands = *request.HAProxyCommands
	}
	hapThreshold := 0
	if request.HAProxyThreshold != nil {
		hapThreshold = *request.HAProxyThreshold
	}
	// Try to add this as a new [FeedbackResponder]. The AddResponder() function will
	// look for and find the object for the [SystemMonitor] if it exists.
	err = agent.AddResponder(
		request.Name,
		sourceMonitorName,
		protocolName,
		ipAddress,
		listenPort,
		hapCommands,
		hapThreshold,
	)
	// If we couldn't add the responder (e.g. because the monitor doesn't exist),
	// fail out to an error.
	if err != nil {
		return
	}
	// Attempt to start the new responder.
	err = agent.StartResponderByName(request.Name)
	// If this failed, remove the new responder and concatenate the errors.
	if err != nil {
		deleteErr := agent.DeleteResponderByName(request.Name)
		err = errors.Join(err, deleteErr)
		return
	}
	agent.unsavedChanges = true
	return
}

func (agent *FeedbackAgent) APIHandleModifyMonitor(request *APIRequest) (err error) {
	// Fetch the monitor this request refers to (if any, otherwise error).
	oldMonitor, err := agent.GetMonitorByName(request.Name)
	if err != nil {
		return
	}
	changed := false
	// Copy the old monitor so that we can apply the changes to it.
	newMonitor := oldMonitor.Copy()
	if request.MetricType != nil {
		newMonitor.MetricType = *request.MetricType
		changed = true
	}
	if request.Interval != nil {
		newMonitor.Interval = *request.Interval
		changed = true
	}
	if request.Params != nil {
		newMonitor.Params = *request.Params
		changed = true
	}
	if !changed {
		err = errors.New("no fields changed in request")
		return
	}
	// Attempt to initialise the new monitor to validate it, else error.
	err = newMonitor.Initialise()
	if err != nil {
		return
	}
	// This is valid, so replace it in the list of monitors.
	agent.Monitors[request.Name] = &newMonitor
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
		if responder.SourceMonitorName == newMonitor.Name {
			responder.SwapMonitorWith(&newMonitor)
		}
	}
	agent.unsavedChanges = true
	return
}

func (agent *FeedbackAgent) APIHandleModifyResponder(request *APIRequest) (err error) {
	// Fetch the responder that this pertains to (otherwise, an error occurs).
	oldResponder, err := agent.GetResponderByName(request.Name)
	if err != nil {
		return
	}
	// Copy the old monitor so that we can apply the changes to it.
	newResponder := oldResponder.Copy()
	// Apply the new config changes for those JSON keys that are set.
	//config := request.ResponderConfig
	if request.SourceMonitorName != nil {
		if request.Name == "api" {
			err = errors.New("API responders cannot have a monitor")
			return
		}
		monName := *request.SourceMonitorName
		var mon *SystemMonitor
		mon, err = agent.GetMonitorByName(monName)
		if err != nil {
			// Monitor name does not exist; error
			return
		}
		newResponder.SourceMonitorName = monName
		newResponder.SourceMonitor = mon
	}
	// Process JSON pointer fields (to determine if they were set or not).
	if request.ProtocolName != nil {
		if request.Name == "api" {
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
	if request.HAProxyCommands != nil {
		newResponder.HAProxyCommands = *request.HAProxyCommands
	}
	if request.HAProxyThreshold != nil {
		newResponder.HAProxyThreshold = *request.HAProxyThreshold
	}
	// Attempt to initialise the new monitor to validate it, else error.
	err = newResponder.Initialise()
	if err != nil {
		return
	}
	// This is valid, so replace it in the list of monitors.
	agent.Responders[request.Name] = &newResponder
	// Preserve the current run state during the swap.
	wasRunning := oldResponder.IsRunning()
	if wasRunning {
		err = oldResponder.Stop()
		if err != nil {
			return
		}
		err = newResponder.Start()
	}
	agent.unsavedChanges = true
	return
}

func (agent *FeedbackAgent) APIHandleDeleteResponder(request *APIRequest) (err error) {
	if request.Name == "api" {
		err = errors.New("cannot delete the API Responder")
		return
	}
	err = agent.DeleteResponderByName(request.Name)
	return
}

func (agent *FeedbackAgent) APIHandleDeleteMonitor(request *APIRequest) (err error) {
	name := request.Name
	// Fetch the monitor that this pertains to (otherwise, an error occurs).
	mon, err := agent.GetMonitorByName(name)
	if err != nil {
		return
	}
	// Search for any responders attached to this monitor;
	// fail if any currently in use.
	for _, responder := range agent.Responders {
		if responder.SourceMonitorName == name {
			err = errors.New("cannot delete monitor '" + name +
				"': currently in use by responder '" +
				responder.ResponderName)
			return
		}
	}
	// Not currently in use; go ahead and delete it, stopping it first
	// if it's running.
	err = mon.Stop()
	if err != nil {
		return
	}
	err = agent.DeleteMonitorByName(name)
	return
}

func (agent *FeedbackAgent) APIHandleStartResponder(request *APIRequest) (err error) {
	res, err := agent.GetResponderByName(request.Name)
	if err == nil {
		err = res.Start()
	}
	return
}

func (agent *FeedbackAgent) APIHandleStartMonitor(request *APIRequest) (err error) {
	mon, err := agent.GetMonitorByName(request.Name)
	if err == nil {
		err = mon.Start()
	}
	return
}

func (agent *FeedbackAgent) APIHandleStopResponder(request *APIRequest) (err error) {
	res, err := agent.GetResponderByName(request.Name)
	if err == nil {
		err = res.Stop()
	}
	return
}

func (agent *FeedbackAgent) APIHandleStopMonitor(request *APIRequest) (err error) {
	mon, err := agent.GetMonitorByName(request.Name)
	if err == nil {
		err = mon.Stop()
	}
	return
}

func (agent *FeedbackAgent) APIHandleRestartResponder(request *APIRequest) (err error) {
	res, err := agent.GetResponderByName(request.Name)
	if err == nil {
		err = res.Restart()
	}
	return
}

func (agent *FeedbackAgent) APIHandleRestartMonitor(request *APIRequest) (err error) {
	mon, err := agent.GetMonitorByName(request.Name)
	if err == nil {
		err = mon.Restart()
	}
	return
}

func (agent *FeedbackAgent) APIHandleGetFeedback(request *APIRequest) (feedback string, err error) {
	res, err := agent.GetResponderByName(request.Name)
	if err == nil {
		feedback, _ = res.GetResponse("")
		feedback = strings.ReplaceAll(feedback, "\n", "")
	}
	return
}

func (agent *FeedbackAgent) APIHandleManualDown(request *APIRequest) (err error) {
	res, err := agent.GetResponderByName(request.Name)
	if err != nil {
		return
	}
	err = res.SetManualCommandDown()
	return
}

func (agent *FeedbackAgent) APIHandleManualUp(request *APIRequest) (err error) {
	res, err := agent.GetResponderByName(request.Name)
	if err != nil {
		return
	}
	err = res.SetManualCommandUp()
	return
}

func (agent *FeedbackAgent) APIHandleManualClear(request *APIRequest) (err error) {
	res, err := agent.GetResponderByName(request.Name)
	if err != nil {
		return
	}
	err = res.SetManualCommandClear()
	return
}

func (agent *FeedbackAgent) APIHandleEnableCommands(request *APIRequest) (err error) {
	res, err := agent.GetResponderByName(request.Name)
	if err != nil {
		return
	}
	err = res.SetManualCommands(true)
	agent.unsavedChanges = true
	return
}

func (agent *FeedbackAgent) APIHandleDisableCommands(request *APIRequest) (err error) {
	res, err := agent.GetResponderByName(request.Name)
	if err != nil {
		return
	}
	err = res.SetManualCommands(false)
	agent.unsavedChanges = true
	return
}

func (agent *FeedbackAgent) APIHandleSetThreshold(request *APIRequest) (err error) {
	res, err := agent.GetResponderByName(request.Name)
	if err != nil {
		return
	}
	if request.HAProxyThreshold == nil {
		err = errors.New("no threshold specified")
		return
	}
	res.SetThreshold(*request.HAProxyThreshold)
	agent.unsavedChanges = true
	return
}

// -------------------------------------------------------------------
// END OF FILE
// -------------------------------------------------------------------
