package shell

import (
	"fmt"
	"io"
	"strings"
)

type FakeInput struct {
	Name string
	Args string
	Env  string
}

type FakeOutput struct {
	Stdout string
	Stderr string
}

func NewFakeInput(name string, args []string, env map[string]string) FakeInput {
	envs := []string{}
	for k, v := range env {
		envs = append(envs, fmt.Sprintf("%s=%s", k, v))
	}
	input := FakeInput{
		Name: name,
		Args: strings.Join(args, ","),
		Env:  strings.Join(envs, ","),
	}
	return input
}

func NewFake(expectations map[FakeInput]FakeOutput) Exec {
	return func(cmd *Command) Result {
		input := NewFakeInput(cmd.Name, cmd.Args, cmd.Env)
		output, ok := expectations[input]
		if !ok {
			err := fmt.Errorf("unexpected input: %v", input)
			return Result{ExitStatus: 1, Error: err}
		}

		n, err := io.WriteString(cmd.Stdout, output.Stdout)
		if err != nil {
			return Result{ExitStatus: 1, Error: err}
		}

		if n != len(output.Stdout) {
			err := fmt.Errorf("insufficient write stdout: wrote only %d of %d", n, len(output.Stdout))
			return Result{ExitStatus: 1, Error: err}
		}

		n2, err := io.WriteString(cmd.Stderr, output.Stderr)
		if err != nil {
			return Result{ExitStatus: 1, Error: err}
		}

		if n2 != len(output.Stderr) {
			err := fmt.Errorf("insufficient write to stderr: wrote only %d of %d", n, len(output.Stderr))
			return Result{ExitStatus: 1, Error: err}
		}

		return Result{ExitStatus: 0, Error: nil}
	}
}
