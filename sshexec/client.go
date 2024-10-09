package sshexec

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

func SSHClient(host string, port int, user, private, passphrase string) (*ssh.Client, error) {
	var sshAuth ssh.AuthMethod
	var err error

	if private != "" {
		sshAuth, err = authorizeWithKey(private, passphrase)
	} else {
		sshAuth, err = authorizeWithSSHAgent()
	}
	if err != nil {
		return nil, err
	}

	// Set up SSH client configuration
	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			sshAuth,
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         time.Second * 30,
	}

	addr := net.JoinHostPort(host, strconv.Itoa(port))

	return ssh.Dial("tcp", addr, config)
}

func authorizeWithKey(key, passphrase string) (ssh.AuthMethod, error) {
	var signer ssh.Signer
	var err error

	if passphrase != "" {
		signer, err = ssh.ParsePrivateKeyWithPassphrase([]byte(key), []byte(passphrase))
	} else {
		signer, err = ssh.ParsePrivateKey([]byte(key))
	}
	if err != nil {
		return nil, err
	}

	return ssh.PublicKeys(signer), nil
}

func authorizeWithSSHAgent() (ssh.AuthMethod, error) {
	socket := os.Getenv("SSH_AUTH_SOCK")
	if socket == "" {
		return nil, fmt.Errorf("SSH_AUTH_SOCK environment variable is not set")
	}

	conn, err := net.Dial("unix", socket)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to SSH_AUTH_SOCK: %w", err)
	}

	agentClient := agent.NewClient(conn)

	return ssh.PublicKeysCallback(func() ([]ssh.Signer, error) {
		signers, err := agentClient.Signers()
		if err != nil {
			conn.Close()
			return nil, err
		}
		return signers, nil
	}), nil
}
