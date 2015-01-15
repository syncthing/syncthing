// Copyright (C) 2014 The Syncthing Authors.
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

// Package model implements folder abstraction and file pulling mechanisms
package model

import (
	"encoding/binary"
	"sort"
	"strings"
	"sync"

	"github.com/syncthing/protocol"
)

type features uint64

const (
	FeatureTemporaryIndex = 1 << iota

	FeatureAllFeatures features = (1 << iota) - 1
)

var featureLabels = map[features]string{
	FeatureTemporaryIndex: "Temporary Index",
}

// We provide a features object as part of the ClusterConfig message options
// field, which accepts pairs of key value strings. Also, Go strings are only
// null terminated when printing.
// "features" is a 64bit int, hence we allocate 8 bytes.
func (f features) Marshal() string {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(f))
	return string(buf)
}

func (f features) String() string {
	features := []string{}

	for key, value := range featureLabels {
		if f&key != 0 {
			features = append(features, value)
		}
	}

	sort.Strings(features)

	if len(features) == 0 {
		return "None"
	}

	return strings.Join(features, "|")
}

func initFeatures(disabled []string) features {
	result := FeatureAllFeatures
	for _, disable := range disabled {
		for feature, label := range featureLabels {
			if strings.ToLower(label) == strings.ToLower(disable) {
				result &= ^feature
				break
			}
		}
	}
	return result
}

type featureSet struct {
	features map[protocol.DeviceID]features
	mut      sync.RWMutex
}

func newFeatureSet() *featureSet {
	return &featureSet{
		features: make(map[protocol.DeviceID]features),
	}
}

func (f *featureSet) FeatureString(device protocol.DeviceID) string {
	f.mut.RLock()
	defer f.mut.RUnlock()
	return f.features[device].String()
}

func (f *featureSet) UpdateFromString(device protocol.DeviceID, input string) {
	if len(input) != 8 {
		return
	}

	f.mut.Lock()
	f.features[device] = features(binary.BigEndian.Uint64([]byte(input)))
	f.mut.Unlock()
}

func (f *featureSet) Clear(device protocol.DeviceID) {
	f.mut.Lock()
	delete(f.features, device)
	f.mut.Unlock()
}

func (f *featureSet) HasFeature(device protocol.DeviceID, feature features) bool {
	f.mut.RLock()
	defer f.mut.RUnlock()
	return f.features[device]&feature != 0
}
