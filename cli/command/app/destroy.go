package app

import (
	"context"
	"fmt"

	"github.com/d3witt/dockboy/caddy"
	"github.com/d3witt/dockboy/cli/command"
	"github.com/d3witt/dockboy/dockerhelper"
	"github.com/urfave/cli/v2"
)

func NewDestroyCmd(dockboyCli *command.Cli) *cli.Command {
	return &cli.Command{
		Name:  "destroy",
		Usage: "Destroy the app and remove it from the Swarm",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "yes",
				Usage: "Skip confirmation prompt",
			},
		},
		Action: func(ctx *cli.Context) error {
			yes := ctx.Bool("yes")

			return runDestroy(ctx.Context, dockboyCli, yes)
		},
	}
}

func runDestroy(ctx context.Context, dockboyCli *command.Cli, yes bool) error {
	conf, err := dockboyCli.AppConfig()
	if err != nil {
		return fmt.Errorf("failed to get app config: %w", err)
	}

	if !yes {
		confirmed, err := command.PromptForConfirmation(dockboyCli.In, dockboyCli.Out, fmt.Sprintf("Are you sure you want to destroy the app '%s'?", conf.Name))
		if err != nil {
			return fmt.Errorf("failed to prompt for confirmation: %w", err)
		}
		if !confirmed {
			return nil
		}
	}

	sshClient, err := dockboyCli.DialMachine()
	if err != nil {
		return err
	}
	defer sshClient.Close()

	dockerClient, err := dockerhelper.DialSSH(sshClient)
	if err != nil {
		return err
	}
	defer dockerClient.Close()

	fmt.Fprintf(dockboyCli.Out, "dockboy: removing service %s...\n", conf.Name)
	if err := dockerClient.ServiceRemove(ctx, conf.Name); err != nil {
		return fmt.Errorf("failed to remove service %s: %w", conf.Name, err)
	}

	fmt.Fprintf(dockboyCli.Out, "dockboy: removing Caddy config for service %s...\n", conf.Name)
	if err := caddy.RemovePublicConfig(ctx, sshClient, dockerClient, conf.Name); err != nil {
		return fmt.Errorf("failed to remove Caddy config for service %s: %w", conf.Name, err)
	}

	fmt.Fprintln(dockboyCli.Out, conf.Name)

	return nil
}
