// statmodel.go
// Feedback Agent Statistics Model
//
// Project:		Loadbalancer.org Feedback Agent v5
// Author: 		Nicholas Turnbull
//				<nicholas.turnbull@loadbalancer.org>
// Revision:	1049 (2024-02-15)
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
	"math"
)

// StatisticsModel provides a cumulative-mode calculation engine that
// receives observations from a given system metric, and performs
// configurable trend analysis on these metrics to detect changes in
// the metric and calculate the appropriate weight score for use in
// load balancing applications. It provides a Z-score algorithm
// that aims to achieve the best of both worlds between direct feedback
// where there is a 1-to-1 correspondence between the observed metric
// and the weight, and a moving average approach, by estimating the
// slope of the data when a statistically significant trend occurs.
// As it is cumulative, this model has an entirely static footprint.
type StatisticsModel struct {
	// The last value of x received as an observation.
	XLastValue float64 `json:"-"`
	// Count of observations in the current state (n).
	XCount uint64 `json:"-"`
	// The significance-adjusted mean of the observations,
	// where shaping is enabled.
	XReportedLoad float64 `json:"-"`
	// Standard deviation (sigma_x) of the set, cumulatively.
	XStdDev float64 `json:"-"`
	// Z-score of the last observation (XValue)
	ZScoreValue float64 `json:"-"`
	// The current sum of x in the current state.
	XSum float64 `json:"-"`
	// The current sum of x^2 in the current state.
	XSquaredSum float64 `json:"-"`
	// Smallest observation value encountered.
	XMin float64 `json:"-"`
	// Largest observation value encountered.
	XMax float64 `json:"-"`
	// Maximum observations before a recentre is forced.
	XCountLimit uint64 `json:"-"`
	// Current sum of Z-scores within the current Z-window.
	ZScoreSum float64 `json:"-"`
	// Current mean of Z-scores within the current Z-window.
	ZScoreMean float64 `json:"-"`
	// Current count of Z-scores comprising the mean Z-score.
	ZSampleCount uint64 `json:"-"`
	// The 2-tailed Z-value threshold for hypothesis significance;
	// a ZMeanThreshold of 1.0 is treated as (-1.0, +1.0) either side
	// of the mean; e.g a value of 1.0 is a range of 2 sigma.
	ZMeanThreshold float64 `json:"z-threshold"`
	// Number of Z-window samples to take before deciding significance.
	// If zero and a ZMeanThreshold is set, significance will be decided
	// after a single observation.
	ZPredictionInterval uint64 `json:"z-interval"`
	// Was this model recentred during the last observation?
	Recentred bool `json:"-"`
	// Is statistics-based shaping enabled?
	ShapingEnabled bool `json:"-"`
	// Have the model parameters been set, so we don't force to defaults?
	ParamsSet bool `json:"-"`
	// The last weight score computed by the model.
	LastResult int64 `json:"-"`
}

// Default parameters for model values, which are the minimum required
// for the model to function sensibly. The SetDefaultParams() function
// below assigns these to the parameters, which is called by the
// addObservation() function if the ParamsSet flag is unset. This
// is intended to ensure that blank values are not inadvertently set
// during normal model operation by a caller causing nonsensical output.

const (
	DefaultXCountLimit         = 0x10000000
	DefaultZMeanThreshold      = 1.0
	DefaultZPredictionInterval = 5
)

// SetDefaultParams sets the default model parameters, and also sets
// the flag to tell NewValue that a set of parameters has been
// configured, which will thereafter stop it from calling again.
func (model *StatisticsModel) SetDefaultParams() {
	model.XCountLimit = DefaultXCountLimit
	model.ZMeanThreshold = DefaultZMeanThreshold
	model.ZPredictionInterval = DefaultZPredictionInterval
	model.ParamsSet = true
}

// ClearModel resets to a zero-state for this statistics model
// without clearing the configuration parameters.
func (model *StatisticsModel) ClearModel() {
	model.XLastValue = 0
	model.XCount = 0
	model.XReportedLoad = 0
	model.XStdDev = 0
	model.ZScoreValue = 0
	model.XSum = 0
	model.XSquaredSum = 0
	model.XMin = 0
	model.XMax = 0
	model.ZScoreSum = 0
	model.ZScoreMean = 0
	model.ZSampleCount = 0
}

// NewValue observes a new value in the set into the statistics model
// by adding it into the sum values and recalculating the Z-scores,
// min-max values, mean and standard deviation.
func (model *StatisticsModel) NewValue(value float64) {
	if !model.ParamsSet {
		model.SetDefaultParams()
	}
	// If the observation count (n) has exceeded the defined limit,
	// recentre the statistics model around the current mean.
	if model.XCount+1 > model.XCountLimit {
		model.RecentreModel()
	} else {
		model.Recentred = false
		// Recalculate statistics within the model, in the right order.
		model.addXValue(value)
		model.updateMinMax()
		model.recalculateMean()
		model.recalculateStdDev()
		model.recalculateZScores()
	}
	if model.ShapingEnabled {
		// Perform the Z-window translation algorithm.
		model.handleZWindow()
	} else {
		// Otherwise, if shaping is disabled, the adjusted mean is the last value.
		model.XReportedLoad = value
	}
	model.setResult()
}

// addXValue takes a new value and adds it to the appropriate sum fields
// as well as the value field (the observation x) used by the
// other functions.
func (model *StatisticsModel) addXValue(value float64) {
	model.XSum += value
	model.XSquaredSum += math.Pow(value, 2)
	model.XCount++
	model.XLastValue = value
}

// recalculateZScores updates the Z-score parameters based on the current state.
func (model *StatisticsModel) recalculateZScores() {
	// Formula:
	//
	// z = (x - mu) / sigma
	//
	// where z equals the number of standard deviations from the mean, x
	// is the most recent observation value, mu is the current mean of the
	// population and sigma is the sample standard deviation.
	if math.Abs(model.XStdDev) > 0 {
		model.ZScoreValue = (model.XLastValue - model.XReportedLoad) /
			model.XStdDev
	} else {
		model.ZScoreValue = 0
	}
	model.ZSampleCount++
	model.ZScoreSum += model.ZScoreValue
	model.ZScoreMean = model.ZScoreSum / float64(model.ZSampleCount)
}

// updateMinMax updates the min/max values if the most recent
// observation has required these to change.
func (model *StatisticsModel) updateMinMax() {
	// If we haven't got at least 2 values yet (e.g. 1 or 0)
	// then the min and max are the last value seen.
	if model.XCount < 2 {
		model.resetMinMax()
	} else {
		// Otherwise, set these based on the new value.
		if model.XMin > model.XLastValue {
			model.XMin = model.XLastValue
		}
		if model.XMax < model.XLastValue {
			model.XMax = model.XLastValue
		}
	}
}

// resetMinMax sets the current minimum and maximum to the last
// observed value in the model.
func (model *StatisticsModel) resetMinMax() {
	model.XMin = model.XLastValue
	model.XMax = model.XLastValue
}

// recalculateMean computes the mean in the current state
// using the cumulative method.
func (model *StatisticsModel) recalculateMean() {
	// Formula:
	//
	//	mu_n = s1 / n
	//
	// where mu_n is the cumulative mean, s1 is the sum of observations,
	// and n is the sample count of the set.
	model.XReportedLoad = model.XSum / float64(model.XCount)
}

func (model *StatisticsModel) recalculateStdDev() {
	// Calculate standard deviation based on the current series model
	// using the cumulative method, which does not require storage of
	// the datum points.
	// Formula:
	//
	// sigma_n = sqrt((s2 / n) - (s1 / n) ^ 2)
	//
	// where s1 is the sum of observations, s2 is the sum of squares
	// n is the sample count of the set and sigma_n is the standard
	// deviation.
	model.XStdDev = math.Sqrt((model.XSquaredSum /
		float64(model.XCount)) -
		math.Pow(model.XSum/float64(model.XCount), 2))
}

// handleZWindow provides the logic and calculations necessary
// to operate the  moving Z-window approach, where the model is
// recentred around the new value of a significant Z-mean detected
// following a specified number of observations. This forces the
// weights for this Real Server to be updated even if the mean has
// not yet caught up to match the newly-significant data.
func (model *StatisticsModel) handleZWindow() {
	// Don't do any work at all if no Z-mean threshold is specified
	// or if we haven't achieved a minimum number of observations.
	if math.Abs(model.ZMeanThreshold) > 0 &&
		model.ZSampleCount >= model.ZPredictionInterval {
		if math.Abs(model.ZScoreMean) >= model.ZMeanThreshold {
			// The null hypothesis is refuted if n observations
			// have taken place yielding values that are, on
			// average, greater than the required number of standard
			// deviations away from the mean. That is, we consider
			// this to be significant based on our model.
			// Translate the Z-mean into the adjusted mean.
			result := model.XReportedLoad + (model.ZScoreMean * model.XStdDev)
			// Constrain the resulting X-mean to be within the boundaries
			// of the min-max observations seen so far (to prevent overshoot/
			// undershoot of the result).
			if result < model.XMin {
				result = model.XMin
			} else if result > model.XMax {
				result = model.XMax
			}
			model.XReportedLoad = result
			model.RecentreModel()
		} else {
			// The null hypothesis cannot be refuted if the result
			// of n observations yielded a Z-mean that does not
			// meet the required threshold for significance; thus
			// we recentre the Z-stats only and not the entire
			// model as this sample was not significant.
			model.recentreZStats()
		}
	}
}

// SetResult sets the last result obtained in the model.
func (model *StatisticsModel) setResult() {
	model.LastResult = int64(math.Round(model.XReportedLoad))
}

// GetResult returns the weight score.
func (model *StatisticsModel) GetResult() int64 {
	return model.LastResult
}

// HasObservations returns if this model has any data yet to calculate.
func (model *StatisticsModel) HasObservations() bool {
	return model.XCount > 0
}

// RecentreModel consolidates the current parameter state in the series
// into a single observation by recentring the statistics.
func (model *StatisticsModel) RecentreModel() {
	model.recentreMean()
	model.recentreZStats()
	model.resetMinMax()
	model.Recentred = true
}

// recentreMean recentres the X-statistics around the "set point" of the
// new X-mean (mu_x).
func (model *StatisticsModel) recentreMean() {
	model.XCount = 1
	model.XSum = model.XReportedLoad
	model.XSquaredSum = math.Pow(model.XReportedLoad, 2)
}

// recentreZStats recentres the Z-statistics around the new Z-mean -
// that is, the centre of our current predicted distribution will now
// equal mu_sigma.
func (model *StatisticsModel) recentreZStats() {
	model.ZSampleCount = 1
	model.ZScoreSum = model.ZScoreValue
	model.ZScoreMean = model.ZScoreValue
}

// -------------------------------------------------------------------
// END OF FILE
// -------------------------------------------------------------------
