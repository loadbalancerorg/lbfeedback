// utilities.go
// General Utility Functions
//
// Project:     Loadbalancer.org Feedback Agent v5
// Author:      Nicholas Turnbull
//              <nicholas.turnbull@loadbalancer.org>
//
// Copyright (C) 2025 Loadbalancer.org Ltd
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
	"crypto/rand"
	"encoding/hex"
	"log"
	"strings"
)

// NullWriter is an I/O writer which does nothing, used for creating
// Loggers that don't log anywhere, so that log output can be selectively
// discarded without changing logging parameters globally.
type NullWriter struct{}

// Write accepts an array of bytes, and does precisely nothing with them.
func (NullWriter) Write([]byte) (int, error) {
	return 0, nil
}

// NewNullLogger returns a new Logger instance that outputs to a [NullWriter],
// which in turn does absolutely nothing.
func NewNullLogger() *log.Logger {
	return log.New(&NullWriter{}, "", log.LstdFlags)
}

// RemoveExtraSpaces converts any repeated spaces in a string into single spaces
// and discards any that are leading or trailing.
func RemoveExtraSpaces(str string) (result string) {
	result = strings.Join(strings.Fields(str), " ")
	return
}

// RandomHexBytes generates a random hex string that is a specified number of bytes long.
func RandomHexBytes(n int) (str string) {
	bytes := make([]byte, n)
	_, err := rand.Read(bytes)
	if err == nil {
		str = hex.EncodeToString(bytes)
	}
	return
}
