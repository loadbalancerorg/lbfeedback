// connector.go
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
	"net"
	"net/http"
	"strconv"
)

// #######################################################################
// ProtocolConnector
// #######################################################################

type ProtocolConnector interface {
	Listen(fbr *FeedbackResponder) (err error)
	Close() (err error)
}

func NewProtocolConnector(protocol string) (conn ProtocolConnector, err error) {
	switch protocol {
	case ServerProtocolHTTP:
		conn = &HTTPConnector{}
	case ServerProtocolTCP:
		conn = &TCPConnector{}
	default:
		err = errors.New("cannot create responder: invalid protocol '" +
			protocol + "' specified")
	}
	return
}

// #################################
// TCPListener
// #################################

type TCPConnector struct {
	tcpListener net.Listener       `json:"-"`
	responder   *FeedbackResponder `json:"-"`
}

func (pc *TCPConnector) Listen(fbr *FeedbackResponder) (err error) {
	pc.responder = fbr
	pc.tcpListener, err = net.Listen("tcp4", ":"+fmt.Sprint(fbr.ListenPort))
	if err != nil {
		return
	}
	var conn net.Conn

	for err == nil {
		// Accept() will block here until the listener is closed.
		conn, err = pc.tcpListener.Accept()
		if conn != nil {
			go pc.handleRequest(conn)
		}
	}
	return
}

func (pc *TCPConnector) handleRequest(c net.Conn) {
	fmt.Fprintf(c, "%s", pc.responder.getFeedbackString())
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
// HTTPListener
// #################################

type HTTPConnector struct {
	httpServer *http.Server       `json:"-"`
	responder  *FeedbackResponder `json:"-"`
}

func (pc *HTTPConnector) Listen(fbr *FeedbackResponder) (err error) {
	pc.responder = fbr
	pc.httpServer = &http.Server{
		Addr:         ":" + strconv.Itoa(fbr.ListenPort),
		Handler:      http.HandlerFunc(pc.handleRequest),
		ReadTimeout:  fbr.RequestTimeout,
		WriteTimeout: fbr.ResponseTimeout,
	}
	// ListenAndServe() will block here until the server returns an
	// error. As we have unlocked the mutex, Stop() will be able to
	// call the method on the HTTP server to tell it to stop.
	err = pc.httpServer.ListenAndServe()
	return
}

func (pc *HTTPConnector) handleRequest(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "%s", pc.responder.getFeedbackString())
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
