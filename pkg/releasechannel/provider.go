package releasechannel

import (
	"fmt"
	"github.com/Masterminds/semver"
	"github.com/PaesslerAG/jsonpath"
	"github.com/go-logr/logr"
	"github.com/twpayne/go-vfs"
	"github.com/variantdev/mod/pkg/cmdsite"
	"github.com/variantdev/mod/pkg/depresolver"
	"github.com/variantdev/mod/pkg/maputil"
	"github.com/variantdev/mod/pkg/vhttpget"
	"gopkg.in/yaml.v3"
	"k8s.io/klog/klogr"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Release struct {
	Version     string
	Description string
}

type Provider struct {
	Name string
	Spec Spec

	fs vfs.FS

	cmdSite *cmdsite.CommandSite

	Logger logr.Logger

	AbsWorkDir string
	cacheDir   string

	GoGetterAbsWorkDir string
	goGetterCacheDir   string

	httpGetter vhttpget.Getter

	dep *depresolver.Resolver
}

type Option interface {
	SetOption(r *Provider) error
}

func New(conf *Config, channelName string, opts ...Option) (*Provider, error) {
	provider := &Provider{
		cmdSite: cmdsite.New(),
	}

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

	if provider.httpGetter == nil {
		provider.httpGetter = vhttpget.New()
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

	if provider.cacheDir == "" {
		provider.cacheDir = ".variant/mod/cache"
	}

	if provider.goGetterCacheDir == "" {
		provider.goGetterCacheDir = provider.cacheDir
	}

	abs := filepath.IsAbs(provider.cacheDir)
	if !abs {
		provider.cacheDir = filepath.Join(provider.AbsWorkDir, provider.cacheDir)
	}

	abs = filepath.IsAbs(provider.goGetterCacheDir)
	if !abs {
		provider.goGetterCacheDir = filepath.Join(provider.GoGetterAbsWorkDir, provider.goGetterCacheDir)
	}

	provider.Logger.V(1).Info("releasechannel.init", "workdir", provider.AbsWorkDir, "cachedir", provider.cacheDir, "gogetterworkdir", provider.GoGetterAbsWorkDir, "gogettercachedir", provider.goGetterCacheDir)

	dep, err := depresolver.New(
		depresolver.Home(provider.cacheDir),
		depresolver.GoGetterHome(provider.goGetterCacheDir),
		depresolver.Logger(provider.Logger),
		depresolver.FS(provider.fs),
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
	versionsFrom := p.Spec.VersionsFrom

	if versionsFrom.JSONPath.Source != "" {
		return p.jsonPath(versionsFrom.JSONPath)
	} else if versionsFrom.DockerImageTags.Source != "" {
		url := fmt.Sprintf("https://registry.hub.docker.com/v2/repositories/%s/tags/", versionsFrom.DockerImageTags.Source)
		return p.httpJsonPath(url, "$.results[*].name")
	} else if versionsFrom.GitTags.Source != "" {
		cmd := fmt.Sprintf("git ls-remote --tags git://%s.git | grep -v { | awk '{ print $2 }' | cut -d'/' -f 3", versionsFrom.GitTags.Source)
		return p.exec(cmd)
	} else if versionsFrom.GitHubReleases.Source != "" {
		host := versionsFrom.GitHubReleases.Host
		if host == "" {
			host = "api.github.com"
		}
		url := fmt.Sprintf("https://%s/repos/%s/releases", host, versionsFrom.GitHubReleases.Source)
		return p.httpJsonPath(url, "$[*].tag_name")
	}
	return nil, fmt.Errorf("no versions provider specified")
}

func (p *Provider) exec(cmd string) (*Release, error) {
	stdout, stderr, err := p.cmdSite.CaptureStrings("sh", []string{"-c", cmd})
	if len(stderr) > 0 {
		p.Logger.V(1).Info(stderr)
	}
	if err != nil {
		return nil, err
	}

	entries := strings.Split(stdout, "\n")

	vs := []string{}

	for _, e := range entries {
		if e != "" {
			vs = append(vs, e)
		}
	}

	return p.versionsToReleases(vs)
}

func (p *Provider) jsonPath(spec JSONPath) (*Release, error) {
	localCopy, err := p.dep.Resolve(spec.Source)
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

	return p.extractVersions(tmp, spec.Versions)
}

func (p *Provider) httpJsonPath(url string, jpath string) (*Release, error) {
	res, err := p.httpGetter.DoRequest(url)
	if err != nil {
		return nil, err
	}

	tmp := interface{}(nil)
	if err := yaml.Unmarshal([]byte(res), &tmp); err != nil {
		return nil, err
	}

	return p.extractVersions(tmp, jpath)
}

func (p *Provider) extractVersions(tmp interface{}, jpath string) (*Release, error) {
	v, err := maputil.RecursivelyCastKeysToStrings(tmp)
	if err != nil {
		return nil, err
	}

	got, err := jsonpath.Get(jpath, v)
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
		return nil, fmt.Errorf("unexpected type of result from jsonpath: \"%s\": %v", jpath, typed)
	}

	if len(raw) == 0 {
		return nil, fmt.Errorf("jsonpath: \"%s\": returned nothing: %v", jpath, v)
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

	return p.versionsToReleases(vs)
}

func (p *Provider) versionsToReleases(vs []string) (*Release, error) {
	vss := make([]*semver.Version, len(vs))
	for i, s := range vs {
		v, err := semver.NewVersion(s)
		if err != nil {
			return nil, fmt.Errorf("parsing version: index %d: %q: %v", i, s, err)
		}
		vss[i] = v
	}

	sort.Sort(semver.Collection(vss))

	return &Release{
		Version: vss[len(vss)-1].String(),
	}, nil
}
