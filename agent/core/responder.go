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

// #######################################################################
// FeedbackResponder
// #######################################################################

// FeedbackResponder implements a Feedback Responder service which uses
// the specified ProtocolConnector to listen for and respond to clients
// from data obtained via the associated SystemMonitor objects.
type FeedbackResponder struct {
	// -- JSON configuration fields for the FeedbackResponder object.
	ProtocolName          string                     `json:"protocol"`
	ListenIPAddress       string                     `json:"ip"`
	ListenPort            string                     `json:"port"`
	FeedbackSources       map[string]*FeedbackSource `json:"feedback-sources,omitempty"`
	RequestTimeout        time.Duration              `json:"request-timeout,omitempty"`
	ResponseTimeout       time.Duration              `json:"response-timeout,omitempty"`
	HAProxyCommands       string                     `json:"haproxy-commands,omitempty"`
	CommandInterval       int                        `json:"command-interval,omitempty"`
	ThresholdScore        int                        `json:"global-threshold,omitempty"`
	ThresholdModeName     string                     `json:"threshold-mode,omitempty"`
	EnableOfflineInterval bool                       `json:"enable-offline-interval,omitempty"`
	LogStateChanges       bool                       `json:"log-state-changes,omitempty"`

	// -- Exported configuration fields.
	ResponderName string            `json:"-"`
	Connector     ProtocolConnector `json:"-"`
	LastError     error             `json:"-"`
	ParentAgent   *FeedbackAgent    `json:"-"`

	// -- Internal configuration fields.
	runState      bool
	mutex         *sync.Mutex
	statusChannel chan int

	// The last command state (online or offline) seen.
	onlineState bool

	// Current HAProxy commands that are enabled for this responder.
	configCommandMask int
	overrideMask      int

	// If DisableCommandInterval is false, the timestamp when the current
	// state expires (and is therefore no longer sent in responses).
	stateExpiry time.Time

	// Force the command to be sent for an entire interval, or
	// allow it to be interrupted if the feedback score falls
	// within the "up" threshold range?
	forceCommandState bool

	// Currently configured threshold mode (from string).
	thresholdModeEnum ThresholdMode
}

// -- Constants for threshold functionality.

type ThresholdMode int

const (
	ThresholdModeAny ThresholdMode = iota
	ThresholdModeNone
	ThresholdModeOverallOnly
	ThresholdModeMetricOnly
)
const (
	ThresholdStringAny         = "any"
	ThresholdStringNone        = "none"
	ThresholdStringOverallOnly = "overall"
	ThresholdStringMetricOnly  = "metric"
)

var thresholdStringToMode = map[string]ThresholdMode{
	ThresholdStringAny:         ThresholdModeAny,
	ThresholdStringNone:        ThresholdModeNone,
	ThresholdStringOverallOnly: ThresholdModeOverallOnly,
	ThresholdStringMetricOnly:  ThresholdModeMetricOnly,
}

// FeedbackSource defines a source mapping for a FeedbackResponder to a
// SystemMonitor with a specified significance and maximum value.
type FeedbackSource struct {
	Significance         float64        `json:"significance"`
	MaxValue             int64          `json:"max-value"`
	Threshold            int64          `json:"source-threshold,omitempty"`
	Monitor              *SystemMonitor `json:"-"`
	RelativeSignificance float64        `json:"-"`
}

const (
	// Default interval for which to send HAProxy commands after a
	// state change. This has been defined as 10 seconds as per MT,
	// but may likely need to be increased by modifying the
	// configuration for many use cases. This is presumably intended
	// to be the most conservative value.

	DefaultCommandInterval = 10
)

// -- Constants for HAProxy command handling.
const (
	// Flagged enums for sending composite feedback commands to HAProxy.
	// These are designed with two properties; one, that the commands are
	// configured as a single list (with the Responder knowing whether
	// they pertain to an online or offline state) and two, that they are
	// sent in a specific syntax order in the HAProxy response to ensure
	// that they are actioned in the right order of precedence. This is
	// done by the order of the enums and with a positive flag being
	// included for both online and offline states.

	HAPEnumNone        = 0x000
	HAPMaskCommand     = 0x0FF
	HAPMaskAll         = 0xFFF
	HAPOnlineFlag      = 0x100
	HAPOfflineFlag     = 0x200
	HAPEnumUp          = 0x101
	HAPEnumReady       = 0x102
	HAPEnumDown        = 0x204
	HAPEnumDrain       = 0x208
	HAPEnumFail        = 0x210
	HAPEnumMaintenance = 0x220
	HAPEnumStopped     = 0x240

	// As per the previous Loadbalancer.org Feedback Agent, the
	// default online command is "up ready" and the default
	// command to offline is "drain" (as requested by MT).

	HAPDefaultOnline  = HAPEnumUp | HAPEnumReady
	HAPDefaultOffline = HAPEnumDrain

	// Strings for sending composite feedback commands to HAProxy.

	HAPCommandNone        = ""
	HAPCommandUp          = "up"
	HAPCommandReady       = "ready"
	HAPCommandDown        = "down"
	HAPCommandDrain       = "drain"
	HAPCommandFail        = "fail"
	HAPCommandMaintenance = "maint"
	HAPCommandStopped     = "stopped"

	// JSON configuration settings for group options (default, none).

	HAPConfigDefault = "default"
	HAPConfigNone    = "none"
)

var commandToEnum = map[string]int{
	HAPCommandNone:        HAPEnumNone,
	HAPCommandUp:          HAPEnumUp,
	HAPCommandReady:       HAPEnumReady,
	HAPCommandDown:        HAPEnumDown,
	HAPCommandDrain:       HAPEnumDrain,
	HAPCommandFail:        HAPEnumFail,
	HAPCommandMaintenance: HAPEnumMaintenance,
	HAPCommandStopped:     HAPEnumStopped,
}

var enumToCommand = map[int]string{
	HAPEnumNone:        HAPCommandNone,
	HAPEnumUp:          HAPCommandUp,
	HAPEnumReady:       HAPCommandReady,
	HAPEnumDown:        HAPCommandDown,
	HAPEnumDrain:       HAPCommandDrain,
	HAPEnumFail:        HAPCommandFail,
	HAPEnumMaintenance: HAPCommandMaintenance,
	HAPEnumStopped:     HAPCommandStopped,
}
var commandEnumOrder = []int{
	HAPEnumNone,
	HAPEnumUp,
	HAPEnumReady,
	HAPEnumDown,
	HAPEnumDrain,
	HAPEnumFail,
	HAPEnumMaintenance,
	HAPEnumStopped,
}

// NewResponder creates a new FeedbackResponder object. This constructor must
// be used when creating a new responder object.
func NewResponder(name string, sources map[string]*FeedbackSource,
	protocol string, ip string, port string, commands string,
	thresholdMode string, threshold int, agent *FeedbackAgent) (
	result *FeedbackResponder, err error) {
	if sources == nil {
		sources = make(map[string]*FeedbackSource)
	}
	// -- Create a new responder containing the base settings.
	fbr := &FeedbackResponder{
		ProtocolName:      protocol,
		ListenIPAddress:   ip,
		ListenPort:        port,
		FeedbackSources:   sources,
		ResponderName:     name,
		ParentAgent:       agent,
		HAProxyCommands:   commands,
		ThresholdModeName: thresholdMode,
		ThresholdScore:    threshold,
		CommandInterval:   DefaultCommandInterval,
	}
	fbr.mutex = &sync.Mutex{}
	err = fbr.Initialise()
	if err == nil {
		result = fbr
	}
	return
}

// Initialise sets up this FeedbackResponder and configures defaults.
func (fbr *FeedbackResponder) Initialise() (err error) {
	// Initialise the mutex, if required.
	if fbr.mutex == nil {
		fbr.mutex = &sync.Mutex{}
	}
	fbr.mutex.Lock()
	defer fbr.mutex.Unlock()
	if fbr.FeedbackSources == nil {
		fbr.FeedbackSources = make(map[string]*FeedbackSource)
	}
	// -- Process/validate parameters.
	if fbr.ProtocolName == ProtocolLegacyAPI {
		alertMsg := "Insecure legacy plaintext HTTP API transport specified in configuration."
		if ForceAPISecure {
			fbr.ProtocolName = ProtocolSecureAPI
			alertMsg += " Forcing to HTTPS mode."
		}
		logrus.Warn(alertMsg)
	}
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
	// Skip source/command initialisation if this is an API responder, or it
	// has no feedback sources defined.
	if fbr.ProtocolName == ProtocolSecureAPI || len(fbr.FeedbackSources) < 1 {
		return
	}
	// This requires unlocking the mutex, and then locking again due to our defer.
	fbr.mutex.Unlock()
	err = fbr.initialiseSources()
	if err != nil {
		return
	}
	err = fbr.ConfigureCommands(fbr.HAProxyCommands, true, false)
	if err != nil {
		return
	}
	err = fbr.ConfigureInterval(fbr.CommandInterval)
	if err != nil {
		return
	}
	err = fbr.ConfigureThresholdMode(fbr.ThresholdModeName)
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
		err = errors.New(
			fbr.getLogHead() + ": no HAProxy command " +
				"configuration specified; perhaps you meant 'none' or " +
				"'default'?",
		)
		return
	}
	err = fbr.setHAPCommandMask(commands, replace, unset)
	return
}

func (fbr *FeedbackResponder) ConfigureInterval(interval int) (err error) {
	if interval < 1 {
		err = errors.New(
			fbr.getLogHead() + ": invalid HAProxy command " +
				"interval; use '0' to always send (interval disabled).",
		)
	}
	fbr.mutex.Lock()
	defer fbr.mutex.Unlock()
	if interval < 1 {
		fbr.CommandInterval = 0
	} else {
		fbr.CommandInterval = interval
	}
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
			err = errors.New(
				"cannot initialise responder: monitor '" +
					key + "' not found",
			)
			return
		}
		// Apply defaults for the significance and max value if
		// they are out of range for the required values.
		if source.Significance < 0.0 || source.Significance > 1.0 {
			err = errors.New(
				"'" + key + "': significance out of range: " +
					"must be between 0.0-1.0",
			)
			return
		}
		if source.MaxValue < 0 {
			err = errors.New(
				"'" + key + "': max value out of range:" +
					"cannot be negative",
			)
			return
		}
		if source.Threshold < 0 || source.Threshold > 100 {
			err = errors.New(
				"'" + key + "': threshold out of range: " +
					"must be between 0 and 100",
			)
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
			"': name '" + key + "', type '" +
			source.Monitor.MetricType + "': " +
			fmt.Sprintf("%.2f", source.Significance) +
			" -> relative " +
			fmt.Sprintf("%.2f", source.RelativeSignificance) + ".",
		)
	}
	return
}

func (fbr *FeedbackResponder) AddFeedbackSource(name string,
	significance *float64, maxValue *int64, threshold *int) (err error) {
	fbr.mutex.Lock()
	defer fbr.mutex.Unlock()
	name = strings.TrimSpace(name)
	if name == "" {
		err = errors.New(
			fbr.getLogHead() +
				": empty monitor name",
		)
		return
	}
	mon, exists := fbr.ParentAgent.Monitors[name]
	if !exists {
		err = errors.New(
			fbr.getLogHead() +
				": monitor '" + name + "' does not exist",
		)
		return
	}
	_, exists = fbr.FeedbackSources[name]
	if exists {
		err = errors.New(
			fbr.getLogHead() +
				": source monitor '" + name +
				"' is already attached to this Responder",
		)
		return
	}
	sigValue := 1.0
	if significance != nil {
		sigValue = *significance
	}
	metricMax := int64(mon.SysMetric.GetDefaultMax())
	if maxValue != nil {
		metricMax = *maxValue
	}
	// Default threshold value is 0 (ignore).
	thresholdValue := 0
	if threshold != nil {
		thresholdValue = *threshold
	}
	newSource := FeedbackSource{
		Monitor:      mon,
		Significance: sigValue,
		MaxValue:     metricMax,
		Threshold:    int64(thresholdValue),
	}
	fbr.FeedbackSources[name] = &newSource
	fbr.mutex.Unlock()
	// The initialiseSources() method of the responder also handles validation
	// of the specified parameters.
	err = fbr.initialiseSources()
	if err != nil {
		// Delete the source if it failed validation.
		delete(fbr.FeedbackSources, name)
	}
	fbr.mutex.Lock()
	return
}

func (fbr *FeedbackResponder) EditFeedbackSource(name string, significance *float64,
	maxValue *int64, threshold *int) (err error) {
	fbr.mutex.Lock()
	defer fbr.mutex.Unlock()
	source, exists := fbr.FeedbackSources[name]
	if !exists {
		err = errors.New(
			fbr.getLogHead() +
				": monitor '" + name +
				"' does not exist as a source for this responder",
		)
		return
	}
	unedited := *source
	if significance != nil {
		source.Significance = *significance
	}
	if maxValue != nil {
		source.MaxValue = *maxValue
	}
	if threshold != nil {
		source.Threshold = int64(*threshold)
	}
	fbr.FeedbackSources[name] = source
	fbr.mutex.Unlock()
	err = fbr.initialiseSources()
	// If initialisation fails, revert the change
	if err != nil {
		fbr.FeedbackSources[name] = &unedited
	}
	fbr.mutex.Lock()
	return
}

func (fbr *FeedbackResponder) DeleteFeedbackSource(name string) (err error) {
	fbr.mutex.Lock()
	defer fbr.mutex.Unlock()
	_, exists := fbr.FeedbackSources[name]
	if !exists {
		err = errors.New(
			fbr.getLogHead() +
				": monitor '" + name +
				"' does not exist as a source for this responder",
		)
		return
	}
	delete(fbr.FeedbackSources, name)
	fbr.mutex.Unlock()
	err = fbr.initialiseSources()
	fbr.mutex.Lock()
	return
}

// setHAPCommandMask configures the HAProxy command parameters for this
// FeedbackResponder, based on whether this should replace any commands
// currently set and if the list of commands provided should be removed
// rather than added.
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
		changeMask = HAPDefaultOnline | HAPDefaultOffline
	default:
		// Look up each command and translate into a mask (if valid).
		split := strings.Split(trimmed, " ")
		for _, command := range split {
			enum, exists := commandToEnum[command]
			if !exists {
				err = errors.New(
					"invalid HAProxy feedback command: '" +
						command + "'",
				)
				return
			}
			// Include this mask by ORing it into the current change mask
			changeMask |= enum
		}
	}
	// Mask off the enum flags (as we don't want these in the field)
	changeMask &= HAPMaskCommand
	// If setting these commands, OR the change mask into the current
	// command mask, otherwise AND NOT to unset them.
	if replace {
		fbr.configCommandMask = changeMask
	} else if !unset {
		fbr.configCommandMask |= changeMask
	} else {
		fbr.configCommandMask &= ^changeMask
	}
	// Convert the resulting command mask back to a string so that the
	// JSON configuration reflects this new state.
	if commands == HAPConfigNone || commands == HAPConfigDefault {
		fbr.HAProxyCommands = commands
	} else {
		fbr.HAProxyCommands = fbr.CommandMaskToString(
			fbr.configCommandMask,
			HAPMaskCommand, HAPMaskAll,
		)
	}
	fbr.resetStateExpiry()
	return
}

// Copy copies this FeedbackResponder into a new object.
func (fbr *FeedbackResponder) Copy() (copy FeedbackResponder) {
	fbr.mutex.Lock()
	defer fbr.mutex.Unlock()
	copy = *fbr
	copy.mutex = &sync.Mutex{}
	copy.runState = false
	return
}

// ParseNetworkPort parses a network port into a sanitised version, returning
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
		err = errors.New(
			"invalid IP address '" + ip +
				"' specified; use 'any' (CLI) or '*' (API) to listen on all IPs",
		)
		return
	}
	result = parsedIP.String()
	return
}

// Start starts the FeedbackResponder service, returning an error in the event
// of failure, by launching the main code of the service as a goroutine.
func (fbr *FeedbackResponder) Start() (err error) {
	fbr.mutex.Lock()
	defer fbr.mutex.Unlock()
	logLine := fbr.getLogHead()
	if len(fbr.FeedbackSources) < 1 &&
		fbr.ProtocolName != ProtocolSecureAPI &&
		fbr.ProtocolName != ProtocolLegacyAPI {
		logrus.Warn(
			"Warning: " + logLine +
				"currently has no monitor sources configured.",
		)
	}
	// Create a new channel for us to know when the worker has initialised or failed.
	initChannel := make(chan int)
	// Launch the worker goroutine for this FeedbackResponder.
	go fbr.run(initChannel)
	fbr.mutex.Unlock()
	// Wait on a result from the initialisation channel.
	result := <-initChannel
	fbr.mutex.Lock()
	// Log the appropriate status.
	if result == ServiceStateRunning && fbr.LastError == nil {
		logLine += "has started (" + strings.ToUpper(fbr.ProtocolName) +
			" on " + fbr.ListenIPAddress + ":" + fbr.ListenPort + ")."
		logrus.Info(logLine)
	} else {
		logLine += "failed to start, error: " + fbr.LastError.Error()
		logrus.Error(logLine)
	}
	// Return whatever the shared field holds for the worker error.
	err = fbr.LastError
	return
}

// Restart attempts to restart the FeedbackResponder service.
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

// Stop stops the service from running.
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

// IsRunning returns whether this FeedbackResponder is running or not.
func (fbr *FeedbackResponder) IsRunning() (state bool) {
	fbr.mutex.Lock()
	defer fbr.mutex.Unlock()
	state = fbr.runState
	return
}

// run is the function to call when the service starts; e.g.
// the worker thread invoked using 'go'.
func (fbr *FeedbackResponder) run(initChannel chan int) {
	if initChannel == nil {
		logrus.Error("FeedbackResponder: run: fatal: invalid channel")
		return
	}
	// Start by obtaining the mutex lock before doing anything else.
	fbr.mutex.Lock()
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
	// Check to see if we're already in a run state
	if fbr.runState {
		fbr.LastError = errors.New("already running")
		return
	}
	// -- Prepare to go into a running state.
	fbr.LastError = nil
	fbr.statusChannel = initChannel
	fbr.runState = true
	fbr.mutex.Unlock()
	// Initialise the current command state of the responder.
	fbr.SetCommandState(true, false, HAPEnumNone)
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

// getLogHead is a utility function for the start of log entries for this FeedbackResponder.
func (fbr *FeedbackResponder) getLogHead() string {
	return "Responder '" + fbr.ResponderName + "' "
}

// SetCommandState sets the current responder command state, resetting the state expiry.
func (fbr *FeedbackResponder) SetCommandState(isOnline bool, force bool, overrideMask int) {
	fbr.mutex.Lock()
	defer fbr.mutex.Unlock()
	fbr.onlineState = isOnline
	fbr.forceCommandState = force
	fbr.overrideMask = overrideMask & HAPMaskCommand
	fbr.resetStateExpiry()
}

// resetStateExpiry resets the current command state expiry only.
func (fbr *FeedbackResponder) resetStateExpiry() {
	fbr.stateExpiry = time.Now().Add(
		time.Second * time.Duration(fbr.CommandInterval),
	)
}

// ConfigureThresholdValue sets the command threshold for this FeedbackResponder.
func (fbr *FeedbackResponder) ConfigureThresholdValue(threshold int) (err error) {
	if threshold < 0 {
		err = errors.New(fbr.getLogHead() + "invalid threshold; cannot be negative")
		return
	}
	fbr.mutex.Lock()
	defer fbr.mutex.Unlock()
	fbr.ThresholdScore = threshold
	return
}

// GetAvailabilityState provides a corrected version of the algorithm mentioned
// on the Loadbalancer.org blog for the older Windows Feedback Agent, which
// calculates an availability score against a maximum value specified for a
// given metric, adjusted by a relative significance score (scaled proportion
// of the total significance for all monitors attached to this responder).
func (fbr *FeedbackResponder) GetAvailabilityState() (availability int, online bool, logText string) {
	// Calculate the overall total load across all monitors by scaling
	// against their maximum value, and then their relative significance.
	// Formula:
	//       s = 100 - ((v_cur / v_max) * sig_rel * 100)
	// where:
	//       s = total load score for this monitor
	//       v_cur = current raw value returned by the stats model
	//       v_max = maximum specified ceiling for the source
	//       sig_rel = fraction of all significances set for this monitor
	//
	// The responder should be in an online state unless otherwise disproven.
	online = true
	// The sum of all load values from each source, multiplied by the relative significance.
	overallLoad := 0
	metricLog, anyLog, overallLog := "", "", ""
	// Process the current load values for all feedback sources.
	for _, source := range fbr.FeedbackSources {
		// Get source load and add into the overall load scaled by its significance.
		sourceLoad := getSourceLoad(source)
		// Check to see if any per-source thresholds have been exceeded, if enabled.
		if fbr.isMetricThresholdEnabled() {
			exceeded, msg := fbr.getThresholdStatus("metric: source '"+
				source.Monitor.Name+"'",
				int(source.Threshold), sourceLoad)
			if exceeded {
				online = false
			}
			metricLog += msg + "\n"
		}
		// Check if we are looking for any threshold value, and if it has been exceeded.
		if fbr.isAnyThresholdEnabled() {
			exceeded, msg := fbr.getThresholdStatus("any: source '"+
				source.Monitor.Name+"'",
				fbr.ThresholdScore, sourceLoad)
			if exceeded {
				online = false
			}
			anyLog += msg + "\n"
		}
		// Add this source's load to the overall load, scaled by the significance.
		overallLoad += int(float64(sourceLoad) * source.RelativeSignificance)
	}
	// Check the overall threshold, if applicable.
	if fbr.isOverallThresholdEnabled() {
		exceeded, msg := fbr.getThresholdStatus("overall",
			fbr.ThresholdScore, overallLoad)
		if exceeded {
			online = false
		}
		overallLog += msg + "\n"
	}
	logText = anyLog + metricLog + overallLog
	// Invert the overall load percentage to give the availability.
	availability = 100 - overallLoad
	return
}

func (fbr *FeedbackResponder) isAnyThresholdEnabled() bool {
	return fbr.thresholdModeEnum == ThresholdModeAny
}

func (fbr *FeedbackResponder) isMetricThresholdEnabled() bool {
	return fbr.thresholdModeEnum == ThresholdModeAny ||
		fbr.thresholdModeEnum == ThresholdModeMetricOnly
}

func (fbr *FeedbackResponder) isOverallThresholdEnabled() bool {
	return fbr.thresholdModeEnum == ThresholdModeAny ||
		fbr.thresholdModeEnum == ThresholdModeOverallOnly
}

func getSourceLoad(source *FeedbackSource) (load int) {
	// Grab the current raw value from the stats model.
	rawValue := source.Monitor.StatsModel.GetResult()
	// Clamp the raw value at the configured max value.
	if rawValue > source.MaxValue {
		rawValue = source.MaxValue
	}
	load = int(math.Round((float64(rawValue) /
		float64(source.MaxValue)) * 100))
	// Constrain total within boundaries.
	if load > 100 {
		load = 100
	} else if load < 0 {
		load = 0
	}
	return
}

func (fbr *FeedbackResponder) getThresholdStatus(name string, threshold int, load int) (
	exceeded bool, msg string) {
	msg = name + ": "
	if threshold <= 0 {
		// If threshold is disabled, then exceeded is always false.
		msg += "threshold is disabled"
	} else {
		msg += "load (" + strconv.Itoa(load) + "%) "
		if load < threshold {
			// If the load is less than the threshold, then exceeded is also false.
			msg += "is within"
		} else {
			// Otherwise, the threshold has been exceeded.
			msg += "has exceeded"
			exceeded = true
		}
		msg += " threshold (" + strconv.Itoa(threshold) + "%)"
	}
	return
}

// HandleFeedback generates a feedback string for this FeedbackResponder.
// It also changes the current online state as of the last query so that
// a command is sent for a specified period of time from the first request.
func (fbr *FeedbackResponder) HandleFeedback() (feedback string) {
	timestamp := time.Now()
	fbr.mutex.Lock()
	defer fbr.mutex.Unlock()
	availability, thresholdState, logMessage := fbr.GetAvailabilityState()
	feedback = strconv.Itoa(availability) + "%"

	// First, work out if we should change state based on the threshold.
	// We do so if the threshold is enabled, the current threshold state
	// has changed, and we aren't in a forced command that hasn't yet
	// expired.
	if ((fbr.thresholdModeEnum != ThresholdModeNone) &&
		(thresholdState != fbr.onlineState)) &&
		(!fbr.forceCommandState || (timestamp.After(fbr.stateExpiry) &&
			(fbr.onlineState || fbr.EnableOfflineInterval))) {
		// SetHACommandState() is used by external code, so it
		// locks and unlocks the responder mutex itself. This means
		// we need to release the mutex first before calling it
		// and locking again for the final defer.
		fbr.mutex.Unlock()
		fbr.SetCommandState(thresholdState, false, HAPEnumNone)
		if fbr.LogStateChanges {
			logrus.Info(fbr.getLogHead() + "has changed threshold state:\n" + logMessage)
		}
		fbr.mutex.Lock()
	}

	// Next, work out whether we send a command for the current state
	// by checking whether it's expired yet, overridden if it's an offline
	// state and the interval is disabled for online states. Note that
	// we have to repeat the logic tests here because the state may
	// have changed above.
	if !timestamp.After(fbr.stateExpiry) ||
		(!fbr.EnableOfflineInterval && !fbr.onlineState) {
		mask := 0
		if fbr.overrideMask != HAPEnumNone {
			mask = fbr.overrideMask
		} else {
			mask = fbr.configCommandMask
		}
		feedback = fbr.GenerateCommandString(fbr.onlineState, mask) +
			" " + feedback
	}
	// The HAProxy specs call for a final newline to be sent.
	feedback += "\n"
	return
}

// GetResponse gets a string response from this FeedbackResponder, which will depend
// on its configuration and what it is supposed to do.
func (fbr *FeedbackResponder) GetResponse(request string) (response string,
	quitAfter bool) {
	if !PanicDebug {
		defer func() {
			err := recover()
			if err != nil {
				logrus.Error("An internal error occurred during a " +
					"response:\n   " + fmt.Sprint(err),
				)
			}
		}()
	}
	if fbr.ProtocolName == ProtocolSecureAPI || fbr.ProtocolName == ProtocolLegacyAPI {
		response, _, quitAfter = fbr.ParentAgent.ReceiveAPIRequest(request)
	} else {
		response = fbr.HandleFeedback()
	}
	return
}

// GenerateCommandString generates an HAProxy command string based on the current
// command mask and a specified online state.
func (fbr *FeedbackResponder) GenerateCommandString(online bool, currentMask int) (
	commands string) {
	state := HAPOfflineFlag
	if online {
		state = HAPOnlineFlag
	}
	commands = fbr.CommandMaskToString(currentMask, HAPMaskCommand, state)
	return
}

// CommandMaskToString converts an HAProxy command mask to a string, ignoring any command
// enums that don't have any bits matching the filter.
func (fbr *FeedbackResponder) CommandMaskToString(commandMask int, enumMask int,
	filter int) (commands string) {
	for _, enum := range commandEnumOrder {
		// Add this command if this enum is for the current state
		// and currently enabled within the configured command mask.
		if (enum&filter > 0) && ((enum & commandMask) == (enum & enumMask)) {
			if commands != "" {
				commands += " "
			}
			commands += enumToCommand[enum]
		}
	}
	return
}

// ConfigureThresholdMode sets the current threshold name and string
// from a specified string value, returning an error (and leaving the mode
// unchanged) if the specified string is invalid.
func (fbr *FeedbackResponder) ConfigureThresholdMode(name string) (err error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		// Set to default if no threshold string is currently configured.
		name = ThresholdStringNone
	}
	if (fbr.ProtocolName == ProtocolSecureAPI || fbr.ProtocolName == ProtocolLegacyAPI) &&
		name != ThresholdStringNone {
		err = errors.New("no threshold mode other than '" + ThresholdStringNone +
			"' is valid for an API responder")
		return
	}
	mode, exists := thresholdStringToMode[name]
	if !exists {
		err = errors.New("threshold mode '" + name + "' is invalid")
		return
	}
	fbr.thresholdModeEnum = mode
	fbr.ThresholdModeName = name
	return
}
