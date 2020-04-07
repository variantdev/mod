package shell

import (
	"io"
)

type Command struct {
	Name           string
	Args           []string
	Stdout, Stderr io.Writer
	Stdin          io.Reader
	Env            map[string]string

	// Dir is the working directory of this command
	Dir string
}

type Exec func(*Command) Result

type Result struct {
	ExitStatus int
	Error      error
}
