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
	"math"
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
// from data obtained via the associated [SystemMonitor] objects.
type FeedbackResponder struct {
	// JSON configuration fields for [FeedbackResponder].
	ProtocolName     string                     `json:"protocol"`
	ListenIPAddress  string                     `json:"ip"`
	ListenPort       string                     `json:"port"`
	FeedbackSources  map[string]*FeedbackSource `json:"feedback-sources,omitempty"`
	RequestTimeout   time.Duration              `json:"request-timeout,omitempty"`
	ResponseTimeout  time.Duration              `json:"response-timeout,omitempty"`
	HAProxyCommands  string                     `json:"haproxy-commands,omitempty"`
	CommandInterval  int                        `json:"command-interval,omitempty"`
	ThresholdEnabled bool                       `json:"threshold-enabled,omitempty"`
	ThresholdScore   int                        `json:"threshold-score,omitempty"`

	// Internal configuration fields.
	ResponderName string            `json:"-"`
	Connector     ProtocolConnector `json:"-"`
	LastError     error             `json:"-"`
	ParentAgent   *FeedbackAgent    `json:"-"`
	runState      bool              `json:"-"`
	mutex         *sync.Mutex       `json:"-"`
	statusChannel chan int          `json:"-"`

	// Lookup tables for command enums and strings.
	commandToEnum    map[string]int `json:"-"`
	enumToCommand    map[int]string `json:"-"`
	commandEnumOrder []int          `json:"-"`

	// The last command state (online or offline) seen.
	lastCommandState bool `json:"-"`

	// Current HAProxy commands that are enabled for this responder.
	commandMask int `json:"-"`

	// If DisableCommandInterval is false, the timestamp when the current
	// state expires (and is therefore no longer sent in responses).
	stateExpiry time.Time `json:"-"`

	// Force the command to be sent for an entire interval, or
	// allow it to be interrupted if the feedback score falls
	// within the "up" threshold range?
	forceCommandState bool `json:"-"`
}

// Defines a source mapping for a [FeedbackResponder] to a
// [SystemMonitor] with a specified significance and maximum value.
type FeedbackSource struct {
	Significance         float64        `json:"significance"`
	MaxValue             int64          `json:"max-value"`
	Monitor              *SystemMonitor `json:"-"`
	RelativeSignificance float64        `json:"-"`
}

const (
	// Flagged enums for sending composite feedback commands to HAProxy.
	// These are designed with two properties; one, that the commands are
	// configured as a single list (with the Responder knowing whether
	// they pertain to an online or offline state) and two, that they are
	// sent in a specific syntax order in the HAProxy response to ensure
	// that they are actioned in the right order of precedence. This is
	// done by the order of the enums and with a positive flag being
	// included for both online and offline states.
	HAPEnumNone    = 0x000
	HAPMaskState   = 0xF00
	HAPMaskCommand = 0x0FF
	HAPMaskAll     = 0xFFF
	HAPOnlineFlag  = 0x100
	HAPOfflineFlag = 0x200
	HAPEnumUp      = 0x101
	HAPEnumReady   = 0x102
	HAPEnumDown    = 0x204
	HAPEnumDrain   = 0x208
	HAPEnumFail    = 0x210
	HAPEnumMaint   = 0x220
	HAPEnumStopped = 0x240

	// As per the previous Loadbalancer.org Feedback Agent, the
	// default online command is "up ready" and the default
	// command to offline is "drain" (as requested by MT).
	HAPEnumDefaultOnline  = HAPEnumUp | HAPEnumReady
	HAPEnumDefaultOffline = HAPEnumDrain

	// Strings for sending composite feedback commands to HAProxy.
	HAPCommandNone    = ""
	HAPCommandUp      = "up"
	HAPCommandReady   = "ready"
	HAPCommandDown    = "down"
	HAPCommandDrain   = "drain"
	HAPCommandFail    = "fail"
	HAPCommandMaint   = "maint"
	HAPCommandStopped = "stopped"

	// JSON configuration settings for group options (default, none).
	HAPConfigDefault = "default"
	HAPConfigNone    = "none"

	// Default interval for which to send HAProxy commands after a
	// state change. This has been defined as 10 seconds as per MT,
	// but may likely need to be increased by modifying the
	// configuration for many use cases. This is presumably intended
	// to be the most conservative value.
	DefaultCommandInterval = 10
)

// Constructor for [FeedbackResponder], which must be used when creating
// a new responder object.
func NewResponder(name string, sources map[string]*FeedbackSource,
	protocol string, ip string, port string, commands string,
	enableThreshold bool, threshold int, agent *FeedbackAgent) (
	result *FeedbackResponder, err error) {
	if sources == nil {
		sources = make(map[string]*FeedbackSource)
	}
	// -- Create a new responder containing the base settings.
	fbr := &FeedbackResponder{
		ProtocolName:     protocol,
		ListenIPAddress:  ip,
		ListenPort:       port,
		FeedbackSources:  sources,
		ResponderName:    name,
		ParentAgent:      agent,
		HAProxyCommands:  commands,
		ThresholdEnabled: enableThreshold,
		ThresholdScore:   threshold,
		CommandInterval:  DefaultCommandInterval,
	}
	fbr.mutex = &sync.Mutex{}
	err = fbr.Initialise()
	if err == nil {
		result = fbr
	}
	return
}

// Create maps and order array for the HAProxy command state
// management functions. These cannot be set as constants in
// Go, unfortunately.
func (fbr *FeedbackResponder) SetHAPCommandMaps() {
	fbr.commandToEnum = map[string]int{
		HAPCommandNone:    HAPEnumNone,
		HAPCommandUp:      HAPEnumUp,
		HAPCommandReady:   HAPEnumReady,
		HAPCommandDown:    HAPEnumDown,
		HAPCommandDrain:   HAPEnumDrain,
		HAPCommandFail:    HAPEnumFail,
		HAPCommandMaint:   HAPEnumMaint,
		HAPCommandStopped: HAPEnumStopped,
	}
	fbr.enumToCommand = map[int]string{
		HAPEnumNone:    HAPCommandNone,
		HAPEnumUp:      HAPCommandUp,
		HAPEnumReady:   HAPCommandReady,
		HAPEnumDown:    HAPCommandDown,
		HAPEnumDrain:   HAPCommandDrain,
		HAPEnumFail:    HAPCommandFail,
		HAPEnumMaint:   HAPCommandMaint,
		HAPEnumStopped: HAPCommandStopped,
	}
	fbr.commandEnumOrder = []int{
		HAPEnumNone,
		HAPEnumUp,
		HAPEnumReady,
		HAPEnumDown,
		HAPEnumDrain,
		HAPEnumFail,
		HAPEnumMaint,
		HAPEnumStopped,
	}
}

// Initialises this [FeedbackResponder] and configures defaults.
func (fbr *FeedbackResponder) Initialise() (err error) {
	// Initialise the mutex, if required.
	if fbr.mutex == nil {
		fbr.mutex = &sync.Mutex{}
	}
	fbr.mutex.Lock()
	defer fbr.mutex.Unlock()
	fbr.SetHAPCommandMaps()
	if fbr.FeedbackSources == nil {
		fbr.FeedbackSources = make(map[string]*FeedbackSource)
	}
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
	// Skip source/command initialisation if this is an API responder.
	if fbr.ProtocolName == ProtocolAPI || len(fbr.FeedbackSources) < 1 {
		return
	}
	commands := fbr.HAProxyCommands
	interval := fbr.CommandInterval
	// This requires unlocking the mutex, and then relocking due to our defer.
	fbr.mutex.Unlock()
	err = fbr.initialiseSources()
	if err != nil {
		return
	}
	fbr.ConfigureCommands(commands, true, false)
	if err != nil {
		return
	}
	fbr.ConfigureInterval(interval)
	if err != nil {
		return
	}
	fbr.mutex.Lock()
	return
}

func (fbr *FeedbackResponder) ConfigureCommands(commands string, replace bool,
	unset bool) (err error) {
	// Configure the HAProxy commands for this responder.
	commands = strings.TrimSpace(commands)
	if commands == "" {
		err = errors.New(fbr.getLogHead() + ": no HAProxy command " +
			"configuration specified; perhaps you meant 'none' or " +
			"'default'?")
		return
	}
	err = fbr.setHAPCommandMask(commands, replace, unset)
	return
}

func (fbr *FeedbackResponder) ConfigureInterval(interval int) (err error) {
	if interval < 1 {
		err = errors.New(fbr.getLogHead() + ": invalid HAProxy command " +
			"interval; use '0' to always send (interval disabled).")
	}
	fbr.setInterval(interval)
	return
}

func (fbr *FeedbackResponder) initialiseSources() (err error) {
	fbr.mutex.Lock()
	defer fbr.mutex.Unlock()
	// Initialise monitors specified for this responder.
	totalSignificance := 0.0
	for key, source := range fbr.FeedbackSources {
		monitor, exists := fbr.ParentAgent.Monitors[key]
		if !exists {
			err = errors.New("cannot initialise responder: monitor '" +
				key + "' not found")
			return
		}
		// Apply defaults for the significance and max value if
		// they are out of range for the required values.
		if source.Significance < 0.0 || source.Significance > 1.0 {
			err = errors.New("'" + key + "': significance out of range: " +
				"must be between 0.0-1.0")
			return
		}
		if source.MaxValue < 0 {
			err = errors.New("'" + key + "': max value out of range:" +
				"cannot be negative")
			return
		}
		source.Monitor = monitor
		// Add this significance to the total so that we can calculate
		// the fraction that each monitor represents of the total significance
		// set across all metrics.
		totalSignificance += source.Significance
		// Log details of this source so the user can see what's configured
		// when the agent is configured.
	}
	logrus.Info(fbr.getLogHead() + ": calculating relative significances, " +
		"total " + fmt.Sprintf("%.2f", totalSignificance) + ".")

	// Set the scaled significance for each source monitor, i.e. the fraction
	// of the total significance values specified that each monitor represents.
	for key, source := range fbr.FeedbackSources {
		source.RelativeSignificance = source.Significance / totalSignificance
		logrus.Info("Responder '" + fbr.ResponderName +
			": name '" + key +
			"', type '" + source.Monitor.MetricType +
			"': " + fmt.Sprintf("%.2f", source.Significance) +
			" -> relative " + fmt.Sprintf("%.2f",
			source.RelativeSignificance) + ".")
	}
	return
}

func (fbr *FeedbackResponder) AddFeedbackSource(name string,
	significance *float64, maxValue *int64) (err error) {
	fbr.mutex.Lock()
	defer fbr.mutex.Unlock()
	name = strings.TrimSpace(name)
	if name == "" {
		err = errors.New(fbr.getLogHead() +
			": empty monitor name")
		return
	}
	mon, exists := fbr.ParentAgent.Monitors[name]
	if !exists {
		err = errors.New(fbr.getLogHead() +
			": monitor '" + name + "' does not exist")
		return
	}
	_, exists = fbr.FeedbackSources[name]
	if exists {
		err = errors.New(fbr.getLogHead() +
			": source monitor '" + name +
			"' is already attached to this Responder")
		return
	}
	if significance == nil {
		*significance = 1.0
	}
	var metricMax int64
	if maxValue != nil {
		metricMax = *maxValue
	} else {
		metricMax = int64(mon.SysMetric.GetDefaultMax())

	}
	newSource := FeedbackSource{
		Monitor:      mon,
		Significance: *significance,
		MaxValue:     metricMax,
	}
	fbr.FeedbackSources[name] = &newSource
	fbr.mutex.Unlock()
	err = fbr.initialiseSources()
	fbr.mutex.Lock()
	return
}

func (fbr *FeedbackResponder) EditFeedbackSource(name string,
	significance *float64, maxValue *int64) (err error) {
	fbr.mutex.Lock()
	defer fbr.mutex.Unlock()
	source, exists := fbr.FeedbackSources[name]
	if !exists {
		err = errors.New(fbr.getLogHead() +
			": monitor '" + name +
			"' does not exist as a source for this responder")
		return
	}
	if significance != nil {
		if *significance < 0.0 || *significance > 1.0 {
			err = errors.New(fbr.getLogHead() +
				": significance out of range")
			return
		}
		source.Significance = *significance
	}
	if maxValue != nil {
		if *maxValue < 0 {
			err = errors.New(fbr.getLogHead() +
				": max value out of range")
			return
		}
		source.MaxValue = *maxValue
	}
	fbr.FeedbackSources[name] = source
	fbr.mutex.Unlock()
	err = fbr.initialiseSources()
	fbr.mutex.Lock()
	return
}

func (fbr *FeedbackResponder) DeleteFeedbackSource(name string) (
	err error) {
	fbr.mutex.Lock()
	defer fbr.mutex.Unlock()
	_, exists := fbr.FeedbackSources[name]
	if !exists {
		err = errors.New(fbr.getLogHead() +
			": monitor '" + name +
			"' does not exist as a source for this responder")
		return
	}
	delete(fbr.FeedbackSources, name)
	fbr.mutex.Unlock()
	err = fbr.initialiseSources()
	fbr.mutex.Lock()
	return
}

// Configures the HAProxy command parameters for this [FeedbackResponder],
// based on whether this should replace any commands currently set and if
// the list of commands provided should be removed rather than added.
func (fbr *FeedbackResponder) setHAPCommandMask(commands string,
	replace bool, unset bool) (err error) {
	fbr.mutex.Lock()
	defer fbr.mutex.Unlock()
	trimmed := RemoveExtraSpaces(commands)
	// If no commands are specified, set the default configuration
	changeMask := 0
	switch trimmed {
	// Error if no command string was supplied
	case "":
		err = errors.New("no commands specified")
	// Set the default mask parameters if requested via the 'default' option
	case HAPConfigNone:
		replace = true
		changeMask = HAPEnumNone
	case HAPConfigDefault:
		changeMask = HAPEnumDefaultOnline | HAPEnumDefaultOffline
	default:
		// Look up each command and translate into a mask (if valid).
		split := strings.Split(trimmed, " ")
		for _, command := range split {
			enum, exists := fbr.commandToEnum[command]
			if !exists {
				err = errors.New("invalid HAProxy feedback command: '" +
					command + "'")
				return
			}
			// Include this mask by ORing it into the current change mask
			changeMask |= enum
		}
	}
	// Mask off the enum flags (as we don't want these in the field)
	changeMask &= HAPMaskCommand
	// If setting these commands, OR the change mask into into the current
	// command mask, otherwise AND NOT to unset them.
	if replace {
		fbr.commandMask = changeMask
	} else if !unset {
		fbr.commandMask |= changeMask
	} else {
		fbr.commandMask &= ^changeMask
	}
	// Convert the resulting command mask back to a string so that the
	// JSON configuration reflects this new state.
	if commands == HAPConfigNone || commands == HAPConfigDefault {
		fbr.HAProxyCommands = commands
	} else {
		fbr.HAProxyCommands = fbr.CommandMaskToString(fbr.commandMask,
			HAPMaskCommand, HAPMaskAll)
	}
	fbr.resetStateExpiry()
	return
}

// Copies this [FeedbackResponder] into a new object.
func (fbr *FeedbackResponder) Copy() (copy FeedbackResponder) {
	fbr.mutex.Lock()
	defer fbr.mutex.Unlock()
	copy = *fbr
	copy.mutex = &sync.Mutex{}
	copy.runState = false
	return
}

// Parses a network port into a sanitised version, returning
// an error if it cannot be parsed.
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

// Starts the [FeedbackResponder] service, returning an error in the event
// of failure, by launching the main code of the service as a goroutine.
func (fbr *FeedbackResponder) Start() (err error) {
	fbr.mutex.Lock()
	defer fbr.mutex.Unlock()
	logLine := fbr.getLogHead()
	if len(fbr.FeedbackSources) < 1 && fbr.ProtocolName != ProtocolAPI {
		logrus.Warn("Warning: " + logLine +
			"currently has no monitor sources configured.")
	}
	initChannel := make(chan int)
	go fbr.run(initChannel)
	fbr.mutex.Unlock()
	result := <-initChannel
	fbr.mutex.Lock()
	if result == ServiceStateRunning && fbr.LastError == nil {
		logLine += "has started (" + strings.ToUpper(fbr.ProtocolName) +
			" on " + fbr.ListenIPAddress + ":" + fbr.ListenPort + ")."
		logrus.Info(logLine)
	} else {
		logLine += "failed to start, error: " + fbr.LastError.Error()
		logrus.Error(logLine)
	}
	err = fbr.LastError
	return
}

// Attempts to restart the [FeedbackResponder] service.
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
			// $$ TO DO: Avoid blocking in the event of malfunction.
		}
	} else {
		err = errors.New("responder is not running")
	}
	return
}

// Returns whether this [FeedbackResponder] is running or not.
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
	// Deferred actions to always perform when this worker
	// goroutine terminates.
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
	// Send current command for the interval (if enabled)
	// when the responder is first launched.
	fbr.resetStateExpiry()
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

// Utility function for the start of log entries for this [FeedbackResponder].
func (fbr *FeedbackResponder) getLogHead() string {
	return "Responder '" + fbr.ResponderName + "' "
}

// Sets the current HAProxy command state, resetting the state expiry.
func (fbr *FeedbackResponder) SetHAPCommandState(isOnline bool, force bool) {
	fbr.mutex.Lock()
	defer fbr.mutex.Unlock()
	fbr.lastCommandState = isOnline
	fbr.forceCommandState = force
	fbr.resetStateExpiry()
}

// Resets the current HAProxy command state expiry.
func (fbr *FeedbackResponder) resetStateExpiry() {
	fbr.stateExpiry = time.Now().Add(time.Second *
		time.Duration(fbr.CommandInterval))
}

// Sets the command threshold for this [FeedbackResponder].
func (fbr *FeedbackResponder) ConfigureThresholdValue(threshold int) (err error) {
	if threshold < 0 {
		err = errors.New(fbr.getLogHead() + "invalid threshold; cannot be negative")
		return
	}
	fbr.setThreshold(threshold)
	return
}

func (fbr *FeedbackResponder) ConfigureThresholdEnabled(enabled bool) {
	fbr.mutex.Lock()
	defer fbr.mutex.Unlock()
	fbr.ThresholdEnabled = enabled
}

func (fbr *FeedbackResponder) setThreshold(threshold int) {
	fbr.mutex.Lock()
	defer fbr.mutex.Unlock()
	fbr.ThresholdScore = threshold
}

func (fbr *FeedbackResponder) setInterval(interval int) {
	fbr.mutex.Lock()
	defer fbr.mutex.Unlock()
	if interval < 1 {
		fbr.CommandInterval = 0
	} else {
		fbr.CommandInterval = interval
	}
}

// A corrected version of the algorithm mentioned on the Loadbalancer.org
// blog for the older Feedback Agent, which calculates an availability
// score against a maximum value specified for a given metric, adjusted
// by a relative significance score (scaled proportion of the total
// significance for all monitors attached to this responder).
func (fbr *FeedbackResponder) AvailabilityScore() (score int) {
	// Calculate the overall score across all monitors by scaling
	// against their maximum value, and then their relative significance.
	// Formula:
	//       s = 100 - ((v_cur / v_max) * sig_rel * 100)
	// where:
	//       s = total availability score for this monitor
	//       v_cur = current raw value returned by the stats model
	//       v_max = maximum specified ceiling for the source
	//       sig_rel = fraction of all significances set for this monitor
	//
	scoreTotal := 0.0
	for _, source := range fbr.FeedbackSources {
		// Skip any monitors with no significance.
		if source.RelativeSignificance <= 0.0 {
			continue
		}
		// Grab the current raw value from the stats model.
		rawValue := source.Monitor.StatsModel.GetResult()
		// Clamp the raw value at the maximum value set.
		if rawValue > source.MaxValue {
			rawValue = source.MaxValue
		}
		scaledValue := (float64(rawValue) /
			float64(source.MaxValue)) *
			source.RelativeSignificance
		scoreTotal += scaledValue
	}
	score = 100 - int(math.Round(scoreTotal*100))
	// Constrain score within boundaries.
	if score > 100 {
		score = 100
	} else if score < 0 {
		score = 0
	}
	return
}

// Handler for generating a feedback string for this [FeedbackResponder].
// It also changes the current online state as of the last query so that
// a command is sent for a specified period of time from the first request.
func (fbr *FeedbackResponder) HandleFeedback() (feedback string) {
	callTime := time.Now()
	fbr.mutex.Lock()
	defer fbr.mutex.Unlock()
	score := fbr.AvailabilityScore()
	feedback = strconv.Itoa(score) + "%"
	thresholdState := score >= fbr.ThresholdScore
	// Change state if either the current state has expired, or if
	// this was not a "forced" state (i.e. set manually via the API/CLI)
	// and therefore can be overriden by the value coming back within
	// the acceptable threshold range.
	if (thresholdState != fbr.lastCommandState) &&
		!(fbr.forceCommandState && callTime.Before(fbr.stateExpiry)) {
		fbr.mutex.Unlock()
		fbr.SetHAPCommandState(thresholdState, false)
		fbr.mutex.Lock()
	}
	// Append the command if 1) the command has not yet timed out or
	// the timeout interval is disabled; and 2) either threshold is
	// enabled, or this is a forced command.
	if (callTime.Before(fbr.stateExpiry) || (fbr.CommandInterval < 1)) &&
		(fbr.ThresholdEnabled || fbr.forceCommandState) {
		feedback = fbr.GenerateCommandString(fbr.lastCommandState) +
			" " + feedback
	}
	// The HAProxy specs call for a final newline to be sent.
	feedback += "\n"
	return
}

// Gets a string response from this [FeedbackResponder], which will depend
// on its configuration and what it is supposed to do.
func (fbr *FeedbackResponder) GetResponse(request string) (response string,
	quitAfter bool) {
	if !PanicDebug {
		defer func() {
			err := recover()
			if err != nil {
				logrus.Error("An internal error occurred during a " +
					"response:\n   " + fmt.Sprint(err))
			}
		}()
	}
	if fbr.ProtocolName == ProtocolAPI {
		response, _, quitAfter = fbr.ParentAgent.ReceiveAPIRequest(request)
	} else {
		response = fbr.HandleFeedback()
	}
	return
}

// Generates an HAProxy command string based on the current
// command mask and a specified online state.
func (fbr *FeedbackResponder) GenerateCommandString(online bool) (
	commands string) {
	state := HAPOfflineFlag
	if online {
		state = HAPOnlineFlag
	}
	commands = fbr.CommandMaskToString(fbr.commandMask, HAPMaskCommand,
		state)
	return
}

// Converts an HAProxy command mask to a string, ignoring any command
// enums that don't have any bits matching the filter.
func (fbr *FeedbackResponder) CommandMaskToString(commandMask int, enumMask int,
	filter int) (commands string) {
	for _, enum := range fbr.commandEnumOrder {
		// Add this command if this enum is for the current state
		// and currently enabled within the configured command mask.
		if (enum&filter > 0) && ((enum & commandMask) == (enum & enumMask)) {
			if commands != "" {
				commands += " "
			}
			commands += fbr.enumToCommand[enum]
		}
	}
	return
}

// -------------------------------------------------------------------
// END OF FILE
// -------------------------------------------------------------------
