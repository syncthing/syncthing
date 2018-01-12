package altsrc

import (
	"flag"

	"gopkg.in/urfave/cli.v1"
)

// WARNING: This file is generated!

// BoolFlag is the flag type that wraps cli.BoolFlag to allow
// for other values to be specified
type BoolFlag struct {
	cli.BoolFlag
	set *flag.FlagSet
}

// NewBoolFlag creates a new BoolFlag
func NewBoolFlag(fl cli.BoolFlag) *BoolFlag {
	return &BoolFlag{BoolFlag: fl, set: nil}
}

// Apply saves the flagSet for later usage calls, then calls the
// wrapped BoolFlag.Apply
func (f *BoolFlag) Apply(set *flag.FlagSet) {
	f.set = set
	f.BoolFlag.Apply(set)
}

// ApplyWithError saves the flagSet for later usage calls, then calls the
// wrapped BoolFlag.ApplyWithError
func (f *BoolFlag) ApplyWithError(set *flag.FlagSet) error {
	f.set = set
	return f.BoolFlag.ApplyWithError(set)
}

// BoolTFlag is the flag type that wraps cli.BoolTFlag to allow
// for other values to be specified
type BoolTFlag struct {
	cli.BoolTFlag
	set *flag.FlagSet
}

// NewBoolTFlag creates a new BoolTFlag
func NewBoolTFlag(fl cli.BoolTFlag) *BoolTFlag {
	return &BoolTFlag{BoolTFlag: fl, set: nil}
}

// Apply saves the flagSet for later usage calls, then calls the
// wrapped BoolTFlag.Apply
func (f *BoolTFlag) Apply(set *flag.FlagSet) {
	f.set = set
	f.BoolTFlag.Apply(set)
}

// ApplyWithError saves the flagSet for later usage calls, then calls the
// wrapped BoolTFlag.ApplyWithError
func (f *BoolTFlag) ApplyWithError(set *flag.FlagSet) error {
	f.set = set
	return f.BoolTFlag.ApplyWithError(set)
}

// DurationFlag is the flag type that wraps cli.DurationFlag to allow
// for other values to be specified
type DurationFlag struct {
	cli.DurationFlag
	set *flag.FlagSet
}

// NewDurationFlag creates a new DurationFlag
func NewDurationFlag(fl cli.DurationFlag) *DurationFlag {
	return &DurationFlag{DurationFlag: fl, set: nil}
}

// Apply saves the flagSet for later usage calls, then calls the
// wrapped DurationFlag.Apply
func (f *DurationFlag) Apply(set *flag.FlagSet) {
	f.set = set
	f.DurationFlag.Apply(set)
}

// ApplyWithError saves the flagSet for later usage calls, then calls the
// wrapped DurationFlag.ApplyWithError
func (f *DurationFlag) ApplyWithError(set *flag.FlagSet) error {
	f.set = set
	return f.DurationFlag.ApplyWithError(set)
}

// Float64Flag is the flag type that wraps cli.Float64Flag to allow
// for other values to be specified
type Float64Flag struct {
	cli.Float64Flag
	set *flag.FlagSet
}

// NewFloat64Flag creates a new Float64Flag
func NewFloat64Flag(fl cli.Float64Flag) *Float64Flag {
	return &Float64Flag{Float64Flag: fl, set: nil}
}

// Apply saves the flagSet for later usage calls, then calls the
// wrapped Float64Flag.Apply
func (f *Float64Flag) Apply(set *flag.FlagSet) {
	f.set = set
	f.Float64Flag.Apply(set)
}

// ApplyWithError saves the flagSet for later usage calls, then calls the
// wrapped Float64Flag.ApplyWithError
func (f *Float64Flag) ApplyWithError(set *flag.FlagSet) error {
	f.set = set
	return f.Float64Flag.ApplyWithError(set)
}

// GenericFlag is the flag type that wraps cli.GenericFlag to allow
// for other values to be specified
type GenericFlag struct {
	cli.GenericFlag
	set *flag.FlagSet
}

// NewGenericFlag creates a new GenericFlag
func NewGenericFlag(fl cli.GenericFlag) *GenericFlag {
	return &GenericFlag{GenericFlag: fl, set: nil}
}

// Apply saves the flagSet for later usage calls, then calls the
// wrapped GenericFlag.Apply
func (f *GenericFlag) Apply(set *flag.FlagSet) {
	f.set = set
	f.GenericFlag.Apply(set)
}

// ApplyWithError saves the flagSet for later usage calls, then calls the
// wrapped GenericFlag.ApplyWithError
func (f *GenericFlag) ApplyWithError(set *flag.FlagSet) error {
	f.set = set
	return f.GenericFlag.ApplyWithError(set)
}

// Int64Flag is the flag type that wraps cli.Int64Flag to allow
// for other values to be specified
type Int64Flag struct {
	cli.Int64Flag
	set *flag.FlagSet
}

// NewInt64Flag creates a new Int64Flag
func NewInt64Flag(fl cli.Int64Flag) *Int64Flag {
	return &Int64Flag{Int64Flag: fl, set: nil}
}

// Apply saves the flagSet for later usage calls, then calls the
// wrapped Int64Flag.Apply
func (f *Int64Flag) Apply(set *flag.FlagSet) {
	f.set = set
	f.Int64Flag.Apply(set)
}

// ApplyWithError saves the flagSet for later usage calls, then calls the
// wrapped Int64Flag.ApplyWithError
func (f *Int64Flag) ApplyWithError(set *flag.FlagSet) error {
	f.set = set
	return f.Int64Flag.ApplyWithError(set)
}

// IntFlag is the flag type that wraps cli.IntFlag to allow
// for other values to be specified
type IntFlag struct {
	cli.IntFlag
	set *flag.FlagSet
}

// NewIntFlag creates a new IntFlag
func NewIntFlag(fl cli.IntFlag) *IntFlag {
	return &IntFlag{IntFlag: fl, set: nil}
}

// Apply saves the flagSet for later usage calls, then calls the
// wrapped IntFlag.Apply
func (f *IntFlag) Apply(set *flag.FlagSet) {
	f.set = set
	f.IntFlag.Apply(set)
}

// ApplyWithError saves the flagSet for later usage calls, then calls the
// wrapped IntFlag.ApplyWithError
func (f *IntFlag) ApplyWithError(set *flag.FlagSet) error {
	f.set = set
	return f.IntFlag.ApplyWithError(set)
}

// IntSliceFlag is the flag type that wraps cli.IntSliceFlag to allow
// for other values to be specified
type IntSliceFlag struct {
	cli.IntSliceFlag
	set *flag.FlagSet
}

// NewIntSliceFlag creates a new IntSliceFlag
func NewIntSliceFlag(fl cli.IntSliceFlag) *IntSliceFlag {
	return &IntSliceFlag{IntSliceFlag: fl, set: nil}
}

// Apply saves the flagSet for later usage calls, then calls the
// wrapped IntSliceFlag.Apply
func (f *IntSliceFlag) Apply(set *flag.FlagSet) {
	f.set = set
	f.IntSliceFlag.Apply(set)
}

// ApplyWithError saves the flagSet for later usage calls, then calls the
// wrapped IntSliceFlag.ApplyWithError
func (f *IntSliceFlag) ApplyWithError(set *flag.FlagSet) error {
	f.set = set
	return f.IntSliceFlag.ApplyWithError(set)
}

// Int64SliceFlag is the flag type that wraps cli.Int64SliceFlag to allow
// for other values to be specified
type Int64SliceFlag struct {
	cli.Int64SliceFlag
	set *flag.FlagSet
}

// NewInt64SliceFlag creates a new Int64SliceFlag
func NewInt64SliceFlag(fl cli.Int64SliceFlag) *Int64SliceFlag {
	return &Int64SliceFlag{Int64SliceFlag: fl, set: nil}
}

// Apply saves the flagSet for later usage calls, then calls the
// wrapped Int64SliceFlag.Apply
func (f *Int64SliceFlag) Apply(set *flag.FlagSet) {
	f.set = set
	f.Int64SliceFlag.Apply(set)
}

// ApplyWithError saves the flagSet for later usage calls, then calls the
// wrapped Int64SliceFlag.ApplyWithError
func (f *Int64SliceFlag) ApplyWithError(set *flag.FlagSet) error {
	f.set = set
	return f.Int64SliceFlag.ApplyWithError(set)
}

// StringFlag is the flag type that wraps cli.StringFlag to allow
// for other values to be specified
type StringFlag struct {
	cli.StringFlag
	set *flag.FlagSet
}

// NewStringFlag creates a new StringFlag
func NewStringFlag(fl cli.StringFlag) *StringFlag {
	return &StringFlag{StringFlag: fl, set: nil}
}

// Apply saves the flagSet for later usage calls, then calls the
// wrapped StringFlag.Apply
func (f *StringFlag) Apply(set *flag.FlagSet) {
	f.set = set
	f.StringFlag.Apply(set)
}

// ApplyWithError saves the flagSet for later usage calls, then calls the
// wrapped StringFlag.ApplyWithError
func (f *StringFlag) ApplyWithError(set *flag.FlagSet) error {
	f.set = set
	return f.StringFlag.ApplyWithError(set)
}

// StringSliceFlag is the flag type that wraps cli.StringSliceFlag to allow
// for other values to be specified
type StringSliceFlag struct {
	cli.StringSliceFlag
	set *flag.FlagSet
}

// NewStringSliceFlag creates a new StringSliceFlag
func NewStringSliceFlag(fl cli.StringSliceFlag) *StringSliceFlag {
	return &StringSliceFlag{StringSliceFlag: fl, set: nil}
}

// Apply saves the flagSet for later usage calls, then calls the
// wrapped StringSliceFlag.Apply
func (f *StringSliceFlag) Apply(set *flag.FlagSet) {
	f.set = set
	f.StringSliceFlag.Apply(set)
}

// ApplyWithError saves the flagSet for later usage calls, then calls the
// wrapped StringSliceFlag.ApplyWithError
func (f *StringSliceFlag) ApplyWithError(set *flag.FlagSet) error {
	f.set = set
	return f.StringSliceFlag.ApplyWithError(set)
}

// Uint64Flag is the flag type that wraps cli.Uint64Flag to allow
// for other values to be specified
type Uint64Flag struct {
	cli.Uint64Flag
	set *flag.FlagSet
}

// NewUint64Flag creates a new Uint64Flag
func NewUint64Flag(fl cli.Uint64Flag) *Uint64Flag {
	return &Uint64Flag{Uint64Flag: fl, set: nil}
}

// Apply saves the flagSet for later usage calls, then calls the
// wrapped Uint64Flag.Apply
func (f *Uint64Flag) Apply(set *flag.FlagSet) {
	f.set = set
	f.Uint64Flag.Apply(set)
}

// ApplyWithError saves the flagSet for later usage calls, then calls the
// wrapped Uint64Flag.ApplyWithError
func (f *Uint64Flag) ApplyWithError(set *flag.FlagSet) error {
	f.set = set
	return f.Uint64Flag.ApplyWithError(set)
}

// UintFlag is the flag type that wraps cli.UintFlag to allow
// for other values to be specified
type UintFlag struct {
	cli.UintFlag
	set *flag.FlagSet
}

// NewUintFlag creates a new UintFlag
func NewUintFlag(fl cli.UintFlag) *UintFlag {
	return &UintFlag{UintFlag: fl, set: nil}
}

// Apply saves the flagSet for later usage calls, then calls the
// wrapped UintFlag.Apply
func (f *UintFlag) Apply(set *flag.FlagSet) {
	f.set = set
	f.UintFlag.Apply(set)
}

// ApplyWithError saves the flagSet for later usage calls, then calls the
// wrapped UintFlag.ApplyWithError
func (f *UintFlag) ApplyWithError(set *flag.FlagSet) error {
	f.set = set
	return f.UintFlag.ApplyWithError(set)
}
