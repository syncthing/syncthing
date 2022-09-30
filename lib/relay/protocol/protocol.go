// Copyright (C) 2015 Audrius Butkevicius and Contributors (see the CONTRIBUTORS file).

package protocol

import (
	"errors"
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
	ResponseWrongToken        = Response{3, "wrong token"}
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
	case RelayFull:
		payload, err = msg.MarshalXDR()
		header.messageType = messageTypeRelayFull
	default:
		err = errors.New("unknown message type")
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

	buf := make([]byte, header.XDRSize())
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}

	if err := header.UnmarshalXDR(buf); err != nil {
		return nil, err
	}

	if header.magic != magic {
		return nil, errors.New("magic mismatch")
	}
	if header.messageLength < 0 || header.messageLength > 1024 {
		return nil, fmt.Errorf("bad length (%d)", header.messageLength)
	}

	buf = make([]byte, int(header.messageLength))
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}

	switch header.messageType {
	case messageTypePing:
		var msg Ping
		err := msg.UnmarshalXDR(buf)
		return msg, err
	case messageTypePong:
		var msg Pong
		err := msg.UnmarshalXDR(buf)
		return msg, err
	case messageTypeJoinRelayRequest:
		var msg JoinRelayRequest

		// In prior versions of the protocol JoinRelayRequest did not have a
		// token field. Trying to unmarshal such a request will result in
		// an error, return msg with an empty token instead.
		if header.messageLength == 0 {
			return msg, nil
		}

		err := msg.UnmarshalXDR(buf)
		return msg, err
	case messageTypeJoinSessionRequest:
		var msg JoinSessionRequest
		err := msg.UnmarshalXDR(buf)
		return msg, err
	case messageTypeResponse:
		var msg Response
		err := msg.UnmarshalXDR(buf)
		return msg, err
	case messageTypeConnectRequest:
		var msg ConnectRequest
		err := msg.UnmarshalXDR(buf)
		return msg, err
	case messageTypeSessionInvitation:
		var msg SessionInvitation
		err := msg.UnmarshalXDR(buf)
		return msg, err
	case messageTypeRelayFull:
		var msg RelayFull
		err := msg.UnmarshalXDR(buf)
		return msg, err
	}

	return nil, errors.New("unknown message type")
}
