package slogutil

import (
	"context"
	"log/slog"
	"strings"
)

type contextKey int

const (
	extraArgs contextKey = iota
)

func funcNameToPkg(fn string) string {
	fn = strings.ToLower(fn)
	fn = strings.TrimPrefix(fn, "github.com/syncthing/syncthing/lib/")
	fn = strings.TrimPrefix(fn, "github.com/syncthing/syncthing/internal/")

	pkgTypFn := strings.Split(fn, ".") // [package, type, method] or [package, function]
	if len(pkgTypFn) <= 2 {
		return pkgTypFn[0]
	}

	pkg := pkgTypFn[0]
	// Remove parenthesis and asterisk from the type name
	typ := strings.TrimLeft(strings.TrimRight(pkgTypFn[1], ")"), "(*")
	// Skip certain type names that add no value
	typ = strings.TrimSuffix(typ, "service")
	switch typ {
	case pkg, "", "serveparams":
		return pkg
	default:
		return pkg + "." + typ
	}
}

// With returns a new context with added log attributes. Arguments should be
// key and value pairs, or slog.Attr instances.
func With(ctx context.Context, args ...any) context.Context {
	extra, _ := ctx.Value(extraArgs).([]slog.Attr)

	for len(args) > 0 {
		var a slog.Attr
		a, args = argsToAttr(args)
		extra = append(extra, a)
	}

	return context.WithValue(ctx, extraArgs, extra)
}

// copy of the unexported method in log/slog, lightly modified
func argsToAttr(args []any) (slog.Attr, []any) {
	const badKey = "!BADKEY"
	switch x := args[0].(type) {
	case string:
		if len(args) == 1 {
			return slog.String(badKey, x), nil
		}
		return slog.Any(x, args[1]), args[2:]

	case slog.Attr:
		return x, args[1:]

	default:
		return slog.Any(badKey, x), args[1:]
	}
}

type expensive struct {
	fn func() any
}

func (e expensive) LogValue() slog.Value {
	return slog.AnyValue(e.fn())
}

func Expensive(fn func() any) expensive {
	return expensive{fn}
}
