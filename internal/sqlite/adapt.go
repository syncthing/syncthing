package sqlite

import (
	"database/sql/driver"
	"errors"

	"github.com/syncthing/syncthing/lib/protocol"
	"google.golang.org/protobuf/proto"
)

type pbMessage[T any] interface {
	*T
	proto.Message
}

func protoValuer[T any, PT pbMessage[T]](v PT) *pbAdapter[T, PT] {
	return &pbAdapter[T, PT]{v}
}

type pbAdapter[T any, PT pbMessage[T]] struct {
	Message PT
}

func (v pbAdapter[T, PT]) Value() (driver.Value, error) {
	return proto.Marshal(v.Message)
}

func (v *pbAdapter[T, PT]) Scan(value any) error {
	bs, ok := value.([]byte)
	if !ok {
		return errors.New("not a byte slice")
	}
	return proto.Unmarshal(bs, v.Message)
}

type dbVector struct {
	protocol.Vector
}

func (v dbVector) Value() (driver.Value, error) {
	return v.String(), nil
}

func (v *dbVector) Scan(value any) error {
	str, ok := value.(string)
	if !ok {
		return errors.New("not a string")
	}
	vec, err := protocol.VectorFromString(str)
	if err != nil {
		return err
	}
	v.Vector = vec

	return nil
}
