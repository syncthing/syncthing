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

func iterMap[A, B any](i iter.Seq2[A, error], fn func(A) B) iter.Seq2[B, error] {
	return func(yield func(B, error) bool) {
		for a, err := range i {
			if err != nil {
				var zero B
				yield(zero, err)
				return
			}
			if !yield(fn(a), nil) {
				return
			}
		}
	}
}

func iterCollect[T any](i iter.Seq2[T, error]) ([]T, error) {
	var s []T
	for v, err := range i {
		if err != nil {
			return nil, err
		}
		s = append(s, v)
	}
	return s, nil
}
