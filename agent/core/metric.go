// connector.go
// System Metrics for the System Monitor Service
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
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
)

// #################################
// Utility Functions
// #################################

// Calculates the mean of an array of float64 values.
func calculateMean(values []float64) float64 {
	var total float64
	for i := 1; i < len(values); i++ {
		total += values[i]
	}
	return float64(total / float64(len(values)))
}

// #######################################################################
// SystemMetric
// #######################################################################

// Defines a "metric" capable of reporting a load score (0-100) to the
// System Metric Monitor. The interface consists of just one function,
// which is to return the current load as a float64 value.
type SystemMetric interface {
	GetLoad() (val float64, err error)
	MetricName() string
}

// #################################
// CPUMetric
// #################################

type CPUMetric struct {
	SampleTime uint64 `json:"-"`
}

// Returns the current CPU metric for the host system.
func (m *CPUMetric) GetLoad() (float64, error) {
	// Force a minimum sampling time of at least 250ms for CPU
	// to prevent an impact on system performance.
	if m.SampleTime < 250 {
		m.SampleTime = 250
	}
	// Whilst the docs for gopsutil indicate that passing "false"
	// to the cpu.Percent() function should result in an overall
	// utilisation figure, it in fact seems to only reflect the
	// first core under Windows. To ensure compatibility, we
	// average this ourselves from the percentages returned.
	corePercentages, error := cpu.Percent(time.Duration(
		m.SampleTime*uint64(time.Millisecond)), true)
	if error == nil {
		return calculateMean(corePercentages), error
	} else {
		return 0.0, error
	}
}

func (m *CPUMetric) MetricName() string {
	return MetricTypeCPU
}

// #################################
// MemoryMetric
// #################################

type MemoryMetric struct{}

func (m *MemoryMetric) GetLoad() (float64, error) {
	vmem, error := mem.VirtualMemory()
	return float64(vmem.UsedPercent), error
}

func (m *MemoryMetric) MetricName() string {
	return MetricTypeRAM
}

// #################################
// ShellMetric
// #################################

type ScriptMetric struct {
	scriptFullPath string
}

func (m *ScriptMetric) GetLoad() (val float64, err error) {
	var output string
	output, err = PlatformExecuteScript(m.scriptFullPath)
	if err == nil {
		output = strings.TrimSpace(output)
		var parsedInt float64
		parsedInt, err = strconv.ParseFloat(output, 64)
		val = float64(parsedInt)
	}
	return
}

func (m *ScriptMetric) MetricName() string {
	return MetricTypeScript
}

// -------------------------------------------------------------------
// END OF FILE
// -------------------------------------------------------------------
