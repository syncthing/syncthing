// Copyright (C) 2015 Audrius Butkevicius and Contributors (see the CONTRIBUTORS file).

package protocol

import (
	"fmt"
	"io"
)

const (
	magic        = 0x9E79BC40
	ProtocolName = "bep-relay"
)

var (
	ResponseSuccess           = Response{0, "success"}
	ResponseNotFound          = Response{1, "not found"}
	ResponseAlreadyConnected  = Response{2, "already connected"}
	ResponseInternalError     = Response{99, "internal error"}
	ResponseUnexpectedMessage = Response{100, "unexpected message"}
)

func WriteMessage(w io.Writer, message interface{}) error {
	header := header{
		magic: magic,
	}

	var payload []byte
	var err error

	switch msg := message.(type) {
	case Ping:
		payload, err = msg.MarshalXDR()
		header.messageType = messageTypePing
	case Pong:
		payload, err = msg.MarshalXDR()
		header.messageType = messageTypePong
	case JoinRelayRequest:
		payload, err = msg.MarshalXDR()
		header.messageType = messageTypeJoinRelayRequest
	case JoinSessionRequest:
		payload, err = msg.MarshalXDR()
		header.messageType = messageTypeJoinSessionRequest
	case Response:
		payload, err = msg.MarshalXDR()
		header.messageType = messageTypeResponse
	case ConnectRequest:
		payload, err = msg.MarshalXDR()
		header.messageType = messageTypeConnectRequest
	case SessionInvitation:
		payload, err = msg.MarshalXDR()
		header.messageType = messageTypeSessionInvitation
	default:
		err = fmt.Errorf("Unknown message type")
	}

	if err != nil {
		return err
	}

	header.messageLength = int32(len(payload))

	headerpayload, err := header.MarshalXDR()
	if err != nil {
		return err
	}

	_, err = w.Write(append(headerpayload, payload...))
	return err
}

func ReadMessage(r io.Reader) (interface{}, error) {
	var header header
	if err := header.DecodeXDR(r); err != nil {
		return nil, err
	}

	if header.magic != magic {
		return nil, fmt.Errorf("magic mismatch")
	}

	switch header.messageType {
	case messageTypePing:
		var msg Ping
		err := msg.DecodeXDR(r)
		return msg, err
	case messageTypePong:
		var msg Pong
		err := msg.DecodeXDR(r)
		return msg, err
	case messageTypeJoinRelayRequest:
		var msg JoinRelayRequest
		err := msg.DecodeXDR(r)
		return msg, err
	case messageTypeJoinSessionRequest:
		var msg JoinSessionRequest
		err := msg.DecodeXDR(r)
		return msg, err
	case messageTypeResponse:
		var msg Response
		err := msg.DecodeXDR(r)
		return msg, err
	case messageTypeConnectRequest:
		var msg ConnectRequest
		err := msg.DecodeXDR(r)
		return msg, err
	case messageTypeSessionInvitation:
		var msg SessionInvitation
		err := msg.DecodeXDR(r)
		return msg, err
	}

	return nil, fmt.Errorf("Unknown message type")
}
