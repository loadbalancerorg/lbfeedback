// agent.go
// Main Agent Service

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
	"fmt"
	"github.com/sirupsen/logrus"
	"io"
	"os"
	"path"
	"strings"
)

// FeedbackAgent represents the main parent service which runs a configured
// set of [SystemMonitor] and [FeedbackResponder] objects, and provides the
// general utility functions for the project.
type FeedbackAgent struct {
	// Agent configuration fields
	LogDir     string                        `json:"log-dir"`
	APIKey     string                        `json:"api-key"`
	Monitors   map[string]*SystemMonitor     `json:"monitors"`
	Responders map[string]*FeedbackResponder `json:"responders"`

	// State parameters for the agent application
	useLocalPath   bool
	configDir      string
	isRunning      bool
	isStarting     bool
	systemSignals  chan os.Signal
	restartSignal  os.Signal
	quitSignal     os.Signal
	unsavedChanges bool
}

// LaunchAgentService creates a new [FeedbackAgent] service and runs it.
func LaunchAgentService() (exitStatus int) {
	// Print the CLI masthead.
	fmt.Println(ShellBanner)
	// $$ TO DO: Pass errors from agent.Run() to show success/
	// failure on the shell (not just in the logs).
	agent := FeedbackAgent{}
	exitStatus = agent.Run()
	return
}

// Run initialises the agent parameters and runs its main function.
func (agent *FeedbackAgent) Run() (exitStatus int) {
	agent.isStarting = true
	agent.useLocalPath = LocalPathMode
	agent.InitialiseLogger()
	agent.PlatformConfigureSignals()
	agent.InitialisePaths()
	logrus.Info("*** [Started] Loadbalancer.org Feedback Agent v" + VersionString)
	exitStatus = agent.agentMain()
	logrus.Info("*** [Stopped] The Feedback Agent has terminated.")
	return
}

// agentMain executes the agent, returning an exit status.
func (agent *FeedbackAgent) agentMain() (exitStatus int) {
	// Try to load a configuration from a config file, or else set up
	// the agent defaults.
	err := agent.LoadOrCreateConfig()
	if err != nil {
		logrus.Error("Configuration of Feedback Agent services failed.")
		exitStatus = ExitStatusError
		return
	}
	// Set up file logging for this agent.
	err = agent.InitialiseFileLogging(agent.LogDir)
	if err != nil {
		logrus.Error("cannot log to file; file logging disabled: " + err.Error())
	}
	// Start the main functions of the agent.
	err = agent.StartAllServices()
	agent.isStarting = false
	if err != nil {
		// We weren't able to successfully run the agent.
		logrus.Fatal(
			"The Feedback Agent failed to launch due to an error. " +
				"Please review the log output.",
		)
		exitStatus = ExitStatusError
		return
	}
	// Otherwise, all seems to be well. Go into the event handle loop.
	logrus.Info("Startup complete; the Feedback Agent has launched.")
	agent.EventHandleLoop()
	// If we're here, we've quit.
	err = agent.StopAllServices()
	if err != nil {
		logrus.Error("Failed to stop all services: " + err.Error() + ".")
		exitStatus = ExitStatusError
		return
	}
	exitStatus = ExitStatusNormal
	return
}

// Initialises the system paths for this [FeedbackAgent].
func (agent *FeedbackAgent) InitialisePaths() {
	if agent.useLocalPath {
		localDir, err := os.Getwd()
		if err == nil {
			agent.configDir = localDir
			agent.LogDir = localDir
			logrus.Info(
				"Local directory config and logs enabled to `" +
					localDir + "`.",
			)
		} else {
			logrus.Error(
				"Failed to get local directory for config and logs; " +
					"keeping system global paths.",
			)
			agent.useLocalPath = false
		}
	} else {
		agent.SetDefaultPaths()
	}
}

// EventHandleLoop blocks until a signal is received from the system based on
// what is registered  for the platform file. In the case of "platform_posix"
// this will be SIGTERM, SIGINT, etc.
func (agent *FeedbackAgent) EventHandleLoop() {
	for {
		// Wait for a signal to occur, and block this goroutine
		// until then, as there is nothing for us to do.
		signal := <-agent.systemSignals
		if signal == agent.restartSignal {
			err := agent.RestartAllServices()
			if err != nil {
				break
			}
		} else {
			break
		}
	}
}

func (agent *FeedbackAgent) EventHandleLoopNew() {
	for {
		select {
		case msg := <-agent.systemSignals:
			if msg == agent.restartSignal {
				err := agent.RestartAllServices()
				if err != nil {
					break
				}
			} else {
				break
			}
		default:
			// Delay timer goes here
		}
	}
}

// Sends the agent event loop a quit signal.
func (agent *FeedbackAgent) SelfSignalQuit() {
	agent.systemSignals <- agent.quitSignal
}

// Sets up logrus to show the timestamp in the correct format.
func (agent *FeedbackAgent) InitialiseLogger() {
	logrus.SetLevel(logrus.DebugLevel)
	formatter := &logrus.TextFormatter{
		TimestampFormat: "2006-01-02 15:04:05",
		FullTimestamp:   true,
		ForceColors:     false,
	}
	logrus.SetFormatter(formatter)
}

// Loads the JSON configuration file (or creates a new default file,
// loading a default configuration) and starts Monitors and Responders.
func (agent *FeedbackAgent) StartAllServices() (err error) {
	logrus.Info("The Feedback Agent is now launching.")
	// -- Start all [SystemMonitor] services.
	for _, monitor := range agent.Monitors {
		err = monitor.Start()
		if err != nil {
			logrus.Error(
				"Error initialising monitor '" +
					monitor.Name + "': " + err.Error(),
			)
			return
		}
	}
	// -- Start all [FeedbackResponder] services.
	// Give the API (if it is configured) first priority to start
	// before any other responders. This is so that if there is a port
	// collision in the JSON config, it is the other service that fails.
	responderStarted := false
	api, _ := agent.GetResponderByName(ResponderNameAPI)
	if api != nil {
		err = api.Start()
		if err != nil {
			logrus.Error(
				"Error initialising API responder: " +
					err.Error(),
			)
		} else {
			responderStarted = true
		}
	}
	for _, responder := range agent.Responders {
		if !responder.IsRunning() {
			err = responder.Start()
			if err != nil {
				logrus.Error(
					"Error initialising responder '" +
						responder.ResponderName + "': " + err.Error(),
				)
			} else {
				responderStarted = true
			}
		}
	}
	// Don't fail with a fatal error if at least one responder started.
	// This is to prevent an issue with a single Responder preventing
	// agent startup (and hence creating a problem where the API
	// cannot then be used to fix the configuration).
	if responderStarted {
		err = nil
	}
	return
}

// Signals all [FeedbackAgent] services to stop.
func (agent *FeedbackAgent) StopAllServices() (err error) {
	logrus.Info("Stopping all Feedback Agent services.")
	var currentErr error
	for _, responder := range agent.Responders {
		currentErr = responder.Stop()
		if currentErr != nil {
			err = errors.Join(err, currentErr)
		}
	}
	for _, monitor := range agent.Monitors {
		currentErr = monitor.Stop()
		if currentErr != nil {
			err = errors.Join(err, currentErr)
		}
	}
	return
}

// Restarts all [FeedbackAgent] services and reloads the configuration.
func (agent *FeedbackAgent) RestartAllServices() (err error) {
	logrus.Info("The Feedback Agent is restarting.")
	// We want to continue to start services even if stopping fails
	// to avoid the agent being left in a broken state (if possible).
	stopErr := agent.StopAllServices()
	startErr := agent.StartAllServices()
	err = errors.Join(stopErr, startErr)
	if err != nil {
		logrus.Error("Error whilst restarting services: " + err.Error())
	} else {
		logrus.Info("Restart complete.")
	}
	return
}

// Gets a [FeedbackResponder] by name from the map.
func (agent *FeedbackAgent) GetResponderByName(name string) (res *FeedbackResponder, err error) {
	// Try to get a pointer to the responder object, if it exists.
	res, exists := agent.Responders[name]
	if !exists || res == nil {
		err = errors.New("responder '" + name + "' does not exist")
		return
	}
	return
}

// Starts a [FeedbackResponder] by name from the map.
func (agent *FeedbackAgent) StartResponderByName(name string) (err error) {
	res, err := agent.GetResponderByName(name)
	if err != nil {
		return
	}
	err = res.Start()
	return
}

// Stops a [FeedbackResponder] by name from the map.
func (agent *FeedbackAgent) StopResponderByName(name string) (err error) {
	res, err := agent.GetResponderByName(name)
	if err != nil {
		return
	}
	if res.IsRunning() {
		err = res.Stop()
	}
	return
}

// Deletes a [FeedbackResponder] by name from the map.
func (agent *FeedbackAgent) DeleteResponderByName(name string) (err error) {
	err = agent.StopResponderByName(name)
	if err != nil {
		return
	}
	delete(agent.Responders, name)
	return
}

// Gets a [SystemMonitor] by name from the map.
func (agent *FeedbackAgent) GetMonitorByName(name string) (mon *SystemMonitor,
	err error) {
	// Try to get a pointer to the monitor object, if it exists.
	mon, exists := agent.Monitors[name]
	if !exists || mon == nil {
		err = errors.New("monitor '" + name + "' not found")
		return
	}
	return
}

// Starts a [SystemMonitor] by name from the map.
func (agent *FeedbackAgent) StartMonitorByName(name string) (err error) {
	mon, err := agent.GetMonitorByName(name)
	if err != nil {
		return
	}
	err = mon.Start()
	return
}

// Stops a [SystemMonitor] by name from the map.
func (agent *FeedbackAgent) StopMonitorByName(name string) (err error) {
	mon, err := agent.GetMonitorByName(name)
	if err != nil {
		return
	}
	err = mon.Stop()
	return
}

// Deletes a [SystemMonitor] by name from the map.
func (agent *FeedbackAgent) DeleteMonitorByName(name string) (err error) {
	err = agent.StopMonitorByName(name)
	if err != nil {
		return
	}
	delete(agent.Monitors, name)
	return
}

// Attempts to load the agent configuration from a JSON file at the
// configured system paths, and if it cannot do so, sets up the
// default agent configuration; this will be written to a new JSON
// file if one currently does not exist.
func (agent *FeedbackAgent) LoadOrCreateConfig() (err error) {
	agent.InitialiseServiceMaps()
	configLoaded := false
	createFile := false
	fullPath := path.Join(agent.configDir, ConfigFileName)
	// First, try to load the file if it exists.
	if FileExists(agent.configDir, ConfigFileName) {
		configLoaded, err = agent.LoadAgentConfig(agent.configDir, ConfigFileName)
		if configLoaded {
			logrus.Info("Configuration loaded successfully from file: " + fullPath)
			return
		} else if err != nil {
			logrus.Error("Failed to load configuration: " + err.Error())
		} else {
			logrus.Error("Failed to load configuration: unknown error.")
		}
	} else {
		logrus.Warn("Config file not found; a new file will be created.")
		createFile = true
	}
	// As we failed to load a configuration file, set up a default config.
	logrus.Warn("No configuration loaded; reverting to default services.")
	err = agent.SetDefaultServiceConfig()
	if err != nil {
		logrus.Error("Failed to set default configuration: " + err.Error())
		return
	}
	logrus.Info("Default services successfully configured.")
	// Create the config file if this is required.
	if createFile {
		// Attempt to save the agent configuration.
		success := false
		success, err = agent.SaveAgentConfigToPaths()
		// Log any errors that happened whilst saving the config.
		if err != nil {
			logrus.Error("Error whilst saving config: " + err.Error())
		}
		// Clear the error as handled if it we succeeded despite an error
		// occurring during the config save.
		if success {
			logrus.Info("Configuration file written successfully to '" + fullPath + "'.")
			err = nil
		}
	}
	return
}

// Checks to see if a file exists at the given directory path and file name.
func FileExists(dirPath string, fileName string) (exists bool) {
	fullPath := path.Join(dirPath, fileName)
	_, err := os.Stat(fullPath)
	if !os.IsNotExist(err) {
		exists = true
	}
	return
}

// Attempts to load the agent configuration from a specified path and name.
func (agent *FeedbackAgent) LoadAgentConfig(dirPath string, fileName string) (
	success bool, err error) {
	fullPath := path.Join(dirPath, fileName)
	// Attempt to open the target file for reading.
	file, err := os.Open(fullPath)
	if err != nil {
		return
	}
	var configData []byte
	configData, err = io.ReadAll(file)
	if err != nil {
		return
	}
	err = agent.JSONToConfig(configData)
	if err != nil {
		return
	}
	success = true
	err = file.Close()
	return
}

// Saves the agent configuration to the default system paths.
func (agent *FeedbackAgent) SaveAgentConfigToPaths() (success bool, err error) {
	success, err = agent.SaveAgentConfig(agent.configDir, ConfigFileName)
	return
}

// Saves the agent configuration to a specified directory and filename.
func (agent *FeedbackAgent) SaveAgentConfig(dirPath string, fileName string) (
	success bool, err error) {
	// Convert the config into a JSON stream for writing to the new file.
	jsonOutput, err := agent.ConfigToJSON()
	if err != nil {
		logrus.Error("Failed to generate config JSON: " + err.Error())
		return
	}
	fullPath := path.Join(dirPath, fileName)
	// Attempt to create or truncate the config file.
	file, err := os.Create(fullPath)
	// If this doesn't exist, handle the possible reason (e.g. path
	// does not exist).
	if err != nil {
		err = CreateDirectoryIfMissing(dirPath)
		if err != nil {
			err = errors.New(
				"Failed to open directory, and could " +
					"not create it: " + dirPath,
			)
			return
		}
		file, err = os.Create(fullPath)
		if err != nil {
			err = errors.New(
				"Failed to open file, and could " +
					"not create it: " + fullPath,
			)
			return
		} else {
			logrus.Info("File not found, created: " + fullPath)
		}
	}
	// Write the JSON config to the new file.
	_, err = file.Write(jsonOutput)
	if err != nil {
		err = errors.New(
			"Failed to save agent configuration: " +
				err.Error(),
		)
		return
	}
	success = true
	agent.unsavedChanges = false
	err = file.Close()
	if err != nil {
		err = errors.New(
			"Failed to close config file: " +
				err.Error(),
		)
	}
	return
}

// Creates a directory if it doesn't exist, and returns an error
// if creation is unsuccessful.
func CreateDirectoryIfMissing(dir string) (err error) {
	_, err = os.ReadDir(dir)
	if err != nil {
		err = os.MkdirAll(dir, DefaultDirPermissions)
		if err == nil {
			logrus.Info("Directory not found, created: " + dir)
		}
	}
	return
}

// Adds a monitor service to this [FeedbackAgent].
func (agent *FeedbackAgent) AddMonitor(name string, metric string, interval int, params MetricParams,
	model *StatisticsModel) (err error) {
	mon, err := NewSystemMonitor(
		name, metric,
		interval, params, model, agent.configDir,
	)
	if err != nil {
		return
	}
	err = agent.AddMonitorObject(mon)
	if err != nil {
		return
	}
	return
}

// Sets the default paths for this [FeedbackAgent]
func (agent *FeedbackAgent) SetDefaultPaths() {
	agent.configDir = DefaultConfigDir
	agent.LogDir = DefaultLogDir
}

// Sets up the agent with a default configuration consisting of one
// CPU monitor and one HTTP responder, connected together.
func (agent *FeedbackAgent) SetDefaultServiceConfig() (err error) {
	agent.InitialiseServiceMaps()
	err = agent.AddMonitor(
		"cpu", MetricTypeCPU,
		CPUMetricMinInterval, nil, nil,
	)
	if err != nil {
		logrus.Error("Error: " + err.Error())
		return
	}
	apiResponder := FeedbackResponder{
		ResponderName:   ResponderNameAPI,
		ProtocolName:    ProtocolSecureAPI,
		ListenIPAddress: "127.0.0.1",
		ListenPort:      "3334",
		FeedbackSources: nil,
	}
	err = agent.AddResponderObject(&apiResponder)
	if err != nil {
		logrus.Error("Error: " + err.Error())
		return
	}
	defaultSources := map[string]*FeedbackSource{
		"cpu": {
			Significance: 1.0,
			MaxValue:     100,
		},
	}
	defaultResponder := FeedbackResponder{
		ResponderName:   "default",
		ProtocolName:    ProtocolTCP,
		ListenIPAddress: "*",
		ListenPort:      "3333",
		HAProxyCommands: HAPConfigDefault,
		FeedbackSources: defaultSources,
		CommandInterval: DefaultCommandInterval,
	}
	err = agent.AddResponderObject(&defaultResponder)
	if err != nil {
		logrus.Error("Error: " + err.Error())
		return
	}
	agent.APIKey = RandomHexBytes(16)
	return
}

// Outputs this agent object's configuration as JSON.
func (agent *FeedbackAgent) ConfigToJSON() (output []byte, err error) {
	output, err = json.MarshalIndent(agent, "", "    ")
	return
}

// Configures the [FeedbackAgent] service from a byte stream of JSON
// configuration data by parsing it.
func (agent *FeedbackAgent) JSONToConfig(config []byte) (err error) {
	parsed := FeedbackAgent{}
	// Parse the JSON into a FeedbackAgent object.
	err = json.Unmarshal(config, &parsed)
	if err != nil {
		err = errors.New("JSON configuration is invalid or corrupted")
		return
	}
	// Set up the services from our parsed configuration.
	err = agent.configureFromObject(&parsed)
	if err != nil {
		return
	}
	return
}

// Sets up file logging given a string specifying the log directory on the
// local system, disabling it entirely if an empty string is supplied.
func (agent *FeedbackAgent) InitialiseFileLogging(dir string) (err error) {
	// Switch off if no path provided.
	if strings.TrimSpace(dir) == "" {
		logrus.Info("No file logging path provided; not enabled.")
		return
	}
	// Create the directory if it is missing; no error on success or if the
	// directory already exists.
	err = CreateDirectoryIfMissing(dir)
	if err != nil {
		return
	}
	fullPath := path.Join(dir, LogFileName)
	file, err := PlatformOpenLogFile(fullPath)
	if err == nil {
		logrus.Info("Logging to file: " + fullPath)
		logrus.SetOutput(io.MultiWriter(os.Stdout, file))
	}
	return
}

// Populates the Monitor and Responder services in this [FeedbackAgent]
// based on the fields set within the parsed object. Any validation errors
// will result in an error being returned.
func (agent *FeedbackAgent) configureFromObject(parsed *FeedbackAgent) (err error) {
	agent.LogDir = parsed.LogDir
	agent.APIKey = parsed.APIKey
	for name, monitor := range parsed.Monitors {
		monitor.Name = name
		err = agent.AddMonitorObject(monitor)
		if err != nil {
			return
		}
	}
	// Create responders from the parsed config.
	for name, responder := range parsed.Responders {
		responder.ResponderName = name
		responder.ParentAgent = agent
		err = agent.AddResponderObject(responder)
		if err != nil {
			return
		}
	}
	return
}

// Adds a monitor object to this [FeedbackAgent].
func (agent *FeedbackAgent) AddMonitorObject(monitor *SystemMonitor) (err error) {
	_, nameExists := agent.Monitors[monitor.Name]
	if nameExists {
		err = errors.New(
			"cannot create monitor '" + monitor.Name +
				"': name already exists",
		)
		return
	}
	monitor.FilePath = agent.configDir
	err = monitor.Initialise()
	if err != nil {
		return
	}
	agent.Monitors[monitor.Name] = monitor
	return
}

func (agent *FeedbackAgent) AddResponderObject(responder *FeedbackResponder) (err error) {
	name := responder.ResponderName
	_, nameExists := agent.Responders[name]
	if nameExists {
		err = errors.New(
			"cannot create responder '" + name +
				"': name already exists",
		)
		return
	}
	responder.ParentAgent = agent
	err = responder.Initialise()
	if err != nil {
		return
	}
	agent.Responders[name] = responder
	return
}

// Creates a Responder associated with a given Monitor, returning an
// error if the Monitor does not exist.
func (agent *FeedbackAgent) AddResponder(name string,
	sources map[string]*FeedbackSource, protocol string, ip string,
	port string, hapCommands string, enableThreshold bool,
	hapThreshold int) (err error) {
	_, nameExists := agent.Responders[name]
	if nameExists {
		err = errors.New(
			"cannot create responder '" + name +
				"': name already exists",
		)
		return
	}
	responder, err := NewResponder(
		name, sources, protocol,
		ip, port, hapCommands,
		enableThreshold, hapThreshold,
		agent,
	)
	if err != nil {
		err = errors.New(
			"cannot create responder '" + name +
				"': " + err.Error(),
		)
		return
	}
	agent.Responders[name] = responder
	return
}

// Clears all configured services from this [FeedbackAgent].
func (agent *FeedbackAgent) InitialiseServiceMaps() {
	agent.Monitors = make(map[string]*SystemMonitor)
	agent.Responders = make(map[string]*FeedbackResponder)
}

// -------------------------------------------------------------------
// END OF FILE
// -------------------------------------------------------------------
