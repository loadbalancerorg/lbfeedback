// lbfeedback.go
// Feedback Agent: Main Launcher
//
// Project:		Loadbalancer.org Feedback Agent v5
// Author: 		Nicholas Turnbull
//				<nicholas.turnbull@loadbalancer.org>
// Revision:	1049 (2024-02-15)
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

package main

import (
	"fmt"
	"os"
	"strings"

	agent "github.com/loadbalancerorg/lbfeedback/agent/core"
)

var ExitStatus int = 0

// The main() function for building the CLI/service binary.
func main() {
	// Defer recovering any panics and terminating the agent.
	if len(os.Args) > 1 && strings.TrimSpace(os.Args[1]) == "run-agent" {
		// We are in the service personality.
		defer func() {
			err := recover()
			if err != nil {
				fmt.Println("Internal error occurred: ", err)
			}
			os.Exit(ExitStatus)
		}()
		ExitStatus = agent.LaunchAgentService()
	} else {
		// We are in the API client personality.
		ExitStatus = agent.RunClientCLI()
	}
}

// -------------------------------------------------------------------
// END OF FILE
// -------------------------------------------------------------------
