package execversionmanager

import (
	"fmt"
	"github.com/go-logr/logr"
	"github.com/k-kinzal/aliases/pkg/aliases/yaml"
	"github.com/twpayne/go-vfs"
	"github.com/variantdev/mod/pkg/cmdsite"
	"github.com/variantdev/mod/pkg/depresolver"
	"github.com/variantdev/mod/pkg/tmpl"
	"k8s.io/klog/klogr"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Executables map[string]Executable `yaml:"executables"`
}

type Executable struct {
	Platforms []Platform `yaml:"platforms"`
}

type Platform struct {
	Source   string          `yaml:"source"`
	Docker   yaml.OptionSpec `yaml:"docker"`
	Selector Selector        `yaml:"selector"`
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

	AbsWorkDir         string
	GoGetterAbsWorkDir string
	CacheDir           string
	GoGetterCacheDir   string

	dep *depresolver.Resolver

	Template *cmdsite.CommandSite

	Values map[string]interface{}
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

func GoGetterWD(goGetterWD string) Option {
	return &goGetterWDOption{d: goGetterWD}
}

type goGetterWDOption struct {
	d string
}

func (s *goGetterWDOption) SetOption(r *ExecVM) error {
	r.GoGetterAbsWorkDir = s.d
	return nil
}

func Values(values map[string]interface{}) Option {
	return &valuesOption{vals: values}
}

type valuesOption struct {
	vals map[string]interface{}
}

func (s *valuesOption) SetOption(r *ExecVM) error {
	r.Values = s.vals
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

	if provider.GoGetterAbsWorkDir == "" {
		provider.GoGetterAbsWorkDir = provider.AbsWorkDir
	}

	if provider.Template == nil {
		provider.Template = cmdsite.New()
	}

	if provider.CacheDir == "" {
		provider.CacheDir = ".variant/mod/cache"
	}

	if provider.GoGetterCacheDir == "" {
		provider.GoGetterCacheDir = provider.CacheDir
	}

	abs := filepath.IsAbs(provider.CacheDir)
	if !abs {
		provider.CacheDir = filepath.Join(provider.AbsWorkDir, provider.CacheDir)
	}

	if !filepath.IsAbs(provider.GoGetterCacheDir) {
		provider.GoGetterCacheDir = filepath.Join(provider.GoGetterAbsWorkDir, provider.GoGetterCacheDir)
	}

	provider.Logger.V(1).Info("execversionmanager.init", "workdir", provider.AbsWorkDir, "cachedir", provider.CacheDir, "gogetterworkdir", provider.GoGetterAbsWorkDir, "gogettercachedir", provider.GoGetterCacheDir)

	dep, err := depresolver.New(
		depresolver.FS(provider.fs),
		depresolver.Home(provider.CacheDir),
		depresolver.Logger(provider.Logger),
		depresolver.GoGetterHome(provider.GoGetterCacheDir),
	)
	if err != nil {
		return nil, err
	}

	provider.dep = dep

	provider.Config = conf

	return provider, nil
}

func (p *ExecVM) getPlatformSpecificBin(name string, platform Platform) (*Bin, error) {
	var localCopy string

	if platform.Source != "" {
		source, err := tmpl.Render("source", platform.Source, p.Values)
		if err != nil {
			return nil, err
		}

		localCopy, err = p.dep.ResolveFile(source)
		if err != nil {
			return nil, err
		}

		renamed := filepath.Join(filepath.Dir(localCopy), name)
		if err := p.fs.Rename(localCopy, renamed); err != nil {
			return nil, err
		}
		if err := p.fs.Chmod(renamed, 0755); err != nil {
			return nil, err
		}

		translated := strings.Replace(renamed, p.CacheDir, p.GoGetterCacheDir, 1)

		localCopy = translated

	} else if platform.Docker.Image != "" {
		var err error
		localCopy, err = p.getDockerAlias(name, platform)
		if err != nil {
			return nil, err
		}
	} else {
		return nil, fmt.Errorf("either source or docker must be specified in platform: %v", platform)
	}

	return &Bin{
		Path: localCopy,
	}, nil
}

func (p *ExecVM) getBin(name string, executable Executable) (*Bin, error) {
	platform, matched, err := getMatchingPlatform(executable)
	if err != nil {
		return nil, fmt.Errorf("matching platform: %v", err)
	}
	if !matched {
		if len(executable.Platforms) > 1 {
			os, arch := OsArch()
			return nil, fmt.Errorf("no platform matched in %q: os=%s, arch=%s", name, os, arch)
		}
		platform = executable.Platforms[0]
	}
	return p.getPlatformSpecificBin(name, platform)
}

func (p *ExecVM) Locate(name string) (*Bin, error) {
	executable, ok := p.Config.Executables[name]
	if !ok {
		return nil, fmt.Errorf("no executable defined: %s", name)
	}

	return p.getBin(name, executable)
}

func (p *ExecVM) Build() error {
	for bin := range p.Config.Executables {
		_, err := p.Locate(bin)
		if err != nil {
			return err
		}
	}

	return nil
}

func (p *ExecVM) Dirs() (map[string]struct{}, error) {
	dirs := map[string]struct{}{}
	for bin := range p.Config.Executables {
		binPath, err := p.Locate(bin)
		if err != nil {
			return nil, err
		}
		dirs[filepath.Dir(binPath.Path)] = struct{}{}
	}
	return dirs, nil
}

func (p *ExecVM) Shell() (*cmdsite.CommandSite, error) {
	dirs, err := p.Dirs()
	if err != nil {
		return nil, err
	}

	return p.ShellFromDirs(dirs), nil
}

func (p *ExecVM) ShellFromDirs(dirs map[string]struct{}) *cmdsite.CommandSite {
	return p.Template.PrependDirsToPath(dirs)
}
