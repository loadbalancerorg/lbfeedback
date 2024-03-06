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
	"strconv"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// [SystemMonitor] defines a System Metric Monitor which is measuring a
// parameter on the local system concurrently, with these values passed
// to a [StatisticsModel] for cumulative calculation into relative feedback
// weights.
type SystemMonitor struct {
	Name          string           `json:"-"`
	Type          string           `json:"type"`
	Interval      int              `json:"interval-ms"`
	Params        MetricParams     `json:"metric-config"`
	StatsModel    *StatisticsModel `json:"stats-config,omitempty"`
	SysMetric     SystemMetric     `json:"-"`
	LastError     error            `json:"-"`
	signal        chan int         `json:"-"`
	isRunning     bool             `json:"-"`
	isInitialised bool             `json:"-"`
	mutex         sync.Mutex       `json:"-"`
}

const (
	SystemMonitorMinInterval int = 100
)

func NewSystemMonitor(name string, metric string, interval int, params MetricParams,
	model *StatisticsModel) (mon *SystemMonitor, err error) {
	mon = &SystemMonitor{
		Name:     name,
		Interval: interval,
		Type:     metric,
		Params:   params,
	}
	err = mon.Initialise()
	return
}

func (monitor *SystemMonitor) Initialise() (err error) {
	monitor.mutex.Lock()
	defer monitor.mutex.Unlock()
	if monitor.Params == nil {
		monitor.Params = make(MetricParams)
	}
	if monitor.StatsModel == nil {
		monitor.StatsModel = &StatisticsModel{}
		monitor.StatsModel.SetDefaultParams()
	}
	monitor.SysMetric, err = NewMetric(monitor.Type,
		monitor.Params)
	if err != nil {
		err = errors.New("failed to initialise monitor '" +
			monitor.Name + "': " + err.Error())
	} else {
		monitor.isInitialised = true
	}
	return
}

// Launches this [SystemMonitor] as a goroutine, returning any errors
// that occurred during the initial setup.
func (monitor *SystemMonitor) Start() error {
	// Start by locking to prevent a race condition with Stop(), and
	// always perform an unlock on the mutex after this function returns.
	monitor.mutex.Lock()
	defer monitor.mutex.Unlock()
	logLine := monitor.getLogHead()
	if !monitor.isInitialised {
		monitor.LastError = errors.New("monitor has not been initialised")
		return monitor.LastError
	}
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
		logrus.Info(monitor.getLogHead() + "is running (" +
			monitor.SysMetric.GetDescription() +
			", interval " + strconv.Itoa(monitor.Interval) + "ms).")
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
		// This function will block here until a quit signal.
		monitor.signal <- AgentSignalQuit
		// Get a lock before returning to protect against a race
		// condition with the run() method.
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
	monitor.parseParams()
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
			time.Sleep(time.Duration(monitor.Interval *
				int(time.Millisecond)))
			monitor.mutex.Lock()
		}
	}
	logrus.Info(monitor.getLogHead() + "has now stopped.")
}

func (monitor *SystemMonitor) parseParams() {
	if monitor.Interval < SystemMonitorMinInterval {
		logrus.Warn(
			monitor.getLogHead() +
				"invalid interval; using minimum of " +
				strconv.Itoa(SystemMonitorMinInterval) +
				"ms.",
		)
		monitor.Interval = SystemMonitorMinInterval
	}
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
