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

const compatibilityYaml = "compatibility.yaml"

type Runtimes struct {
	RuntimeEntries []RuntimeEntry `yaml:"runtimes"`
}

type RuntimeEntry map[string]any

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
	keys := make([]string, 0, len(compInfos))

	for key := range compInfos {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	compInfo0 := CompInfo{"", make(map[string]string)}
	for k, v := range compInfos[keys[0]].MinOSVersion {
		compInfo0.MinOSVersion[k] = v
	}

	// Copy any settings from the previous runtime into the next runtime, if
	// missing in that runtime.
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

	return compInfos, nil
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

func generateCompatibilityJson(dir string, rt string) error {
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

func GenerateCompatibilityJson(dir string) error {
	return generateCompatibilityJson(dir, runtime.Version())
}
