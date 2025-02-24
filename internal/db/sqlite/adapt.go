package sqlite

import (
	"database/sql/driver"
	"errors"

	"github.com/syncthing/syncthing/internal/gen/bep"
	"github.com/syncthing/syncthing/internal/gen/dbproto"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
	"google.golang.org/protobuf/proto"
)

type dbVector struct { //nolint:recvcheck
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
	if str == "" {
		v.Vector = protocol.Vector{}
		return nil
	}
	vec, err := protocol.VectorFromString(str)
	if err != nil {
		return err
	}
	v.Vector = vec

	return nil
}

type indirectFI struct {
	FiProtobuf []byte
	BlProtobuf []byte
}

func (i indirectFI) FileInfo() (protocol.FileInfo, error) {
	var fi bep.FileInfo
	if err := proto.Unmarshal(i.FiProtobuf, &fi); err != nil {
		return protocol.FileInfo{}, err
	}
	if len(i.BlProtobuf) > 0 {
		var bl dbproto.BlockList
		if err := proto.Unmarshal(i.BlProtobuf, &bl); err != nil {
			return protocol.FileInfo{}, err
		}
		fi.Blocks = bl.Blocks
	}
	fi.Name = osutil.NativeFilename(fi.Name)
	return protocol.FileInfoFromDB(&fi), nil
}
