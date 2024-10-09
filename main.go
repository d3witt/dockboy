package main

import (
	"fmt"
	"log"
	"log/slog"
	"os"

	"github.com/d3witt/dockboy/cli/command"
	"github.com/d3witt/dockboy/cli/command/app"
	"github.com/d3witt/dockboy/cli/command/machine"
	"github.com/d3witt/dockboy/streams"
	"github.com/urfave/cli/v2"
)

var version = "dev" // set by build script

func main() {
	dockboyCli := &command.Cli{
		In:  streams.StdIn,
		Out: streams.StdOut,
		Err: streams.StdErr,
	}

	app := &cli.App{
		Name:    "dockboy",
		Usage:   "Manage your SSH keys and remote machines",
		Version: version,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "debug",
				Usage: "Enable debug output",
			},
		},
		Before: func(ctx *cli.Context) error {
			if ctx.Bool("debug") {
				slog.SetLogLoggerLevel(slog.LevelDebug)
			}
			return nil
		},
		Commands: []*cli.Command{
			app.NewInitCmd(dockboyCli),
			app.NewDeployCmd(dockboyCli),
			app.NewLogsCommand(dockboyCli),
			app.NewDestroyCmd(dockboyCli),
			app.NewInfoCmd(dockboyCli),
			machine.NewPurgeCmd(dockboyCli),
			machine.NewExecuteCmd(dockboyCli),
		},
		Suggest:   true,
		Reader:    dockboyCli.In,
		Writer:    dockboyCli.Out,
		ErrWriter: dockboyCli.Err,
		ExitErrHandler: func(ctx *cli.Context, err error) {
			if err != nil {
				fmt.Fprintf(dockboyCli.Err, "error: %v\n", err)
				os.Exit(0)
			}
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
