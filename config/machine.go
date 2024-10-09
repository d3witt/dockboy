package config

import (
	"errors"
	"net"
)

type Machine struct {
	IP           net.IP `toml:"ip"`
	Port         int    `toml:"port,omitempty"`
	User         string `toml:"user,omitempty"`
	IdentityFile string `toml:"identity_file,omitempty"`
	Passphrase   string `toml:"passphrase,omitempty"`
}

var ErrMachineNotFound = errors.New("machine not found")

func (c *Config) GetMachine() (Machine, error) {
	machine := c.Machine
	setMachineDefaults(&machine)

	return machine, nil
}

func setMachineDefaults(machine *Machine) {
	if machine.Port == 0 {
		machine.Port = 22
	}

	if machine.User == "" {
		machine.User = "root"
	}
}
