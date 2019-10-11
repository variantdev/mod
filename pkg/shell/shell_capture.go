package shell

import (
	"bufio"
	"strings"
)

type CaptureResult struct {
	ExitStatus int
	Stdout     string
	Stderr     string
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
		if err := scanner.Err(); err != nil {
			panic(err)
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
