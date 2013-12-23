package flags

import (
	"reflect"
	"sort"
	"strings"
	"unsafe"
)

type lookup struct {
	shortNames map[string]*Option
	longNames  map[string]*Option

	required map[*Option]bool
	commands map[string]*Command
}

func newCommand(name string, shortDescription string, longDescription string, data interface{}) *Command {
	return &Command{
		Group: newGroup(shortDescription, longDescription, data),
		Name:  name,
	}
}

func (c *Command) scanSubCommandHandler(parentg *Group) scanHandler {
	f := func(realval reflect.Value, sfield *reflect.StructField) (bool, error) {
		mtag := newMultiTag(string(sfield.Tag))

		if err := mtag.Parse(); err != nil {
			return true, err
		}

		subcommand := mtag.Get("command")

		if len(subcommand) != 0 {
			ptrval := reflect.NewAt(realval.Type(), unsafe.Pointer(realval.UnsafeAddr()))

			shortDescription := mtag.Get("description")
			longDescription := mtag.Get("long-description")

			if _, err := c.AddCommand(subcommand, shortDescription, longDescription, ptrval.Interface()); err != nil {
				return true, err
			}

			return true, nil
		}

		return parentg.scanSubGroupHandler(realval, sfield)
	}

	return f
}

func (c *Command) scan() error {
	return c.scanType(c.scanSubCommandHandler(c.Group))
}

func (c *Command) eachCommand(f func(*Command), recurse bool) {
	f(c)

	for _, cc := range c.commands {
		if recurse {
			cc.eachCommand(f, true)
		} else {
			f(cc)
		}
	}
}

func (c *Command) eachActiveGroup(f func(g *Group)) {
	c.eachGroup(f)

	if c.Active != nil {
		c.Active.eachActiveGroup(f)
	}
}

func (c *Command) addHelpGroups(showHelp func() error) {
	if !c.hasBuiltinHelpGroup {
		c.addHelpGroup(showHelp)
		c.hasBuiltinHelpGroup = true
	}

	for _, cc := range c.commands {
		cc.addHelpGroups(showHelp)
	}
}

func (c *Command) makeLookup() lookup {
	ret := lookup{
		shortNames: make(map[string]*Option),
		longNames:  make(map[string]*Option),

		required: make(map[*Option]bool),
		commands: make(map[string]*Command),
	}

	c.eachGroup(func(g *Group) {
		for _, option := range g.options {
			if option.Required && option.canCli() {
				ret.required[option] = true
			}

			if option.ShortName != 0 {
				ret.shortNames[string(option.ShortName)] = option
			}

			if len(option.LongName) > 0 {
				ret.longNames[option.LongName] = option
			}
		}
	})

	for _, subcommand := range c.commands {
		ret.commands[subcommand.Name] = subcommand
	}

	return ret
}

func (c *Command) groupByName(name string) *Group {
	if grp := c.Group.groupByName(name); grp != nil {
		return grp
	}

	for _, subc := range c.commands {
		prefix := subc.Name + "."

		if strings.HasPrefix(name, prefix) {
			if grp := subc.groupByName(name[len(prefix):]); grp != nil {
				return grp
			}
		} else if name == subc.Name {
			return subc.Group
		}
	}

	return nil
}

type commandList []*Command

func (c commandList) Less(i, j int) bool {
	return c[i].Name < c[j].Name
}

func (c commandList) Len() int {
	return len(c)
}

func (c commandList) Swap(i, j int) {
	c[i], c[j] = c[j], c[i]
}

func (c *Command) sortedCommands() []*Command {
	ret := make(commandList, len(c.commands))
	copy(ret, c.commands)

	sort.Sort(ret)
	return []*Command(ret)
}
