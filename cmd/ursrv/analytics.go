package main

import (
	"sort"
	"strings"
)

type analytic struct {
	Key        string
	Count      int
	Percentage float64
	Items      []analytic `json:",omitempty"`
}

type analyticList []analytic

func (l analyticList) Less(a, b int) bool {
	if l[a].Key == "Others" {
		return true
	}
	if l[b].Key == "Others" {
		return false
	}
	return l[b].Count < l[a].Count // inverse
}

func (l analyticList) Swap(a, b int) {
	l[a], l[b] = l[b], l[a]
}

func (l analyticList) Len() int {
	return len(l)
}

// Returns a list of frequency analytics for a given list of strings.
func analyticsFor(ss []string, cutoff int) []analytic {
	m := make(map[string]int)
	t := 0
	for _, s := range ss {
		m[s]++
		t++
	}

	l := make([]analytic, 0, len(m))
	for k, c := range m {
		l = append(l, analytic{
			Key:        k,
			Count:      c,
			Percentage: 100 * float64(c) / float64(t),
		})
	}

	sort.Sort(analyticList(l))

	if cutoff > 0 && len(l) > cutoff {
		c := 0
		for _, i := range l[cutoff:] {
			c += i.Count
		}
		l = append(l[:cutoff], analytic{
			Key:        "Others",
			Count:      c,
			Percentage: 100 * float64(c) / float64(t),
		})
	}

	return l
}

func statsForInts(data []int) [4]float64 {
	var res [4]float64
	if len(data) == 0 {
		return res
	}

	sort.Ints(data)
	res[0] = float64(data[int(float64(len(data))*0.05)])
	res[1] = float64(data[len(data)/2])
	res[2] = float64(data[int(float64(len(data))*0.95)])
	res[3] = float64(data[len(data)-1])
	return res
}

func statsForFloats(data []float64) [4]float64 {
	var res [4]float64
	if len(data) == 0 {
		return res
	}

	sort.Float64s(data)
	res[0] = data[int(float64(len(data))*0.05)]
	res[1] = data[len(data)/2]
	res[2] = data[int(float64(len(data))*0.95)]
	res[3] = data[len(data)-1]
	return res
}

func group(by func(string) string, as []analytic, perGroup int) []analytic {
	var res []analytic

next:
	for _, a := range as {
		group := by(a.Key)
		for i := range res {
			if res[i].Key == group {
				res[i].Count += a.Count
				res[i].Percentage += a.Percentage
				if len(res[i].Items) < perGroup {
					res[i].Items = append(res[i].Items, a)
				}
				continue next
			}
		}
		res = append(res, analytic{
			Key:        group,
			Count:      a.Count,
			Percentage: a.Percentage,
			Items:      []analytic{a},
		})
	}

	sort.Sort(analyticList(res))
	return res
}

func byVersion(s string) string {
	parts := strings.Split(s, ".")
	if len(parts) >= 2 {
		return strings.Join(parts[:2], ".")
	}
	return s
}

func byPlatform(s string) string {
	parts := strings.Split(s, "-")
	if len(parts) >= 2 {
		return parts[0]
	}
	return s
}
