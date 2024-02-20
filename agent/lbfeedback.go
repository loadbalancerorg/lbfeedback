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

	agent "github.com/loadbalancerorg/lbfeedback/agent/core"
)

var TerminalBanner string = `
     ▄ █           Loadbalancer.org Feedback Agent Service
     █ █ █▄▄       Copyright (C) 2024 Loadbalancer.org Limited
     █ █ ▄ █       Licensed under the GNU General Public License v3

This program comes with ABSOLUTELY NO WARRANTY. This is free software, and 
you are welcome to redistribute it under certain conditions. For further
information, please read the LICENSE file distributed with this program.
`

// Main function for compiling the CLI/service binary.
func main() {
	agent := agent.FeedbackAgent{}
	fmt.Println(TerminalBanner)
	agent.Run()
}

// -------------------------------------------------------------------
// END OF FILE
// -------------------------------------------------------------------
