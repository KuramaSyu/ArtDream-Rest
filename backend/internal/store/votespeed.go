package store

import (
	"math"
	"sort"
)

// Dynamic vote-speed calibration. A voter's response time is interpreted
// relative to their own history: a fast decision is an obvious one and should
// count with full confidence, while a slow decision means the two images were
// close and counts less. The speed scale is 1 (slowest) to 10 (fastest) with
// 5.5 as the neutral default.
const (
	// VoteSpeedMin and VoteSpeedMax bound the stored vote_speed value.
	VoteSpeedMin = 1.0
	VoteSpeedMax = 10.0
	// VoteSpeedNeutral leaves the vote weight unchanged.
	VoteSpeedNeutral = 5.5
	// VoteSpeedCalibrationVotes is the number of votes a voter needs before
	// their own timing baseline is trusted. Earlier votes stay neutral but
	// still provide timing samples.
	VoteSpeedCalibrationVotes = 10
	// VoteSpeedTrimFraction drops the slowest and fastest share of samples so
	// outliers (a voter leaving the page open) do not define the scale.
	VoteSpeedTrimFraction = 0.10
	// VoteSpeedMaxFactor is the weight multiplier of the fastest vote; the
	// slowest gets VoteSpeedMin/VoteSpeedNeutral. Neutral (5.5) means 1.
	VoteSpeedMaxFactor = VoteSpeedMax / VoteSpeedNeutral
)

// VoteSpeedFor derives the speed score of a new vote from the voter's previous
// durations. durationMs is the current vote's response time.
func VoteSpeedFor(priorVotes int, durations []int64, durationMs *int64) float64 {
	if durationMs == nil || *durationMs < 0 {
		return VoteSpeedNeutral
	}
	if priorVotes < VoteSpeedCalibrationVotes {
		return VoteSpeedNeutral
	}
	samples := trimDurations(durations, VoteSpeedTrimFraction)
	if len(samples) < 2 {
		return VoteSpeedNeutral
	}
	fastest, slowest := samples[0], samples[len(samples)-1]
	if slowest <= fastest {
		return VoteSpeedNeutral
	}
	ms := *durationMs
	if ms < fastest {
		ms = fastest
	}
	if ms > slowest {
		ms = slowest
	}
	ratio := float64(ms-fastest) / float64(slowest-fastest)
	speed := VoteSpeedMax - (VoteSpeedMax-VoteSpeedMin)*ratio
	if speed < VoteSpeedMin {
		speed = VoteSpeedMin
	}
	if speed > VoteSpeedMax {
		speed = VoteSpeedMax
	}
	return math.Round(speed*1000) / 1000
}

// trimDurations drops the lowest and highest fraction of the samples and
// returns the remaining durations sorted ascending.
func trimDurations(durations []int64, fraction float64) []int64 {
	samples := make([]int64, 0, len(durations))
	for _, d := range durations {
		if d >= 0 {
			samples = append(samples, d)
		}
	}
	if len(samples) == 0 {
		return samples
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	cut := int(float64(len(samples)) * fraction)
	if len(samples)-2*cut < 1 {
		cut = 0
	}
	return samples[cut : len(samples)-cut]
}

// EffectiveWeight applies the speed score to the raw judgment weight. Fast
// votes count with more confidence (up to VoteSpeedMaxFactor), slow ones are
// discounted proportionally. The neutral speed (5.5) leaves the weight
// untouched, so older votes and votes without timing data behave exactly as
// before.
func (v *Vote) EffectiveWeight() float64 {
	if v == nil {
		return 0
	}
	speed := v.VoteSpeed
	if speed <= 0 {
		speed = VoteSpeedNeutral
	}
	factor := speed / VoteSpeedNeutral
	if factor > VoteSpeedMaxFactor {
		factor = VoteSpeedMaxFactor
	}
	if factor < 0 {
		factor = 0
	}
	return v.Weight * factor
}
