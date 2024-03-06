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
	"errors"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/sirupsen/logrus"
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

func GetParamValueString(key string, params MetricParams) (str string, err error) {
	value, exists := params[key]
	if exists {
		str = value
	} else {
		err = errors.New("missing parameter: '" + key + "'")
	}
	return
}

// #######################################################################
// SystemMetric
// #######################################################################

type MetricParams map[string]string

// [SystemMetric] defines a "metric" capable of reporting a load score
// (0.0-100.0, as a float) to the System Metric Monitor.
type SystemMetric interface {
	Configure(params MetricParams) (err error)
	GetLoad() (val float64, err error)
	GetMetricName() string
	GetDescription() string
}

func NewMetric(metric string, params MetricParams) (mc SystemMetric, err error) {
	switch metric {
	case MetricTypeCPU:
		mc = &CPUMetric{}
	case MetricTypeRAM:
		mc = &MemoryMetric{}
	case MetricTypeScript:
		mc = &ScriptMetric{}
	default:
		err = errors.New("unrecognised metric type: '" + metric + "'")
		return
	}
	err = mc.Configure(params)
	if err != nil {
		err = errors.New("configuration failed for metric type '" +
			metric + "': " + err.Error())
	}
	return
}

// #################################
// CPUMetric
// #################################

type CPUMetric struct {
	SampleTime uint64
}

const (
	MetricTypeCPU          = "cpu"
	ParamKeySampleTime     = "sampling-ms"
	CPUMetricMinSampleTime = 100
)

func (m *CPUMetric) Configure(params MetricParams) (err error) {
	defaultWarn := ""
	defaultSampleTime := false
	sampleTime, exists := params[ParamKeySampleTime]
	if !exists {
		defaultWarn += "no sample time specified"
	} else {
		timeInt, _ := strconv.Atoi(sampleTime)
		m.SampleTime = uint64(timeInt)
	}
	// Force a minimum sampling time for a CPU metric
	// to prevent an impact on system performance.
	if m.SampleTime < CPUMetricMinSampleTime {
		if defaultWarn == "" {
			defaultWarn = "sample time too low"
		}
		defaultSampleTime = true
	}
	if defaultSampleTime {
		defaultWarn += "; using default of " +
			strconv.Itoa(CPUMetricMinSampleTime) + "ms."
		m.SampleTime = CPUMetricMinSampleTime
		params[ParamKeySampleTime] = strconv.Itoa(CPUMetricMinSampleTime)
	}
	if defaultWarn != "" {
		logrus.Warn("Metric type '" + m.GetMetricName() +
			"': " + defaultWarn)
	}
	return
}

// Returns the current CPU metric for the host system.
func (m *CPUMetric) GetLoad() (float64, error) {
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

func (m *CPUMetric) GetMetricName() string {
	return MetricTypeCPU
}

func (m *CPUMetric) GetDescription() string {
	return "CPU"
}

// #################################
// MemoryMetric
// #################################

type MemoryMetric struct{}

const MetricTypeRAM = "ram"

func (m *MemoryMetric) Configure(params MetricParams) (err error) {
	return
}

func (m *MemoryMetric) GetLoad() (float64, error) {
	vmem, error := mem.VirtualMemory()
	return float64(vmem.UsedPercent), error
}

func (m *MemoryMetric) GetMetricName() string {
	return MetricTypeRAM
}

func (m *MemoryMetric) GetDescription() string {
	return "RAM"
}

// #################################
// ShellMetric
// #################################

type ScriptMetric struct {
	scriptName     string
	scriptFullPath string
}

const MetricTypeScript = "script"

func (m *ScriptMetric) Configure(params MetricParams) (err error) {
	scriptName, err := GetParamValueString("script-name", params)
	if err != nil {
		return
	}
	scriptPath, err := GetParamValueString("script-path", params)
	if err != nil {
		return
	}
	m.scriptFullPath = path.Join(scriptPath, scriptName)
	m.scriptName = scriptName
	return
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

func (m *ScriptMetric) GetMetricName() string {
	return MetricTypeScript
}

func (m *ScriptMetric) GetDescription() string {
	return "script '" + m.scriptName + "'"
}

// -------------------------------------------------------------------
// END OF FILE
// -------------------------------------------------------------------
