// api_schema.go
// Schema Definitions for the Feedback Agent API
//
// Project:     Loadbalancer.org Feedback Agent v5
// Author:      Nicholas Turnbull
//              <nicholas.turnbull@loadbalancer.org>
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

// APIRequest defines a request received from a client to the agent.
type APIRequest struct {
	// Global API request fields that apply to any request.
	APIKey     string `json:"api-key,omitempty"`
	ID         int    `json:"id,omitempty"`
	Action     string `json:"action,omitempty"`
	Type       string `json:"type,omitempty"`
	TargetName string `json:"target-name,omitempty"`

	// API fields for FeedbackResponder operations.
	ProtocolName     *string                     `json:"protocol,omitempty"`
	ListenIPAddress  *string                     `json:"ip,omitempty"`
	ListenPort       *string                     `json:"port,omitempty"`
	FeedbackSources  *map[string]*FeedbackSource `json:"feedback-sources,omitempty"`
	RequestTimeout   *int                        `json:"request-timeout,omitempty"`
	ResponseTimeout  *int                        `json:"response-timeout,omitempty"`
	CommandList      *string                     `json:"command-list,omitempty"`
	CommandInterval  *int                        `json:"command-interval,omitempty"`
	ThresholdEnabled *bool                       `json:"threshold-enabled,omitempty"`
	ThresholdScore   *int                        `json:"threshold-max,omitempty"`

	// API fields for SourceMonitor operations.
	SourceMonitorName  *string  `json:"monitor,omitempty"`
	SourceSignificance *float64 `json:"significance,omitempty"`
	SourceMaxValue     *int64   `json:"max-value,omitempty"`

	// API fields for SystemMonitor operations.
	MetricType     *string       `json:"metric-type,omitempty"`
	MetricInterval *int          `json:"interval-ms,omitempty"`
	MetricParams   *MetricParams `json:"metric-config,omitempty"`
}

// APIResponse defines a response to be sent from the agent to a client.
type APIResponse struct {
	APIName         string                     `json:"service-name"`
	Version         string                     `json:"version"`
	ID              *int                       `json:"id,omitempty"`
	Tag             string                     `json:"tag,omitempty"`
	Request         *APIRequest                `json:"request,omitempty"`
	Success         bool                       `json:"success"`
	Output          string                     `json:"output,omitempty"`
	Error           string                     `json:"error-name,omitempty"`
	Message         string                     `json:"message,omitempty"`
	AgentConfig     *FeedbackAgent             `json:"current-config,omitempty"`
	ServiceStatus   []APIServiceStatus         `json:"status,omitempty"`
	FeedbackSources map[string]*FeedbackSource `json:"feedback-sources,omitempty"`
}

type APIServiceStatus struct {
	ServiceType   string `json:"type"`
	ServiceName   string `json:"name"`
	ServiceStatus string `json:"status"`
}

type APIConfig struct {
	IPAddress string
	Port      string
	Key       string
}
