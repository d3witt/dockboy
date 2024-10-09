package app

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"strings"
	"time"

	"github.com/d3witt/dockboy/caddy"
	"github.com/d3witt/dockboy/cli/command"
	"github.com/d3witt/dockboy/dockerhelper"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/swarm"
	"github.com/docker/docker/client"
	"github.com/urfave/cli/v2"
	"golang.org/x/crypto/ssh"
)

func NewDeployCmd(dockboyCli *command.Cli) *cli.Command {
	return &cli.Command{
		Name:  "deploy",
		Usage: "Deploy the app to the Swarm",
		Action: func(ctx *cli.Context) error {
			return runDeploy(ctx.Context, dockboyCli)
		},
	}
}

func runDeploy(ctx context.Context, dockboyCli *command.Cli) error {
	conf, err := dockboyCli.AppConfig()
	if err != nil {
		return err
	}

	sshClient, err := dockboyCli.DialMachine()
	if err != nil {
		return err
	}
	defer sshClient.Close()

	if err := prepare(ctx, dockboyCli, sshClient); err != nil {
		return err
	}

	dockerClient, err := dockerhelper.DialSSH(sshClient)
	if err != nil {
		return err
	}
	defer dockerClient.Close()

	replicas := conf.Replicas
	if replicas == 0 {
		replicas = 1
	}

	networks := []string{dockerhelper.DockboyInternalNetwork}
	if len(conf.Public.Address) > 0 {
		networks = append(networks, dockerhelper.DockboyPublicNetwork)
	}

	secrets, err := parseSecrets(conf.Secrets)
	if err != nil {
		return err
	}

	var healthCheck *container.HealthConfig
	if len(conf.Healthcheck.Test) > 0 {
		healthCheck = &container.HealthConfig{
			Test:        conf.Healthcheck.Test,
			Interval:    time.Duration(conf.Healthcheck.Interval),
			Timeout:     time.Duration(conf.Healthcheck.Timeout),
			Retries:     conf.Healthcheck.Retries,
			StartPeriod: time.Duration(conf.Healthcheck.StartPeriod),
		}
	}

	mounts := parseVolumes(conf.Volumes)

	if err := sendImage(ctx, dockboyCli, dockerClient, conf.Image); err != nil {
		return err
	}

	order := conf.Deploy.Order
	if order == "" {
		order = swarm.UpdateOrderStopFirst
	}

	if err := dockerhelper.DeployService(ctx, dockboyCli.Out, dockerClient, conf.Name, conf.Image, replicas, networks, conf.Env, conf.Label, secrets, healthCheck, mounts, order); err != nil {
		return err
	}

	if len(conf.Public.Address) > 0 {
		publicConfig := []caddy.ProxyConfig{
			{
				Address:    conf.Public.Address,
				TargetPort: conf.Public.TargetPort,
			},
		}

		fmt.Fprintf(dockboyCli.Out, "dockboy: configuring public access for %s\n", conf.Public.Address)
		if err := caddy.AddPublicConfig(ctx, sshClient, dockerClient, conf.Name, publicConfig, conf.Name); err != nil {
			return fmt.Errorf("failed to configure public access: %w", err)
		}
	}

	fmt.Fprintln(dockboyCli.Out, conf.Name)

	return nil
}

func prepare(ctx context.Context, dockboyCli *command.Cli, sshClient *ssh.Client) error {
	if err := checkDockerInstalled(dockboyCli, sshClient); err != nil {
		return err
	}

	dockerClient, err := dockerhelper.DialSSH(sshClient)
	if err != nil {
		return err
	}
	defer dockerClient.Close()

	inactive, err := dockerhelper.IsSwarmInactive(ctx, dockerClient)
	if err != nil {
		return fmt.Errorf("could not check Swarm status on host %s: %w", sshClient.RemoteAddr().String(), err)
	}
	if inactive {
		host, _, err := net.SplitHostPort(sshClient.RemoteAddr().String())
		if err != nil {
			return fmt.Errorf("could not parse host address: %w", err)
		}
		fmt.Fprintln(dockboyCli.Out, "dockboy: Swarm is not active. Initializing...")
		if err := dockerhelper.InitSwarm(ctx, dockerClient, host); err != nil {
			slog.ErrorContext(ctx, "Failed to initialize Swarm", "machine", sshClient.RemoteAddr().String(), "error", err)
			return fmt.Errorf("could not initialize Swarm on host %s: %w", sshClient.RemoteAddr().String(), err)
		}
	}

	fmt.Fprintln(dockboyCli.Out, "dockboy: preparing networks...")
	if err := dockerhelper.CreateNetworkIfNotExists(ctx, dockerClient, dockerhelper.DockboyInternalNetwork); err != nil {
		return err
	}
	if err := dockerhelper.CreateNetworkIfNotExists(ctx, dockerClient, dockerhelper.DockboyPublicNetwork); err != nil {
		return err
	}

	fmt.Fprintln(dockboyCli.Out, "dockboy: preparing Caddy service...")
	return caddy.DeployCaddyService(ctx, dockboyCli.Out, dockerClient, dockerhelper.DockboyPublicNetwork)
}

func checkDockerInstalled(dockboyCli *command.Cli, client *ssh.Client) error {
	if !dockerhelper.IsDockerInstalled(client) {
		fmt.Fprintf(dockboyCli.Out, "dockboy: Docker is not installed on host %s. Installing...\n", client.RemoteAddr().String())
		if err := dockerhelper.InstallDocker(client); err != nil {
			return fmt.Errorf("could not install Docker on host %s: %w", client.RemoteAddr().String(), err)
		}
	}
	return nil
}

func sendImage(ctx context.Context, dockboyCli *command.Cli, remote *client.Client, imageName string) error {
	fmt.Fprintln(dockboyCli.Out, "dockboy: sending image to remote host...")

	local, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return fmt.Errorf("failed to create local Docker client: %w", err)
	}
	defer local.Close()

	inspect, _, err := local.ImageInspectWithRaw(ctx, imageName)
	if err != nil {
		return fmt.Errorf("failed to inspect image on local client: %w", err)
	}

	remoteImages, err := remote.ImageList(ctx, image.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list images on remote client: %w", err)
	}

	for _, img := range remoteImages {
		if img.ID == inspect.ID {
			return nil
		}
	}

	reader, err := local.ImageSave(ctx, []string{imageName})
	if err != nil {
		return fmt.Errorf("failed to save image on source: %w", err)
	}
	defer reader.Close()

	resp, err := remote.ImageLoad(ctx, reader, true)
	if err != nil {
		return fmt.Errorf("failed to load image on remote client: %w", err)
	}
	defer resp.Body.Close()

	_, err = io.Copy(io.Discard, resp.Body)
	if err != nil {
		return fmt.Errorf("error reading response from remote client: %w", err)
	}

	return nil
}

func parseSecrets(secrets map[string]string) (map[string][]byte, error) {
	res := make(map[string][]byte)
	for key, value := range secrets {
		if value == "" {
			continue
		}

		if strings.HasSuffix(key, "_file") {
			content, err := os.ReadFile(value)
			if err != nil {
				return res, fmt.Errorf("failed to read secret file %s: %w", value, err)
			}
			res[strings.TrimSuffix(key, "_file")] = content
		} else {
			res[key] = []byte(value)
		}
	}

	return res, nil
}

func parseVolumes(volumes map[string]string) []mount.Mount {
	res := make([]mount.Mount, 0, len(volumes))
	for source, target := range volumes {
		res = append(res, mount.Mount{
			Type:   mount.TypeVolume,
			Source: source,
			Target: target,
		})
	}
	return res
}
