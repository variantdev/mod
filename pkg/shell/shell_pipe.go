package shell

import (
	"errors"
	"fmt"
	"io"
	"os"
)

// Pipe runs the command with
func (s *Shell) Pipe(cmd *Command) (<-chan Result, io.ReadCloser, io.ReadCloser) {
	res := make(chan Result, 1)

	wstdout, stdout, err := s.pipeStdout(cmd)
	if err != nil {
		res <- Result{Error: fmt.Errorf("unable to pipe stdout: %v", err)}
		return res, nil, nil
	}

	wstderr, stderr, err := s.pipeStderr(cmd)
	if err != nil {
		res <- Result{Error: fmt.Errorf("unable to pipe stderr: %v", err)}
		return res, nil, nil
	}

	go func() {
		res <- s.Wait(cmd)
		wstdout.Close()
		wstderr.Close()
	}()

	return res, stdout, stderr
}

func (s *Shell) pipeStdout(cmd *Command) (*os.File, io.ReadCloser, error) {
	if cmd.Stdout != nil {
		return nil, nil, errors.New("exec: Stdout already set")
	}
	pr, pw, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}
	cmd.Stdout = pw
	return pw, pr, nil
}

func (s *Shell) pipeStderr(cmd *Command) (*os.File, io.ReadCloser, error) {
	if cmd.Stderr != nil {
		return nil, nil, errors.New("exec: Stderr already set")
	}
	pr, pw, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}
	cmd.Stderr = pw
	return pw, pr, nil
}
