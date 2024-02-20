// sysmon.go
// Feedback Agent System Monitor
//
// Project:		Loadbalancer.org Feedback Agent v5
// Author: 		Nicholas Turnbull <nicholas.turnbull@loadbalancer.org>
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
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// [SystemMonitor] defines a System Metric Monitor which is measuring a
// parameter on the local system concurrently, with these values passed
// to a [StatisticsModel] for cumulative calculation into relative feedback
// weights.
type SystemMonitor struct {
	// JSON configuration fields; even those these are duplicated
	// within other types, they are here purely for loading/saving
	// the config structure and not for the main service logic.

	// $ TO DO: There are places in the code which still refer to these
	// outside of the context of the JSON config that need repointing
	// to the SystemMetric object.
	MetricType     string `json:"type"`
	ScriptFileName string `json:"script-name,omitempty"`
	PollInterval   int    `json:"interval-ms"`
	SampleTime     uint64 `json:"sample-ms"`

	// The actual type fields that we're working with.
	Name       string           `json:"-"`
	SysMetric  SystemMetric     `json:"-"`
	StatsModel *StatisticsModel `json:"-"`
	LastError  error            `json:"-"`
	signal     chan int         `json:"-"`
	isRunning  bool             `json:"-"`
	mutex      sync.Mutex       `json:"-"`
}

func NewSystemMonitor(name string, metric string,
	interval int, sampleTime uint64, scriptName string,
	scriptDir string) (mon *SystemMonitor, err error) {
	var sysmetric SystemMetric
	switch metric {
	case MetricTypeCPU:
		sysmetric = &CPUMetric{
			SampleTime: sampleTime,
		}
	case MetricTypeRAM:
		sysmetric = &MemoryMetric{}
	case MetricTypeScript:
		sysmetric = &ScriptMetric{
			scriptFullPath: path.Join(scriptDir, scriptName),
		}
	default:
		err = errors.New("unrecognised metric type: '" + metric + "'")
		return
	}
	mon = &SystemMonitor{
		Name:           name,
		SampleTime:     sampleTime,
		ScriptFileName: scriptName,
		PollInterval:   interval,
		SysMetric:      sysmetric,
		MetricType:     sysmetric.MetricName(),
		StatsModel:     &StatisticsModel{},
	}
	// $ TO DO: Provide configurability for the stats model.
	mon.StatsModel.SetDefaultParams()

	return
}

// Launches this [SystemMonitor] as a goroutine, returning any errors that
// occurred during the initial setup.
func (monitor *SystemMonitor) Start() error {
	// Start by locking to prevent a race condition with Stop(),
	// and always perform an unlock on the mutex after this function returns.
	monitor.mutex.Lock()
	defer monitor.mutex.Unlock()
	logLine := monitor.getLogHead()
	if !monitor.isRunning {
		monitor.LastError = nil
		go monitor.run()
		monitor.mutex.Unlock()
		// Block here using the mutex until either runMonitor() has
		// initialised or failed to start, so that we can handle success/error.
		monitor.mutex.Lock()
	} else {
		// There is already a goroutine running for this monitor; fail with error.
		monitor.LastError = errors.New("monitor already running")
	}
	// Report success or error based on the result.
	if monitor.LastError != nil {
		logLine += "failed to start; error: " + monitor.LastError.Error()
		logrus.Error(logLine)
	} else {
		var metricDesc string
		if monitor.MetricType == MetricTypeScript {
			metricDesc = "script '" + monitor.ScriptFileName + "'"
		} else {
			metricDesc = strings.ToUpper(monitor.MetricType)
		}
		logrus.Info(monitor.getLogHead() + "is running (" +
			metricDesc +
			", interval " + strconv.Itoa(monitor.PollInterval) + "ms).")
	}
	return monitor.LastError
}

// Stops this system monitor service.
func (monitor *SystemMonitor) Stop() {
	// Capture the running status within a lock cycle to prevent
	// a possible race condition with Start().
	monitor.mutex.Lock()
	defer monitor.mutex.Unlock()
	if monitor.isRunning {
		monitor.mutex.Unlock()
		// This function will block here until the quit signal is received.
		monitor.signal <- AgentSignalQuit
		// Get lock before returning to protect against a race condition with run().
		monitor.mutex.Lock()
	}
}

// The main worker function for the [SystemMonitor] type.
func (monitor *SystemMonitor) run() {
	// Lock the mutex straight away on first launch.
	monitor.mutex.Lock()
	defer monitor.mutex.Unlock()
	monitor.signal = make(chan int)
	monitor.isRunning = true
	for monitor.isRunning {
		select {
		case <-monitor.signal:
			// Exit the run loop if we catch the signal to
			// tell us to stop. This could be used for more
			// advanced functionality in the future.
			monitor.isRunning = false
		default:
			// As we are still running, get a sample from our
			// metric and pass it to the stats model, waiting
			// for the required poll interval before iterating.
			value, err := monitor.getMetricSample()
			if err == nil {
				monitor.StatsModel.NewValue(value)
				if monitor.LastError != nil {
					logrus.Info(monitor.getLogHead() +
						"sampling has now succeeded; error cleared.")
					monitor.LastError = nil
				}
			} else if monitor.LastError == nil {
				logrus.Error(monitor.getLogHead() +
					"failed to sample metric: " +
					err.Error())
				logrus.Warn("The above error will be logged only once.")
				monitor.LastError = err
			}
			// Unlock the mutex during the wait, and lock
			// after it has concluded as we are resuming.
			monitor.mutex.Unlock()
			time.Sleep(time.Duration(monitor.PollInterval *
				int(time.Millisecond)))
			monitor.mutex.Lock()
		}
	}
	logrus.Info(monitor.getLogHead() + "has now stopped.")
}

// Generates the head of a log message.
func (monitor *SystemMonitor) getLogHead() string {
	return "System Metric Monitor '" + monitor.Name + "' "
}

// Gets a sample from the metric that this thread is measuring.
func (monitor *SystemMonitor) getMetricSample() (value float64, err error) {
	value, err = monitor.SysMetric.GetLoad()
	return
}

// Returns the current feedback weight for this monitor thread.
func (monitor *SystemMonitor) CurrentWeight() (result int64) {
	monitor.mutex.Lock()
	result = monitor.StatsModel.GetWeightScore()
	monitor.mutex.Unlock()
	return result
}

// -------------------------------------------------------------------
// END OF FILE
// -------------------------------------------------------------------
