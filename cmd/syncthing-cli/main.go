package main

import (
	"github.com/AudriusButkevicius/cli"
	"time"
)

func main() {
	app := cli.NewApp()
	app.Name = "syncthing-cli"
	app.Author = "Audrius Butkevičius"
	app.Email = "audrius.butkevicius@gmail.com"
	app.Compiled = time.Now()
	app.Usage = "Syncthing command line interface"
	app.Version = "0.1"
	app.HideHelp = true

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "endpoint, e",
			Value: "http://127.0.0.1:8080",
			Usage: "End point to connect to",
		},
		cli.StringFlag{
			Name:  "apikey, k",
			Value: "",
			Usage: "API Key",
		},
		cli.StringFlag{
			Name:  "username, u",
			Value: "",
			Usage: "Username",
		},
		cli.StringFlag{
			Name:  "password, p",
			Value: "",
			Usage: "Password",
		},
	}

	app.Commands = []cli.Command{
		repositoryCommand,
		nodeCommand,
		guiCommand,
		optionsCommand,
		reportCommand,
		errorCommand,
	}
	app.Commands = append(app.Commands, generalCommands...)
	app.RunAndExitOnError()
}
