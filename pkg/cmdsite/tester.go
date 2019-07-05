package cmdsite

import (
	"fmt"
	"io"
	"strings"
)

type CommandInput struct {
	Name string
	Args string
	Env  string
}

type CommandOutput struct {
	Stdout string
	Stderr string
}

func NewInput(name string, args []string, env map[string]string) CommandInput {
	envs := []string{}
	for k, v := range env {
		envs = append(envs, fmt.Sprintf("%s=%s", k, v))
	}
	input := CommandInput{
		Name: name,
		Args: strings.Join(args, ","),
		Env:  strings.Join(envs, ","),
	}
	return input
}

func NewTester(expectations map[CommandInput]CommandOutput) RunCommand {
	return func(name string, args []string, stdout, stderr io.Writer, env map[string]string) error {
		input := NewInput(name, args, env)
		output, ok := expectations[input]
		if !ok {
			return fmt.Errorf("unexpected input: %v", input)
		}

		n, err := io.WriteString(stdout, output.Stdout)
		if err != nil {
			return err
		}

		if n != len(output.Stdout) {
			return fmt.Errorf("insufficient write stdout: wrote only %d of %d", n, len(output.Stdout))
		}

		n2, err := io.WriteString(stderr, output.Stderr)
		if err != nil {
			return err
		}

		if n2 != len(output.Stderr) {
			return fmt.Errorf("insufficient write to stderr: wrote only %d of %d", n, len(output.Stderr))
		}

		return nil
	}
}
