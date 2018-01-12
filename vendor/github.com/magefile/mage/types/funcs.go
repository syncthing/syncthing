package types

import (
	"context"
	"fmt"
)

// FuncType indicates a prototype of build job function
type FuncType int

// FuncTypes
const (
	InvalidType FuncType = iota
	VoidType
	ErrorType
	ContextVoidType
	ContextErrorType
)

// FuncCheck tests if a function is one of FuncType
func FuncCheck(fn interface{}) error {
	switch fn.(type) {
	case func():
		return nil
	case func() error:
		return nil
	case func(context.Context):
		return nil
	case func(context.Context) error:
		return nil
	}
	return fmt.Errorf("Invalid type for dependent function: %T. Dependencies must be func(), func() error, func(context.Context) or func(context.Context) error", fn)
}

// FuncTypeWrap wraps a valid FuncType to FuncContextError
func FuncTypeWrap(fn interface{}) func(context.Context) error {
	if FuncCheck(fn) == nil {
		switch f := fn.(type) {
		case func():
			return func(context.Context) error {
				f()
				return nil
			}
		case func() error:
			return func(context.Context) error {
				return f()
			}
		case func(context.Context):
			return func(ctx context.Context) error {
				f(ctx)
				return nil
			}
		case func(context.Context) error:
			return f
		}
	}
	return nil
}
