// Copyright (C) 2016 The Protocol Authors.

//go:generate -command genxdr go run ../../vendor/github.com/calmh/xdr/cmd/genxdr/main.go
//go:generate genxdr -o hello_v0.13_xdr.go hello_v0.13.go

package protocol

var (
	Version13HelloMagic uint32 = 0x9F79BC40
)

type Version13HelloMessage struct {
	DeviceName    string // max:64
	ClientName    string // max:64
	ClientVersion string // max:64
}

func (m Version13HelloMessage) Magic() uint32 {
	return Version13HelloMagic
}

func (m Version13HelloMessage) Marshal() ([]byte, error) {
	return m.MarshalXDR()
}
