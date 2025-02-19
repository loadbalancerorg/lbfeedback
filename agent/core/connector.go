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
	"crypto/tls"
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

// NewFeedbackConnector creates a new connector for a given protocol and (if required)
// the configuration path containing the TLS certificate and key.
func NewFeedbackConnector(protocol string) (conn ProtocolConnector, err error) {
	switch protocol {
	case ProtocolTCP:
		conn = &TCPConnector{}
	case ProtocolHTTP:
		conn = &HTTPConnector{}
	case ProtocolHTTPS, ProtocolSecureAPI:
		conn = &HTTPConnector{
			enableTLS: true,
		}
	default:
		err = errors.New("invalid protocol '" + protocol + "' specified")
	}
	return
}

// #################################
// TCPConnector
// #################################

type TCPConnector struct {
	tcpListener net.Listener
	responder   *FeedbackResponder
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
	httpServer *http.Server
	responder  *FeedbackResponder
	enableTLS  bool
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
	// ListenAndServe/ListenAndServeTLS will block here until the server
	// returns an error. As we have unlocked the mutex in the parent Responder,
	// fbr.Stop will be able to call the method on the HTTP server to tell it to stop.
	if pc.enableTLS {
		// -- This responder is in HTTPS mode with TLS.
		// Sanity check that a TLS certificate is configured first.
		if fbr.ParentAgent.TLSCertificate == nil {
			err = errors.New("empty TLS certificate; unable to serve HTTPS")
			return
		}
		// Set the certificate in the TLS config for the server
		pc.httpServer.TLSConfig = &tls.Config{
			Certificates: []tls.Certificate{
				*fbr.ParentAgent.TLSCertificate,
			},
		}
		// ListenAndServeTLS will ignore the path strings as we have specified
		// the TLS config in the server object above, so these are empty.
		err = pc.httpServer.ListenAndServeTLS("", "")
	} else {
		// -- This responder is in HTTP mode.
		err = pc.httpServer.ListenAndServe()
	}
	// Report an error if the result was anything other than the server closing.
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
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
		logrus.Error("failed to read HTTP request body: " + err.Error())
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
