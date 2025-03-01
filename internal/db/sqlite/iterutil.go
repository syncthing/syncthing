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

func iterStructsErrFn[T any](rows *sqlx.Rows, err error) (iter.Seq[T], func() error) {
	if err != nil {
		return func(yield func(T) bool) {}, func() error { return err }
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
			if !yield(*v) {
				return
			}
		}
		if err := rows.Err(); err != nil && retErr == nil {
			retErr = err
		}
	}, func() error { return retErr }
}

func iterMapErrFn[A, B any](i iter.Seq[A], errFn func() error, mapFn func(A) (B, error)) (iter.Seq[B], func() error) {
	var retErr error
	return func(yield func(B) bool) {
			for v := range i {
				mapped, err := mapFn(v)
				if err != nil {
					retErr = err
					return
				}
				if !yield(mapped) {
					return
				}
			}
		}, func() error {
			if prevErr := errFn(); prevErr != nil {
				return prevErr
			}
			return retErr
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
