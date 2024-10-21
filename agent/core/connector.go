// connector.go
//
// Network Protocol Connectors for the Feedback Responder
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
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/sirupsen/logrus"
)

// #######################################################################
// ProtocolConnector
// #######################################################################

type ProtocolConnector interface {
	Listen(fbr *FeedbackResponder) (err error)
	Close() (err error)
}

func NewFeedbackConnector(protocol string) (conn ProtocolConnector, err error) {
	switch protocol {
	case ProtocolHTTP, ProtocolAPI:
		conn = &HTTPConnector{}
	case ProtocolTCP:
		conn = &TCPConnector{}
	default:
		err = errors.New("invalid protocol '" + protocol + "' specified")
	}
	return
}

// #################################
// TCPConnector
// #################################

type TCPConnector struct {
	tcpListener net.Listener       `json:"-"`
	responder   *FeedbackResponder `json:"-"`
}

func (pc *TCPConnector) Listen(fbr *FeedbackResponder) (err error) {
	pc.responder = fbr
	addressString := strings.TrimSpace(fbr.ListenIPAddress)
	if addressString == "*" {
		addressString = ""
	}
	addressString = ":" + strings.TrimSpace(fbr.ListenPort)
	pc.tcpListener, err = net.Listen("tcp", addressString)
	if err != nil {
		logrus.Error("TCP error: " + err.Error())
		return
	}
	var conn net.Conn
	for err == nil {
		// Accept() will block here until an error occurs (e.g. if
		// the listener is closed) or a request is received from a client.
		conn, err = pc.tcpListener.Accept()
		if conn != nil {
			go pc.handleRequest(conn)
		}
	}
	return
}

func (pc *TCPConnector) handleRequest(c net.Conn) {
	response, _ := pc.responder.GetResponse("")
	fmt.Fprintf(c, "%s", response)
	// Always force-close the connection after returning the feedback value.
	// This is to cope with an issue with ldirectord which will hang until the
	// connection is closed from the server
	c.Close()
}

func (pc *TCPConnector) Close() (err error) {
	if pc.tcpListener != nil {
		// This will unblock listenTCP() as the listener will then
		// return an error having stopped.
		err = pc.tcpListener.Close()
	}
	return
}

// #################################
// HTTPConnector
// #################################

type HTTPConnector struct {
	httpServer *http.Server       `json:"-"`
	responder  *FeedbackResponder `json:"-"`
}

func (pc *HTTPConnector) Listen(fbr *FeedbackResponder) (err error) {
	pc.responder = fbr
	ip := strings.TrimSpace(fbr.ListenIPAddress)
	if ip == "*" {
		ip = ""
	}
	port := strings.TrimSpace(fbr.ListenPort)
	if port == "" {
		err = errors.New("invalid port specified")
		return
	}
	pc.httpServer = &http.Server{
		Addr:         ip + ":" + port,
		Handler:      http.HandlerFunc(pc.handleRequest),
		ReadTimeout:  fbr.RequestTimeout,
		WriteTimeout: fbr.ResponseTimeout,
	}
	// ListenAndServe() will block here until the server returns an
	// error. As we have unlocked the mutex, Stop() will be able to
	// call the method on the HTTP server to tell it to stop.
	err = pc.httpServer.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		logrus.Error("HTTP error: " + err.Error())
	}
	return
}

func (pc *HTTPConnector) handleRequest(w http.ResponseWriter, r *http.Request) {
	// Read in the entire request body.
	body, err := io.ReadAll(r.Body)
	// Can't return the error here, since this is a callback from http
	// $ TO DO: Deal with what happens if we can't read the HTTP body
	if err != nil {
		logrus.Error("failed to read HTTP response")
	}
	response, quitAfterResponse := pc.responder.GetResponse(string(body))
	// Send response to writer (and therefore to the client).
	fmt.Fprintf(w, "%s", response)
	// If this was an API action requiring the agent to now quit, perform it.
	if quitAfterResponse {
		pc.responder.ParentAgent.SelfSignalQuit()
	}
}

func (pc *HTTPConnector) Close() (err error) {
	if pc.httpServer != nil {
		// This will unblock listenHTTP() as the server will then
		// return an error having stopped.
		err = pc.httpServer.Shutdown(context.Background())
	}
	return
}

// -------------------------------------------------------------------
// END OF FILE
// -------------------------------------------------------------------
