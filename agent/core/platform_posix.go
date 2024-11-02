//go:build linux || freebsd || netbsd || openbsd || darwin
// +build linux freebsd netbsd openbsd darwin

// platform_posix.go
// Platform-Specific Code - POSIX Operating Systems
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

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"github.com/shirou/gopsutil/v3/net"
)

const (
	DefaultDirPermissions  fs.FileMode = 0755
	DefaultFilePermissions fs.FileMode = 0644

	// Platform specific paths
	DefaultConfigDir = "/opt/lbfeedback"
	DefaultLogDir    = "/var/log/lbfeedback"

	ExitStatusNormal = 0
	ExitStatusError  = 1
)

func PlatformMain() (exitStatus int) {
	if len(os.Args) > 1 && strings.TrimSpace(os.Args[1]) == "run-agent" {
		// We are in the agent daemon personality.
		exitStatus = LaunchAgentService()
	} else {
		// We are in the API client personality.
		exitStatus = RunClientCLI()
	}
	return
}

func (agent *FeedbackAgent) PlatformConfigureSignals() {
	agent.systemSignals = make(chan os.Signal, 1)
	agent.restartSignal = syscall.SIGHUP
	agent.quitSignal = syscall.SIGQUIT
	signal.Notify(agent.systemSignals, syscall.SIGHUP, syscall.SIGINT,
		syscall.SIGQUIT, syscall.SIGTERM)

}

func PlatformPrintRunInstructions() {
	fmt.Println("To run the Agent (either interactively or from " +
		"a startup script), \n" +
		"  use the 'run-agent' command.")
}

func PlatformExecuteScript(fullPath string) (out string, err error) {
	var bytes []byte
	bytes, err = exec.Command("bash", "-c", fullPath).Output()
	out = string(bytes)
	return
}

func PlatformOpenLogFile(fullPath string) (file *os.File, err error) {
	file, err = os.OpenFile(fullPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		DefaultFilePermissions)
	return
}

func PlatformGetConnnectionCount() (val int, err error) {
	connList, err := net.Connections("all")
	if err != nil {
		return
	}
	val = len(connList)
	return
}

func PlatformPrintHelpMessage() {
	fmt.Println(HelpText)
}

// -------------------------------------------------------------------
// END OF FILE
// -------------------------------------------------------------------
