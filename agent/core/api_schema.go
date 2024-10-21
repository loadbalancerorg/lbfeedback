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

// Defines an [APIRequest] received from a client to the agent.
type APIRequest struct {
	// Global API request fields that apply to any request.
	ID      int    `json:"id,omitempty"`
	APIKey  string `json:"api-key,omitempty"`
	Action  string `json:"action,omitempty"`
	Service string `json:"service,omitempty"`
	Name    string `json:"name,omitempty"`

	// API fields for [FeedbackResponder] operations.
	SourceMonitorName *string `json:"monitor,omitempty"`
	ProtocolName      *string `json:"protocol,omitempty"`
	ListenIPAddress   *string `json:"ip,omitempty"`
	ListenPort        *string `json:"port,omitempty"`
	RequestTimeout    *int    `json:"request-timeout,omitempty"`
	ResponseTimeout   *int    `json:"response-timeout,omitempty"`
	// HAProxy command fields.
	HAProxyCommands  *bool `json:"send-commands,omitempty"`
	HAProxyThreshold *int  `json:"threshold-value,omitempty"`

	// API fields for [SystemMonitor] operations.
	MetricType *string       `json:"metric-type,omitempty"`
	Interval   *int          `json:"interval-ms,omitempty"`
	Params     *MetricParams `json:"metric-config,omitempty"`
}

// Defines an [APIResponse] to be sent from the agent to a client.
type APIResponse struct {
	APIName       string             `json:"service-name"`
	Version       string             `json:"version"`
	ID            *int               `json:"id,omitempty"`
	Tag           string             `json:"tag,omitempty"`
	Request       *APIRequest        `json:"request,omitempty"`
	Success       bool               `json:"success"`
	Output        string             `json:"output,omitempty"`
	Error         string             `json:"error-name,omitempty"`
	Message       string             `json:"message,omitempty"`
	AgentConfig   *FeedbackAgent     `json:"current-config,omitempty"`
	ServiceStatus []APIServiceStatus `json:"status,omitempty"`
}

type APIServiceStatus struct {
	ServiceType   string `json:"type"`
	ServiceName   string `json:"name"`
	ServiceStatus string `json:"status"`
}

// -------------------------------------------------------------------
// END OF FILE
// -------------------------------------------------------------------
