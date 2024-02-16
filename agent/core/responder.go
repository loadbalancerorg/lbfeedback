// responder.go
// Feedback HTTP/TCP Responder
//
// Project:		Loadbalancer.org Feedback Agent v3
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
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

type FeedbackResponder struct {
	Name                string         `json:"-"`
	SourceMonitorName   string         `json:"input-monitor"`
	SourceMonitorThread *SystemMonitor `json:"-"`
	ProtocolName        string         `json:"protocol"`
	ListenPort          int            `json:"port"`
	RequestTimeout      time.Duration  `json:"-"`
	ResponseTimeout     time.Duration  `json:"-"`
	LastError           error          `json:"-"`
	httpServer          *http.Server   `json:"-"`
	isRunning           bool           `json:"-"`
	mutex               sync.Mutex     `json:"-"`
}

func (fbr *FeedbackResponder) Start() error {
	fbr.mutex.Lock()
	defer fbr.mutex.Unlock()
	if !fbr.isRunning {
		// Launch the goroutine whilst the mutex is still locked.
		go fbr.run()
		// Unlock the mutex to unblock the run() function.
		fbr.mutex.Unlock()
		// Use the mutex to block here until run() has either initialised
		// or failed so we can return the error result.
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
	return fbr.LastError
}

func (fbr *FeedbackResponder) run() {
	fbr.mutex.Lock()
	defer fbr.mutex.Unlock()
	if !fbr.isRunning {
		fbr.isRunning = true
		switch fbr.ProtocolName {
		case ServerProtocolHTTP:
			fbr.listenHTTP()
		// $$ Temporarily removed due to ldirectord issue
		//case ServerProtocolTCP:
		//	fbr.httpServer = nil
		//	fbr.listenTCP()
		default:
			fbr.LastError = errors.New("invalid protocol '" +
				fbr.ProtocolName + "' specified")
		}
		logrus.Info(fbr.getLogHead() + "has now stopped.")
	} else {
		fbr.LastError = errors.New("responder is already running")
	}
}

func (fbr *FeedbackResponder) getLogHead() string {
	return "Feedback Responder '" + fbr.Name + "' "
}

func (fbr *FeedbackResponder) listenHTTP() {
	fbr.httpServer = &http.Server{
		Addr:         ":" + strconv.Itoa(fbr.ListenPort),
		Handler:      http.HandlerFunc(fbr.HTTPHandler),
		ReadTimeout:  fbr.RequestTimeout,
		WriteTimeout: fbr.ResponseTimeout,
	}
	fbr.mutex.Unlock()
	// ListenAndServe() will block here until the server returns an
	// error. As we have unlocked the mutex, Stop() will be able to
	// call the method on the HTTP server to tell it to stop.
	fbr.LastError = fbr.httpServer.ListenAndServe()
	fbr.mutex.Lock()
}

func (fbr *FeedbackResponder) Stop() {
	// Lock the mutex and always unlock when we're finished.
	fbr.mutex.Lock()
	defer fbr.mutex.Unlock()
	if fbr.isRunning {
		if fbr.httpServer != nil {
			// This will unblock listenHTTP() as the server will then
			// return an error having stopped.
			fbr.LastError = fbr.httpServer.Shutdown(context.Background())
		}
	}
}

func (fbr *FeedbackResponder) HTTPHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "%d", fbr.SourceMonitorThread.StatsModel.GetWeightScore())
}

/*
func (fbr *FeedbackResponder) listenTCP() {
	// $$ TEMPORARILY REMOVED - ldirectord issue, as discussed with dsaunders
}
*/

// -------------------------------------------------------------------
// END OF FILE
// -------------------------------------------------------------------
