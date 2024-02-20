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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// #######################################################################
// FeedbackResponder
// #######################################################################

// [FeedbackResponder] implements a Feedback Responder service which uses
// the specified [ProtocolConnector] to listen for and respond to clients
// from data obtained via the associated [SystemMonitor].
type FeedbackResponder struct {
	ResponderName       string            `json:"-"`
	SourceMonitorName   string            `json:"input-monitor"`
	SourceMonitorThread *SystemMonitor    `json:"-"`
	ProtocolName        string            `json:"protocol"`
	Connector           ProtocolConnector `json:"-"`
	ListenPort          int               `json:"port"`
	RequestTimeout      time.Duration     `json:"-"`
	ResponseTimeout     time.Duration     `json:"-"`
	LastError           error             `json:"-"`
	isRunning           bool              `json:"-"`
	mutex               sync.Mutex        `json:"-"`
}

// Constructor for [FeedbackResponder], which must be used when creating
// a new responder object.
func NewResponder(name string, monitor *SystemMonitor,
	protocol string, port int) (fbr *FeedbackResponder, err error) {
	if monitor == nil {
		err = errors.New("cannot create responder: no monitor specified")
		return
	}
	// Create a generic responder containing the base settings.
	fbr = &FeedbackResponder{
		ProtocolName:        protocol,
		ListenPort:          port,
		SourceMonitorThread: monitor,
		ResponderName:       name,
		SourceMonitorName:   monitor.Name,
	}
	switch protocol {
	case ServerProtocolHTTP:
		fbr.Connector = &HTTPConnector{}
	case ServerProtocolTCP:
		fbr.Connector = &TCPConnector{}
	default:
		err = errors.New("cannot create responder: invalid protocol '" +
			protocol + "' specified")
		fbr = nil
	}
	return
}

// Starts the Feedback Agent service, returning an error in the event
// of failure, by launching the main code of the service as a goroutine.
func (fbr *FeedbackResponder) StartService() (err error) {
	fbr.mutex.Lock()
	defer fbr.mutex.Unlock()
	if !fbr.isRunning {
		// Launch the goroutine whilst the mutex is still locked.
		go fbr.goroutine()
		// Unlock the mutex to unblock the goroutine() function.
		fbr.mutex.Unlock()
		// Use the mutex to block here until goroutine() has either
		// initialised or failed so we can return the error result.
		fbr.mutex.Lock()

	} else {
		fbr.LastError = errors.New("responder is already running")
	}
	logLine := fbr.getLogHead()
	if fbr.LastError == nil {
		logLine += "is running (" + strings.ToUpper(fbr.ProtocolName) +
			", localhost port " + strconv.Itoa(fbr.ListenPort) + ")."
		logrus.Info(logLine)
	} else {
		logLine += "failed, error: " + fbr.LastError.Error()
	}
	err = fbr.LastError
	return
}

// Stops the service from running.
func (fbr *FeedbackResponder) StopService() (err error) {
	// Lock the mutex and always unlock when we're finished.
	fbr.mutex.Lock()
	defer fbr.mutex.Unlock()
	if fbr.isRunning {
		fbr.isRunning = false
		err = fbr.Connector.Close()
	}
	return
}

// The goroutine function to call when the service starts; e.g.
// the worker thread.
func (fbr *FeedbackResponder) goroutine() {
	fbr.mutex.Lock()
	defer fbr.mutex.Unlock()
	var err error
	if !fbr.isRunning {
		fbr.isRunning = true
		fbr.mutex.Unlock()
		// Call the Listen() method of the protocol connector, which
		// will block here until it quits.
		err = fbr.Connector.Listen(fbr)
		fbr.mutex.Lock()
		logrus.Info(fbr.getLogHead() + "has now stopped.")
	} else {
		err = errors.New("responder is already running")
	}
	if err != nil {
		fbr.LastError = err
	}
}

func (fbr *FeedbackResponder) getLogHead() string {
	return "Feedback Responder '" + fbr.ResponderName + "' "
}

func (fbr *FeedbackResponder) getFeedbackString() string {
	return fmt.Sprintf("%d%%\n", fbr.SourceMonitorThread.
		StatsModel.GetWeightScore())
}

// -------------------------------------------------------------------
// END OF FILE
// -------------------------------------------------------------------
