// Copyright 2012 Jesse van den Kieboom. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package flags

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"reflect"
	"strings"
	"unicode/utf8"
)

type alignmentInfo struct {
	maxLongLen      int
	hasShort        bool
	hasValueName    bool
	terminalColumns int
}

func (p *Parser) getAlignmentInfo() alignmentInfo {
	ret := alignmentInfo{
		maxLongLen:      0,
		hasShort:        false,
		hasValueName:    false,
		terminalColumns: getTerminalColumns(),
	}

	if ret.terminalColumns <= 0 {
		ret.terminalColumns = 80
	}

	p.eachActiveGroup(func(grp *Group) {
		for _, info := range grp.options {
			if info.ShortName != 0 {
				ret.hasShort = true
			}

			lv := utf8.RuneCountInString(info.ValueName)

			if lv != 0 {
				ret.hasValueName = true
			}

			l := utf8.RuneCountInString(info.LongName) + lv

			if l > ret.maxLongLen {
				ret.maxLongLen = l
			}
		}
	})

	return ret
}

func (p *Parser) writeHelpOption(writer *bufio.Writer, option *Option, info alignmentInfo) {
	line := &bytes.Buffer{}

	distanceBetweenOptionAndDescription := 2
	paddingBeforeOption := 2

	line.WriteString(strings.Repeat(" ", paddingBeforeOption))

	if option.ShortName != 0 {
		line.WriteRune(defaultShortOptDelimiter)
		line.WriteRune(option.ShortName)
	} else if info.hasShort {
		line.WriteString("  ")
	}

	descstart := info.maxLongLen + paddingBeforeOption + distanceBetweenOptionAndDescription

	if info.hasShort {
		descstart += 2
	}

	if info.maxLongLen > 0 {
		descstart += 4
	}

	if info.hasValueName {
		descstart += 3
	}

	if len(option.LongName) > 0 {
		if option.ShortName != 0 {
			line.WriteString(", ")
		} else if info.hasShort {
			line.WriteString("  ")
		}

		line.WriteString(defaultLongOptDelimiter)
		line.WriteString(option.LongName)
	}

	if option.canArgument() {
		line.WriteRune(defaultNameArgDelimiter)

		if len(option.ValueName) > 0 {
			line.WriteString(option.ValueName)
		}
	}

	written := line.Len()
	line.WriteTo(writer)

	if option.Description != "" {
		dw := descstart - written
		writer.WriteString(strings.Repeat(" ", dw))

		def := ""
		defs := option.Default

		if len(option.DefaultMask) != 0 {
			if option.DefaultMask != "-" {
				def = option.DefaultMask
			}
		} else if len(defs) == 0 && option.canArgument() {
			var showdef bool

			switch option.field.Type.Kind() {
			case reflect.Func, reflect.Ptr:
				showdef = !option.value.IsNil()
			case reflect.Slice, reflect.String, reflect.Array:
				showdef = option.value.Len() > 0
			case reflect.Map:
				showdef = !option.value.IsNil() && option.value.Len() > 0
			default:
				zeroval := reflect.Zero(option.field.Type)
				showdef = !reflect.DeepEqual(zeroval.Interface(), option.value.Interface())
			}

			if showdef {
				def, _ = convertToString(option.value, option.tag)
			}
		} else if len(defs) != 0 {
			def = strings.Join(defs, ", ")
		}

		var desc string

		if def != "" {
			desc = fmt.Sprintf("%s (%v)", option.Description, def)
		} else {
			desc = option.Description
		}

		writer.WriteString(wrapText(desc,
			info.terminalColumns-descstart,
			strings.Repeat(" ", descstart)))
	}

	writer.WriteString("\n")
}

func maxCommandLength(s []*Command) int {
	if len(s) == 0 {
		return 0
	}

	ret := len(s[0].Name)

	for _, v := range s[1:] {
		l := len(v.Name)

		if l > ret {
			ret = l
		}
	}

	return ret
}

// WriteHelp writes a help message containing all the possible options and
// their descriptions to the provided writer. Note that the HelpFlag parser
// option provides a convenient way to add a -h/--help option group to the
// command line parser which will automatically show the help messages using
// this method.
func (p *Parser) WriteHelp(writer io.Writer) {
	if writer == nil {
		return
	}

	wr := bufio.NewWriter(writer)
	aligninfo := p.getAlignmentInfo()

	cmd := p.Command

	for cmd.Active != nil {
		cmd = cmd.Active
	}

	if p.Name != "" {
		wr.WriteString("Usage:\n")
		wr.WriteString(" ")

		allcmd := p.Command

		for allcmd != nil {
			var usage string

			if allcmd == p.Command {
				if len(p.Usage) != 0 {
					usage = p.Usage
				} else {
					usage = "[OPTIONS]"
				}
			} else if us, ok := allcmd.data.(Usage); ok {
				usage = us.Usage()
			} else {
				usage = fmt.Sprintf("[%s-OPTIONS]", allcmd.Name)
			}

			if len(usage) != 0 {
				fmt.Fprintf(wr, " %s %s", allcmd.Name, usage)
			} else {
				fmt.Fprintf(wr, " %s", allcmd.Name)
			}

			allcmd = allcmd.Active
		}

		fmt.Fprintln(wr)

		if len(cmd.LongDescription) != 0 {
			fmt.Fprintln(wr)

			t := wrapText(cmd.LongDescription,
				aligninfo.terminalColumns,
				"")

			fmt.Fprintln(wr, t)
		}
	}

	p.eachActiveGroup(func(grp *Group) {
		first := true

		for _, info := range grp.options {
			if info.canCli() {
				if first {
					fmt.Fprintf(wr, "\n%s:\n", grp.ShortDescription)
					first = false
				}

				p.writeHelpOption(wr, info, aligninfo)
			}
		}
	})

	scommands := cmd.sortedCommands()

	if len(scommands) > 0 {
		maxnamelen := maxCommandLength(scommands)

		fmt.Fprintln(wr)
		fmt.Fprintln(wr, "Available commands:")

		for _, c := range scommands {
			fmt.Fprintf(wr, "  %s", c.Name)

			if len(c.ShortDescription) > 0 {
				pad := strings.Repeat(" ", maxnamelen-len(c.Name))
				fmt.Fprintf(wr, "%s  %s", pad, c.ShortDescription)
			}

			fmt.Fprintln(wr)
		}
	}

	wr.Flush()
}
