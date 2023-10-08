// Copyright (C) 2022 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

import (
	"strconv"
	"strings"

	webauthnLib "github.com/go-webauthn/webauthn/webauthn"
)

func NewWebauthnHandle(cfg Wrapper) (*webauthnLib.WebAuthn, error) {
	guiCfg := cfg.GUI()

	displayName := "Syncthing"
	if dev, ok := cfg.Device(cfg.MyID()); ok && dev.Name != "" {
		displayName = "Syncthing @ " + dev.Name
	}

	rpId := guiCfg.WebauthnRpId
	if rpId == "" {
		rpId = "localhost"
	}

	origin := guiCfg.WebauthnOrigin
	if origin == "" {
		port := strconv.Itoa(DefaultGUIPort)
		addressSplits := strings.Split(guiCfg.RawAddress, ":")
		if len(addressSplits) > 0 {
			port = addressSplits[len(addressSplits)-1]
		}
		if port == "443" {
			origin = "https://" + rpId
		} else {
			origin = "https://" + rpId + ":" + port
		}
	}

	return webauthnLib.New(&webauthnLib.Config{
		RPDisplayName: displayName,
		RPID:          rpId,
		RPOrigin:      origin,
	})
}
