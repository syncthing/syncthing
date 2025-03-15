package sqlite

import (
	"database/sql/driver"
	"errors"
	"iter"

	"github.com/jmoiron/sqlx"
	"github.com/syncthing/syncthing/internal/gen/bep"
	"github.com/syncthing/syncthing/internal/gen/dbproto"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
	"google.golang.org/protobuf/proto"
)

// iterStructs returns an iterator over the given struct type by scanning
// the SQL rows. `rows` is closed when the iterator exits.
func iterStructs[T any](rows *sqlx.Rows, err error) (iter.Seq[T], func() error) {
	if err != nil {
		return func(_ func(T) bool) {}, func() error { return err }
	}

	var retErr error
	return func(yield func(T) bool) {
		defer rows.Close()
		for rows.Next() {
			v := new(T)
			if err := rows.StructScan(v); err != nil {
				retErr = err
				break
			}
			if cleanuper, ok := any(v).(interface{ cleanup() }); ok {
				cleanuper.cleanup()
			}
			if !yield(*v) {
				return
			}
		}
		if err := rows.Err(); err != nil && retErr == nil {
			retErr = err
		}
	}, func() error { return retErr }
}

// dbVector is a wrapper that allows protocol.Vector values to be serialized
// to and from the database.
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
		return wrap(err)
	}
	v.Vector = vec

	return nil
}

// indirectFI constructs a FileInfo from separate marshalled FileInfo and
// BlockList bytes.
type indirectFI struct {
	Name       string // not used, must be present as dest for Need iterator
	FiProtobuf []byte
	BlProtobuf []byte
	Size       int64 // not used
	Modified   int64 // not used
}

func (i indirectFI) FileInfo() (protocol.FileInfo, error) {
	var fi bep.FileInfo
	if err := proto.Unmarshal(i.FiProtobuf, &fi); err != nil {
		return protocol.FileInfo{}, wrap(err, "unmarshal fileinfo")
	}
	if len(i.BlProtobuf) > 0 {
		var bl dbproto.BlockList
		if err := proto.Unmarshal(i.BlProtobuf, &bl); err != nil {
			return protocol.FileInfo{}, wrap(err, "unmarshal blocklist")
		}
		fi.Blocks = bl.Blocks
	}
	fi.Name = osutil.NativeFilename(fi.Name)
	return protocol.FileInfoFromDB(&fi), nil
}

func prefixEnd(s string) string {
	if s == "" {
		panic("bug: cannot represent end prefix for empty string")
	}
	bs := []byte(s)
	for i := len(bs) - 1; i >= 0; i-- {
		if bs[i] < 0xff {
			bs[i]++
			break
		}
	}
	return string(bs)
}
