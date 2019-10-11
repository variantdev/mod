package shell

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

type Command struct {
	Name           string
	Args           []string
	Stdout, Stderr io.Writer
	Stdin          io.Reader
	Env            map[string]string
}

type Exec func(*Command) Result

func DefaultExec(c *Command) Result {
	cmd := exec.Command(c.Name, c.Args...)
	var env []string
	for n, v := range c.Env {
		env = append(env, fmt.Sprintf("%s=%s", n, v))
	}
	cmd.Env = env
	if c.Stdin != nil {
		cmd.Stdin = c.Stdin
	}
	if c.Stdout != nil {
		cmd.Stdout = c.Stdout
	}
	if c.Stderr != nil {
		cmd.Stderr = c.Stderr
	}
	if err := cmd.Start(); err != nil {
		return Result{ExitStatus: 1, Error: err}
	}
	if err := cmd.Wait(); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			waitStatus := exitError.Sys().(syscall.WaitStatus)
			return Result{ExitStatus: waitStatus.ExitStatus(), Error: exitError}
		} else {
			return Result{ExitStatus: 1, Error: err}
		}
	}
	waitStatus := cmd.ProcessState.Sys().(syscall.WaitStatus)
	return Result{ExitStatus: waitStatus.ExitStatus(), Error: nil}
}

type Result struct {
	ExitStatus int
	Error      error
}

type CaptureResult struct {
	ExitStatus int
	Stdout     string
	Stderr     string
}

type Shell struct {
	Exec Exec
}

// Wait runs the command and wait until it returns
func (s *Shell) Wait(cmd *Command) Result {
	return s.Exec(cmd)
}

// Interact runs the command interactively, inheriting os.(Stdin|Stdout|Stderr) to the command
func (s *Shell) Interact(cmd *Command) Result {
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return s.Exec(cmd)
}

// Pipe runs the command with
func (s *Shell) Pipe(cmd *Command) (<-chan Result, io.ReadCloser, io.ReadCloser) {
	res := make(chan Result, 1)

	stdout, err := s.pipeStdout(cmd)
	if err != nil {
		res <- Result{Error: fmt.Errorf("unable to pipe stdout: %v", err)}
		return res, nil, nil
	}

	stderr, err := s.pipeStderr(cmd)
	if err != nil {
		res <- Result{Error: fmt.Errorf("unable to pipe stderr: %v", err)}
		return res, nil, nil
	}

	go func() {
		res <- s.Wait(cmd)
		stdout.Close()
		stderr.Close()
	}()

	return res, stdout, stderr
}

type CaptureOpts struct {
	LogStdout      func(string)
	LogStderr      func(string)
	OutToErrPrefix string
}

func (s *Shell) Capture(cmd *Command, opts ...CaptureOpts) (*CaptureResult, error) {
	var logStdout, logStderr func(string)

	opt := CaptureOpts{}
	for _, o := range opts {
		if o.LogStdout != nil {
			opt.LogStdout = o.LogStdout
		}
		if o.LogStderr != nil {
			opt.LogStderr = o.LogStderr
		}
		if o.OutToErrPrefix != "" {
			opt.OutToErrPrefix = o.OutToErrPrefix
		}
	}

	if opt.LogStdout != nil {
		logStdout = opt.LogStdout
	} else {
		logStdout = func(_ string) {}
	}

	if opt.LogStderr != nil {
		logStderr = opt.LogStderr
	} else {
		logStderr = func(_ string) {}
	}

	var errOutPrefix string

	if opt.OutToErrPrefix != "" {
		errOutPrefix = opt.OutToErrPrefix
	} else {
		errOutPrefix = "variant.stderr: "
	}

	res, cmdReader, errReader := s.Pipe(cmd)
	channels := struct {
		Stdout chan string
		Stderr chan string
	}{
		Stdout: make(chan string),
		Stderr: make(chan string),
	}

	scanner := bufio.NewScanner(cmdReader)
	go func() {
		defer func() {
			close(channels.Stdout)
		}()
		for scanner.Scan() {
			text := scanner.Text()
			if errOutPrefix != "" && strings.HasPrefix(text, errOutPrefix) {
				channels.Stderr <- strings.SplitN(text, errOutPrefix, 2)[1]
			} else {
				channels.Stdout <- text
			}
		}
	}()

	errScanner := bufio.NewScanner(errReader)
	go func() {
		defer func() {
			close(channels.Stderr)
		}()
		for errScanner.Scan() {
			text := errScanner.Text()
			channels.Stderr <- text
		}
	}()

	stdoutEnded := false
	stderrEnded := false

	stderrCapture := ""
	stdoutCapture := ""

	// Coordinating stdout/stderr in this single place to not screw up message ordering
	for {
		select {
		case text, ok := <-channels.Stdout:
			if ok {
				logStdout(text)
				if stdoutCapture != "" {
					stdoutCapture += "\n"
				}
				stdoutCapture += text
			} else {
				stdoutEnded = true
			}
		case text, ok := <-channels.Stderr:
			if ok {
				logStderr(text)
				if stderrCapture != "" {
					stderrCapture += "\n"
				}
				stderrCapture += text
			} else {
				stderrEnded = true
			}
		}
		if stdoutEnded && stderrEnded {
			break
		}
	}

	r := <-res

	return &CaptureResult{
		ExitStatus: r.ExitStatus,
		Stdout:     stdoutCapture,
		Stderr:     stderrCapture,
	}, r.Error
}

func (s *Shell) pipeStdout(cmd *Command) (io.ReadCloser, error) {
	if cmd.Stdout != nil {
		return nil, errors.New("exec: Stdout already set")
	}
	pr, pw, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	cmd.Stdout = pw
	return pr, nil
}

func (s *Shell) pipeStderr(cmd *Command) (io.ReadCloser, error) {
	if cmd.Stderr != nil {
		return nil, errors.New("exec: Stderr already set")
	}
	pr, pw, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = pw
	return pr, nil
}
