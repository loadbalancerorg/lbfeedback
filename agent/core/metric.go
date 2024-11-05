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
	"github.com/shirou/gopsutil/v3/disk"
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
	GetDefaultMax() float64
	GetMinInterval() int
}

func NewMetric(metric string, params MetricParams, configPath string) (
	mc SystemMetric, err error) {
	switch metric {
	case MetricTypeCPU:
		mc = &CPUMetric{}
	case MetricTypeRAM:
		mc = &MemoryMetric{}
	case MetricTypeDiskUsage:
		mc = &DiskUsageMetric{}
	case MetricTypeNetConnections:
		mc = &NetConnectionsMetric{}
	case MetricTypeScript:
		// For security, the script path is not included with the
		// [MetricParams] array so it can't be changed via the JSON
		// or an API call; this is set from the agent config directory.
		mc = &ScriptMetric{
			ScriptPath: configPath,
		}
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
	CPUMetricMinSampleTime = 500
	CPUMetricDefaultMax    = 100
	CPUMetricMinInterval   = 500
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

func (m *CPUMetric) GetDefaultMax() float64 {
	return CPUMetricDefaultMax
}

func (m *CPUMetric) GetMetricName() string {
	return MetricTypeCPU
}

func (m *CPUMetric) GetDescription() string {
	return "CPU"
}

func (m *CPUMetric) GetMinInterval() int {
	return CPUMetricMinInterval
}

// #################################
// MemoryMetric
// #################################

type MemoryMetric struct{}

const (
	MetricTypeRAM           = "ram"
	MemoryMetricDefaultMax  = 100
	MemoryMetricMinInterval = 500
)

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

func (m *MemoryMetric) GetDefaultMax() float64 {
	return MemoryMetricDefaultMax
}

func (m *MemoryMetric) GetMinInterval() int {
	return MemoryMetricMinInterval
}

// #################################
// ShellMetric
// #################################

type ScriptMetric struct {
	ScriptName string
	ScriptPath string
}

const (
	MetricTypeScript        = "script"
	ScriptMetricDefaultMax  = 100
	ScriptMetricMinInterval = 3000
	ParamKeyScriptName      = "script-name"
)

func (m *ScriptMetric) Configure(params MetricParams) (err error) {
	scriptName, err := GetParamValueString(ParamKeyScriptName, params)
	if err != nil {
		return
	}
	m.ScriptName = scriptName
	return
}

func (m *ScriptMetric) GetLoad() (val float64, err error) {
	var output string
	output, err = PlatformExecuteScript(path.Join(m.ScriptPath,
		m.ScriptName))
	if err == nil {
		output = strings.TrimSpace(output)
		var parsed float64
		parsed, err = strconv.ParseFloat(output, 64)
		val = float64(parsed)
	}
	return
}

func (m *ScriptMetric) GetMetricName() string {
	return MetricTypeScript
}

func (m *ScriptMetric) GetDescription() string {
	return "script '" + m.ScriptName + "'"
}

func (m *ScriptMetric) GetDefaultMax() float64 {
	return ScriptMetricDefaultMax
}
func (m *ScriptMetric) GetMinInterval() int {
	return ScriptMetricMinInterval
}

// #################################
// DiskUsageMetric
// #################################

type DiskUsageMetric struct {
	DiskPath string
}

const (
	MetricTypeDiskUsage  = "disk-usage"
	ParamKeyDiskPath     = "disk-path"
	DiskUsageDefaultMax  = 100
	DiskUsageMinInterval = 3000
)

func (m *DiskUsageMetric) Configure(params MetricParams) (err error) {
	diskPath, err := GetParamValueString(ParamKeyDiskPath, params)
	if err != nil {
		return
	}
	m.DiskPath = diskPath
	return
}

func (m *DiskUsageMetric) GetLoad() (val float64, err error) {
	stats, err := disk.Usage(m.DiskPath)
	if err != nil {
		return
	}
	val = stats.UsedPercent
	return
}

func (m *DiskUsageMetric) GetMetricName() string {
	return MetricTypeDiskUsage
}

func (m *DiskUsageMetric) GetDescription() string {
	return "disk-usage, path '" + m.DiskPath + "'"
}

func (m *DiskUsageMetric) GetDefaultMax() float64 {
	return DiskUsageDefaultMax
}

func (m *DiskUsageMetric) GetMinInterval() int {
	return DiskUsageMinInterval
}

// #################################
// NetConnectionsMetric
// #################################

type NetConnectionsMetric struct{}

const (
	MetricTypeNetConnections  = "netconn"
	NetConnectionsDefaultMax  = 2000
	NetConnectionsMinInterval = 3000
)

func (m *NetConnectionsMetric) Configure(params MetricParams) (err error) {
	return
}

func (m *NetConnectionsMetric) GetLoad() (val float64, err error) {
	intVal, err := PlatformGetConnnectionCount()
	if err != nil {
		return
	}
	val = float64(intVal)
	return
}

func (m *NetConnectionsMetric) GetMetricName() string {
	return MetricTypeNetConnections
}

func (m *NetConnectionsMetric) GetDescription() string {
	return "netconn"
}

func (m *NetConnectionsMetric) GetDefaultMax() float64 {
	return NetConnectionsDefaultMax
}

func (m *NetConnectionsMetric) GetMinInterval() int {
	return NetConnectionsMinInterval
}

// -------------------------------------------------------------------
// END OF FILE
// -------------------------------------------------------------------
