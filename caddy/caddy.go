package caddy

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/d3witt/dockboy/dockerhelper"
	"github.com/d3witt/dockboy/sshexec"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/swarm"
	"github.com/docker/docker/client"
	"golang.org/x/crypto/ssh"
)

const (
	caddyServiceName = "dockboy-caddy"
	caddyImage       = "caddy:latest"
	caddyDataVolume  = "caddy_data"
	caddySitesVolume = "caddy_sites"
)

type ProxyConfig struct {
	Address    string
	TargetPort int
}

func DeployCaddyService(ctx context.Context, out io.Writer, remote *client.Client, network string) error {
	services, err := remote.ServiceList(ctx, types.ServiceListOptions{
		Filters: filters.NewArgs(filters.Arg("name", caddyServiceName)),
	})
	if err != nil {
		return fmt.Errorf("failed to list services: %w", err)
	}
	if len(services) > 0 {
		return nil
	}

	spec := swarm.ServiceSpec{
		Annotations: swarm.Annotations{
			Name: caddyServiceName,
		},
		TaskTemplate: swarm.TaskSpec{
			ContainerSpec: &swarm.ContainerSpec{
				Image: caddyImage,
				Mounts: []mount.Mount{
					{
						Type:   mount.TypeVolume,
						Source: caddyDataVolume,
						Target: "/data",
					},
					{
						Type:   mount.TypeVolume,
						Source: caddySitesVolume,
						Target: "/etc/caddy/sites",
					},
				},
				Command: []string{
					"/bin/sh",
					"-c",
					"echo 'import sites/*.conf' > /etc/caddy/Caddyfile && caddy run --config /etc/caddy/Caddyfile --adapter caddyfile",
				},
			},
			RestartPolicy: &swarm.RestartPolicy{
				Condition: swarm.RestartPolicyConditionAny,
			},
			Networks: []swarm.NetworkAttachmentConfig{
				{
					Target: network,
				},
			},
			LogDriver: &swarm.Driver{
				Name: "local",
				Options: map[string]string{
					"max-size": "100m",
					"max-file": "3",
				},
			},
		},
		EndpointSpec: &swarm.EndpointSpec{
			Ports: []swarm.PortConfig{
				{
					Protocol:      swarm.PortConfigProtocolTCP,
					TargetPort:    80,
					PublishedPort: 80,
					PublishMode:   swarm.PortConfigPublishModeIngress,
				},
				{
					Protocol:      swarm.PortConfigProtocolTCP,
					TargetPort:    443,
					PublishedPort: 443,
					PublishMode:   swarm.PortConfigPublishModeIngress,
				},
				{
					Protocol:      swarm.PortConfigProtocolUDP,
					TargetPort:    80,
					PublishedPort: 80,
					PublishMode:   swarm.PortConfigPublishModeIngress,
				},
				{
					Protocol:      swarm.PortConfigProtocolUDP,
					TargetPort:    443,
					PublishedPort: 443,
					PublishMode:   swarm.PortConfigPublishModeIngress,
				},
			},
		},
	}

	_, err = remote.ServiceCreate(ctx, spec, types.ServiceCreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create Caddy service: %w", err)
	}

	if err := dockerhelper.WaitForService(ctx, out, remote, caddyServiceName); err != nil {
		return fmt.Errorf("failed to wait for Caddy service to be running: %w", err)
	}

	return nil
}

func AddPublicConfig(ctx context.Context, sshClient *ssh.Client, remote *client.Client, clientID string, configs []ProxyConfig, upstream string) error {
	caddyfileSnippet := generateCaddyfileContent(configs, upstream)
	clientConfigPath := fmt.Sprintf("/etc/caddy/sites/%s.conf", clientID)

	if err := writeToCaddyContainer(ctx, sshClient, remote, clientConfigPath, caddyfileSnippet); err != nil {
		return fmt.Errorf("failed to write client config: %w", err)
	}

	if err := executeInCaddyContainer(ctx, sshClient, remote, "caddy", "reload", "--config", "/etc/caddy/Caddyfile"); err != nil {
		return fmt.Errorf("failed to reload Caddy: %w", err)
	}

	return nil
}

func RemovePublicConfig(ctx context.Context, sshClient *ssh.Client, remote *client.Client, clientID string) error {
	clientConfigPath := fmt.Sprintf("/etc/caddy/sites/%s.conf", clientID)

	if err := executeInCaddyContainer(ctx, sshClient, remote, "rm", "-f", clientConfigPath); err != nil {
		slog.WarnContext(ctx, "Failed to remove client config", "clientID", clientID, "error", err)
	}

	if err := executeInCaddyContainer(ctx, sshClient, remote, "caddy", "reload", "--config", "/etc/caddy/Caddyfile"); err != nil {
		return fmt.Errorf("failed to reload Caddy: %w", err)
	}

	return nil
}

func generateCaddyfileContent(configs []ProxyConfig, defaultUpstream string) string {
	var content string

	for _, config := range configs {
		if config.Address == "" {
			continue
		}

		upstream := defaultUpstream
		if config.TargetPort != 0 {
			upstream = fmt.Sprintf("%s:%d", defaultUpstream, config.TargetPort)
		}

		content += fmt.Sprintf("%s {\n\treverse_proxy %s\n}\n\n", config.Address, upstream)
	}

	return content
}

func writeToCaddyContainer(ctx context.Context, sshClient *ssh.Client, remote *client.Client, filePath, content string) error {
	escapedContent := escapeForShell(content)

	dir := filepath.Dir(filePath)
	if err := executeInCaddyContainer(ctx, sshClient, remote, "mkdir", "-p", dir); err != nil {
		return fmt.Errorf("failed to create directory for %s: %w", filePath, err)
	}

	command := fmt.Sprintf("\"echo '%s' > %s\"", escapedContent, filePath)
	if err := executeInCaddyContainer(ctx, sshClient, remote, "sh", "-c", command); err != nil {
		return fmt.Errorf("failed to write content to %s: %w", filePath, err)
	}

	return nil
}

func executeInCaddyContainer(ctx context.Context, sshClient *ssh.Client, remote *client.Client, cmd ...string) error {
	containerID, err := getCaddyContainerID(ctx, remote)
	if err != nil {
		return err
	}

	dockerCmd := append([]string{"docker", "exec", containerID}, cmd...)
	if out, err := sshexec.Command(sshClient, dockerCmd[0], dockerCmd[1:]...).CombinedOutput(); err != nil {
		return fmt.Errorf("failed to execute command: %w\n%s", err, out)
	}

	return nil
}

func getCaddyContainerID(ctx context.Context, remote *client.Client) (string, error) {
	services, err := remote.ServiceList(ctx, types.ServiceListOptions{
		Filters: filters.NewArgs(filters.Arg("name", caddyServiceName)),
	})
	if err != nil {
		return "", fmt.Errorf("failed to list services: %w", err)
	}
	if len(services) == 0 {
		return "", fmt.Errorf("Caddy service not found")
	}

	tasks, err := remote.TaskList(ctx, types.TaskListOptions{
		Filters: filters.NewArgs(
			filters.Arg("service", services[0].ID),
			filters.Arg("desired-state", "running"),
		),
	})
	if err != nil {
		return "", fmt.Errorf("failed to list tasks: %w", err)
	}
	if len(tasks) == 0 {
		return "", fmt.Errorf("no running tasks found for Caddy service")
	}

	return tasks[0].Status.ContainerStatus.ContainerID, nil
}

func escapeForShell(s string) string {
	// Escape backslashes first to prevent double escaping
	s = strings.ReplaceAll(s, `\`, `\\`)
	// Escape single quotes
	s = strings.ReplaceAll(s, `'`, `'\''`)
	return s
}
