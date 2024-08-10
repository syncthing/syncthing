// Copyright (C) 2024 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package upgrade

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	// CompatibilityJson is name of compatibility.json file.
	CompatibilityJson = "compatibility.json"

	compatibilityYaml = "compatibility.yaml"
)

// Runtimes defines the structure of the compatibility.yaml file.
type Runtimes struct {
	RuntimeEntries []RuntimeEntry `yaml:"runtimes"`
}

// RuntimeEntry is an entry in the compatibility.yaml file.
type RuntimeEntry map[string]any

// CompInfo is the structure of the compatibility.json file.
type CompInfo struct {
	Runtime      string            `json:"runtime"`
	MinOSVersion map[string]string `json:"minOSVersion"`
}

// CompInfos is map of CompInfo's where the key is the runtime version (goX.YY).
type CompInfos map[string]CompInfo

func loadCompatibilityYaml(dir string) (CompInfos, error) {
	compatibilityPath := filepath.Join(dir, compatibilityYaml)
	data, err := os.ReadFile(compatibilityPath)
	if err != nil {
		return nil, err
	}

	runtimes := Runtimes{}
	if err := yaml.Unmarshal(data, &runtimes); err != nil {
		return nil, err
	}

	compInfos := make(CompInfos, len(runtimes.RuntimeEntries))

	for i, entry := range runtimes.RuntimeEntries {
		rt := ""
		minOSVersion := make(map[string]string)
		for k, v := range entry {
			if k == "runtime" {
				rt = strings.TrimSpace(v.(string))
				continue
			}
			if k == "minOSVersion" {
				switch m := v.(type) {
				case RuntimeEntry:
					for mk, mv := range m {
						minOSVersion[strings.TrimSpace(mk)] = strings.TrimSpace(mv.(string))
					}
				case nil:
				default:
					return nil, fmt.Errorf("%s: entry %d is a %T not a map[string]any", compatibilityPath, i, v)
				}
			}
		}
		compInfo := CompInfo{
			Runtime:      rt,
			MinOSVersion: minOSVersion,
		}
		compInfos[rt] = compInfo
	}

	return fillCompatibilityYaml(compInfos), nil
}

// Copy any settings from each previous runtime entry into the next runtime
// entry, if missing.
func fillCompatibilityYaml(compInfos CompInfos) CompInfos {
	keys := make([]string, 0, len(compInfos))

	for key := range compInfos {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	compInfo0 := CompInfo{"", make(map[string]string)}
	for k, v := range compInfos[keys[0]].MinOSVersion {
		compInfo0.MinOSVersion[k] = v
	}

	for _, rt := range keys[1:] {
		for k0, v0 := range compInfo0.MinOSVersion {
			v, ok := compInfos[rt].MinOSVersion[k0]
			if ok {
				compInfo0.MinOSVersion[k0] = v
			} else {
				compInfos[rt].MinOSVersion[k0] = v0
			}
		}
	}

	return compInfos
}

func saveCompatibilityJson(dir string, compInfos CompInfos, rt string) error {
	parts := strings.Split(rt, ".")
	if len(parts) < 2 {
		return fmt.Errorf("Go version %q is not in the form gox.y", rt)
	}
	majorMinor := strings.Join(parts[:2], ".")
	compInfo, ok := compInfos[majorMinor]
	if !ok {
		return fmt.Errorf("Runtime %v not found in %q", majorMinor, compatibilityYaml)
	}
	data, err := json.Marshal(compInfo)
	if err != nil {
		return err
	}
	compatibilityPath := filepath.Join(dir, CompatibilityJson)
	err = os.WriteFile(compatibilityPath, data, 0o644)
	if err != nil {
		return err
	}
	return nil
}

func genCompatibilityJson(dir string, rt string) error {
	compInfos, err := loadCompatibilityYaml(dir)
	if err != nil {
		return err
	}

	err = saveCompatibilityJson(dir, compInfos, rt)
	if err != nil {
		return err
	}
	return nil
}

// GenerateCompatibilityJson generates compatibility.json for the
// runtime.Version() entry in compatibility.yaml.
func GenerateCompatibilityJson(dir string) error {
	return genCompatibilityJson(dir, runtime.Version())
}
