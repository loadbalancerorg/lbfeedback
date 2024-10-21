// constants.go
// Feedback Agent: Project Build Settings and Constants
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

const (
	VersionString       string = "5.2.1-beta"
	ProtocolHTTP        string = "http"
	ProtocolTCP         string = "tcp"
	ProtocolAPI         string = "http-api"
	APIName             string = "lbfeedback-api"
	ServiceStateStopped int    = 1
	ServiceStateRunning int    = 2
	ServiceStateFailed  int    = 3
	LogFileName         string = "agent.log"
	ConfigFileName      string = "agent-config.json"
	LocalPathMode       bool   = false
	CopyrightYear       string = "2024"
)

var ShellBanner string = `
     ▄ █           Loadbalancer.org Feedback Agent v` + VersionString + `
     █ █ █▄▄       Copyright (C) ` + CopyrightYear + ` Loadbalancer.org Limited
     █ █ ▄ █       Licensed under the GNU General Public License v3

This program comes with ABSOLUTELY NO WARRANTY. This is free software, and 
you are welcome to redistribute it under certain conditions. For further
information, please read the LICENSE file distributed with this program.
`

// -------------------------------------------------------------------
// END OF FILE
// -------------------------------------------------------------------
