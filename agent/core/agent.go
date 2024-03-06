// agent.go
// Feedback Agent Service
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
	"encoding/json"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
)

// The [FeedbackAgent] represents the main parent service which runs a configured
// set of [SystemMonitor] and [FeedbackResponder] objects, and provides the
// general utility functions for the project.
type FeedbackAgent struct {
	LogDir        string                        `json:"log-dir"`
	Monitors      map[string]*SystemMonitor     `json:"monitors"`
	Responders    map[string]*FeedbackResponder `json:"responders"`
	ConfigDir     string                        `json:"-"`
	stateRunning  bool                          `json:"-"`
	systemSignals chan os.Signal                `json:"-"`
	restartSignal os.Signal                     `json:"-"`
}

// The "main" method for the agent application.
func (agent *FeedbackAgent) Run() {
	logrus.SetLevel(logrus.DebugLevel)
	agent.PlatformConfigureSignals()
	agent.setLogFormatter()
	err := agent.performStartup()
	if err == nil {
		agent.performSignalWaitLoop()
		agent.stopServices()
	}
	logrus.Info("*** The Feedback Agent has terminated.")
}

// Performs the entire startup process for the [FeedbackAgent].
func (agent *FeedbackAgent) performStartup() (err error) {
	agent.PlatformSetDefaultPaths()
	err = agent.loadAndStart()
	if err == nil {
		agent.stateRunning = true
		logrus.Info("Startup complete; the Feedback Agent is running.")
	} else {
		agent.stateRunning = false
		logrus.Fatal("The Feedback Agent failed to start due to a fatal error; terminating.")
	}
	return
}

// Waits until a signal is received from the system based on what is registered
// for the platform file. In the case of "platform_posix" this will be SIGTERM,
// SIGINT, etc.
func (agent *FeedbackAgent) performSignalWaitLoop() {
	for agent.stateRunning {
		// Wait for a signal to occur, and block this goroutine
		// until then, as there is nothing for us to do.
		signal := <-agent.systemSignals
		if signal == agent.restartSignal {
			agent.performRestart()
		} else {
			agent.stateRunning = false
			break
		}
	}
}

// Loads the JSON configuration file (or creates a new default file,
// loading a default configuration) and starts Monitors and Responders.
func (agent *FeedbackAgent) loadAndStart() (err error) {
	agent.clearServices()
	err = agent.loadOrCreateConfigFile()
	if err != nil {
		logrus.Error("Configuration of Feedback Agent services failed.")
		return
	} else {
		logrus.Info("The Feedback Agent is now starting up.")
	}
	for _, monitor := range agent.Monitors {
		err = monitor.Start()
		if err != nil {
			logrus.Error("Error initialising monitor '" +
				monitor.Name + "': " + err.Error())
			return
		}
	}
	for _, responder := range agent.Responders {
		err = responder.StartService()
		if err != nil {
			logrus.Error("Error initialising responder '" +
				responder.ResponderName + "': " + err.Error())
			return
		}
	}

	return
}

// Signals all [FeedbackAgent] services to stop.
func (agent *FeedbackAgent) stopServices() {
	logrus.Info("Stopping all Feedback Agent services.")
	for _, responder := range agent.Responders {
		responder.StopService()
	}
	for _, monitor := range agent.Monitors {
		monitor.Stop()
	}
	agent.stateRunning = false
	logrus.Info("All services have stopped.")
}

// Restarts all [FeedbackAgent] services and reloads the configuration.
func (agent *FeedbackAgent) performRestart() {
	logrus.Info("The Feedback Agent is restarting.")
	agent.stopServices()
	agent.loadAndStart()
	logrus.Info("Restart complete.")
}

// Tries to load a [FeedbackAgent] config JSON file, and if it doesn't exist,
// creates a new one and writes it with a default configuration (including)
// creating the directory path). An error is returned in the event of failure.
func (agent *FeedbackAgent) loadOrCreateConfigFile() (err error) {
	deleteAfterClose := false
	fullPath := path.Join(agent.ConfigDir, ConfigFileName)

	configFile, created, err := OpenOrCreateFile(agent.ConfigDir, ConfigFileName)
	if err != nil {
		// Failed to either open the config file for reading or create for writing.
		logrus.Error(err.Error())
	} else if created {
		// If the file was created, then this is in write rather than read mode.
		agent.setDefaultServiceConfig()
		_, err = configFile.Write(agent.configToJSON())
		if err != nil {
			logrus.Error("Failed to write defaults to config file: " +
				err.Error())
			// Delete the new (still empty) config file as we failed to write it
			deleteAfterClose = true

		} else {
			logrus.Info("Successfully wrote defaults to config file: " +
				fullPath)
		}
	} else {
		// If the file wasn't created, this is in read rather than write mode.
		var configData []byte
		configData, err = io.ReadAll(configFile)
		if err != nil {
			logrus.Error("Failed to read config file: " + err.Error())
			return
		}
		err = agent.configureFromJSON(configData)
		if err != nil {
			logrus.Error("Config failure from JSON: " + err.Error())
			return
		} else {
			logrus.Info("Configured from JSON successfully.")
		}
	}
	if configFile != nil {
		// Attempt to close the file; there's nothing much we can do
		// if this fails to close, unfortunately.
		_ = configFile.Close()
		if deleteAfterClose {
			// If we ended up creating a blank JSON file, try to remove it
			// if possible; again, there's not much to do if this fails.
			_ = os.Remove(fullPath)
		}
	}
	return
}

// OpenOrCreateFile tries to open a file for both reading if it can.
// Otherwise, it will create the full path to the file if its directories
// do not exist and open the file for writing (for creating it).
func OpenOrCreateFile(dir string, filename string) (file *os.File,
	created bool, err error) {
	// Build the full path from the directory and filename.
	fullPath := filepath.Join(dir, filename)
	// Attempt to open the target file for reading.
	file, err = os.Open(fullPath)
	// If this doesn't exist, try to create it and fail if not.
	if err != nil {
		err = CreateDirectoryIfMissing(dir)
		if err != nil {
			err = errors.New("Failed to open directory, and could " +
				"not create it: " + dir)
			return
		}
		file, err = os.Create(fullPath)
		if err != nil {
			err = errors.New("Failed to open file, and could " +
				"not create it: " + fullPath)
		} else {
			logrus.Info("File not found, created: " + filename)
			created = true
		}
	}
	return
}

// Create the directory if it doesn't exist, and returns an error
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

// Sets up the agent with a default configuration consisting of one
// CPU monitor and one HTTP responder, connected together.
func (agent *FeedbackAgent) setDefaultServiceConfig() {
	agent.clearServices()
	logrus.Info("Setting default service configuration.")
	mon, err := NewSystemMonitor("mon1", MetricTypeCPU,
		SystemMonitorMinInterval, nil, nil)
	if err != nil {
		logrus.Error("Failed to create default system monitor: " +
			err.Error())
		return
	}
	err = agent.addMonitor(mon)
	if err != nil {
		logrus.Error("Failed to add default system monitor: " +
			err.Error())
		return
	}
	agent.addResponder("res1", "mon1", ServerProtocolTCP, 3333)
	logrus.Info("Default configuration set.")
}

// Sets up logrus to show the timestamp in the correct format.
func (agent *FeedbackAgent) setLogFormatter() {
	fmt := &logrus.TextFormatter{
		TimestampFormat: "2006-01-02 15:04:05",
		FullTimestamp:   true,
		ForceColors:     false,
	}
	logrus.SetFormatter(fmt)
}

// Outputs this agent object's configuration as JSON.
func (agent *FeedbackAgent) configToJSON() []byte {
	jsonOutput, _ := json.MarshalIndent(agent, "", "    ")
	return jsonOutput
}

// Configures the [FeedbackAgent] service from a byte stream of JSON
// configuration data by parsing it.
func (agent *FeedbackAgent) configureFromJSON(config []byte) (err error) {
	parsed := FeedbackAgent{}
	// Parse the JSON into a FeedbackAgent object.
	err = json.Unmarshal(config, &parsed)
	if err != nil {
		err = errors.New("JSON configuration is invalid or corrupted")
		return
	}
	// Set up file logging for this agent.
	err = agent.configureFileLogging(parsed.LogDir)
	if err != nil {
		err = errors.New("cannot log to file: " + err.Error())
		return
	}
	// Set up the services from our parsed configuration.
	err = agent.populateServices(&parsed)
	if err != nil {
		return
	}
	return
}

// Sets up file logging given a string specifying the log directory on the
// local system, disabling it entirely if an empty string is supplied.
func (agent *FeedbackAgent) configureFileLogging(dir string) (err error) {
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
	file, err := os.OpenFile(fullPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		DefaultFilePermissions)
	logrus.Info("*** Loadbalancer.org Feedback Agent Version: v" + VersionString)
	if err == nil {
		logrus.SetOutput(io.MultiWriter(os.Stdout, file))
		logrus.Info("Logging to file: " + fullPath)
	}
	return
}

// Populates the Monitor and Responder services in this [FeedbackAgent]
// based on the fields set within the parsed object. Any validation errors
// will result in an error being returned.
func (agent *FeedbackAgent) populateServices(parsed *FeedbackAgent) (err error) {
	for name, monitor := range parsed.Monitors {
		monitor.Name = name
		err = agent.addMonitor(monitor)
		if err != nil {
			return
		}
		err = monitor.Initialise()
		if err != nil {
			return
		}
	}
	// Create responders from the parsed config.
	for name, responder := range parsed.Responders {
		err = agent.addResponder(
			name,
			responder.SourceMonitorName,
			responder.ProtocolName,
			responder.ListenPort,
		)
		if err != nil {
			return
		}
	}
	return
}

func (agent *FeedbackAgent) addMonitor(monitor *SystemMonitor) (err error) {
	_, nameExists := agent.Monitors[monitor.Name]
	if nameExists {
		err = errors.New("cannot create monitor '" + monitor.Name +
			"': name already exists")
		return
	}
	agent.Monitors[monitor.Name] = monitor
	return
}

// Creates a Responder associated with a given Monitor, returning an
// error if the Monitor does not exist.
func (agent *FeedbackAgent) addResponder(name string, monitorName string,
	protocol string, port int) (err error) {
	_, nameExists := agent.Responders[name]
	if nameExists {
		err = errors.New("cannot create responder '" + name +
			"': name already exists")
		return
	}
	monitor, monitorExists := agent.Monitors[monitorName]
	if !monitorExists {
		err = errors.New("cannot create responder '" + name +
			"': assigned monitor '" + monitorName +
			"' does not exist")
		return
	}
	var responder *FeedbackResponder
	responder, err = NewResponder(name, monitor, protocol, port)
	if err != nil {
		err = errors.New("cannot create responder '" + name +
			"': " + err.Error())
		return
	}
	agent.Responders[name] = responder
	return
}

// Clears all configured services from this [FeedbackAgent].
func (agent *FeedbackAgent) clearServices() {
	agent.Monitors = make(map[string]*SystemMonitor)
	agent.Responders = make(map[string]*FeedbackResponder)
}

// -------------------------------------------------------------------
// END OF FILE
// -------------------------------------------------------------------
