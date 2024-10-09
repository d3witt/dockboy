# Dock-Boy

[![Go Report Card](https://goreportcard.com/badge/github.com/d3witt/dockboy)](https://goreportcard.com/report/github.com/d3witt/dockboy)
[![Go Reference](https://pkg.go.dev/badge/github.com/d3witt/dockboy.svg)](https://pkg.go.dev/github.com/d3witt/dockboy)
![GitHub release](https://img.shields.io/github/v/release/d3witt/dockboy)

Dock-Boy is a tiny deployment tool for small teams that works everywhere.

Built on Docker Swarm and Caddy, it sets up everything automatically and gets your apps running with minimal fuss. No complex configurations, no steep learning curve—just deploy and go.

-   🚀 **Simple & Clear.** Deploy your app with just a couple of commands.
-   🌎 **Works Anywhere.** Cloud provider or garage server? It just works
-   🔐 **SSL Included.** Automatically provision SSL certificates.
-   ⚡️ **Zero Downtime.** Seamless deployments without interruption.
-   🛡️ **Secure.** Encrypted secrets stored locally on your machine.
-   🗂️ **Multiple Apps.** Deploy unlimited apps on a single server.

## 🚀 Installation

See [releases](https://github.com/d3witt/dockboy/releases) for pre-built binaries.

On Unix:

```
env CGO_ENABLED=0 go install -ldflags="-s -w" github.com/d3witt/dockboy@latest
```

On Windows cmd:

```
set CGO_ENABLED=0
go install -ldflags="-s -w" github.com/d3witt/dockboy@latest
```

On Windows powershell:

```
$env:CGO_ENABLED = '0'
go install -ldflags="-s -w" github.com/d3witt/dockboy@latest
```

## 📄 How It Works

Run `dockboy init` in your project directory to create a `dockboy.toml` configuration file. This file contains all the information Dockboy needs to deploy your app.

```toml
name = 'dockboy-web'
image = 'dockboy-web:latest'

[public]
address = ':80'
target_port = 80

[machine]
ip = '163.92.16.213'
```

After you've set up your configuration, run `dockboy deploy` to deploy your app to the server.

```shell

~$ dockboy deploy
dockboy: Docker is not installed on host 163.92.16.213. Installing...
dockboy: Swarm is not active. Initializing...
dockboy: preparing networks...
dockboy: preparing Caddy service...
dockboy: service 'dockboy-caddy' is running.
dockboy: sending image to remote host...
dockboy: creating service 'dockboy-web'...
swarm: creating container dockboy-web.1.5oic3vwbc1jzi27y107ndnwp0...
swarm: starting container dockboy-web.1.5oic3vwbc1jzi27y107ndnwp0...
swarm: container dockboy-web.1.5oic3vwbc1jzi27y107ndnwp0 is healthy.
dockboy: service 'dockboy-web' is running.
dockboy: configuring public access for :80
dockboy-web
```

That's it! Your app is now live and ready to use.

## 📖 Commands

```shell
NAME:
   dockboy - Deploy your apps anywhere

USAGE:
   dockboy [global options] command [command options]

VERSION:
   dev

COMMANDS:
   init     Initialize a new dockboy config
   deploy   Deploy the app to the Swarm
   logs     Fetch the logs
   destroy  Destroy the app and remove it from the Swarm
   info     Display information about the app
   prune    Delete unused data for containers, images, volumes, and networks
   exec     Execute command on machine
   help, h  Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --debug        Enable debug output (default: false)
   --help, -h     show help
   --version, -v  print the version
```

## 📝 Config File

Full configuration file example:

```toml
name = 'dockboy-web'
image = 'dockboy-web:latest'

[public]
address = ':80'
target_port = 80

[env]
MACHINE_IP = "163.92.16.213"

[secrets]
password = "dockboy"
my_data_file = "./my_data.md"

[volumes]
viking-web-data = "/data"

[healthcheck]
test = ["CMD", "wget", "-q", "-t 1", "--spider", "http://127.0.0.1"]
interval = "30s"
timeout = "10s"
retries = 3

[machine]
ip = '163.92.16.213'
user = 'root
port = 22
identity_file = '/home/user/.ssh/id_rsa'
passphrase = 'password'

[deploy]
order = "start-first"
```

#### `name` (required)

The name of the app. Used to name the Docker service and the Caddy reverse proxy configuration file (/etc/caddy/sites/name). It should be unique.

#### `image` (required)

The Docker image to deploy. Dock-Boy will fetch this image from the local Docker daemon, so ensure it is available.

#### `public` (optional)

Defines how your app will be accessible on the internet.

-   `address` - The address to listen in, it can be a port or a domain.
-   `target_port` - The port in the container to forward traffic to.

#### `env` (optional)

Environment variables to set in the container.

#### `secrets` (optional)

Secrets to pass to the container. The value can be a string or a path to a file if the secret name ends with \_file. Access secrets in the container at /run/secrets/secret_name (without the \_file suffix).

#### `volumes` (optional)

Volumes help you persist data across deployments. The key is the name of the volume, and the value is the path on the container where the volume should be mounted.

#### `healthcheck` (optional)

Healthcheck configuration for the service.

-   `test` - The command to run to check the health of the service.
-   `interval` - The time between health checks.
-   `timeout` - The time to wait for the health check to complete.
-   `retries` - The number of retries before the service is considered unhealthy.

#### `machine` (required)

Defines the server where the app will be deployed.

-   `ip` - The IP address of the server.
-   `user` - The username to use when connecting to the server. Default is `root`.
-   `port` - The SSH port to use when connecting to the server. Default is `22`.
-   `identity_file` - The path to the SSH private key file.
-   `passphrase` - The passphrase for the SSH private key file.

#### `deploy` (optional)

-   `order`: The deployment order (`start-first` or `stop-first`, default: `stop-first`). Set to `start-first` for zero downtime deployments.

## Single Server

Dock-Boy is built on top of Docker Swarm but intentionally supports only single-server deployment. Using a single server is often enough to start; it keeps things simple, reduces costs, and avoids unnecessary complexity. This allows you to focus on more important things, like building something people want.

## Transparency

Dock-Boy operates the Docker daemon on your server from your local machine via an SSH connection. There’s no need to install Dock-Boy on your server. You can view every command Dock-Boy runs by adding the --debug flag.

## 🤝 Missing a Feature?

Feel free to open a new issue, or contact me.

## 📘 License

Dockboy is provided under the [MIT License](https://github.com/d3witt/dockboy/blob/main/LICENSE).
