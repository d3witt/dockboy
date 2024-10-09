package machine

import (
	"context"
	"fmt"
	"strings"

	"github.com/d3witt/dockboy/cli/command"
	"github.com/d3witt/dockboy/dockerhelper"
	"github.com/docker/docker/api/types/filters"
	"github.com/urfave/cli/v2"
)

func NewPurgeCmd(dockboyCli *command.Cli) *cli.Command {
	return &cli.Command{
		Name:  "prune",
		Usage: "Delete unused data for containers, images, volumes, and networks",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "yes",
				Usage: "Automatically confirm the purge operation without prompting.",
			},
		},
		Action: func(ctx *cli.Context) error {
			yes := ctx.Bool("yes")

			return runPurge(ctx.Context, dockboyCli, yes)
		},
	}
}

func runPurge(ctx context.Context, dockboyCli *command.Cli, yes bool) error {
	if !yes {
		message := "You want to prune machine. Are you sure?"
		confirmed, err := command.PromptForConfirmation(dockboyCli.In, dockboyCli.Out, message)
		if err != nil {
			return err
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

	fmt.Fprintln(dockboyCli.Out, "dockboy: prune containers...")
	containersReport, err := dockerClient.ContainersPrune(ctx, filters.Args{})
	if err != nil {
		return err
	}
	if len(containersReport.ContainersDeleted) > 0 {
		fmt.Fprintf(dockboyCli.Out, "dockboy: deleted containers: %s\n", strings.Join(containersReport.ContainersDeleted, ", "))
	} else {
		fmt.Fprintln(dockboyCli.Out, "dockboy: no containers deleted")
	}

	fmt.Fprintln(dockboyCli.Out, "dockboy: prune images...")
	imagesReport, err := dockerClient.ImagesPrune(ctx, filters.Args{})
	if err != nil {
		return err
	}

	if len(imagesReport.ImagesDeleted) > 0 {
		fmt.Fprintf(dockboyCli.Out, "dockboy: deleted images: %d\n", len(imagesReport.ImagesDeleted))
	} else {
		fmt.Fprintln(dockboyCli.Out, "dockboy: no images deleted")
	}

	fmt.Fprintln(dockboyCli.Out, "dockboy: prune volumes...")
	volumesReport, err := dockerClient.VolumesPrune(ctx, filters.Args{})
	if err != nil {
		return err
	}

	if len(volumesReport.VolumesDeleted) > 0 {
		fmt.Fprintf(dockboyCli.Out, "dockboy: deleted volumes: %s\n", strings.Join(volumesReport.VolumesDeleted, ", "))
	} else {
		fmt.Fprintln(dockboyCli.Out, "dockboy: no volumes deleted")
	}

	fmt.Fprintln(dockboyCli.Out, "dockboy: prune networks...")
	networksReport, err := dockerClient.NetworksPrune(ctx, filters.Args{})
	if err != nil {
		return err
	}

	if len(networksReport.NetworksDeleted) > 0 {
		fmt.Fprintf(dockboyCli.Out, "dockboy: deleted networks: %s\n", strings.Join(networksReport.NetworksDeleted, ", "))
	} else {
		fmt.Fprintln(dockboyCli.Out, "dockboy: no networks deleted")
	}

	return nil
}
