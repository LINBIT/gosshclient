package sshclient

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/terminal"
)

// SSHClient represents a high level ssh client
type SSHClient struct {
	hostPort  string
	sshConfig ssh.ClientConfig
	client    *ssh.Client
	session   *ssh.Session
}

// NewSSHClient returns a high level ssh client
func NewSSHClient(hostPort string, sshconfig ssh.ClientConfig) *SSHClient {
	return &SSHClient{
		hostPort:  hostPort,
		sshConfig: sshconfig,
	}
}

func (s *SSHClient) getClient() error {
	if s.client != nil {
		return nil
	}

	client, err := ssh.Dial("tcp", s.hostPort, &s.sshConfig)
	if err != nil {
		return err
	}
	s.client = client
	return nil
}

func (s *SSHClient) getSession() error {
	if s.session != nil {
		return nil
	}

	session, err := s.client.NewSession()
	if err != nil {
		return err
	}
	s.session = session
	return nil
}

// Dial creates an ssh client as well as its session
// After a successful call to Dial(), one should also always call Close()
func (s *SSHClient) Dial() error {
	if err := s.getClient(); err != nil {
		return err
	}

	if err := s.getSession(); err != nil {
		// cleanup client
		if cerr := s.Close(); cerr != nil {
			return fmt.Errorf("session error: %v, cleanup error: %v", err, cerr)
		}
		return fmt.Errorf("session error: %v", err)
	}

	return nil
}

func (s *SSHClient) mustBeConnected() error {
	if s.session == nil || s.client == nil {
		return errors.New("sshclient not connected, did you call Dial()?")
	}
	return nil
}

// Close closes the underlying ssh session and client
func (s *SSHClient) Close() error {
	if s.session != nil {
		if err := s.session.Wait(); err != nil {
			return err
		}
		s.session = nil
	}
	if s.client != nil {
		if err := s.client.Close(); err != nil {
			return err
		}
		s.client = nil
	}

	return nil
}

func (s *SSHClient) stdinPipe() (io.WriteCloser, error) {
	if err := s.mustBeConnected(); err != nil {
		return nil, err
	}
	return s.session.StdinPipe()
}

// StdoutPipe creates an ssh.session if it does not exist and calls StdoutPipe on it.
func (s *SSHClient) StdoutPipe() (io.Reader, error) {
	if err := s.mustBeConnected(); err != nil {
		return nil, err
	}
	return s.session.StdoutPipe()
}

// StderrPipe creates an ssh.session if it does not exist and calls StderrPipe on it.
func (s *SSHClient) StderrPipe() (io.Reader, error) {
	if err := s.mustBeConnected(); err != nil {
		return nil, err
	}
	return s.session.StderrPipe()
}

// ExecScript executes a (shell) script line by line.
// After return, you can not re-use the sshclient
func (s *SSHClient) ExecScript(script string) error {
	if err := s.mustBeConnected(); err != nil {
		return err
	}
	// users are supposed to call Close(), but to be sure...
	defer s.Close()

	inp, err := s.stdinPipe()
	if err != nil {
		return err
	}

	if err := s.session.Shell(); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(inp, script); err != nil {
		return err
	}

	inp.Close()
	err = s.session.Wait()
	s.session = nil

	return err
}

// Shell executes an interactive ssh shell
// After return, you can not re-use the sshclient
func (s *SSHClient) Shell() error {
	if err := s.mustBeConnected(); err != nil {
		return err
	}
	// users are supposed to call Close(), but to be sure...
	defer s.Close()

	fd := int(os.Stdin.Fd())
	state, err := terminal.MakeRaw(fd)
	if err != nil {
		return err
	}
	defer terminal.Restore(fd, state)

	w, h, err := terminal.GetSize(fd)
	if err != nil {
		return err
	}

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}

	if err = s.session.RequestPty("xterm", h, w, modes); err != nil {
		return err
	}

	s.session.Stdin = os.Stdin
	s.session.Stdout = os.Stdout
	s.session.Stderr = os.Stderr

	if err := s.session.Shell(); err != nil {
		return err
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGWINCH)

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for sig := range sigChan {
			switch sig {
			case syscall.SIGWINCH:
				fd := int(os.Stdout.Fd())
				w, h, _ = terminal.GetSize(fd)
				s.session.WindowChange(h, w)
			}
		}
	}()

	err = s.session.Wait()
	s.session = nil

	close(sigChan)
	wg.Wait()

	return err
}
