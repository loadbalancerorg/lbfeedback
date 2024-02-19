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
	"net"
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
	tcpListener         net.Listener   `json:"-"`
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
	var err error
	if !fbr.isRunning {
		fbr.isRunning = true
		fbr.mutex.Unlock()
		switch fbr.ProtocolName {
		case ServerProtocolHTTP:
			err = fbr.listenHTTP()
		case ServerProtocolTCP:
			fbr.httpServer = nil
			err = fbr.listenTCP()
		default:
			err = errors.New("invalid protocol '" +
				fbr.ProtocolName + "' specified")
		}
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
	return "Feedback Responder '" + fbr.Name + "' "
}

func (fbr *FeedbackResponder) Stop() (err error) {
	// Lock the mutex and always unlock when we're finished.
	fbr.mutex.Lock()
	defer fbr.mutex.Unlock()
	logrus.Info("Attempting to stop " + fbr.getLogHead())
	if fbr.isRunning {
		fbr.isRunning = false
		if fbr.httpServer != nil {
			// This will unblock listenHTTP() as the server will then
			// return an error having stopped.
			err = fbr.httpServer.Shutdown(context.Background())
		}
		if fbr.tcpListener != nil {
			// This will unblock listenTCP() as the listener will then
			// return an error having stopped.
			err = fbr.tcpListener.Close()
		}
	}
	return
}

func (fbr *FeedbackResponder) getFeedbackString() string {
	return fmt.Sprintf("%d%%\n", fbr.SourceMonitorThread.
		StatsModel.GetWeightScore())
}

func (fbr *FeedbackResponder) listenHTTP() (err error) {
	fbr.httpServer = &http.Server{
		Addr:         ":" + strconv.Itoa(fbr.ListenPort),
		Handler:      http.HandlerFunc(fbr.handleHTTP),
		ReadTimeout:  fbr.RequestTimeout,
		WriteTimeout: fbr.ResponseTimeout,
	}
	// ListenAndServe() will block here until the server returns an
	// error. As we have unlocked the mutex, Stop() will be able to
	// call the method on the HTTP server to tell it to stop.
	err = fbr.httpServer.ListenAndServe()
	return
}

func (fbr *FeedbackResponder) handleHTTP(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "%s", fbr.getFeedbackString())
}

func (fbr *FeedbackResponder) listenTCP() (err error) {
	fbr.tcpListener, err = net.Listen("tcp4", ":"+fmt.Sprint(fbr.ListenPort))
	if err != nil {
		return
	}
	var conn net.Conn

	for err == nil {
		// Accept() will block here until the listener is closed.
		conn, err = fbr.tcpListener.Accept()
		if conn != nil {
			go fbr.handleTCP(conn)
		}
	}
	return
}

func (fbr *FeedbackResponder) handleTCP(c net.Conn) {
	fmt.Fprintf(c, "%s", fbr.getFeedbackString())
	// Always force-close the connection after returning the feedback value.
	// This is to cope with an issue with ldirectord which will hang until the
	// connection is closed from the server
	c.Close()
}

// -------------------------------------------------------------------
// END OF FILE
// -------------------------------------------------------------------
