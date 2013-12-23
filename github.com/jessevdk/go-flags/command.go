package flags

// Command represents an application command. Commands can be added to the
// parser (which itself is a command) and are selected/executed when its name
// is specified on the command line. The Command type embeds a Group and
// therefore also carries a set of command specific options.
type Command struct {
	// Embedded, see Group for more information
	*Group

	// The name by which the command can be invoked
	Name string

	// The active sub command (set by parsing) or nil
	Active *Command

	commands            []*Command
	hasBuiltinHelpGroup bool
}

// Commander is an interface which can be implemented by any command added in
// the options. When implemented, the Execute method will be called for the last
// specified (sub)command providing the remaining command line arguments.
type Commander interface {
	// Execute will be called for the last active (sub)command. The
	// args argument contains the remaining command line arguments. The
	// error that Execute returns will be eventually passed out of the
	// Parse method of the Parser.
	Execute(args []string) error
}

// Usage is an interface which can be implemented to show a custom usage string
// in the help message shown for a command.
type Usage interface {
	// Usage is called for commands to allow customized printing of command
	// usage in the generated help message.
	Usage() string
}

// AddCommand adds a new command to the parser with the given name and data. The
// data needs to be a pointer to a struct from which the fields indicate which
// options are in the command. The provided data can implement the Command and
// Usage interfaces.
func (c *Command) AddCommand(command string, shortDescription string, longDescription string, data interface{}) (*Command, error) {
	cmd := newCommand(command, shortDescription, longDescription, data)

	if err := cmd.scan(); err != nil {
		return nil, err
	}

	c.commands = append(c.commands, cmd)
	return cmd, nil
}

// AddGroup adds a new group to the command with the given name and data. The
// data needs to be a pointer to a struct from which the fields indicate which
// options are in the group.
func (c *Command) AddGroup(shortDescription string, longDescription string, data interface{}) (*Group, error) {
	group := newGroup(shortDescription, longDescription, data)

	if err := group.scanType(c.scanSubCommandHandler(group)); err != nil {
		return nil, err
	}

	c.groups = append(c.groups, group)
	return group, nil
}

// Commands returns a list of subcommands of this command.
func (c *Command) Commands() []*Command {
	return c.commands
}

// Find locates the subcommand with the given name and returns it. If no such
// command can be found Find will return nil.
func (c *Command) Find(name string) *Command {
	for _, cc := range c.commands {
		if cc.Name == name {
			return cc
		}
	}

	return nil
}
