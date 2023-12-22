package aggregate

import "github.com/syncthing/syncthing/cmd/ursrv/report"

// add sets a key in a nested map, initializing things if needed as we go.
func add(storage map[string]map[string]int, parent, child string, value int) {
	n, ok := storage[parent]
	if !ok {
		n = make(map[string]int)
		storage[parent] = n
	}
	n[child] += value
}

// inc makes sure that even for unused features, we initialize them in the
// feature map. Furthermore, this acts as a helper that accepts booleans
// to increment by one, or integers to increment by that integer.
func inc(storage map[string]int, key string, i interface{}) {
	cv := storage[key]
	switch v := i.(type) {
	case bool:
		if v {
			cv++
		}
	case int:
		cv += v
	}
	storage[key] = cv
}

type sortableFeatureList []report.Feature

func (l sortableFeatureList) Less(a, b int) bool {
	if l[a].Pct != l[b].Pct {
		return l[a].Pct < l[b].Pct
	}
	return l[a].Key > l[b].Key
}

func (l sortableFeatureList) Swap(a, b int) {
	l[a], l[b] = l[b], l[a]
}

func (l sortableFeatureList) Len() int {
	return len(l)
}
