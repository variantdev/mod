package shell

import (
	"bytes"
	"testing"
)

func TestCapture(t *testing.T) {
	sh := Shell{
		Exec: DefaultExec,
	}

	hello := &Command{
		Name: "sh",
		Args: []string{"-c", "echo hello; echo err1 1>&2; echo world; echo err2 1>&2"},
	}

	stdoutLog := &bytes.Buffer{}
	logStdout := func(s string) {
		stdoutLog.WriteString(s)
	}

	stderrLog := &bytes.Buffer{}
	logStderr := func(s string) {
		stderrLog.WriteString(s)
	}

	opts := CaptureOpts{
		LogStdout:      logStdout,
		LogStderr:      logStderr,
		OutToErrPrefix: "",
	}

	res, err := sh.Capture(hello, opts)

	{
		actual := stdoutLog.String()
		expected := "helloworld"
		if actual != expected {
			t.Errorf("unexpected stdout logged: expected=%s, got=%s", expected, actual)
		}
	}

	{
		actual := stderrLog.String()
		expected := "err1err2"
		if actual != expected {
			t.Errorf("unexpected stderr logged: expected=%s, got=%s", expected, actual)
		}
	}

	{
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if res.ExitStatus != 0 {
			t.Errorf("unexpected exit status: expected=0, got=%d", res.ExitStatus)
		}
	}

	{
		actual := res.Stdout
		expected := "hello\nworld"
		if actual != expected {
			t.Errorf("unexpected stdout captured: expected=%s, got=%s", expected, actual)
		}
	}

	{
		actual := res.Stderr
		expected := "err1\nerr2"
		if actual != expected {
			t.Errorf("unexpected stderr captured: expected=%s, got=%s", expected, actual)
		}
	}
}
