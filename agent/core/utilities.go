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

type NullWriter struct{}

func (NullWriter) Write([]byte) (int, error) {
	return 0, nil
}

func NewNullLogger() *log.Logger {
	return log.New(&NullWriter{}, "", log.LstdFlags)
}

func RemoveExtraSpaces(str string) (result string) {
	result = strings.Join(strings.Fields(str), " ")
	return
}

// Generates a random hex string for a specified number of bytes.
func RandomHexBytes(n int) (str string) {
	bytes := make([]byte, n)
	_, err := rand.Read(bytes)
	if err == nil {
		str = hex.EncodeToString(bytes)
	}
	return
}
