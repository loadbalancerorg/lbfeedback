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
	VersionString       string = "5.3.6"
	ProtocolHTTP        string = "http"
	ProtocolHTTPS       string = "https"
	ProtocolTCP         string = "tcp"
	ProtocolSecureAPI   string = "https-api"
	ProtocolLegacyAPI   string = "http-api"
	ResponderNameAPI    string = "api"
	ApplicationName     string = "Loadbalancer.org Feedback Agent"
	AppIdentifier       string = "lbfeedback"
	ServiceStateStopped int    = 1
	ServiceStateRunning int    = 2
	ServiceStateFailed  int    = 3
	LogFileName         string = "agent.log"
	ConfigFileName      string = "agent-config.json"
	LocalPathMode       bool   = false
	CopyrightYear       string = "2025"
	PanicDebug          bool   = false
	ForceAPISecure      bool   = true
	TLSCertExpiryHours  int    = 48
)

var ShellBanner string = `
     ▄ █           ` + ApplicationName + " v" + VersionString + `
     █ █ █▄▄       Copyright (C) ` + CopyrightYear + ` Loadbalancer.org Limited
     █ █ ▄ █       Licensed under the GNU General Public License v3

This program comes with ABSOLUTELY NO WARRANTY. This is free software, and 
you are welcome to redistribute it under certain conditions. For further
information, please read the LICENSE file distributed with this program.
`

var HelpText string = `SYNTAX:
  lbfeedback [action] [type] [parameters]

ACTIONS:
  run-agent: Runs the Agent interactively or from a startup script.
 
All other Actions are followed by an Action Type, as follows:
  add, edit, delete, start, restart, stop:
     monitor, responder, source
  get:
     config, feedback, sources
  set:
     commands, threshold
  force:
     halt, drain, online, save-config
  send:
     online, offline

Note that the running Agent service will automatically save any configuration
changes to its JSON configuration file if they are successful, and no service
restart is required as they are applied immediately.
  
PARAMETERS:
  -name               Name identifier of a service. 
                      For the 'force' and 'send' HAProxy command actions, 
                      omitting this parameter will apply the action to all 
                      Feedback Responders for which HAProxy commands are not 
                      disabled; see also '-command-list' below.
  -command-list       List of HAProxy commands to enable, space-separated.
                      Example: -command-list up down
                      These are automatically detected as pertaining to online
                      or offline states. There are special options as follows:
                      'none'    Disable all HAProxy commands.
                      'default' Send 'drain' for offline, 'up ready' for online.
  -protocol           Protocol name for a Responder. Options: 'tcp', 'http'.
  -ip                 Listen IP address for a Responder.
  -port               Port to listen on for a Responder.
                      'any'     Listen on all ports for the specified IP.
  -request-timeout    Request timeout (ms).
  -response-timeout   Response timeout (ms).
  -threshold-enabled  Enable HAProxy automatic command threshold (true/false).
  -threshold-min      Minimum availability for an online state (percent).
  -command-interval   Time interval to send HAProxy commands for (ms, 
                      default 10000).
  -monitor            Name identifier of a target Monitor.
  -significance       Significance value (floating-point; e.g. 1.0). This
                      is converted into a Relative Significance by summing
                      the significance of all sources within a Responder
                      and calculating their ratio.
  -max-value          Maximum value for a given metric against which to
                      scale its availability.
  -metric-type        Type of metric. Options: 'cpu', 'ram', 'disk-usage',
                      'netconn', 'script'.
  -sampling-ms        For 'cpu' metrics, the sample window duration (ms).
  -script-name        For 'script' metrics, the name of the script to run from
                      the Feedback Agent configuration directory.
  -disk-path          For 'disk-usage' metrics, the local filesystem path to
                      monitor for available disk space.

EXAMPLES:
   lbfeedback get config
   lbfeedback add monitor -name ram -metric-type ram
   lbfeedback add source -name default -monitor ram
   lbfeedback force offline -name default
                      
Please note that this is an extremely brief outline of the available
CLI configuration commands for controlling the Feedback Agent. For
further information, please consult the accompanying documentation or
contact Loadbalancer.org Support for assistance.`

// -------------------------------------------------------------------
// END OF FILE
// -------------------------------------------------------------------
