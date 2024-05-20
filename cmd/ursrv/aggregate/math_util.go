package aggregate

import (
	"math"
	"slices"

	"github.com/syncthing/syncthing/lib/ur"
	"golang.org/x/exp/constraints"
)

const (
	percentileThreshold = 100
	floatPrecision      = 6
	noPrecision         = 0
)

type Numerical interface {
	constraints.Integer | constraints.Float
}

// Average returns the average of the slice values.
func Average[E Numerical, S ~[]E](s S, precision int) E {
	var total E = 0
	if len(s) == 0 {
		return 0
	}

	for _, value := range s {
		if s, ok := any(value).(float64); ok {
			if math.IsNaN(s) {
				continue
			}
		}
		total += value
	}

	result := float64(total) / float64(len(s)) // total / len(s)

	if precision == 0 {
		return E(math.Round(result))
	}

	return E(roundFloat(float64(result), precision))
}

// Median returns the median value in the slice. The slice will become
// sorted.
func Median[E Numerical, S ~[]E](data S, precision int) float64 {
	l := len(data)
	if l == 0 {
		return 0
	}
	slices.Sort(data)
	if l%2 == 0 {
		return roundFloat(float64((data[l/2-1]+data[l/2])/2), precision)
	}
	return roundFloat(float64(data[l/2]), precision)
}

// Median returns the sum of the slice values. The slice will become
// sorted.
func Sum[E Numerical, S ~[]E](data S) E {
	l := len(data)
	if l == 0 {
		return 0
	}
	slices.Sort(data)

	var sum E = 0

	for _, value := range data {
		sum += value
	}
	return sum
}

// Obtain the 5th, 50th, 95th and 100th percentiles. Requires [threshold] data
// points or more.
func Percentiles[E Numerical, S ~[]E](data S, threshold int) []E {
	percentiles := make([]E, 4)
	l := len(data)
	if l == 0 || l < threshold {
		return percentiles
	}
	slices.Sort(data)

	percentiles[0] = E(data[int(float64(len(data))*0.05)]) // 5th
	percentiles[1] = E(data[len(data)/2])                  // 50th
	percentiles[2] = E(data[int(float64(len(data))*0.95)]) // 95th
	percentiles[3] = E(data[len(data)-1])                  // 100th

	return percentiles
}

func roundFloat(value float64, precision int) float64 {
	ratio := math.Pow(10, float64(precision))
	return math.Round(value*ratio) / ratio
}

func floatStats(data []float64) *ur.FloatStatistic {
	count, sum, min, max, med, avg, percentiles := calculateStatistics(data)
	return &ur.FloatStatistic{
		Count:       count,
		Sum:         roundFloat(sum, floatPrecision),
		Min:         min,
		Max:         max,
		Med:         med,
		Avg:         avg,
		Percentiles: percentiles,
	}
}

func intStats(data []int64) *ur.IntegerStatistic {
	count, sum, min, max, med, avg, percentiles := calculateStatistics(data)
	return &ur.IntegerStatistic{
		Count:       count,
		Sum:         sum,
		Min:         min,
		Max:         max,
		Med:         med,
		Avg:         avg,
		Percentiles: percentiles,
	}
}

func calculateStatistics[E Numerical](data []E) (count int64, sum, min, max E, med, avg float64, percentiles []E) {
	slices.Sort(data)
	count = int64(len(data))
	sum = Sum(data)
	min = data[0]
	max = data[len(data)-1]
	med = Median(data, floatPrecision)
	avg = roundFloat(float64(sum)/float64(count), floatPrecision)
	percentiles = Percentiles(data, percentileThreshold)
	return
}
