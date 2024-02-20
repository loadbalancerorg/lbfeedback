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
// load balancing applications. It provides a novel Z-score algorithm
// that achieves the best of both worlds between direct feedback
// where there is a 1-to-1 correspondence between the observed metric
// and the weight, and a moving average approach, by estimating the
// slope of the data when a statistically significant trend occurs.
// As it is cumulative, this model has an entirely static footprint.
type StatisticsModel struct {
	// The last value of x received as an observation.
	XLastValue float64
	// Count of observations in the current state (n).
	XCount uint64
	// The significance-adjusted mean of the observations.
	XAdjustedMean float64
	// Standard deviation (sigma_x) of the set, cumulatively.
	XStdDev float64
	// Z-score of the last observation (XValue)
	ZScoreValue float64
	// The current sum of x in the current state.
	XSum float64
	// The current sum of x^2 in the current state.
	XSquaredSum float64
	// Smallest observation value encountered.
	XMin float64
	// Largest observation value encountered.
	XMax float64
	// Maximum observations before a recentre is forced.
	XCountLimit uint64
	// Current sum of Z-scores within the current Z-window.
	ZScoreSum float64
	// Current mean of Z-scores within the current Z-window.
	ZScoreMean float64
	// Current count of Z-scores comprising the mean Z-score.
	ZSampleCount uint64
	// The 2-tailed Z-value threshold for hypothesis significance;
	// a ZMeanThreshold of 1.0 is treated as (-1.0, +1.0) either side
	// of the mean; e.g a value of 1.0 is a range of 2 sigma.
	ZMeanThreshold float64
	// Number of Z-window samples to take before deciding significance.
	// If zero and a ZMeanThreshold is set, significance will be decided
	// after a single observation.
	ZPredictionInterval uint64
	// Was this model recentred during the last observation?
	Recentred bool
	// Is statistics calculation disabled (for direct mode)?
	StatsDisabled bool
	// Maximum value for a returned weight score
	WeightCeiling int64
	// Minimum value for a returned weight score
	WeightFloor int64
	// The scaling factor for the raw metric used to convert it into
	// the weight score range, prior to integer conversion.
	WeightScalingFactor float64
	// Should a "higher" score of the metric result in a "better" score?
	InverseWeight bool
	// Have the model parameters been set, so we don't force to defaults?
	ParamsSet bool
	// The last weight score computed by the model.
	weightScore int64
}

// Default parameters for model values, which are the minimum required
// for the model to function sensibly. The SetDefaultParams() function
// below assigns these to the parameters, which is called by the
// addObservation() function if the ParamsSet flag is unset. This
// is intended to ensure that blank values are not inadvertently set
// during normal model operation by a caller causing nonsensical output.

const (
	DefaultXCountLimit         = 0x100000000
	DefaultWeightCeiling       = 99
	DefaultWeightFloor         = 0
	DefaultWeightScalingFactor = 1.0
	DefaultZMeanThreshold      = 1.0
	DefaultZPredictionInterval = 5
)

// Sets the default model parameters, and also sets the flag
// to tell NewValue() that a set of parameters has been
// configured, which will thereafter stop it from calling again.
func (model *StatisticsModel) SetDefaultParams() {
	model.XCountLimit = DefaultXCountLimit
	model.WeightCeiling = DefaultWeightCeiling
	model.WeightFloor = DefaultWeightFloor
	model.WeightScalingFactor = DefaultWeightScalingFactor
	model.ZMeanThreshold = DefaultZMeanThreshold
	model.ZPredictionInterval = DefaultZPredictionInterval
	model.ParamsSet = true
}

// Resets to a zero-state for this statistics model without clearing
// the configuration parameters.
func (model *StatisticsModel) ClearModel() {
	model.XLastValue = 0
	model.XCount = 0
	model.XAdjustedMean = 0
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

// Observes a new value in the set into the statistics model
// by adding it into the sum values and recalculating the Z-scores,
// min-max values, mean and standard deviation.
func (model *StatisticsModel) NewValue(value float64) {
	if !model.ParamsSet {
		model.SetDefaultParams()
	}
	if !model.StatsDisabled {
		model.Recentred = false
		// If the observation count (n) has exceeded the defined limit,
		// recentre the model around the current mean.
		if model.XCount+1 > model.XCountLimit {
			model.RecentreModel()
		} else {
			// Recalculate statistics within the model, in the right order.
			model.addXValue(value)
			model.updateMinMax()
			model.recalcMean()
			model.recalcStdDev()
			model.recalcZScores()
		}
		// Perform the Z-window operations and if necessary, recentre model.
		model.handleZWindow()
	} else {
		model.XAdjustedMean = value
	}
	// Compute the weight score from the current data
	model.calculateWeightScore()
}

// Takes a new value and adds it to the appropriate sum fields
// as well as the value field (the observation x) used by the
// other functions.
func (model *StatisticsModel) addXValue(value float64) {
	model.XSum += value
	model.XSquaredSum += math.Pow(value, 2)
	model.XCount++
	model.XLastValue = value
}

// Updates the Z-score parameters based on the current state.
// Formula:
//
// z = (x - mu) / sigma
//
// where z equals the number of standard deviations from the mean, x
// is the most recent observation value, mu is the current mean of the
// population and sigma is the sample standard deviation.
func (model *StatisticsModel) recalcZScores() {
	if math.Abs(model.XStdDev) > 0 {
		model.ZScoreValue = (model.XLastValue - model.XAdjustedMean) /
			model.XStdDev
	} else {
		model.ZScoreValue = 0
	}
	model.ZSampleCount++
	model.ZScoreSum += model.ZScoreValue
	model.ZScoreMean = model.ZScoreSum / float64(model.ZSampleCount)
}

// Updates the min/max values if the most recent observation has
// required these to change.
func (model *StatisticsModel) updateMinMax() {
	// If we haven't got at least 2 values yet (e.g. 1 or 0)
	// then the min and max are the last value seen.
	if model.XCount < 2 {
		model.XMin = model.XLastValue
		model.XMax = model.XLastValue
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

// Updates the mean in the current state.
// Formula:
//
//	mu_n = s1 / n
//
// where mu_n is the cumulative mean, s1 is the sum of observations,
// and n is the sample count of the set.
func (model *StatisticsModel) recalcMean() {
	model.XAdjustedMean = model.XSum / float64(model.XCount)
}

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
func (model *StatisticsModel) recalcStdDev() {
	model.XStdDev = math.Sqrt((model.XSquaredSum /
		float64(model.XCount)) -
		math.Pow(model.XSum/float64(model.XCount), 2))
}

// Provides the logic and calculations necessary to operate the
// moving Z-window approach, where the model is recentred around
// the new value of a significant Z-mean detected following a
// specified number of observations. This forces the weights for
// this Real Server to be updated even if the mean has not yet
// caught up to match the newly-significant data.
func (model *StatisticsModel) handleZWindow() {
	// Don't do any work at all if no Z-mean threshold is specified
	// or if we haven't achieved a minimum number of observations.
	if math.Abs(model.ZMeanThreshold) > 0 {
		// Act on the result if the Z-sample count has been reached.
		if model.ZSampleCount >= model.ZPredictionInterval {
			if math.Abs(model.ZScoreMean) >= model.ZMeanThreshold {
				// The null hypothesis is refuted if n observations
				// have taken place yielding values that are, on
				// average, greater than the required number of standard
				// deviations away from the mean. That is, we consider
				// this to be significant based on our model.
				// Translate the Z-mean into the adjusted mean.
				model.XAdjustedMean += model.ZScoreMean * model.XStdDev
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
}

// Computes a weight score (e.g. a RIP weighting for HAProxy) based on
// the parameters configured, which can be made to behave in various ways
// based on application requirements. If the weight parameters are set
// to default, this will return a score between 0-100 representing the
// "positive" score (e.g. how "good" this node is to use). If the metric
// in use is inverted (higher means worse), the InverseWeight switch
// should be set in the model to true to accomplish this behaviour.
func (model *StatisticsModel) calculateWeightScore() {
	fraction := (model.XAdjustedMean / 100.0)
	if fraction < 0.0 {
		fraction = 0.0
	} else if fraction > 1.0 {
		fraction = 1.0
	}
	// Scale the value based on the range of the minimum and maximum
	score := int64(math.Floor(fraction*float64(model.WeightCeiling-
		model.WeightFloor))) + model.WeightFloor
	if !model.InverseWeight {
		score = model.WeightCeiling - score
	}
	model.weightScore = score
}

// Accessor for the weight score.
func (model *StatisticsModel) GetWeightScore() int64 {
	// "Fail safe" to avoid offlining Real Servers in the event that we
	// do not yet have enough data to provide a score, by returning
	// the maximum score unless we have at least one observation.
	if model.XCount < 1 {
		return model.WeightCeiling
	} else {
		return model.weightScore
	}
}

// Returns whether or not this model has any data yet to calculate.
func (model *StatisticsModel) HasObservations() bool {
	return (model.XCount > 0)
}

// Consolidates the current parameter state in the series
// into a single observation by recentring the statistics.
func (model *StatisticsModel) RecentreModel() {
	model.recentreMean()
	model.recentreZStats()
	model.Recentred = true
}

// Recentres the X-statistics around the "set point" of the
// new X-mean (mu_x).
func (model *StatisticsModel) recentreMean() {
	model.XCount = 1
	model.XSum = model.XAdjustedMean
	model.XSquaredSum = math.Pow(model.XAdjustedMean, 2)
}

// Recentres the Z-statistics around the new Z-mean - that is,
// the centre of our current predicted distribution will now
// equal mu_sigma.
func (model *StatisticsModel) recentreZStats() {
	model.ZSampleCount = 1
	model.ZScoreSum = model.ZScoreValue
	model.ZScoreMean = model.ZScoreValue
}

// -------------------------------------------------------------------
// END OF FILE
// -------------------------------------------------------------------
