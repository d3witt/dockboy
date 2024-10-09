package app

import (
	"fmt"

	"github.com/d3witt/dockboy/cli/command"
	"github.com/d3witt/dockboy/config"
	"github.com/urfave/cli/v2"
)

func NewInitCmd(dockboyCli *command.Cli) *cli.Command {
	return &cli.Command{
		Name:  "init",
		Usage: "Initialize a new dockboy config",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "name",
				Aliases: []string{"n"},
				Usage:   "Name of the app",
			},
		},
		Action: func(ctx *cli.Context) error {
			name := ctx.String("name")

			_, err := config.NewDefaultConfig(name)
			if err != nil {
				return err
			}

			fmt.Println("dockboy.toml")

			return nil
		},
	}
}
