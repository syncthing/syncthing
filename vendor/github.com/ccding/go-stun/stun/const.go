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
	DefaultServerAddr   = "stun1.l.google.com:19302"
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
	NAT_ERROR NATType = iota
	NAT_UNKNOWN
	NAT_NONE
	NAT_BLOCKED
	NAT_FULL
	NAT_SYMETRIC
	NAT_RESTRICTED
	NAT_PORT_RESTRICTED
	NAT_SYMETRIC_UDP_FIREWALL
)

func (nat NATType) String() string {
	switch nat {
	case NAT_ERROR:
		return "Test failed"
	case NAT_UNKNOWN:
		return "Unexpected response from the STUN server"
	case NAT_BLOCKED:
		return "UDP is blocked"
	case NAT_FULL:
		return "Full cone NAT"
	case NAT_SYMETRIC:
		return "Symetric NAT"
	case NAT_RESTRICTED:
		return "Restricted NAT"
	case NAT_PORT_RESTRICTED:
		return "Port restricted NAT"
	case NAT_NONE:
		return "Not behind a NAT"
	case NAT_SYMETRIC_UDP_FIREWALL:
		return "Symetric UDP firewall"
	}
	return "Unknown"
}

const (
	error_TRY_ALTERNATE                  = 300
	error_BAD_REQUEST                    = 400
	error_UNAUTHORIZED                   = 401
	error_UNASSIGNED_402                 = 402
	error_FORBIDDEN                      = 403
	error_UNKNOWN_attribute              = 420
	error_ALLOCATION_MISMATCH            = 437
	error_STALE_NONCE                    = 438
	error_UNASSIGNED_439                 = 439
	error_ADDRESS_FAMILY_NOT_SUPPORTED   = 440
	error_WRONG_CREDENTIALS              = 441
	error_UNSUPPORTED_TRANSPORT_PROTOCOL = 442
	error_PEER_ADDRESS_FAMILY_MISMATCH   = 443
	error_CONNECTION_ALREADY_EXISTS      = 446
	error_CONNECTION_TIMEOUT_OR_FAILURE  = 447
	error_ALLOCATION_QUOTA_REACHED       = 486
	error_ROLE_CONFLICT                  = 487
	error_SERVER_error                   = 500
	error_INSUFFICIENT_CAPACITY          = 508
)
const (
	attribute_FAMILY_IPV4 = 0x01
	attribute_FAMILY_IPV6 = 0x02
)

const (
	attribute_MAPPED_ADDRESS           = 0x0001
	attribute_RESPONSE_ADDRESS         = 0x0002
	attribute_CHANGE_REQUEST           = 0x0003
	attribute_SOURCE_ADDRESS           = 0x0004
	attribute_CHANGED_ADDRESS          = 0x0005
	attribute_USERNAME                 = 0x0006
	attribute_PASSWORD                 = 0x0007
	attribute_MESSAGE_INTEGRITY        = 0x0008
	attribute_ERROR_CODE               = 0x0009
	attribute_UNKNOWN_attributeS       = 0x000A
	attribute_REFLECTED_FROM           = 0x000B
	attribute_CHANNEL_NUMBER           = 0x000C
	attribute_LIFETIME                 = 0x000D
	attribute_BANDWIDTH                = 0x0010
	attribute_XOR_PEER_ADDRESS         = 0x0012
	attribute_DATA                     = 0x0013
	attribute_REALM                    = 0x0014
	attribute_NONCE                    = 0x0015
	attribute_XOR_RELAYED_ADDRESS      = 0x0016
	attribute_REQUESTED_ADDRESS_FAMILY = 0x0017
	attribute_EVEN_PORT                = 0x0018
	attribute_REQUESTED_TRANSPORT      = 0x0019
	attribute_DONT_FRAGMENT            = 0x001A
	attribute_XOR_MAPPED_ADDRESS       = 0x0020
	attribute_TIMER_VAL                = 0x0021
	attribute_RESERVATION_TOKEN        = 0x0022
	attribute_PRIORITY                 = 0x0024
	attribute_USE_CANDIDATE            = 0x0025
	attribute_PADDING                  = 0x0026
	attribute_RESPONSE_PORT            = 0x0027
	attribute_CONNECTION_ID            = 0x002A
	attribute_XOR_MAPPED_ADDRESS_EXP   = 0x8020
	attribute_SOFTWARE                 = 0x8022
	attribute_ALTERNATE_SERVER         = 0x8023
	attribute_CACHE_TIMEOUT            = 0x8027
	attribute_FINGERPRINT              = 0x8028
	attribute_ICE_CONTROLLED           = 0x8029
	attribute_ICE_CONTROLLING          = 0x802A
	attribute_RESPONSE_ORIGIN          = 0x802B
	attribute_OTHER_ADDRESS            = 0x802C
	attribute_ECN_CHECK_STUN           = 0x802D
	attribute_CISCO_FLOWDATA           = 0xC000
)

const (
	type_BINDING_REQUEST                   = 0x0001
	type_BINDING_RESPONSE                  = 0x0101
	type_BINDING_ERROR_RESPONSE            = 0x0111
	type_SHARED_SECRET_REQUEST             = 0x0002
	type_SHARED_SECRET_RESPONSE            = 0x0102
	type_SHARED_ERROR_RESPONSE             = 0x0112
	type_ALLOCATE                          = 0x0003
	type_ALLOCATE_RESPONSE                 = 0x0103
	type_ALLOCATE_ERROR_RESPONSE           = 0x0113
	type_REFRESH                           = 0x0004
	type_REFRESH_RESPONSE                  = 0x0104
	type_REFRESH_ERROR_RESPONSE            = 0x0114
	type_SEND                              = 0x0006
	type_SEND_RESPONSE                     = 0x0106
	type_SEND_ERROR_RESPONSE               = 0x0116
	type_DATA                              = 0x0007
	type_DATA_RESPONSE                     = 0x0107
	type_DATA_ERROR_RESPONSE               = 0x0117
	type_CREATE_PERMISIION                 = 0x0008
	type_CREATE_PERMISIION_RESPONSE        = 0x0108
	type_CREATE_PERMISIION_ERROR_RESPONSE  = 0x0118
	type_CHANNEL_BINDING                   = 0x0009
	type_CHANNEL_BINDING_RESPONSE          = 0x0109
	type_CHANNEL_BINDING_ERROR_RESPONSE    = 0x0119
	type_CONNECT                           = 0x000A
	type_CONNECT_RESPONSE                  = 0x010A
	type_CONNECT_ERROR_RESPONSE            = 0x011A
	type_CONNECTION_BIND                   = 0x000B
	type_CONNECTION_BIND_RESPONSE          = 0x010B
	type_CONNECTION_BIND_ERROR_RESPONSE    = 0x011B
	type_CONNECTION_ATTEMPT                = 0x000C
	type_CONNECTION_ATTEMPT_RESPONSE       = 0x010C
	type_CONNECTION_ATTEMPT_ERROR_RESPONSE = 0x011C
)
