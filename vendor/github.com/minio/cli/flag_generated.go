package cli

import (
	"flag"
	"strconv"
	"time"
)

// WARNING: This file is generated!

// BoolFlag is a flag with type bool
type BoolFlag struct {
	Name        string
	Usage       string
	EnvVar      string
	Hidden      bool
	Destination *bool
}

// String returns a readable representation of this value
// (for usage defaults)
func (f BoolFlag) String() string {
	return FlagStringer(f)
}

// GetName returns the name of the flag
func (f BoolFlag) GetName() string {
	return f.Name
}

// Bool looks up the value of a local BoolFlag, returns
// false if not found
func (c *Context) Bool(name string) bool {
	return lookupBool(name, c.flagSet)
}

// GlobalBool looks up the value of a global BoolFlag, returns
// false if not found
func (c *Context) GlobalBool(name string) bool {
	if fs := lookupGlobalFlagSet(name, c); fs != nil {
		return lookupBool(name, fs)
	}
	return false
}

func lookupBool(name string, set *flag.FlagSet) bool {
	f := set.Lookup(name)
	if f != nil {
		parsed, err := strconv.ParseBool(f.Value.String())
		if err != nil {
			return false
		}
		return parsed
	}
	return false
}

// BoolTFlag is a flag with type bool that is true by default
type BoolTFlag struct {
	Name        string
	Usage       string
	EnvVar      string
	Hidden      bool
	Destination *bool
}

// String returns a readable representation of this value
// (for usage defaults)
func (f BoolTFlag) String() string {
	return FlagStringer(f)
}

// GetName returns the name of the flag
func (f BoolTFlag) GetName() string {
	return f.Name
}

// BoolT looks up the value of a local BoolTFlag, returns
// false if not found
func (c *Context) BoolT(name string) bool {
	return lookupBoolT(name, c.flagSet)
}

// GlobalBoolT looks up the value of a global BoolTFlag, returns
// false if not found
func (c *Context) GlobalBoolT(name string) bool {
	if fs := lookupGlobalFlagSet(name, c); fs != nil {
		return lookupBoolT(name, fs)
	}
	return false
}

func lookupBoolT(name string, set *flag.FlagSet) bool {
	f := set.Lookup(name)
	if f != nil {
		parsed, err := strconv.ParseBool(f.Value.String())
		if err != nil {
			return false
		}
		return parsed
	}
	return false
}

// DurationFlag is a flag with type time.Duration (see https://golang.org/pkg/time/#ParseDuration)
type DurationFlag struct {
	Name        string
	Usage       string
	EnvVar      string
	Hidden      bool
	Value       time.Duration
	Destination *time.Duration
}

// String returns a readable representation of this value
// (for usage defaults)
func (f DurationFlag) String() string {
	return FlagStringer(f)
}

// GetName returns the name of the flag
func (f DurationFlag) GetName() string {
	return f.Name
}

// Duration looks up the value of a local DurationFlag, returns
// 0 if not found
func (c *Context) Duration(name string) time.Duration {
	return lookupDuration(name, c.flagSet)
}

// GlobalDuration looks up the value of a global DurationFlag, returns
// 0 if not found
func (c *Context) GlobalDuration(name string) time.Duration {
	if fs := lookupGlobalFlagSet(name, c); fs != nil {
		return lookupDuration(name, fs)
	}
	return 0
}

func lookupDuration(name string, set *flag.FlagSet) time.Duration {
	f := set.Lookup(name)
	if f != nil {
		parsed, err := time.ParseDuration(f.Value.String())
		if err != nil {
			return 0
		}
		return parsed
	}
	return 0
}

// Float64Flag is a flag with type float64
type Float64Flag struct {
	Name        string
	Usage       string
	EnvVar      string
	Hidden      bool
	Value       float64
	Destination *float64
}

// String returns a readable representation of this value
// (for usage defaults)
func (f Float64Flag) String() string {
	return FlagStringer(f)
}

// GetName returns the name of the flag
func (f Float64Flag) GetName() string {
	return f.Name
}

// Float64 looks up the value of a local Float64Flag, returns
// 0 if not found
func (c *Context) Float64(name string) float64 {
	return lookupFloat64(name, c.flagSet)
}

// GlobalFloat64 looks up the value of a global Float64Flag, returns
// 0 if not found
func (c *Context) GlobalFloat64(name string) float64 {
	if fs := lookupGlobalFlagSet(name, c); fs != nil {
		return lookupFloat64(name, fs)
	}
	return 0
}

func lookupFloat64(name string, set *flag.FlagSet) float64 {
	f := set.Lookup(name)
	if f != nil {
		parsed, err := strconv.ParseFloat(f.Value.String(), 64)
		if err != nil {
			return 0
		}
		return parsed
	}
	return 0
}

// GenericFlag is a flag with type Generic
type GenericFlag struct {
	Name   string
	Usage  string
	EnvVar string
	Hidden bool
	Value  Generic
}

// String returns a readable representation of this value
// (for usage defaults)
func (f GenericFlag) String() string {
	return FlagStringer(f)
}

// GetName returns the name of the flag
func (f GenericFlag) GetName() string {
	return f.Name
}

// Generic looks up the value of a local GenericFlag, returns
// nil if not found
func (c *Context) Generic(name string) interface{} {
	return lookupGeneric(name, c.flagSet)
}

// GlobalGeneric looks up the value of a global GenericFlag, returns
// nil if not found
func (c *Context) GlobalGeneric(name string) interface{} {
	if fs := lookupGlobalFlagSet(name, c); fs != nil {
		return lookupGeneric(name, fs)
	}
	return nil
}

func lookupGeneric(name string, set *flag.FlagSet) interface{} {
	f := set.Lookup(name)
	if f != nil {
		parsed, err := f.Value, error(nil)
		if err != nil {
			return nil
		}
		return parsed
	}
	return nil
}

// Int64Flag is a flag with type int64
type Int64Flag struct {
	Name        string
	Usage       string
	EnvVar      string
	Hidden      bool
	Value       int64
	Destination *int64
}

// String returns a readable representation of this value
// (for usage defaults)
func (f Int64Flag) String() string {
	return FlagStringer(f)
}

// GetName returns the name of the flag
func (f Int64Flag) GetName() string {
	return f.Name
}

// Int64 looks up the value of a local Int64Flag, returns
// 0 if not found
func (c *Context) Int64(name string) int64 {
	return lookupInt64(name, c.flagSet)
}

// GlobalInt64 looks up the value of a global Int64Flag, returns
// 0 if not found
func (c *Context) GlobalInt64(name string) int64 {
	if fs := lookupGlobalFlagSet(name, c); fs != nil {
		return lookupInt64(name, fs)
	}
	return 0
}

func lookupInt64(name string, set *flag.FlagSet) int64 {
	f := set.Lookup(name)
	if f != nil {
		parsed, err := strconv.ParseInt(f.Value.String(), 0, 64)
		if err != nil {
			return 0
		}
		return parsed
	}
	return 0
}

// IntFlag is a flag with type int
type IntFlag struct {
	Name        string
	Usage       string
	EnvVar      string
	Hidden      bool
	Value       int
	Destination *int
}

// String returns a readable representation of this value
// (for usage defaults)
func (f IntFlag) String() string {
	return FlagStringer(f)
}

// GetName returns the name of the flag
func (f IntFlag) GetName() string {
	return f.Name
}

// Int looks up the value of a local IntFlag, returns
// 0 if not found
func (c *Context) Int(name string) int {
	return lookupInt(name, c.flagSet)
}

// GlobalInt looks up the value of a global IntFlag, returns
// 0 if not found
func (c *Context) GlobalInt(name string) int {
	if fs := lookupGlobalFlagSet(name, c); fs != nil {
		return lookupInt(name, fs)
	}
	return 0
}

func lookupInt(name string, set *flag.FlagSet) int {
	f := set.Lookup(name)
	if f != nil {
		parsed, err := strconv.ParseInt(f.Value.String(), 0, 64)
		if err != nil {
			return 0
		}
		return int(parsed)
	}
	return 0
}

// IntSliceFlag is a flag with type *IntSlice
type IntSliceFlag struct {
	Name   string
	Usage  string
	EnvVar string
	Hidden bool
	Value  *IntSlice
}

// String returns a readable representation of this value
// (for usage defaults)
func (f IntSliceFlag) String() string {
	return FlagStringer(f)
}

// GetName returns the name of the flag
func (f IntSliceFlag) GetName() string {
	return f.Name
}

// IntSlice looks up the value of a local IntSliceFlag, returns
// nil if not found
func (c *Context) IntSlice(name string) []int {
	return lookupIntSlice(name, c.flagSet)
}

// GlobalIntSlice looks up the value of a global IntSliceFlag, returns
// nil if not found
func (c *Context) GlobalIntSlice(name string) []int {
	if fs := lookupGlobalFlagSet(name, c); fs != nil {
		return lookupIntSlice(name, fs)
	}
	return nil
}

func lookupIntSlice(name string, set *flag.FlagSet) []int {
	f := set.Lookup(name)
	if f != nil {
		parsed, err := (f.Value.(*IntSlice)).Value(), error(nil)
		if err != nil {
			return nil
		}
		return parsed
	}
	return nil
}

// Int64SliceFlag is a flag with type *Int64Slice
type Int64SliceFlag struct {
	Name   string
	Usage  string
	EnvVar string
	Hidden bool
	Value  *Int64Slice
}

// String returns a readable representation of this value
// (for usage defaults)
func (f Int64SliceFlag) String() string {
	return FlagStringer(f)
}

// GetName returns the name of the flag
func (f Int64SliceFlag) GetName() string {
	return f.Name
}

// Int64Slice looks up the value of a local Int64SliceFlag, returns
// nil if not found
func (c *Context) Int64Slice(name string) []int64 {
	return lookupInt64Slice(name, c.flagSet)
}

// GlobalInt64Slice looks up the value of a global Int64SliceFlag, returns
// nil if not found
func (c *Context) GlobalInt64Slice(name string) []int64 {
	if fs := lookupGlobalFlagSet(name, c); fs != nil {
		return lookupInt64Slice(name, fs)
	}
	return nil
}

func lookupInt64Slice(name string, set *flag.FlagSet) []int64 {
	f := set.Lookup(name)
	if f != nil {
		parsed, err := (f.Value.(*Int64Slice)).Value(), error(nil)
		if err != nil {
			return nil
		}
		return parsed
	}
	return nil
}

// StringFlag is a flag with type string
type StringFlag struct {
	Name        string
	Usage       string
	EnvVar      string
	Hidden      bool
	Value       string
	Destination *string
}

// String returns a readable representation of this value
// (for usage defaults)
func (f StringFlag) String() string {
	return FlagStringer(f)
}

// GetName returns the name of the flag
func (f StringFlag) GetName() string {
	return f.Name
}

// String looks up the value of a local StringFlag, returns
// "" if not found
func (c *Context) String(name string) string {
	return lookupString(name, c.flagSet)
}

// GlobalString looks up the value of a global StringFlag, returns
// "" if not found
func (c *Context) GlobalString(name string) string {
	if fs := lookupGlobalFlagSet(name, c); fs != nil {
		return lookupString(name, fs)
	}
	return ""
}

func lookupString(name string, set *flag.FlagSet) string {
	f := set.Lookup(name)
	if f != nil {
		parsed, err := f.Value.String(), error(nil)
		if err != nil {
			return ""
		}
		return parsed
	}
	return ""
}

// StringSliceFlag is a flag with type *StringSlice
type StringSliceFlag struct {
	Name   string
	Usage  string
	EnvVar string
	Hidden bool
	Value  *StringSlice
}

// String returns a readable representation of this value
// (for usage defaults)
func (f StringSliceFlag) String() string {
	return FlagStringer(f)
}

// GetName returns the name of the flag
func (f StringSliceFlag) GetName() string {
	return f.Name
}

// StringSlice looks up the value of a local StringSliceFlag, returns
// nil if not found
func (c *Context) StringSlice(name string) []string {
	return lookupStringSlice(name, c.flagSet)
}

// GlobalStringSlice looks up the value of a global StringSliceFlag, returns
// nil if not found
func (c *Context) GlobalStringSlice(name string) []string {
	if fs := lookupGlobalFlagSet(name, c); fs != nil {
		return lookupStringSlice(name, fs)
	}
	return nil
}

func lookupStringSlice(name string, set *flag.FlagSet) []string {
	f := set.Lookup(name)
	if f != nil {
		parsed, err := (f.Value.(*StringSlice)).Value(), error(nil)
		if err != nil {
			return nil
		}
		return parsed
	}
	return nil
}

// Uint64Flag is a flag with type uint64
type Uint64Flag struct {
	Name        string
	Usage       string
	EnvVar      string
	Hidden      bool
	Value       uint64
	Destination *uint64
}

// String returns a readable representation of this value
// (for usage defaults)
func (f Uint64Flag) String() string {
	return FlagStringer(f)
}

// GetName returns the name of the flag
func (f Uint64Flag) GetName() string {
	return f.Name
}

// Uint64 looks up the value of a local Uint64Flag, returns
// 0 if not found
func (c *Context) Uint64(name string) uint64 {
	return lookupUint64(name, c.flagSet)
}

// GlobalUint64 looks up the value of a global Uint64Flag, returns
// 0 if not found
func (c *Context) GlobalUint64(name string) uint64 {
	if fs := lookupGlobalFlagSet(name, c); fs != nil {
		return lookupUint64(name, fs)
	}
	return 0
}

func lookupUint64(name string, set *flag.FlagSet) uint64 {
	f := set.Lookup(name)
	if f != nil {
		parsed, err := strconv.ParseUint(f.Value.String(), 0, 64)
		if err != nil {
			return 0
		}
		return parsed
	}
	return 0
}

// UintFlag is a flag with type uint
type UintFlag struct {
	Name        string
	Usage       string
	EnvVar      string
	Hidden      bool
	Value       uint
	Destination *uint
}

// String returns a readable representation of this value
// (for usage defaults)
func (f UintFlag) String() string {
	return FlagStringer(f)
}

// GetName returns the name of the flag
func (f UintFlag) GetName() string {
	return f.Name
}

// Uint looks up the value of a local UintFlag, returns
// 0 if not found
func (c *Context) Uint(name string) uint {
	return lookupUint(name, c.flagSet)
}

// GlobalUint looks up the value of a global UintFlag, returns
// 0 if not found
func (c *Context) GlobalUint(name string) uint {
	if fs := lookupGlobalFlagSet(name, c); fs != nil {
		return lookupUint(name, fs)
	}
	return 0
}

func lookupUint(name string, set *flag.FlagSet) uint {
	f := set.Lookup(name)
	if f != nil {
		parsed, err := strconv.ParseUint(f.Value.String(), 0, 64)
		if err != nil {
			return 0
		}
		return uint(parsed)
	}
	return 0
}
