package flags

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
)

type iniValue struct {
	Name  string
	Value string
}

type iniSection []iniValue
type ini map[string]iniSection

func readFullLine(reader *bufio.Reader) (string, error) {
	var line []byte

	for {
		l, more, err := reader.ReadLine()

		if err != nil {
			return "", err
		}

		if line == nil && !more {
			return string(l), nil
		}

		line = append(line, l...)

		if !more {
			break
		}
	}

	return string(line), nil
}

func optionIniName(option *Option) string {
	name := option.tag.Get("_read-ini-name")

	if len(name) != 0 {
		return name
	}

	name = option.tag.Get("ini-name")

	if len(name) != 0 {
		return name
	}

	return option.field.Name
}

func writeGroupIni(group *Group, namespace string, writer io.Writer, options IniOptions) {
	var sname string

	if len(namespace) != 0 {
		sname = namespace + "." + group.ShortDescription
	} else {
		sname = group.ShortDescription
	}

	sectionwritten := false
	comments := (options & IniIncludeComments) != IniNone

	for _, option := range group.options {
		if option.isFunc() {
			continue
		}

		if len(option.tag.Get("no-ini")) != 0 {
			continue
		}

		val := option.value

		if (options&IniIncludeDefaults) == IniNone &&
			reflect.DeepEqual(val, option.defaultValue) {
			continue
		}

		if !sectionwritten {
			fmt.Fprintf(writer, "[%s]\n", sname)
			sectionwritten = true
		}

		if comments {
			fmt.Fprintf(writer, "; %s\n", option.Description)
		}

		oname := optionIniName(option)

		switch val.Type().Kind() {
		case reflect.Slice:
			for idx := 0; idx < val.Len(); idx++ {
				v, _ := convertToString(val.Index(idx), option.tag)
				fmt.Fprintf(writer, "%s = %s\n", oname, v)
			}

			if val.Len() == 0 {
				fmt.Fprintf(writer, "; %s =\n", oname)
			}
		case reflect.Map:
			for _, key := range val.MapKeys() {
				k, _ := convertToString(key, option.tag)
				v, _ := convertToString(val.MapIndex(key), option.tag)

				fmt.Fprintf(writer, "%s = %s:%s\n", oname, k, v)
			}

			if val.Len() == 0 {
				fmt.Fprintf(writer, "; %s =\n", oname)
			}
		default:
			v, _ := convertToString(val, option.tag)

			if len(v) != 0 {
				fmt.Fprintf(writer, "%s = %s\n", oname, v)
			} else {
				fmt.Fprintf(writer, "%s =\n", oname)
			}
		}

		if comments {
			fmt.Fprintln(writer)
		}
	}

	if sectionwritten && !comments {
		fmt.Fprintln(writer)
	}
}

func writeCommandIni(command *Command, namespace string, writer io.Writer, options IniOptions) {
	command.eachGroup(func(group *Group) {
		writeGroupIni(group, namespace, writer, options)
	})

	for _, c := range command.commands {
		var nns string

		if len(namespace) != 0 {
			nns = c.Name + "." + nns
		} else {
			nns = c.Name
		}

		writeCommandIni(c, nns, writer, options)
	}
}

func writeIni(parser *IniParser, writer io.Writer, options IniOptions) {
	writeCommandIni(parser.parser.Command, "", writer, options)
}

func readIniFromFile(filename string) (ini, error) {
	file, err := os.Open(filename)

	if err != nil {
		return nil, err
	}

	defer file.Close()

	return readIni(file, filename)
}

func readIni(contents io.Reader, filename string) (ini, error) {
	ret := make(ini)

	reader := bufio.NewReader(contents)

	// Empty global section
	section := make(iniSection, 0, 10)
	sectionname := ""

	ret[sectionname] = section

	var lineno uint

	for {
		line, err := readFullLine(reader)

		if err == io.EOF {
			break
		}

		if err != nil {
			return nil, err
		}

		lineno++
		line = strings.TrimSpace(line)

		// Skip empty lines and lines starting with ; (comments)
		if len(line) == 0 || line[0] == ';' {
			continue
		}

		if line[0] == '[' {
			if line[0] != '[' || line[len(line)-1] != ']' {
				return nil, &IniError{
					Message:    "malformed section header",
					File:       filename,
					LineNumber: lineno,
				}
			}

			name := strings.TrimSpace(line[1 : len(line)-1])

			if len(name) == 0 {
				return nil, &IniError{
					Message:    "empty section name",
					File:       filename,
					LineNumber: lineno,
				}
			}

			sectionname = name
			section = ret[name]

			if section == nil {
				section = make(iniSection, 0, 10)
				ret[name] = section
			}

			continue
		}

		// Parse option here
		keyval := strings.SplitN(line, "=", 2)

		if len(keyval) != 2 {
			return nil, &IniError{
				Message:    fmt.Sprintf("malformed key=value (%s)", line),
				File:       filename,
				LineNumber: lineno,
			}
		}

		name := strings.TrimSpace(keyval[0])
		value := strings.TrimSpace(keyval[1])

		section = append(section, iniValue{
			Name:  name,
			Value: value,
		})

		ret[sectionname] = section
	}

	return ret, nil
}

func (i *IniParser) matchingGroups(name string) []*Group {
	if len(name) == 0 {
		var ret []*Group

		i.parser.eachGroup(func(g *Group) {
			ret = append(ret, g)
		})

		return ret
	}

	g := i.parser.groupByName(name)

	if g != nil {
		return []*Group{g}
	}

	return nil
}

func (i *IniParser) parse(ini ini) error {
	p := i.parser

	for name, section := range ini {
		groups := i.matchingGroups(name)

		if len(groups) == 0 {
			return newError(ErrUnknownGroup,
				fmt.Sprintf("could not find option group `%s'", name))
		}

		for _, inival := range section {
			var opt *Option

			for _, group := range groups {
				opt = group.optionByName(inival.Name, func(o *Option, n string) bool {
					return strings.ToLower(o.tag.Get("ini-name")) == strings.ToLower(n)
				})

				if opt != nil && len(opt.tag.Get("no-ini")) != 0 {
					opt = nil
				}

				if opt != nil {
					break
				}
			}

			if opt == nil {
				if (p.Options & IgnoreUnknown) == None {
					return newError(ErrUnknownFlag,
						fmt.Sprintf("unknown option: %s", inival.Name))
				}

				continue
			}

			pval := &inival.Value

			if !opt.canArgument() && len(inival.Value) == 0 {
				pval = nil
			}

			if err := opt.set(pval); err != nil {
				return wrapError(err)
			}

			opt.tag.Set("_read-ini-name", inival.Name)
		}
	}

	return nil
}
