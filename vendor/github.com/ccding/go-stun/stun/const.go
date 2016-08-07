// Copyright 2013, Cong Ding. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Author: Cong Ding <dinggnu@gmail.com>

package stun

// Default server address and client name.
const (
	DefaultServerAddr   = "stun.ekiga.net:3478"
	DefaultSoftwareName = "StunClient"
)

const (
	magicCookie = 0x2112A442
	fingerprint = 0x5354554e
)

// NATType is the type of NAT described by int.
type NATType int

// NAT types.
const (
	NATError NATType = iota
	NATUnknown
	NATNone
	NATBlocked
	NATFull
	NATSymetric
	NATRestricted
	NATPortRestricted
	NATSymetricUDPFirewall
)

var natStr map[NATType]string

func init() {
	natStr = map[NATType]string{
		NATError:               "Test failed",
		NATUnknown:             "Unexpected response from the STUN server",
		NATBlocked:             "UDP is blocked",
		NATFull:                "Full cone NAT",
		NATSymetric:            "Symetric NAT",
		NATRestricted:          "Restricted NAT",
		NATPortRestricted:      "Port restricted NAT",
		NATNone:                "Not behind a NAT",
		NATSymetricUDPFirewall: "Symetric UDP firewall",
	}
}

func (nat NATType) String() string {
	if s, ok := natStr[nat]; ok {
		return s
	}
	return "Unknown"
}

const (
	errorTryAlternate                 = 300
	errorBadRequest                   = 400
	errorUnauthorized                 = 401
	errorUnassigned402                = 402
	errorForbidden                    = 403
	errorUnknownAttribute             = 420
	errorAllocationMismatch           = 437
	errorStaleNonce                   = 438
	errorUnassigned439                = 439
	errorAddressFamilyNotSupported    = 440
	errorWrongCredentials             = 441
	errorUnsupportedTransportProtocol = 442
	errorPeerAddressFamilyMismatch    = 443
	errorConnectionAlreadyExists      = 446
	errorConnectionTimeoutOrFailure   = 447
	errorAllocationQuotaReached       = 486
	errorRoleConflict                 = 487
	errorServerError                  = 500
	errorInsufficientCapacity         = 508
)
const (
	attributeFamilyIPv4 = 0x01
	attributeFamilyIPV6 = 0x02
)

const (
	attributeMappedAddress          = 0x0001
	attributeResponseAddress        = 0x0002
	attributeChangeRequest          = 0x0003
	attributeSourceAddress          = 0x0004
	attributeChangedAddress         = 0x0005
	attributeUsername               = 0x0006
	attributePassword               = 0x0007
	attributeMessageIntegrity       = 0x0008
	attributeErrorCode              = 0x0009
	attributeUnknownAttributes      = 0x000a
	attributeReflectedFrom          = 0x000b
	attributeChannelNumber          = 0x000c
	attributeLifetime               = 0x000d
	attributeBandwidth              = 0x0010
	attributeXorPeerAddress         = 0x0012
	attributeData                   = 0x0013
	attributeRealm                  = 0x0014
	attributeNonce                  = 0x0015
	attributeXorRelayedAddress      = 0x0016
	attributeRequestedAddressFamily = 0x0017
	attributeEvenPort               = 0x0018
	attributeRequestedTransport     = 0x0019
	attributeDontFragment           = 0x001a
	attributeXorMappedAddress       = 0x0020
	attributeTimerVal               = 0x0021
	attributeReservationToken       = 0x0022
	attributePriority               = 0x0024
	attributeUseCandidate           = 0x0025
	attributePadding                = 0x0026
	attributeResponsePort           = 0x0027
	attributeConnectionID           = 0x002a
	attributeXorMappedAddressExp    = 0x8020
	attributeSoftware               = 0x8022
	attributeAlternateServer        = 0x8023
	attributeCacheTimeout           = 0x8027
	attributeFingerprint            = 0x8028
	attributeIceControlled          = 0x8029
	attributeIceControlling         = 0x802a
	attributeResponseOrigin         = 0x802b
	attributeOtherAddress           = 0x802c
	attributeEcnCheckStun           = 0x802d
	attributeCiscoFlowdata          = 0xc000
)

const (
	typeBindingRequest                 = 0x0001
	typeBindingResponse                = 0x0101
	typeBindingErrorResponse           = 0x0111
	typeSharedSecretRequest            = 0x0002
	typeSharedSecretResponse           = 0x0102
	typeSharedErrorResponse            = 0x0112
	typeAllocate                       = 0x0003
	typeAllocateResponse               = 0x0103
	typeAllocateErrorResponse          = 0x0113
	typeRefresh                        = 0x0004
	typeRefreshResponse                = 0x0104
	typeRefreshErrorResponse           = 0x0114
	typeSend                           = 0x0006
	typeSendResponse                   = 0x0106
	typeSendErrorResponse              = 0x0116
	typeData                           = 0x0007
	typeDataResponse                   = 0x0107
	typeDataErrorResponse              = 0x0117
	typeCreatePermisiion               = 0x0008
	typeCreatePermisiionResponse       = 0x0108
	typeCreatePermisiionErrorResponse  = 0x0118
	typeChannelBinding                 = 0x0009
	typeChannelBindingResponse         = 0x0109
	typeChannelBindingErrorResponse    = 0x0119
	typeConnect                        = 0x000a
	typeConnectResponse                = 0x010a
	typeConnectErrorResponse           = 0x011a
	typeConnectionBind                 = 0x000b
	typeConnectionBindResponse         = 0x010b
	typeConnectionBindErrorResponse    = 0x011b
	typeConnectionAttempt              = 0x000c
	typeConnectionAttemptResponse      = 0x010c
	typeConnectionAttemptErrorResponse = 0x011c
)
