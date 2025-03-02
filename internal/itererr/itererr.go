package itererr

import "iter"

func Map[A, B any](i iter.Seq2[A, error], fn func(A) B) iter.Seq2[B, error] {
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

func Map2[A, B any](i iter.Seq2[A, error], fn func(A) (B, error)) iter.Seq2[B, error] {
	return func(yield func(B, error) bool) {
		for a, err := range i {
			var b B
			if err == nil {
				b, err = fn(a)
			}
			if err != nil {
				yield(b, err)
				return
			}
			if !yield(b, nil) {
				return
			}
		}
	}
}

func MapErr[A, B any](errptr *error, i iter.Seq[A], fn func(A) (B, error)) iter.Seq[B] {
	return func(yield func(B) bool) {
		for a := range i {
			b, err := fn(a)
			if err != nil {
				*errptr = err
				return
			}
			if !yield(b) {
				return
			}
		}
	}
}

func Collect[T any](i iter.Seq2[T, error]) ([]T, error) {
	var s []T
	for v, err := range i {
		if err != nil {
			return nil, err
		}
		s = append(s, v)
	}
	return s, nil
}

func Error[T any](err error) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		var zero T
		yield(zero, err)
	}
}

func Zip[T any](it iter.Seq[T], errFn func() error) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		for v := range it {
			if !yield(v, nil) {
				break
			}
		}
		if err := errFn(); err != nil {
			var zero T
			yield(zero, err)
		}
	}
}
