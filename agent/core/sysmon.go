// sysmon.go
// Feedback Agent System Monitor
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
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// [SystemMonitor] defines a System Metric Monitor that measures a
// parameter on the local system concurrently, with these values passed
// to a [StatisticsModel] for cumulative calculation into relative feedback
// weights.
type SystemMonitor struct {
	Name          string           `json:"-"`
	MetricType    string           `json:"metric-type"`
	Interval      int              `json:"interval-ms"`
	Params        MetricParams     `json:"metric-config,omitempty"`
	StatsModel    *StatisticsModel `json:"stats-config,omitempty"`
	SysMetric     SystemMetric     `json:"-"`
	LastError     error            `json:"-"`
	signalChannel chan int         `json:"-"`
	statusChannel chan int         `json:"-"`
	runState      bool             `json:"-"`
	isInitialised bool             `json:"-"`
	mutex         *sync.Mutex      `json:"-"`
}

const (
	SystemMonitorMinInterval int = 200
)

func NewSystemMonitor(name string, metric string, interval int,
	params MetricParams, model *StatisticsModel) (mon *SystemMonitor,
	err error) {
	mon = &SystemMonitor{
		Name:          name,
		Interval:      interval,
		MetricType:    metric,
		Params:        params,
		signalChannel: make(chan int),
		statusChannel: make(chan int),
	}
	err = mon.Initialise()
	return
}

func (monitor *SystemMonitor) Copy() (copy SystemMonitor) {
	copy = *monitor
	copy.mutex = nil
	copy.runState = false
	return
}

func (monitor *SystemMonitor) Initialise() (err error) {
	if monitor.mutex == nil {
		monitor.mutex = &sync.Mutex{}
	}
	monitor.mutex.Lock()
	defer monitor.mutex.Unlock()
	if monitor.Params == nil {
		monitor.Params = make(MetricParams)
	}
	if monitor.StatsModel == nil {
		monitor.StatsModel = &StatisticsModel{}
		monitor.StatsModel.SetDefaultParams()
	}
	monitor.SysMetric, err = NewMetric(monitor.MetricType,
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
func (monitor *SystemMonitor) Start() (err error) {
	// Try and launch the goroutine and wait for whether it succeeded or not.
	initChannel := make(chan int)
	go monitor.run(initChannel)
	status := <-initChannel
	// Lock the mutex to avoid a race condition with the goroutine
	// itself and with Stop(), and the other state change functions.
	monitor.mutex.Lock()
	defer monitor.mutex.Unlock()
	if status == ServiceStateRunning && monitor.LastError == nil {
		logrus.Info(monitor.getLogHead() + "has started (" +
			monitor.SysMetric.GetDescription() +
			", interval " + strconv.Itoa(monitor.Interval) + "ms).")
		// As this has been a successful start, the init channel
		// now becomes this Monitor's output channel. (Again, we
		// currently have the mutex, remember.)
		monitor.statusChannel = initChannel
	}
	return monitor.LastError
}

// Stops this [SystemMonitor] service.
func (monitor *SystemMonitor) Stop() (err error) {
	// Capture the running status within a lock cycle to prevent
	// a possible race condition with Start().
	if monitor.IsRunning() {
		stopped := false
		for !stopped {
			// Send the child goroutine a quit message
			monitor.signalChannel <- ServiceStateStopped
			// Check for a successful stopped reply
			if <-monitor.statusChannel == ServiceStateStopped {
				logrus.Info(monitor.getLogHead() +
					"has stopped.")
				stopped = true
			}
		}
	} else {
		err = errors.New("monitor is not running")
	}
	return
}

// Restarts this [SystemMonitor] service.
func (monitor *SystemMonitor) Restart() (err error) {
	err = monitor.Stop()
	if err != nil {
		return
	}
	err = monitor.Start()
	return
}

func (monitor *SystemMonitor) IsRunning() (state bool) {
	monitor.mutex.Lock()
	defer monitor.mutex.Unlock()
	state = monitor.runState
	return
}

// The main worker function for the [SystemMonitor] type.
func (monitor *SystemMonitor) run(initChannel chan int) {
	// Lock the mutex straight away on first launch.
	monitor.mutex.Lock()
	defer monitor.mutex.Unlock()
	var err error
	if !monitor.isInitialised {
		err = errors.New("monitor is not initialised")
	} else if monitor.runState {
		err = errors.New("monitor is already running")
	}
	if err != nil {
		monitor.LastError = err
		initChannel <- ServiceStateFailed
		return
	}
	monitor.enforceInterval()
	// We currently have the mutex, so it's safe to set the channel
	// and the current state parameters. None of the state change
	// functions will touch this until they get the lock.
	monitor.runState = true
	monitor.LastError = nil
	monitor.signalChannel = make(chan int)
	initChannel <- ServiceStateRunning
	metricFailed := false
	for monitor.runState {
		select {
		case msg := <-monitor.signalChannel:
			if msg == ServiceStateStopped {
				// Exit the run loop if we catch the signal to
				// tell us to stop.
				monitor.runState = false
			} else {
				logrus.Error("monitor caught unknown signal, ignoring: " +
					strconv.Itoa(msg))
			}
		default:
			// As we are still running, get a sample from our
			// metric and pass it to the stats model, waiting
			// for the required poll interval before iterating.
			value, err := monitor.getMetricSample()
			if err == nil {
				monitor.StatsModel.NewValue(value)
				if monitor.LastError != nil && metricFailed {
					logrus.Info(monitor.getLogHead() +
						"sampling has now succeeded; error cleared.")
					metricFailed = false
					monitor.LastError = nil
				}
			} else if monitor.LastError == nil {
				logrus.Error(monitor.getLogHead() +
					"failed to sample metric: " +
					err.Error())
				logrus.Warn("The above error will be logged only once.")
				metricFailed = true
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
	monitor.sendStoppedStatus()
}

func (monitor *SystemMonitor) sendStoppedStatus() {
	// Announce that we've now stopped on the status channel.
	monitor.statusChannel <- ServiceStateStopped
}

func (monitor *SystemMonitor) enforceInterval() {
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
func (monitor *SystemMonitor) CurrentAvailability() (result int64) {
	monitor.mutex.Lock()
	result = monitor.StatsModel.GetAvailabilityScore()
	monitor.mutex.Unlock()
	return result
}

// -------------------------------------------------------------------
// END OF FILE
// -------------------------------------------------------------------
