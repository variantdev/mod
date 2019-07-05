package cmdsite

import (
	"bytes"
	"io"
	"k8s.io/klog"
	"os"
	"os/exec"
	"strings"
)

type RunCommand func(name string, args []string, stdout, stderr io.Writer, env map[string]string) error

type CommandSite struct {
	RunCmd RunCommand

	Env map[string]string
}

type Option interface {
	Set(*CommandSite)
}

func RunCmd(runcmd RunCommand) Option {
	return &runcmdOption{
		runcmd: runcmd,
	}
}

type runcmdOption struct {
	runcmd RunCommand
}

func (o *runcmdOption) Set(site *CommandSite) {
	site.RunCmd = o.runcmd
}

func New(opt ...Option) *CommandSite {
	site := &CommandSite{
		RunCmd: nil,
		Env:    map[string]string{},
	}

	for _, o := range opt {
		o.Set(site)
	}

	return site
}

func (s *CommandSite) RunCommand(cmd string, args []string, stdout, stderr io.Writer) error {
	return s.RunCmd(cmd, args, stdout, stderr, s.Env)
}

func (r *CommandSite) CaptureStrings(binary string, args []string) (string, string, error) {
	stdout, stderr, err := r.CaptureBytes(binary, args)

	var so, se string

	if stdout != nil {
		so = string(stdout)
	}

	if stderr != nil {
		se = string(stderr)
	}

	return so, se, err
}

func (r *CommandSite) CaptureBytes(binary string, args []string) ([]byte, []byte, error) {
	klog.V(1).Infof("running %s %s", binary, strings.Join(args, " "))
	_, err := exec.LookPath(binary)
	if err != nil {
		return nil, nil, err
	}

	var stdout, stderr bytes.Buffer
	err = r.RunCommand(binary, args, &stdout, &stderr)
	if err != nil {
		klog.V(1).Info(stderr.String())
	}
	return stdout.Bytes(), stderr.Bytes(), err
}

func (r *CommandSite) SetPath(path string) *CommandSite {
	runCmd := DefaultRunCommand
	if r.RunCmd != nil {
		runCmd = r.RunCmd
	}

	site := *r
	site.RunCmd = func(cmd string, args []string, stdout io.Writer, stderr io.Writer, env map[string]string) error {
		newenv := map[string]string{}
		for k, v := range env {
			newenv[k] = v
		}
		newenv["PATH"] = path
		return runCmd(cmd, args, stdout, stderr, newenv)
	}
	return &site
}

func (r *CommandSite) PrependPath(path string) *CommandSite {
	return r.SetPath(path + ":" + os.Getenv("PATH"))
}

func (r *CommandSite) PrependDirsToPath(dirs map[string]struct{}) *CommandSite {
	var path string
	for d := range dirs {
		if path == "" {
			path = d
		} else {
			path = d + ":" + path
		}
	}

	return r.PrependPath(path)
}
