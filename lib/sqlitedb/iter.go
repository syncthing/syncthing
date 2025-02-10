package sqlitedb

import (
	"iter"

	"github.com/jmoiron/sqlx"
	"google.golang.org/protobuf/proto"
)

func iterStructs[T any](rows *sqlx.Rows, err error) iter.Seq2[T, error] {
	return iterFunc[T](rows, err, (*sqlx.Rows).StructScan)
}

func iterValues[T any](rows *sqlx.Rows, err error) iter.Seq2[T, error] {
	return iterFunc[T](rows, err, func(rows *sqlx.Rows, v any) error { return rows.Scan(v) })
}

func iterFunc[T any](rows *sqlx.Rows, err error, scan func(*sqlx.Rows, any) error) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		defer rows.Close()
		var zero T
		if err != nil {
			yield(zero, err)
			return
		}
		for rows.Next() {
			v := new(T)
			if err := scan(rows, v); err != nil {
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

func iterProto[T any, PT pbMessage[T]](i iter.Seq2[[]byte, error]) iter.Seq2[*T, error] {
	return func(yield func(*T, error) bool) {
		for bs, err := range i {
			if err != nil {
				yield(nil, err)
				return
			}
			var v T
			if err := proto.Unmarshal(bs, PT(&v)); err != nil {
				yield(nil, err)
				return
			}
			if !yield(&v, nil) {
				return
			}
		}
	}
}

func iterMap[A, B, C any](i iter.Seq2[A, C], fn func(A) B) iter.Seq2[B, C] {
	return func(yield func(B, C) bool) {
		for a, c := range i {
			if !yield(fn(a), c) {
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
