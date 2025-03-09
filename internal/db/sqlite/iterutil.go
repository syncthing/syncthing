package sqlite

import (
	"iter"

	"github.com/jmoiron/sqlx"
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
			if !yield(*v) {
				return
			}
		}
		if err := rows.Err(); err != nil && retErr == nil {
			retErr = err
		}
	}, func() error { return retErr }
}
