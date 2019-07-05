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
	pp, err := p.GetProvider()
	if err != nil {
		return nil, err
	}

	return pp.Latest()
}

type ReleaseProvider interface {
	Latest() (*Release, error)
	All() ([]*Release, error)
}

func newExecProvider(cmd string, r *Provider) *execProvider {
	return &execProvider{
		cmd:     cmd,
		runtime: r,
	}
}

func newGetterProvider(spec GetterJSONPath, r *Provider) *getterJsonPathProvider {
	return &getterJsonPathProvider{
		spec:    spec,
		runtime: r,
	}
}

func newDockerHubImageTagsProvider(spec DockerImageTags, r *Provider) *httpJsonPathProvider {
	url := fmt.Sprintf("https://registry.hub.docker.com/v2/repositories/%s/tags/", spec.Source)
	return &httpJsonPathProvider{
		url:      url,
		jsonpath: "$.results[*].name",
		runtime:  r,
	}
}

func newGitHubReleasesProvider(spec GitHubReleases, r *Provider) *httpJsonPathProvider {
	host := spec.Host
	if host == "" {
		host = "api.github.com"
	}
	url := fmt.Sprintf("https://%s/repos/%s/releases", host, spec.Source)

	return &httpJsonPathProvider{
		url:      url,
		jsonpath: "$[*].tag_name",
		runtime:  r,
	}
}

type execProvider struct {
	cmd string

	runtime *Provider
}

var _ ReleaseProvider = &execProvider{}

func (p *execProvider) Latest() (*Release, error) {
	return p.runtime.execLatest(p.cmd)
}

func (p *execProvider) All() ([]*Release, error) {
	return p.runtime.execAll(p.cmd)
}

type getterJsonPathProvider struct {
	spec GetterJSONPath

	runtime *Provider
}

var _ ReleaseProvider = &getterJsonPathProvider{}

func (p *getterJsonPathProvider) Latest() (*Release, error) {
	return p.runtime.getterJsonPathLatest(p.spec)
}

func (p *getterJsonPathProvider) All() ([]*Release, error) {
	return p.runtime.getterJsonPath(p.spec)
}

type httpJsonPathProvider struct {
	url, jsonpath string

	runtime *Provider
}

var _ ReleaseProvider = &httpJsonPathProvider{}

func (p *httpJsonPathProvider) Latest() (*Release, error) {
	return p.runtime.httpJsonPathLatest(p.url, p.jsonpath)
}

func (p *httpJsonPathProvider) All() ([]*Release, error) {
	return p.runtime.httpJsonPath(p.url, p.jsonpath)
}

func (p *Provider) exec(cmd string) ([]string, error) {
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

	return vs, nil
}

func (p *Provider) execAll(cmd string) ([]*Release, error) {
	vs, err := p.exec(cmd)
	if err != nil {
		return nil, err
	}

	return p.versionsToReleases(vs)
}
func (p *Provider) execLatest(cmd string) (*Release, error) {
	vs, err := p.exec(cmd)
	if err != nil {
		return nil, err
	}

	return p.versionsToLatestRelease(vs)
}

func (p *Provider) getterJsonPathLatest(spec GetterJSONPath) (*Release, error) {
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

	return p.extractLatestVersion(tmp, spec.Versions)
}

func (p *Provider) getterJsonPath(spec GetterJSONPath) ([]*Release, error) {
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

func (p *Provider) httpJsonPathLatest(url string, jpath string) (*Release, error) {
	res, err := p.httpGetter.DoRequest(url)
	if err != nil {
		return nil, err
	}

	tmp := interface{}(nil)
	if err := yaml.Unmarshal([]byte(res), &tmp); err != nil {
		return nil, err
	}

	return p.extractLatestVersion(tmp, jpath)
}

func (p *Provider) httpJsonPath(url string, jpath string) ([]*Release, error) {
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

func (p *Provider) extractVersions(tmp interface{}, jpath string) ([]*Release, error) {
	vs, err := p.extractVersionStrings(tmp, jpath)
	if err != nil {
		return nil, err
	}

	return p.versionsToReleases(vs)
}

func (p *Provider) versionsToReleases(vs []string) ([]*Release, error) {
	vss, err := p.versionStringsToSemvers(vs)
	if err != nil {
		return nil, err
	}

	rs := []*Release{}

	for _, ver := range vss {
		rs = append(rs, semverToRelease(ver))
	}

	return rs, nil
}

func semverToRelease(ver *semver.Version) *Release {
	return &Release{
		Version: ver.String(),
	}
}

func (p *Provider) extractLatestVersion(tmp interface{}, jpath string) (*Release, error) {
	vs, err := p.extractVersionStrings(tmp, jpath)
	if err != nil {
		return nil, err
	}

	return p.versionsToLatestRelease(vs)
}

func (p *Provider) extractVersionStrings(tmp interface{}, jpath string) ([]string, error) {
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

	return vs, nil
}

func (p *Provider) versionStringsToSemvers(vs []string) ([]*semver.Version, error) {
	vss := make([]*semver.Version, len(vs))
	for i, s := range vs {
		v, err := semver.NewVersion(s)
		if err != nil {
			return nil, fmt.Errorf("parsing version: index %d: %q: %v", i, s, err)
		}
		vss[i] = v
	}

	sort.Sort(semver.Collection(vss))

	return vss, nil
}

func (p *Provider) versionsToLatestRelease(vs []string) (*Release, error) {
	vss, err := p.versionStringsToSemvers(vs)
	if err != nil {
		return nil, err
	}

	return &Release{
		Version: vss[len(vss)-1].String(),
	}, nil
}

func (p *Provider) GetProvider() (ReleaseProvider, error) {
	versionsFrom := p.Spec.VersionsFrom

	if versionsFrom.JSONPath.Source != "" {
		return newGetterProvider(versionsFrom.JSONPath, p), nil
	} else if versionsFrom.DockerImageTags.Source != "" {
		return newDockerHubImageTagsProvider(versionsFrom.DockerImageTags, p), nil
	} else if versionsFrom.GitTags.Source != "" {
		cmd := fmt.Sprintf("git ls-remote --tags git://%s.git | grep -v { | awk '{ print $2 }' | cut -d'/' -f 3", versionsFrom.GitTags.Source)
		return newExecProvider(cmd, p), nil
	} else if versionsFrom.GitHubReleases.Source != "" {
		return newGitHubReleasesProvider(versionsFrom.GitHubReleases, p), nil
	}
	return nil, fmt.Errorf("no versions provider specified")
}

func (p *Provider) GetVersions() ([]*Release, error) {
	pp, err := p.GetProvider()
	if err != nil {
		return nil, err
	}

	return pp.All()
}
