package shell

import (
	"os"
)

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
