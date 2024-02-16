// constants.go
// Feedback Agent - Project Constants
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

const (
	VersionString          = "3.1.6-alpha"
	ServerProtocolHTTP     = "http"
	ServerProtocolTCP      = "tcp"
	MetricTypeCPU          = "cpu"
	MetricTypeRAM          = "ram"
	MetricTypeScript       = "script"
	AgentSignalQuit        = 1
	LogFileName            = "agent.log"
	ConfigFileName         = "agent-config.json"
	DefaultDirPermissions  = 0755
	DefaultFilePermissions = 0644
)

// -------------------------------------------------------------------
// END OF FILE
// -------------------------------------------------------------------
