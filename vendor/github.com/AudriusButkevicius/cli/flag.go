package cli

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// This flag enables bash-completion for all commands and subcommands
var BashCompletionFlag = BoolFlag{
	Name: "generate-bash-completion",
}

// This flag prints the version for the application
var VersionFlag = BoolFlag{
	Name:  "version, v",
	Usage: "print the version",
}

// This flag prints the help for all commands and subcommands
var HelpFlag = BoolFlag{
	Name:  "help, h",
	Usage: "show help",
}

// Flag is a common interface related to parsing flags in cli.
// For more advanced flag parsing techniques, it is recomended that
// this interface be implemented.
type Flag interface {
	fmt.Stringer
	// Apply Flag settings to the given flag set
	Apply(*flag.FlagSet)
	getName() string
}

func flagSet(name string, flags []Flag) *flag.FlagSet {
	set := flag.NewFlagSet(name, flag.ContinueOnError)

	for _, f := range flags {
		f.Apply(set)
	}
	return set
}

func eachName(longName string, fn func(string)) {
	parts := strings.Split(longName, ",")
	for _, name := range parts {
		name = strings.Trim(name, " ")
		fn(name)
	}
}

// Generic is a generic parseable type identified by a specific flag
type Generic interface {
	Set(value string) error
	String() string
}

// GenericFlag is the flag type for types implementing Generic
type GenericFlag struct {
	Name   string
	Value  Generic
	Usage  string
	EnvVar string
}

func (f GenericFlag) String() string {
	return withEnvHint(f.EnvVar, fmt.Sprintf("%s%s %v\t`%v` %s", prefixFor(f.Name), f.Name, f.Value, "-"+f.Name+" option -"+f.Name+" option", f.Usage))
}

func (f GenericFlag) Apply(set *flag.FlagSet) {
	val := f.Value
	if f.EnvVar != "" {
		if envVal := os.Getenv(f.EnvVar); envVal != "" {
			val.Set(envVal)
		}
	}

	eachName(f.Name, func(name string) {
		set.Var(f.Value, name, f.Usage)
	})
}

func (f GenericFlag) getName() string {
	return f.Name
}

type StringSlice []string

func (f *StringSlice) Set(value string) error {
	*f = append(*f, value)
	return nil
}

func (f *StringSlice) String() string {
	return fmt.Sprintf("%s", *f)
}

func (f *StringSlice) Value() []string {
	return *f
}

type StringSliceFlag struct {
	Name   string
	Value  *StringSlice
	Usage  string
	EnvVar string
}

func (f StringSliceFlag) String() string {
	firstName := strings.Trim(strings.Split(f.Name, ",")[0], " ")
	pref := prefixFor(firstName)
	return withEnvHint(f.EnvVar, fmt.Sprintf("%s '%v'\t%v", prefixedNames(f.Name), pref+firstName+" option "+pref+firstName+" option", f.Usage))
}

func (f StringSliceFlag) Apply(set *flag.FlagSet) {
	if f.EnvVar != "" {
		if envVal := os.Getenv(f.EnvVar); envVal != "" {
			newVal := &StringSlice{}
			for _, s := range strings.Split(envVal, ",") {
				newVal.Set(s)
			}
			f.Value = newVal
		}
	}

	eachName(f.Name, func(name string) {
		set.Var(f.Value, name, f.Usage)
	})
}

func (f StringSliceFlag) getName() string {
	return f.Name
}

type IntSlice []int

func (f *IntSlice) Set(value string) error {

	tmp, err := strconv.Atoi(value)
	if err != nil {
		return err
	} else {
		*f = append(*f, tmp)
	}
	return nil
}

func (f *IntSlice) String() string {
	return fmt.Sprintf("%d", *f)
}

func (f *IntSlice) Value() []int {
	return *f
}

type IntSliceFlag struct {
	Name   string
	Value  *IntSlice
	Usage  string
	EnvVar string
}

func (f IntSliceFlag) String() string {
	firstName := strings.Trim(strings.Split(f.Name, ",")[0], " ")
	pref := prefixFor(firstName)
	return withEnvHint(f.EnvVar, fmt.Sprintf("%s '%v'\t%v", prefixedNames(f.Name), pref+firstName+" option "+pref+firstName+" option", f.Usage))
}

func (f IntSliceFlag) Apply(set *flag.FlagSet) {
	if f.EnvVar != "" {
		if envVal := os.Getenv(f.EnvVar); envVal != "" {
			newVal := &IntSlice{}
			for _, s := range strings.Split(envVal, ",") {
				err := newVal.Set(s)
				if err != nil {
					fmt.Fprintf(os.Stderr, err.Error())
				}
			}
			f.Value = newVal
		}
	}

	eachName(f.Name, func(name string) {
		set.Var(f.Value, name, f.Usage)
	})
}

func (f IntSliceFlag) getName() string {
	return f.Name
}

type BoolFlag struct {
	Name   string
	Usage  string
	EnvVar string
}

func (f BoolFlag) String() string {
	return withEnvHint(f.EnvVar, fmt.Sprintf("%s\t%v", prefixedNames(f.Name), f.Usage))
}

func (f BoolFlag) Apply(set *flag.FlagSet) {
	val := false
	if f.EnvVar != "" {
		if envVal := os.Getenv(f.EnvVar); envVal != "" {
			envValBool, err := strconv.ParseBool(envVal)
			if err == nil {
				val = envValBool
			}
		}
	}

	eachName(f.Name, func(name string) {
		set.Bool(name, val, f.Usage)
	})
}

func (f BoolFlag) getName() string {
	return f.Name
}

type BoolTFlag struct {
	Name   string
	Usage  string
	EnvVar string
}

func (f BoolTFlag) String() string {
	return withEnvHint(f.EnvVar, fmt.Sprintf("%s\t%v", prefixedNames(f.Name), f.Usage))
}

func (f BoolTFlag) Apply(set *flag.FlagSet) {
	val := true
	if f.EnvVar != "" {
		if envVal := os.Getenv(f.EnvVar); envVal != "" {
			envValBool, err := strconv.ParseBool(envVal)
			if err == nil {
				val = envValBool
			}
		}
	}

	eachName(f.Name, func(name string) {
		set.Bool(name, val, f.Usage)
	})
}

func (f BoolTFlag) getName() string {
	return f.Name
}

type StringFlag struct {
	Name   string
	Value  string
	Usage  string
	EnvVar string
}

func (f StringFlag) String() string {
	var fmtString string
	fmtString = "%s %v\t%v"

	if len(f.Value) > 0 {
		fmtString = "%s '%v'\t%v"
	} else {
		fmtString = "%s %v\t%v"
	}

	return withEnvHint(f.EnvVar, fmt.Sprintf(fmtString, prefixedNames(f.Name), f.Value, f.Usage))
}

func (f StringFlag) Apply(set *flag.FlagSet) {
	if f.EnvVar != "" {
		if envVal := os.Getenv(f.EnvVar); envVal != "" {
			f.Value = envVal
		}
	}

	eachName(f.Name, func(name string) {
		set.String(name, f.Value, f.Usage)
	})
}

func (f StringFlag) getName() string {
	return f.Name
}

type IntFlag struct {
	Name   string
	Value  int
	Usage  string
	EnvVar string
}

func (f IntFlag) String() string {
	return withEnvHint(f.EnvVar, fmt.Sprintf("%s '%v'\t%v", prefixedNames(f.Name), f.Value, f.Usage))
}

func (f IntFlag) Apply(set *flag.FlagSet) {
	if f.EnvVar != "" {
		if envVal := os.Getenv(f.EnvVar); envVal != "" {
			envValInt, err := strconv.ParseUint(envVal, 10, 64)
			if err == nil {
				f.Value = int(envValInt)
			}
		}
	}

	eachName(f.Name, func(name string) {
		set.Int(name, f.Value, f.Usage)
	})
}

func (f IntFlag) getName() string {
	return f.Name
}

type Float64Flag struct {
	Name   string
	Value  float64
	Usage  string
	EnvVar string
}

func (f Float64Flag) String() string {
	return withEnvHint(f.EnvVar, fmt.Sprintf("%s '%v'\t%v", prefixedNames(f.Name), f.Value, f.Usage))
}

func (f Float64Flag) Apply(set *flag.FlagSet) {
	if f.EnvVar != "" {
		if envVal := os.Getenv(f.EnvVar); envVal != "" {
			envValFloat, err := strconv.ParseFloat(envVal, 10)
			if err == nil {
				f.Value = float64(envValFloat)
			}
		}
	}

	eachName(f.Name, func(name string) {
		set.Float64(name, f.Value, f.Usage)
	})
}

func (f Float64Flag) getName() string {
	return f.Name
}

func prefixFor(name string) (prefix string) {
	if len(name) == 1 {
		prefix = "-"
	} else {
		prefix = "--"
	}

	return
}

func prefixedNames(fullName string) (prefixed string) {
	parts := strings.Split(fullName, ",")
	for i, name := range parts {
		name = strings.Trim(name, " ")
		prefixed += prefixFor(name) + name
		if i < len(parts)-1 {
			prefixed += ", "
		}
	}
	return
}

func withEnvHint(envVar, str string) string {
	envText := ""
	if envVar != "" {
		envText = fmt.Sprintf(" [$%s]", envVar)
	}
	return str + envText
}
