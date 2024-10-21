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
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

const (
	DefaultDirPermissions  fs.FileMode = 0755
	DefaultFilePermissions fs.FileMode = 0644

	// Platform specific paths
	ConfigDir = "/opt/lbfeedback"
	LogDir    = "/var/log/lbfeedback"

	ExitStatusNormal     = 0
	ExitStatusParamError = 1
)

func (agent *FeedbackAgent) PlatformConfigureSignals() {
	agent.systemSignals = make(chan os.Signal, 1)
	agent.restartSignal = syscall.SIGHUP
	agent.quitSignal = syscall.SIGQUIT
	signal.Notify(agent.systemSignals, syscall.SIGHUP, syscall.SIGINT,
		syscall.SIGQUIT, syscall.SIGTERM)

}

func PlatformExecuteScript(fullPath string) (out string, err error) {
	//logrus.Debug("PlatformExecuteScript: called")
	var bytes []byte
	bytes, err = exec.Command("bash", "-c", fullPath).Output()
	out = string(bytes)
	//logrus.Debug("PlatformExecuteScript: output=" + out)
	return
}

func PlatformOpenLogFile(fullPath string) (file *os.File, err error) {
	file, err = os.OpenFile(fullPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		DefaultFilePermissions)
	return
}

// -------------------------------------------------------------------
// END OF FILE
// -------------------------------------------------------------------
