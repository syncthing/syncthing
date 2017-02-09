// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package upgrade

// SigningKey is the public key used to verify signed upgrades. It must match
// the private key used to sign binaries for the built in upgrade mechanism to
// accept an upgrade. Keys and signatures can be created and verified with the
// stsigtool utility. The build script creates signed binaries when given the
// -sign option.
var SigningKey = []byte(`-----BEGIN EC PUBLIC KEY-----
MIGbMBAGByqGSM49AgEGBSuBBAAjA4GGAAQA1iRk+p+DsmolixxVKcpEVlMDPOeQ
1dWthURMqsjxoJuDAe5I98P/A0kXSdBI7avm5hXhX2opJ5TAyBZLHPpDTRoBg4WN
7jUpeAjtPoVVxvOh37qDeDVcjCgJbbDTPKbjxq/Ae3SHlQMRcoes7lVY1+YJ8dPk
2oPfjA6jtmo9aVbf/uo=
-----END EC PUBLIC KEY-----`)
