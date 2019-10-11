package shell

import (
	"fmt"
	"os/exec"
	"syscall"
)

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
