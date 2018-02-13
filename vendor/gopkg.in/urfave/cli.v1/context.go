package cli

import (
	"errors"
	"flag"
	"reflect"
	"strings"
	"syscall"
)

// Context is a type that is passed through to
// each Handler action in a cli application. Context
// can be used to retrieve context-specific Args and
// parsed command-line options.
type Context struct {
	App           *App
	Command       Command
	shellComplete bool
	flagSet       *flag.FlagSet
	setFlags      map[string]bool
	parentContext *Context
}

// NewContext creates a new context. For use in when invoking an App or Command action.
func NewContext(app *App, set *flag.FlagSet, parentCtx *Context) *Context {
	c := &Context{App: app, flagSet: set, parentContext: parentCtx}

	if parentCtx != nil {
		c.shellComplete = parentCtx.shellComplete
	}

	return c
}

// NumFlags returns the number of flags set
func (c *Context) NumFlags() int {
	return c.flagSet.NFlag()
}

// Set sets a context flag to a value.
func (c *Context) Set(name, value string) error {
	c.setFlags = nil
	return c.flagSet.Set(name, value)
}

// GlobalSet sets a context flag to a value on the global flagset
func (c *Context) GlobalSet(name, value string) error {
	globalContext(c).setFlags = nil
	return globalContext(c).flagSet.Set(name, value)
}

// IsSet determines if the flag was actually set
func (c *Context) IsSet(name string) bool {
	if c.setFlags == nil {
		c.setFlags = make(map[string]bool)

		c.flagSet.Visit(func(f *flag.Flag) {
			c.setFlags[f.Name] = true
		})

		c.flagSet.VisitAll(func(f *flag.Flag) {
			if _, ok := c.setFlags[f.Name]; ok {
				return
			}
			c.setFlags[f.Name] = false
		})

		// XXX hack to support IsSet for flags with EnvVar
		//
		// There isn't an easy way to do this with the current implementation since
		// whether a flag was set via an environment variable is very difficult to
		// determine here. Instead, we intend to introduce a backwards incompatible
		// change in version 2 to add `IsSet` to the Flag interface to push the
		// responsibility closer to where the information required to determine
		// whether a flag is set by non-standard means such as environment
		// variables is avaliable.
		//
		// See https://github.com/urfave/cli/issues/294 for additional discussion
		flags := c.Command.Flags
		if c.Command.Name == "" { // cannot == Command{} since it contains slice types
			if c.App != nil {
				flags = c.App.Flags
			}
		}
		for _, f := range flags {
			eachName(f.GetName(), func(name string) {
				if isSet, ok := c.setFlags[name]; isSet || !ok {
					return
				}

				val := reflect.ValueOf(f)
				if val.Kind() == reflect.Ptr {
					val = val.Elem()
				}

				envVarValue := val.FieldByName("EnvVar")
				if !envVarValue.IsValid() {
					return
				}

				eachName(envVarValue.String(), func(envVar string) {
					envVar = strings.TrimSpace(envVar)
					if _, ok := syscall.Getenv(envVar); ok {
						c.setFlags[name] = true
						return
					}
				})
			})
		}
	}

	return c.setFlags[name]
}

// GlobalIsSet determines if the global flag was actually set
func (c *Context) GlobalIsSet(name string) bool {
	ctx := c
	if ctx.parentContext != nil {
		ctx = ctx.parentContext
	}

	for ; ctx != nil; ctx = ctx.parentContext {
		if ctx.IsSet(name) {
			return true
		}
	}
	return false
}

// FlagNames returns a slice of flag names used in this context.
func (c *Context) FlagNames() (names []string) {
	for _, flag := range c.Command.Flags {
		name := strings.Split(flag.GetName(), ",")[0]
		if name == "help" {
			continue
		}
		names = append(names, name)
	}
	return
}

// GlobalFlagNames returns a slice of global flag names used by the app.
func (c *Context) GlobalFlagNames() (names []string) {
	for _, flag := range c.App.Flags {
		name := strings.Split(flag.GetName(), ",")[0]
		if name == "help" || name == "version" {
			continue
		}
		names = append(names, name)
	}
	return
}

// Parent returns the parent context, if any
func (c *Context) Parent() *Context {
	return c.parentContext
}

// value returns the value of the flag coressponding to `name`
func (c *Context) value(name string) interface{} {
	return c.flagSet.Lookup(name).Value.(flag.Getter).Get()
}

// Args contains apps console arguments
type Args []string

// Args returns the command line arguments associated with the context.
func (c *Context) Args() Args {
	args := Args(c.flagSet.Args())
	return args
}

// NArg returns the number of the command line arguments.
func (c *Context) NArg() int {
	return len(c.Args())
}

// Get returns the nth argument, or else a blank string
func (a Args) Get(n int) string {
	if len(a) > n {
		return a[n]
	}
	return ""
}

// First returns the first argument, or else a blank string
func (a Args) First() string {
	return a.Get(0)
}

// Tail returns the rest of the arguments (not the first one)
// or else an empty string slice
func (a Args) Tail() []string {
	if len(a) >= 2 {
		return []string(a)[1:]
	}
	return []string{}
}

// Present checks if there are any arguments present
func (a Args) Present() bool {
	return len(a) != 0
}

// Swap swaps arguments at the given indexes
func (a Args) Swap(from, to int) error {
	if from >= len(a) || to >= len(a) {
		return errors.New("index out of range")
	}
	a[from], a[to] = a[to], a[from]
	return nil
}

func globalContext(ctx *Context) *Context {
	if ctx == nil {
		return nil
	}

	for {
		if ctx.parentContext == nil {
			return ctx
		}
		ctx = ctx.parentContext
	}
}

func lookupGlobalFlagSet(name string, ctx *Context) *flag.FlagSet {
	if ctx.parentContext != nil {
		ctx = ctx.parentContext
	}
	for ; ctx != nil; ctx = ctx.parentContext {
		if f := ctx.flagSet.Lookup(name); f != nil {
			return ctx.flagSet
		}
	}
	return nil
}

func copyFlag(name string, ff *flag.Flag, set *flag.FlagSet) {
	switch ff.Value.(type) {
	case *StringSlice:
	default:
		set.Set(name, ff.Value.String())
	}
}

func normalizeFlags(flags []Flag, set *flag.FlagSet) error {
	visited := make(map[string]bool)
	set.Visit(func(f *flag.Flag) {
		visited[f.Name] = true
	})
	for _, f := range flags {
		parts := strings.Split(f.GetName(), ",")
		if len(parts) == 1 {
			continue
		}
		var ff *flag.Flag
		for _, name := range parts {
			name = strings.Trim(name, " ")
			if visited[name] {
				if ff != nil {
					return errors.New("Cannot use two forms of the same flag: " + name + " " + ff.Name)
				}
				ff = set.Lookup(name)
			}
		}
		if ff == nil {
			continue
		}
		for _, name := range parts {
			name = strings.Trim(name, " ")
			if !visited[name] {
				copyFlag(name, ff, set)
			}
		}
	}
	return nil
}
