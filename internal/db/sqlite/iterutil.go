package sqlite

import (
	"iter"

	"github.com/jmoiron/sqlx"
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

func iterStructsErr[T any](errptr *error, rows *sqlx.Rows, err error) iter.Seq[T] {
	return func(yield func(T) bool) {
		if err != nil {
			*errptr = err
			return
		}
		defer rows.Close()
		for rows.Next() {
			v := new(T)
			if err := rows.StructScan(v); err != nil {
				*errptr = err
				return
			}
			if !yield(*v) {
				break
			}
		}
		if err := rows.Err(); err != nil {
			*errptr = err
		}
	}
}
