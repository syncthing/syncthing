// Copyright (C) 2015 Audrius Butkevicius and Contributors (see the CONTRIBUTORS file).

//go:generate -command genxdr go run ../../syncthing/Godeps/_workspace/src/github.com/calmh/xdr/cmd/genxdr/main.go
//go:generate genxdr -o packets_xdr.go packets.go

package protocol

import (
	"unsafe"
)

const (
	Magic        = 0x9E79BC40
	HeaderSize   = unsafe.Sizeof(&Header{})
	ProtocolName = "bep-relay"
)

const (
	MessageTypePing int32 = iota
	MessageTypePong
	MessageTypeJoinRequest
	MessageTypeConnectRequest
	MessageTypeSessionInvitation
)

type Header struct {
	Magic         uint32
	MessageType   int32
	MessageLength int32
}

type Ping struct{}
type Pong struct{}
type JoinRequest struct{}

type ConnectRequest struct {
	ID []byte // max:32
}

type SessionInvitation struct {
	Key          []byte // max:32
	Address      []byte // max:32
	Port         uint16
	ServerSocket bool
}
