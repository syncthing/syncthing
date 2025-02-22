package sqlite

import (
	"iter"

	"github.com/jmoiron/sqlx"
	"google.golang.org/protobuf/proto"
)

func iterStructs[T any](rows *sqlx.Rows, err error) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		var zero T
		if err != nil {
			yield(zero, err)
			return
		}
		defer rows.Close()
		for rows.Next() {
			v := new(T)
			if err := rows.StructScan(v); err != nil {
				yield(zero, err)
				return
			}
			if !yield(*v, nil) {
				return
			}
		}
		if err := rows.Err(); err != nil {
			yield(zero, err)
		}
	}
}

func iterProtos[T any, PT pbMessage[T]](rows *sqlx.Rows, err error) iter.Seq2[*T, error] {
	return func(yield func(*T, error) bool) {
		if err != nil {
			yield(nil, err)
			return
		}
		defer rows.Close()
		var bs []byte
		for rows.Next() {
			v := new(T)
			if err := rows.Scan(&bs); err != nil {
				yield(nil, err)
				return
			}
			if err := proto.Unmarshal(bs, PT(v)); err != nil {
				yield(nil, err)
				return
			}
			if !yield(v, nil) {
				return
			}
		}
		if err := rows.Err(); err != nil {
			yield(nil, err)
		}
	}
}

func unlockIter[K, V any](unlock func(), it iter.Seq2[K, V]) iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		defer unlock()
		for k, v := range it {
			if !yield(k, v) {
				break
			}
		}
	}
}
