package config

import (
	"github.com/pelletier/go-toml/v2"
)

type Config struct {
	Name        string            `toml:"name"`
	Image       string            `toml:"image"`
	Machine     Machine           `toml:"machine"`
	Public      PublicConfig      `toml:"public,omitempty"`
	Replicas    uint64            `toml:"replicas,omitempty"`
	Volumes     map[string]string `toml:"volumes,omitempty"`
	Env         map[string]string `toml:"env,omitempty"`
	Secrets     map[string]string `toml:"secrets,omitempty"`
	Label       map[string]string `toml:"label,omitempty"`
	Healthcheck HealthConfig      `toml:"healthcheck,omitempty"`
	Deploy      DeployConfig      `toml:"deploy,omitempty"`
}

type DeployConfig struct {
	Order string `toml:"order,omitempty"`
}

type PublicConfig struct {
	Address    string `toml:"address,omitempty"`
	TargetPort int    `toml:"target_port,omitempty"`
}

type HealthConfig struct {
	Test          []string `toml:"test,omitempty"`
	Interval      Duration `toml:"interval,omitempty"`
	Timeout       Duration `toml:"timeout,omitempty"`
	StartInterval Duration `toml:"start_interval,omitempty"`
	StartPeriod   Duration `toml:"start_period,omitempty"`
	Retries       int      `toml:"retries,omitempty"`
}

func defaultConfig(name string) Config {
	return Config{
		Name:    name,
		Image:   "hashicorp/http-echo:latest",
		Machine: Machine{},
		Public:  PublicConfig{},
	}
}

func (c Config) Save() error {
	data, err := toml.Marshal(&c)
	if err != nil {
		return err
	}

	return writeConfigFile(configFile, data)
}
