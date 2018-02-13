package cli

// CommandCategories is a slice of *CommandCategory.
type CommandCategories []*CommandCategory

// CommandCategory is a category containing commands.
type CommandCategory struct {
	Name     string
	Commands Commands
}

func (c CommandCategories) Less(i, j int) bool {
	return c[i].Name < c[j].Name
}

func (c CommandCategories) Len() int {
	return len(c)
}

func (c CommandCategories) Swap(i, j int) {
	c[i], c[j] = c[j], c[i]
}

// AddCommand adds a command to a category.
func (c CommandCategories) AddCommand(category string, command Command) CommandCategories {
	for _, commandCategory := range c {
		if commandCategory.Name == category {
			commandCategory.Commands = append(commandCategory.Commands, command)
			return c
		}
	}
	return append(c, &CommandCategory{Name: category, Commands: []Command{command}})
}

// VisibleCommands returns a slice of the Commands with Hidden=false
func (c *CommandCategory) VisibleCommands() []Command {
	ret := []Command{}
	for _, command := range c.Commands {
		if !command.Hidden {
			ret = append(ret, command)
		}
	}
	return ret
}
