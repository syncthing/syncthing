package protoutil

import (
	"fmt"

	"google.golang.org/protobuf/proto"
)

func MarshalTo(buf []byte, pb proto.Message) (int, error) {
	if sz := proto.Size(pb); len(buf) < sz {
		return 0, fmt.Errorf("buffer too small")
	} else if sz == 0 {
		return 0, nil
	}
	opts := proto.MarshalOptions{}
	bs, err := opts.MarshalAppend(buf[:0], pb)
	if err != nil {
		return 0, err
	}
	if &buf[0] != &bs[0] {
		panic("can't happen: slice was reallocated")
	}
	return len(bs), nil
}
