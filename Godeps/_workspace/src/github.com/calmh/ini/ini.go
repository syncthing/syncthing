// Package ini provides trivial parsing of .INI format files.
package ini

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

// Config is a parsed INI format file.
type Config struct {
	sections []section
	comments []string
}

type section struct {
	name     string
	comments []string
	options  []option
}

type option struct {
	name, value string
}

var (
	iniSectionRe = regexp.MustCompile(`^\[(.+)\]$`)
	iniOptionRe  = regexp.MustCompile(`^([^\s=]+)\s*=\s*(.+?)$`)
)

// Sections returns the list of sections in the file.
func (c *Config) Sections() []string {
	var sections []string
	for _, sect := range c.sections {
		sections = append(sections, sect.name)
	}
	return sections
}

// Options returns the list of options in a given section.
func (c *Config) Options(section string) []string {
	var options []string
	for _, sect := range c.sections {
		if sect.name == section {
			for _, opt := range sect.options {
				options = append(options, opt.name)
			}
			break
		}
	}
	return options
}

// OptionMap returns the map option => value for a given section.
func (c *Config) OptionMap(section string) map[string]string {
	options := make(map[string]string)
	for _, sect := range c.sections {
		if sect.name == section {
			for _, opt := range sect.options {
				options[opt.name] = opt.value
			}
			break
		}
	}
	return options
}

// Comments returns the list of comments in a given section.
// For the empty string, returns the file comments.
func (c *Config) Comments(section string) []string {
	if section == "" {
		return c.comments
	}
	for _, sect := range c.sections {
		if sect.name == section {
			return sect.comments
		}
	}
	return nil
}

// AddComments appends the comment to the list of comments for the section.
func (c *Config) AddComment(sect, comment string) {
	if sect == "" {
		c.comments = append(c.comments, comment)
		return
	}

	for i, s := range c.sections {
		if s.name == sect {
			c.sections[i].comments = append(s.comments, comment)
			return
		}
	}

	c.sections = append(c.sections, section{
		name:     sect,
		comments: []string{comment},
	})
}

// Parse reads the given io.Reader and returns a parsed Config object.
func Parse(stream io.Reader) Config {
	var cfg Config
	var curSection string

	scanner := bufio.NewScanner(bufio.NewReader(stream))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			comment := strings.TrimLeft(line, ";# ")
			cfg.AddComment(curSection, comment)
		} else if len(line) > 0 {
			if m := iniSectionRe.FindStringSubmatch(line); len(m) > 0 {
				curSection = m[1]
			} else if m := iniOptionRe.FindStringSubmatch(line); len(m) > 0 {
				key := m[1]
				val := m[2]
				if !strings.Contains(val, "\"") {
					// If val does not contain any quote characers, we can make it
					// a quoted string and safely let strconv.Unquote sort out any
					// escapes
					val = "\"" + val + "\""
				}
				if val[0] == '"' {
					val, _ = strconv.Unquote(val)
				}

				cfg.Set(curSection, key, val)
			}
		}
	}
	return cfg
}

// Write writes the sections and options to the io.Writer in INI format.
func (c *Config) Write(out io.Writer) error {
	for _, cmt := range c.comments {
		fmt.Fprintln(out, "; "+cmt)
	}
	if len(c.comments) > 0 {
		fmt.Fprintln(out)
	}

	for _, sect := range c.sections {
		fmt.Fprintf(out, "[%s]\n", sect.name)
		for _, cmt := range sect.comments {
			fmt.Fprintln(out, "; "+cmt)
		}
		for _, opt := range sect.options {
			val := opt.value
			if len(val) == 0 {
				continue
			}

			// Quote the string if it begins or ends with space
			needsQuoting := val[0] == ' ' || val[len(val)-1] == ' '

			if !needsQuoting {
				// Quote the string if it contains any unprintable characters
				for _, r := range val {
					if !strconv.IsPrint(r) {
						needsQuoting = true
						break
					}
				}
			}

			if needsQuoting {
				val = strconv.Quote(val)
			}

			fmt.Fprintf(out, "%s=%s\n", opt.name, val)
		}
		fmt.Fprintln(out)
	}
	return nil
}

// Get gets the value from the specified section and key name, or the empty
// string if either the section or the key is missing.
func (c *Config) Get(section, key string) string {
	for _, sect := range c.sections {
		if sect.name == section {
			for _, opt := range sect.options {
				if opt.name == key {
					return opt.value
				}
			}
			return ""
		}
	}
	return ""
}

// Set sets a value for an option in a section. If the option exists, it's
// value will be overwritten. If the option does not exist, it will be added.
// If the section does not exist, it will be added and the option added to it.
func (c *Config) Set(sectionName, key, value string) {
	for i, sect := range c.sections {
		if sect.name == sectionName {
			for j, opt := range sect.options {
				if opt.name == key {
					c.sections[i].options[j].value = value
					return
				}
			}
			c.sections[i].options = append(sect.options, option{key, value})
			return
		}
	}

	c.sections = append(c.sections, section{
		name:    sectionName,
		options: []option{{key, value}},
	})
}
