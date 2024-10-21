// responder.go
// Feedback Responder Service
//
// Project:		Loadbalancer.org Feedback Agent v5
// Author: 		Nicholas Turnbull
//				<nicholas.turnbull@loadbalancer.org>
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
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	HAProxyCommandUp   string = "up"
	HAProxyCommandDown string = "down"
)

// #######################################################################
// FeedbackResponder
// #######################################################################

// [FeedbackResponder] implements a Feedback Responder service which uses
// the specified [ProtocolConnector] to listen for and respond to clients
// from data obtained via the associated [SystemMonitor].
type FeedbackResponder struct {
	ResponderName      string            `json:"-"`
	SourceMonitorName  string            `json:"input-monitor,omitempty"`
	ProtocolName       string            `json:"protocol,omitempty"`
	ListenIPAddress    string            `json:"ip,omitempty"`
	ListenPort         string            `json:"port,omitempty"`
	HAProxyCommands    bool              `json:"haproxy-commands,omitempty"`
	HAProxyThreshold   int               `json:"haproxy-threshold,omitempty"`
	ManualHAProxyState string            `json:"-"`
	SourceMonitor      *SystemMonitor    `json:"-"`
	Connector          ProtocolConnector `json:"-"`
	RequestTimeout     time.Duration     `json:"request-timeout,omitempty"`
	ResponseTimeout    time.Duration     `json:"response-timeout,omitempty"`
	LastError          error             `json:"-"`
	ParentAgent        *FeedbackAgent    `json:"-"`
	runState           bool              `json:"-"`
	mutex              *sync.Mutex       `json:"-"`
	statusChannel      chan int          `json:"-"`
}

// Constructor for [FeedbackResponder], which must be used when creating
// a new responder object.
func NewResponder(name string, monitor *SystemMonitor, protocol string,
	ip string, port string, hapCommands bool, hapThreshold int, agent *FeedbackAgent) (result *FeedbackResponder,
	err error) {
	// -- Create a new responder containing the base settings.
	fbr := &FeedbackResponder{
		ProtocolName:     protocol,
		ListenIPAddress:  ip,
		ListenPort:       port,
		SourceMonitor:    monitor,
		ResponderName:    name,
		ParentAgent:      agent,
		HAProxyCommands:  hapCommands,
		HAProxyThreshold: hapThreshold,
	}
	if monitor == nil && protocol != ProtocolAPI {
		err = errors.New("cannot create responder: no monitor provided")
		return
	}
	if monitor != nil {
		fbr.SourceMonitorName = monitor.Name
	}
	err = fbr.Initialise()
	if err == nil {
		result = fbr
	}
	return
}

func (fbr *FeedbackResponder) Initialise() (err error) {
	if fbr.mutex == nil {
		fbr.mutex = &sync.Mutex{}
	}
	fbr.mutex.Lock()
	defer fbr.mutex.Unlock()
	// -- Process/validate parameters.
	// Network protocol for the responder (defined in the Connector).
	fbr.Connector, err = NewFeedbackConnector(fbr.ProtocolName)
	if err != nil {
		return
	}
	fbr.ListenIPAddress, err = ParseIPAddress(fbr.ListenIPAddress)
	if err != nil {
		return
	}
	fbr.ListenPort, err = ParseNetworkPort(fbr.ListenPort)
	if err != nil {
		return
	}
	return
}

func (fbr *FeedbackResponder) Copy() (copy FeedbackResponder) {
	fbr.mutex.Lock()
	defer fbr.mutex.Unlock()
	copy = *fbr
	copy.mutex = &sync.Mutex{}
	copy.runState = false
	return
}

func (fbr *FeedbackResponder) SwapMonitorWith(mon *SystemMonitor) {
	fbr.mutex.Lock()
	defer fbr.mutex.Unlock()
	fbr.SourceMonitorName = mon.Name
	fbr.SourceMonitor = mon
}

func ParseNetworkPort(port string) (result string, err error) {
	// Validate and sanitise port
	var parsedPort int
	parsedPort, err = strconv.Atoi(strings.TrimSpace(port))
	if err != nil || parsedPort < 1 || parsedPort > 65535 {
		err = errors.New("invalid port '" + port + "'")
		return
	}
	result = strconv.Itoa(parsedPort)
	return
}

func ParseIPAddress(ip string) (result string, err error) {
	// Validate and sanitise IP address
	ip = strings.TrimSpace(ip)
	// Handle a wildcard IP address specification
	if ip == "*" {
		result = "*"
		return
	}
	// Otherwise, try to parse it
	parsedIP := net.ParseIP(strings.TrimSpace(fmt.Sprint(ip)))
	if parsedIP == nil {
		err = errors.New("invalid IP address '" + ip +
			"' specified; use 'any' (CLI) or '*' (API) to listen on all IPs")
		return
	}
	result = parsedIP.String()
	return
}

// Starts the Feedback Agent service, returning an error in the event
// of failure, by launching the main code of the service as a goroutine.
func (fbr *FeedbackResponder) Start() (err error) {
	initChannel := make(chan int)
	go fbr.run(initChannel)
	result := <-initChannel
	fbr.mutex.Lock()
	defer fbr.mutex.Unlock()
	logLine := fbr.getLogHead()
	if result == ServiceStateRunning && fbr.LastError == nil {
		logLine += "has started (" + strings.ToUpper(fbr.ProtocolName) +
			" on " + fbr.ListenIPAddress + ":" + fbr.ListenPort + ")."
		logrus.Info(logLine)
	} else {
		logLine += "failed to start, error: " + fbr.LastError.Error()
	}
	err = fbr.LastError
	return
}

func (fbr *FeedbackResponder) Restart() (err error) {
	if fbr.IsRunning() {
		err = fbr.Stop()
		if err != nil {
			return
		}
	}
	err = fbr.Start()
	return
}

// Stops the service from running.
func (fbr *FeedbackResponder) Stop() (err error) {
	if fbr.IsRunning() {
		fbr.mutex.Lock()
		err = fbr.Connector.Close()
		fbr.mutex.Unlock()
		// Check for a successful stopped reply
		// $$ TO DO: Implement sleep/timeout using select
		for <-fbr.statusChannel != ServiceStateStopped {
			// Wait on channel
		}
	} else {
		err = errors.New("responder is not running")
	}
	return
}

func (fbr *FeedbackResponder) IsRunning() (state bool) {
	fbr.mutex.Lock()
	defer fbr.mutex.Unlock()
	state = fbr.runState
	return
}

// The run function to call when the service starts; e.g.
// the worker thread.
func (fbr *FeedbackResponder) run(initChannel chan int) {
	// Start by obtaining the mutex lock before doing anything else.
	fbr.mutex.Lock()
	if initChannel == nil {
		fbr.LastError = errors.New("failed; missing channel")
	}
	// Deferred actions to always perform when this worker goroutine terminates.
	defer func() {
		// Handle if we exited due to a panic.
		if recover() != nil {
			fbr.LastError = errors.New("fatal error")
		}
		// Release the mutex and signal that we've stopped.
		fbr.mutex.Unlock()
		initChannel <- ServiceStateStopped
	}()
	if fbr.runState {
		fbr.LastError = errors.New("already running")
		return
	}
	// -- Prepare to go into a running state.
	fbr.LastError = nil
	fbr.statusChannel = initChannel
	fbr.runState = true
	fbr.mutex.Unlock()
	// -- We are now running.
	// Announce that we are now running to whatever called us.
	fbr.statusChannel <- ServiceStateRunning
	// Call the Listen() method of the protocol connector, which
	// will block here until it quits.
	fbr.LastError = fbr.Connector.Listen(fbr)
	// -- Go to a non-running state.
	fbr.mutex.Lock()
	fbr.runState = false
	logrus.Info(fbr.getLogHead() + "has stopped.")
}

func (fbr *FeedbackResponder) getLogHead() string {
	return "Responder '" + fbr.ResponderName + "' "
}

func (fbr *FeedbackResponder) SetManualCommandDown() (err error) {
	fbr.mutex.Lock()
	defer fbr.mutex.Unlock()
	err = fbr.ValidateManualCommand()
	if err != nil {
		return
	}
	fbr.ManualHAProxyState = HAProxyCommandDown
	return
}

func (fbr *FeedbackResponder) SetManualCommandUp() (err error) {
	fbr.mutex.Lock()
	defer fbr.mutex.Unlock()
	err = fbr.ValidateManualCommand()
	if err != nil {
		return
	}
	fbr.ManualHAProxyState = HAProxyCommandUp
	return
}

func (fbr *FeedbackResponder) SetManualCommandClear() (err error) {
	fbr.mutex.Lock()
	defer fbr.mutex.Unlock()
	err = fbr.ValidateManualCommand()
	if err != nil {
		return
	}
	fbr.ManualHAProxyState = ""
	return
}

func (fbr *FeedbackResponder) SetManualCommands(state bool) (err error) {
	fbr.mutex.Lock()
	defer fbr.mutex.Unlock()
	fbr.HAProxyCommands = state
	return
}

func (fbr *FeedbackResponder) SetThreshold(threshold int) {
	fbr.mutex.Lock()
	defer fbr.mutex.Unlock()
	fbr.HAProxyThreshold = threshold
}

func (fbr *FeedbackResponder) ValidateManualCommand() (err error) {
	if !fbr.HAProxyCommands {
		err = errors.New("HAProxy commands are not enabled for this responder")
	}
	return
}

func (fbr *FeedbackResponder) GenerateFeedbackString() (feedback string) {
	score := int(fbr.SourceMonitor.StatsModel.GetAvailabilityScore())
	feedback = strconv.Itoa(score) + "%"
	if fbr.HAProxyCommands {
		// If a manual HAProxy command state has been specified, this
		// will always be sent and overrides the threshold.
		command := strings.TrimSpace(fbr.ManualHAProxyState)
		if command == "" {
			// Otherwise, send the appropriate command based on the score.
			if score >= fbr.HAProxyThreshold {
				command = HAProxyCommandUp
			} else {
				command = HAProxyCommandDown
			}
		}
		feedback += " " + command
	}
	feedback += "\n"
	return
}

func (fbr *FeedbackResponder) GetResponse(request string) (response string, quitAfter bool) {
	defer func() {
		err := recover()
		if err != nil {
			logrus.Error("A fatal error occurred during a response.")
		}
	}()
	if fbr.ProtocolName == ProtocolAPI {
		response, _, quitAfter = fbr.ParentAgent.ReceiveAPIRequest(request)
	} else {
		response = fbr.GenerateFeedbackString()
	}
	return
}

// -------------------------------------------------------------------
// END OF FILE
// -------------------------------------------------------------------
