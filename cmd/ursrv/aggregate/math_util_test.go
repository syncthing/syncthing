package aggregate

import (
	"slices"
	"testing"
)

func TestMathematicalFunctions(t *testing.T) {
	var intCases = []struct {
		input       []int
		med         float64
		percentiles []int
		sum         int
		avg         int
	}{
		{
			input:       []int{5, 1, 200, 15},
			med:         10,
			percentiles: []int{0, 0, 0, 0}, // Makes no sense to calculate percentiles with too few data points.
			sum:         221,
			avg:         55,
		},
		{
			input:       make([]int, 100),
			med:         49, // 49.5
			percentiles: []int{5, 50, 95, 99},
			sum:         4950,
			avg:         50, // 50.5
		},
	}

	for _, ic := range intCases {
		if ic.input[0] == 0 {
			for i := range ic.input {
				ic.input[i] = i
			}
		}
		med := Median(ic.input, 2)
		perc := Percentiles(ic.input, 100)
		sum := Sum(ic.input)
		avg := Average(ic.input, 0)
		if med != ic.med {
			t.Errorf("incorrect median, got %f, expected %f", med, ic.med)
		}
		if !slices.Equal(perc, ic.percentiles) {
			t.Errorf("incorrect percentiles, got %v, expected %v", perc, ic.percentiles)
		}
		if sum != ic.sum {
			t.Errorf("incorrect sum, got %d, expected %d", sum, ic.sum)
		}
		if avg != ic.avg {
			t.Errorf("incorrect avg, got %d, expected %d", avg, ic.avg)
		}
	}
}
