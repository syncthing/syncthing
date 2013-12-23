package flags

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"
)

type parseState struct {
	arg     string
	args    []string
	retargs []string
	err     error

	command *Command
	lookup  lookup
}

func (p *parseState) eof() bool {
	return len(p.args) == 0
}

func (p *parseState) pop() string {
	if p.eof() {
		return ""
	}

	p.arg = p.args[0]
	p.args = p.args[1:]

	return p.arg
}

func (p *parseState) peek() string {
	if p.eof() {
		return ""
	}

	return p.args[0]
}

func (p *parseState) checkRequired() error {
	required := p.lookup.required

	if len(required) == 0 {
		return nil
	}

	names := make([]string, 0, len(required))

	for k := range required {
		names = append(names, "`"+k.String()+"'")
	}

	var msg string

	if len(names) == 1 {
		msg = fmt.Sprintf("the required flag %s was not specified", names[0])
	} else {
		msg = fmt.Sprintf("the required flags %s and %s were not specified",
			strings.Join(names[:len(names)-1], ", "), names[len(names)-1])
	}

	p.err = newError(ErrRequired, msg)
	return p.err
}

func (p *parseState) estimateCommand() error {
	commands := p.command.sortedCommands()
	cmdnames := make([]string, len(commands))

	for i, v := range commands {
		cmdnames[i] = v.Name
	}

	var msg string

	if len(p.retargs) != 0 {
		c, l := closestChoice(p.retargs[0], cmdnames)
		msg = fmt.Sprintf("Unknown command `%s'", p.retargs[0])

		if float32(l)/float32(len(c)) < 0.5 {
			msg = fmt.Sprintf("%s, did you mean `%s'?", msg, c)
		} else if len(cmdnames) == 1 {
			msg = fmt.Sprintf("%s. You should use the %s command",
				msg,
				cmdnames[0])
		} else {
			msg = fmt.Sprintf("%s. Please specify one command of: %s or %s",
				msg,
				strings.Join(cmdnames[:len(cmdnames)-1], ", "),
				cmdnames[len(cmdnames)-1])
		}
	} else {
		if len(cmdnames) == 1 {
			msg = fmt.Sprintf("Please specify the %s command", cmdnames[0])
		} else {
			msg = fmt.Sprintf("Please specify one command of: %s or %s",
				strings.Join(cmdnames[:len(cmdnames)-1], ", "),
				cmdnames[len(cmdnames)-1])
		}
	}

	return newError(ErrRequired, msg)
}

func (p *Parser) parseOption(s *parseState, name string, option *Option, canarg bool, argument *string) (retoption *Option, err error) {
	if !option.canArgument() {
		if argument != nil {
			msg := fmt.Sprintf("bool flag `%s' cannot have an argument", option)
			return option, newError(ErrNoArgumentForBool, msg)
		}

		err = option.set(nil)
	} else if argument != nil {
		err = option.set(argument)
	} else if canarg && !s.eof() {
		arg := s.pop()
		err = option.set(&arg)
	} else if option.OptionalArgument {
		option.clear()

		for _, v := range option.OptionalValue {
			err = option.set(&v)

			if err != nil {
				break
			}
		}
	} else {
		msg := fmt.Sprintf("expected argument for flag `%s'", option)
		err = newError(ErrExpectedArgument, msg)
	}

	if err != nil {
		if _, ok := err.(*Error); !ok {
			msg := fmt.Sprintf("invalid argument for flag `%s' (expected %s): %s",
				option,
				option.value.Type(),
				err.Error())

			err = newError(ErrMarshal, msg)
		}
	}

	return option, err
}

func (p *Parser) parseLong(s *parseState, name string, argument *string) (option *Option, err error) {
	if option := s.lookup.longNames[name]; option != nil {
		// Only long options that are required can consume an argument
		// from the argument list
		canarg := !option.OptionalArgument

		return p.parseOption(s, name, option, canarg, argument)
	}

	return nil, newError(ErrUnknownFlag, fmt.Sprintf("unknown flag `%s'", name))
}

func (p *Parser) splitShortConcatArg(s *parseState, optname string) (string, *string) {
	c, n := utf8.DecodeRuneInString(optname)

	if n == len(optname) {
		return optname, nil
	}

	first := string(c)

	if option := s.lookup.shortNames[first]; option != nil && option.canArgument() {
		arg := optname[n:]
		return first, &arg
	}

	return optname, nil
}

func (p *Parser) parseShort(s *parseState, optname string, argument *string) (option *Option, err error) {
	if argument == nil {
		optname, argument = p.splitShortConcatArg(s, optname)
	}

	for i, c := range optname {
		shortname := string(c)

		if option = s.lookup.shortNames[shortname]; option != nil {
			// Only the last short argument can consume an argument from
			// the arguments list, and only if it's non optional
			canarg := (i+utf8.RuneLen(c) == len(optname)) && !option.OptionalArgument

			if _, err := p.parseOption(s, shortname, option, canarg, argument); err != nil {
				return option, err
			}
		} else {
			return nil, newError(ErrUnknownFlag, fmt.Sprintf("unknown flag `%s'", shortname))
		}

		// Only the first option can have a concatted argument, so just
		// clear argument here
		argument = nil
	}

	return option, nil
}

func (p *Parser) parseNonOption(s *parseState) error {
	if cmd := s.lookup.commands[s.arg]; cmd != nil {
		if err := s.checkRequired(); err != nil {
			return err
		}

		s.command.Active = cmd

		s.command = cmd
		s.lookup = cmd.makeLookup()
	} else if (p.Options & PassAfterNonOption) != None {
		// If PassAfterNonOption is set then all remaining arguments
		// are considered positional
		s.retargs = append(append(s.retargs, s.arg), s.args...)
		s.args = []string{}
	} else {
		s.retargs = append(s.retargs, s.arg)
	}

	return nil
}

func (p *Parser) showBuiltinHelp() error {
	var b bytes.Buffer

	p.WriteHelp(&b)
	return newError(ErrHelp, b.String())
}

func (p *Parser) printError(err error) error {
	if err != nil && (p.Options&PrintErrors) != None {
		fmt.Fprintln(os.Stderr, err)
	}

	return err
}
