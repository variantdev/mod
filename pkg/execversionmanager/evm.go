package execversionmanager

import (
	"fmt"
	"github.com/go-logr/logr"
	"github.com/twpayne/go-vfs"
	"github.com/variantdev/mod/pkg/cmdsite"
	"github.com/variantdev/mod/pkg/depresolver"
	"io"
	"k8s.io/klog/klogr"
	"os"
	"path/filepath"
)

type Config struct {
	Executables map[string]Executable `yaml:"executables"`
}

type Executable struct {
	Platforms []Platform `yaml:"platforms"`
}

type Platform struct {
	Source   string   `yaml:"source"`
	Selector Selector `yaml:"selector"`
}

type Selector struct {
	MatchLabels MatchLabels `yaml:"matchLabels"`
}

func (l Selector) Matches(set map[string]string) bool {
	return l.MatchLabels.Matches(set)
}

type MatchLabels struct {
	OS   string `yaml:"os"`
	Arch string `yaml:"arch"`
}

func (l MatchLabels) Matches(set map[string]string) bool {
	return set["os"] == l.OS && set["arch"] == l.Arch
}

type Bin struct {
	Path string
}

// ExecVM is an executable version manager that is capable of installing dependent executables and
// running any command in a shell context that the dependent executables are available in PATH
// It is something similar to `bundle exec` or `rbenv exec`, but for any set of executables
type ExecVM struct {
	Name   string
	Config *Config

	fs vfs.FS

	Logger logr.Logger

	AbsWorkDir     string
	CacheDirectory string

	dep *depresolver.Resolver

	Template *cmdsite.CommandSite
}

type Option interface {
	SetOption(r *ExecVM) error
}

func Logger(logger logr.Logger) Option {
	return &loggerOption{l: logger}
}

type loggerOption struct {
	l logr.Logger
}

func (s *loggerOption) SetOption(r *ExecVM) error {
	r.Logger = s.l
	return nil
}

func FS(fs vfs.FS) Option {
	return &fsOption{f: fs}
}

type fsOption struct {
	f vfs.FS
}

func (s *fsOption) SetOption(r *ExecVM) error {
	r.fs = s.f
	return nil
}

func WD(wd string) Option {
	return &wdOption{d: wd}
}

type wdOption struct {
	d string
}

func (s *wdOption) SetOption(r *ExecVM) error {
	r.AbsWorkDir = s.d
	return nil
}

func New(conf *Config, opts ...Option) (*ExecVM, error) {
	provider := &ExecVM{}

	for _, o := range opts {
		if err := o.SetOption(provider); err != nil {
			return nil, err
		}
	}

	if provider.Logger == nil {
		provider.Logger = klogr.New()
	}

	if provider.fs == nil {
		provider.fs = vfs.HostOSFS
	}

	if provider.AbsWorkDir == "" {
		path, err := os.Getwd()
		if err != nil {
			return nil, err
		}

		abs, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		provider.AbsWorkDir = abs
	}

	if provider.Template == nil {
		provider.Template = cmdsite.New()
	}

	if provider.CacheDirectory == "" {
		provider.CacheDirectory = ".variant/mod/cache"
	}

	abs := filepath.IsAbs(provider.CacheDirectory)
	if !abs {
		provider.CacheDirectory = filepath.Join(provider.AbsWorkDir, provider.CacheDirectory)
	}

	provider.Logger.V(1).Info("init", "workdir", provider.AbsWorkDir, "cachedir", provider.CacheDirectory)

	dep, err := depresolver.New(
		depresolver.Home(provider.CacheDirectory),
		depresolver.Logger(provider.Logger),
	)
	if err != nil {
		return nil, err
	}

	provider.dep = dep

	provider.Config = conf

	return provider, nil
}

func (p *ExecVM) getPlatformSpecificBin(platform Platform) (*Bin, error) {
	localCopy, err := p.dep.Resolve(platform.Source)
	if err != nil {
		return nil, err
	}

	return &Bin{
		Path: localCopy,
	}, nil
}

func (p *ExecVM) getBin(executable Executable) (*Bin, error) {
	platform, matched, err := getMatchingPlatform(executable)
	if err != nil {
		return nil, fmt.Errorf("matching platform: %v", err)
	}
	if !matched {
		os, arch := osArch()
		return nil, fmt.Errorf("no platform matched: os=%s, arch=%s", os, arch)
	}
	return p.getPlatformSpecificBin(platform)
}

func (p *ExecVM) Locate(name string) (*Bin, error) {
	executable, ok := p.Config.Executables[name]
	if !ok {
		return nil, fmt.Errorf("no executable defined: %s", name)
	}

	return p.getBin(executable)
}

func (p *ExecVM) Shell() (*cmdsite.CommandSite, error) {
	dirs := map[string]struct{}{}
	for bin := range p.Config.Executables {
		binPath, err := p.Locate(bin)
		if err != nil {
			return nil, err
		}
		dirs[filepath.Dir(binPath.Path)] = struct{}{}
	}

	path := os.Getenv("PATH")
	for d := range dirs {
		path = d + ":" + path
	}

	runcmd := *p.Template
	runcmd.RunCmd = func(cmd string, args []string, stdout io.Writer, stderr io.Writer, env map[string]string) error {
		newenv := map[string]string{}
		for k, v := range env {
			newenv[k] = v
		}
		newenv["PATH"] = path
		return cmdsite.DefaultRunCommand(cmd, args, stdout, stderr, newenv)
	}

	return &runcmd, nil
}
