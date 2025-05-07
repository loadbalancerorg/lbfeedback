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
	"errors"
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

// PointerHandleIntValue filters an input integer pointer, setting
// the return value to nil if the input is less than zero.
func PointerHandleIntValue(input *int) (output *int) {
	if input == nil || *input < 0 {
		output = nil
	} else {
		output = input
	}
	return
}

// PointerHandleInt64Value provides the same functionality as PointerHandleIntValue,
// but for 64-bit integer pointers.
func PointerHandleInt64Value(input *int64) (output *int64) {
	if input == nil || *input < 0 {
		output = nil
	} else {
		output = input
	}
	return
}

// PointerHandleStringValue filters an input string pointer, setting
// the output to nil if the string is empty.
func PointerHandleStringValue(input *string) (output *string) {
	if input != nil && strings.TrimSpace(*input) == "" {
		output = nil
	} else {
		output = input
	}
	return
}

// PointerHandleBoolString converts a string representation of a boolean
// state ("true" or "false") into a bool pointer type if is not nil. This
// is to provide a workaround for an annoying bug in the Go flags package.
func PointerHandleBoolString(input *string) (output *bool) {
	valueTrue := true
	valueFalse := false
	if input != nil {
		str := strings.ToLower(strings.TrimSpace(*input))
		if str == "true" {
			output = &valueTrue
		} else if str == "false" {
			output = &valueFalse
		}
	}
	return
}

// StandardiseNameIdentifier validates and standardises an object name identifier.
func StandardiseNameIdentifier(in string) (out string, err error) {
	// Sanitise the name string
	str := strings.ToLower(strings.TrimSpace(in))
	// If it's empty, return an error, otherwise return the string
	if str == "" {
		err = errors.New("name not specified")
	} else {
		out = str
	}
	return
}
