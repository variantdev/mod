package releasechannel

import (
	"context"
	"fmt"
	"github.com/Masterminds/semver"
	"github.com/PaesslerAG/gval"
	"github.com/PaesslerAG/jsonpath"
	"github.com/go-logr/logr"
	"github.com/twpayne/go-vfs"
	"github.com/variantdev/mod/pkg/depresolver"
	"github.com/variantdev/mod/pkg/maputil"
	"gopkg.in/yaml.v3"
	"k8s.io/klog/klogr"
	"os"
	"path/filepath"
	"sort"
)

type Config struct {
	ReleaseChannels map[string]Spec `yaml:"releaseChannels"`
}

type Spec struct {
	Source      string `yaml:"source"`
	Versions    string `yaml:"versions"`
	Description string `yaml:"description"`
}

type Release struct {
	Version     string
	Description string
}

type Provider struct {
	Name string
	Spec Spec

	fs vfs.FS

	Logger logr.Logger

	AbsWorkDir     string
	CacheDirectory string

	dep *depresolver.Resolver
}

type Option interface {
	SetOption(r *Provider) error
}

func Logger(logger logr.Logger) Option {
	return &loggerOption{l: logger}
}

type loggerOption struct {
	l logr.Logger
}

func (s *loggerOption) SetOption(r *Provider) error {
	r.Logger = s.l
	return nil
}

func FS(fs vfs.FS) Option {
	return &fsOption{f: fs}
}

type fsOption struct {
	f vfs.FS
}

func (s *fsOption) SetOption(r *Provider) error {
	r.fs = s.f
	return nil
}

func WD(wd string) Option {
	return &wdOption{d: wd}
}

type wdOption struct {
	d string
}

func (s *wdOption) SetOption(r *Provider) error {
	r.AbsWorkDir = s.d
	return nil
}

func New(conf *Config, channelName string, opts ...Option) (*Provider, error) {
	provider := &Provider{}

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

	ch, ok := conf.ReleaseChannels[channelName]
	if !ok {
		return nil, fmt.Errorf("release channel \"%s\" is not defined: %v", channelName, conf)
	}

	provider.Name = channelName
	provider.Spec = ch

	return provider, nil
}

func (p *Provider) Latest() (*Release, error) {
	localCopy, err := p.dep.Resolve(p.Spec.Source)
	if err != nil {
		return nil, err
	}

	bs, err := p.fs.ReadFile(localCopy)
	if err != nil {
		return nil, err
	}

	tmp := interface{}(nil)
	if err := yaml.Unmarshal(bs, &tmp); err != nil {
		return nil, err
	}

	v, err := maputil.CastKeysToStrings(tmp)
	if err != nil {
		return nil, err
	}

	builder := gval.Full(jsonpath.WildcardExtension())

	path, err := builder.NewEvaluable(p.Spec.Versions)
	if err != nil {
		return nil, err
	}

	got, err := path(context.Background(), v)
	if err != nil {
		return nil, err
	}

	raw := []interface{}{}
	switch typed := got.(type) {
	case []interface{}:
		raw = typed
	case map[string]interface{}:
		raw = append(raw, typed)
	default:
		return nil, fmt.Errorf("unexpected type of result from jsonpath: \"%s\": %v", p.Spec.Versions, typed)
	}

	if len(raw) == 0 {
		return nil, fmt.Errorf("jsonpath: \"%s\": returned nothing: %v", p.Spec.Versions, v)
	}

	vs := []string{}
	for _, r := range raw {
		switch typed := r.(type) {
		case map[interface{}]interface{}:
			for k, _ := range typed {
				vs = append(vs, k.(string))
			}
		case map[string]interface{}:
			for k, _ := range typed {
				vs = append(vs, k)
			}
		case string:
			vs = append(vs, typed)
		default:
			return nil, fmt.Errorf("jsonpath: unexpected type of result: %T=%v", typed, typed)
		}
	}

	vss := make([]*semver.Version, len(vs))
	for i, s := range vs {
		v, err := semver.NewVersion(s)
		if err != nil {
			return nil, fmt.Errorf("parsing version: %s: %v", s, err)
		}
		vss[i] = v
	}

	if len(vss) == 0 {
		return nil, fmt.Errorf("jsonpath: \"%s\": no result: %v", p.Spec.Versions, vs)
	}

	sort.Sort(semver.Collection(vss))

	return &Release{
		Version: vss[len(vss)-1].String(),
	}, nil
}
