package flags

import (
	"strings"
)

// Windows uses a front slash for both short and long options.  Also it uses
// a colon for name/argument delimter.
const (
	defaultShortOptDelimiter = '/'
	defaultLongOptDelimiter  = "/"
	defaultNameArgDelimiter  = ':'
)

func argumentIsOption(arg string) bool {
	// Windows-style options allow front slash for the option
	// delimiter.
	return len(arg) > 0 && (arg[0] == '-' || arg[0] == '/')
}

// stripOptionPrefix returns the option without the prefix and whether or
// not the option is a long option or not.
func stripOptionPrefix(optname string) (prefix string, name string, islong bool) {
	// Determine if the argument is a long option or not.  Windows
	// typically supports both long and short options with a single
	// front slash as the option delimiter, so handle this situation
	// nicely.
	possplit := 0

	if strings.HasPrefix(optname, "--") {
		possplit = 2
		islong = true
	} else if strings.HasPrefix(optname, "-") {
		possplit = 1
		islong = false
	} else if strings.HasPrefix(optname, "/") {
		possplit = 1
		islong = len(optname) > 2
	}

	return optname[:possplit], optname[possplit:], islong
}

// splitOption attempts to split the passed option into a name and an argument.
// When there is no argument specified, nil will be returned for it.
func splitOption(prefix string, option string, islong bool) (string, *string) {
	if len(option) == 0 {
		return option, nil
	}

	// Windows typically uses a colon for the option name and argument
	// delimiter while POSIX typically uses an equals.  Support both styles,
	// but don't allow the two to be mixed.  That is to say /foo:bar and
	// --foo=bar are acceptable, but /foo=bar and --foo:bar are not.
	var pos int

	if prefix == "/" {
		pos = strings.Index(option, ":")
	} else if len(prefix) > 0 {
		pos = strings.Index(option, "=")
	}

	if (islong && pos >= 0) || (!islong && pos == 1) {
		rest := option[pos+1:]
		return option[:pos], &rest
	}

	return option, nil
}

// addHelpGroup adds a new group that contains default help parameters.
func (c *Command) addHelpGroup(showHelp func() error) *Group {
	// Windows CLI applications typically use /? for help, so make both
	// that available as well as the POSIX style h and help.
	var help struct {
		ShowHelpWindows func() error `short:"?" description:"Show this help message"`
		ShowHelpPosix   func() error `short:"h" long:"help" description:"Show this help message"`
	}

	help.ShowHelpWindows = showHelp
	help.ShowHelpPosix = showHelp

	ret, _ := c.AddGroup("Help Options", "", &help)
	return ret
}
