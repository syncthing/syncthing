package flags

import (
	"fmt"
	"io"
	"strings"
	"time"
)

func formatForMan(wr io.Writer, s string) {
	for {
		idx := strings.IndexRune(s, '`')

		if idx < 0 {
			fmt.Fprintf(wr, "%s", s)
			break
		}

		fmt.Fprintf(wr, "%s", s[:idx])

		s = s[idx+1:]
		idx = strings.IndexRune(s, '\'')

		if idx < 0 {
			fmt.Fprintf(wr, "%s", s)
			break
		}

		fmt.Fprintf(wr, "\\fB%s\\fP", s[:idx])
		s = s[idx+1:]
	}
}

func writeManPageOptions(wr io.Writer, grp *Group) {
	grp.eachGroup(func(group *Group) {
		for _, opt := range group.options {
			if !opt.canCli() {
				continue
			}

			fmt.Fprintln(wr, ".TP")
			fmt.Fprintf(wr, "\\fB")

			if opt.ShortName != 0 {
				fmt.Fprintf(wr, "-%c", opt.ShortName)
			}

			if len(opt.LongName) != 0 {
				if opt.ShortName != 0 {
					fmt.Fprintf(wr, ", ")
				}

				fmt.Fprintf(wr, "--%s", opt.LongName)
			}

			fmt.Fprintln(wr, "\\fP")
			formatForMan(wr, opt.Description)
			fmt.Fprintln(wr, "")
		}
	})
}

func writeManPageSubCommands(wr io.Writer, name string, root *Command) {
	commands := root.sortedCommands()

	for _, c := range commands {
		var nn string

		if len(name) != 0 {
			nn = name + " " + c.Name
		} else {
			nn = c.Name
		}

		writeManPageCommand(wr, nn, c)
	}
}

func writeManPageCommand(wr io.Writer, name string, command *Command) {
	fmt.Fprintf(wr, ".SS %s\n", name)
	fmt.Fprintln(wr, command.ShortDescription)

	if len(command.LongDescription) > 0 {
		fmt.Fprintln(wr, "")

		cmdstart := fmt.Sprintf("The %s command", command.Name)

		if strings.HasPrefix(command.LongDescription, cmdstart) {
			fmt.Fprintf(wr, "The \\fI%s\\fP command", command.Name)

			formatForMan(wr, command.LongDescription[len(cmdstart):])
			fmt.Fprintln(wr, "")
		} else {
			formatForMan(wr, command.LongDescription)
			fmt.Fprintln(wr, "")
		}
	}

	writeManPageOptions(wr, command.Group)
	writeManPageSubCommands(wr, name, command)
}

// WriteManPage writes a basic man page in groff format to the specified
// writer.
func (p *Parser) WriteManPage(wr io.Writer) {
	t := time.Now()

	fmt.Fprintf(wr, ".TH %s 1 \"%s\"\n", p.Name, t.Format("2 January 2006"))
	fmt.Fprintln(wr, ".SH NAME")
	fmt.Fprintf(wr, "%s \\- %s\n", p.Name, p.ShortDescription)
	fmt.Fprintln(wr, ".SH SYNOPSIS")

	usage := p.Usage

	if len(usage) == 0 {
		usage = "[OPTIONS]"
	}

	fmt.Fprintf(wr, "\\fB%s\\fP %s\n", p.Name, usage)
	fmt.Fprintln(wr, ".SH DESCRIPTION")

	formatForMan(wr, p.LongDescription)
	fmt.Fprintln(wr, "")

	fmt.Fprintln(wr, ".SH OPTIONS")

	writeManPageOptions(wr, p.Command.Group)

	if len(p.commands) > 0 {
		fmt.Fprintln(wr, ".SH COMMANDS")

		writeManPageSubCommands(wr, "", p.Command)
	}
}
