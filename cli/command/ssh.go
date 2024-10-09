package command

import (
	"fmt"
	"os"

	"github.com/d3witt/dockboy/sshexec"
	"golang.org/x/crypto/ssh"
)

func (c *Cli) DialMachine() (*ssh.Client, error) {
	conf, err := c.AppConfig()
	if err != nil {
		return nil, err
	}

	m, err := conf.GetMachine()
	if err != nil {
		return nil, err
	}

	var private, passphrase string
	if m.IdentityFile != "" {
		key, err := os.ReadFile(m.IdentityFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read identity file: %w", err)
		}
		private = string(key)
		passphrase = m.Passphrase
	}

	return sshexec.SSHClient(m.IP.String(), m.Port, m.User, private, passphrase)
}
