package machine

import (
	"fmt"
	"strings"

	"github.com/d3witt/dockboy/cli/command"
	"github.com/d3witt/dockboy/sshexec"
	"github.com/urfave/cli/v2"
	"golang.org/x/crypto/ssh"
)

func NewExecuteCmd(dockboyCli *command.Cli) *cli.Command {
	return &cli.Command{
		Name:      "exec",
		Usage:     "Execute command on machine",
		ArgsUsage: "CMD ARGS...",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "tty",
				Aliases: []string{"t"},
				Usage:   "Allocate a pseudo-TTY",
			},
		},
		Action: func(ctx *cli.Context) error {
			cmd := strings.Join(ctx.Args().Slice(), " ")
			tty := ctx.Bool("tty")

			return runExecute(dockboyCli, cmd, tty)
		},
	}
}

func runExecute(dockboyCli *command.Cli, cmd string, tty bool) error {
	sshClient, err := dockboyCli.DialMachine()
	if err != nil {
		return err
	}
	defer sshClient.Close()

	if tty {
		return executeTTY(dockboyCli, sshClient, cmd)
	}

	return executeCmd(dockboyCli, sshClient, cmd)
}

func executeCmd(dockboyCli *command.Cli, client *ssh.Client, cmd string) error {
	sshCmd := sshexec.Command(client, cmd)
	output, err := sshCmd.CombinedOutput()

	if err != nil && !isExitError(err) {
		return err
	}

	fmt.Fprint(dockboyCli.Out, string(output))
	return nil
}

func executeTTY(dockboyCli *command.Cli, client *ssh.Client, cmd string) error {
	sshCmd := sshexec.Command(client, cmd)

	w, h, err := dockboyCli.In.Size()
	if err != nil {
		return fmt.Errorf("get terminal size: %w", err)
	}

	sshCmd.Stdin = dockboyCli.In
	sshCmd.Stdout = dockboyCli.Out
	sshCmd.Stderr = dockboyCli.Err
	sshCmd.SetPty(h, w)

	if err := dockboyCli.Out.MakeRaw(); err != nil {
		return err
	}
	defer dockboyCli.Out.Restore()

	if err := sshCmd.Run(); err != nil && !isExitError(err) {
		return err
	}

	return nil
}

func isExitError(err error) bool {
	_, ok := err.(*sshexec.ExitError)
	return ok
}
