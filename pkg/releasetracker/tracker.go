package releasetracker

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
	Semver      *semver.Version
	Version     string
	Description string
}

type Tracker struct {
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
	SetOption(r *Tracker) error
}

func New(conf Spec, opts ...Option) (*Tracker, error) {
	provider := &Tracker{
		cmdSite: cmdsite.New(),
	}

	for _, o := range opts {
		if err := o.SetOption(provider); err != nil {
			return nil, err
		}
	}

	if provider.cmdSite.RunCmd == nil {
		provider.cmdSite.RunCmd = cmdsite.DefaultRunCommand
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

	provider.Spec = conf

	return provider, nil
}

func (p *Tracker) Latest(constraint string) (*Release, error) {
	if constraint == "" {
		constraint = "> 0.0.0"
	}

	cons, err := semver.NewConstraint(constraint)
	if err != nil {
		return nil, err
	}

	all, err := p.GetReleases()
	if err != nil {
		return nil, err
	}

	var latestVer semver.Version
	var latest *Release

	for _, r := range all {
		if !cons.Check(r.Semver) {
			continue
		}
		if latestVer.LessThan(r.Semver) {
			latestVer = *r.Semver
			latest = r
		}
	}

	if latest == nil {
		vers := []string{}
		for _, r := range all {
			vers = append(vers, r.Semver.String())
		}
		return nil, fmt.Errorf("no semver matching %q found in %v", constraint, vers)
	}

	return latest, nil
}

type ReleaseProvider interface {
	All() ([]*Release, error)
}

func newExecProvider(cmd string, args []string, r *Tracker) *execProvider {
	return &execProvider{
		command: cmd,
		args:    args,
		runtime: r,
	}
}

func newShellProvider(cmd string, r *Tracker) *shellProvider {
	return &shellProvider{
		script:  cmd,
		runtime: r,
	}
}

func newGetterProvider(spec GetterJSONPath, r *Tracker) *getterJsonPathProvider {
	return &getterJsonPathProvider{
		spec:    spec,
		runtime: r,
	}
}

func newDockerHubImageTagsProvider(spec DockerImageTags, r *Tracker) *httpJsonPathProvider {
	url := fmt.Sprintf("https://registry.hub.docker.com/v2/repositories/%s/tags/", spec.Source)
	return &httpJsonPathProvider{
		url:      url,
		jsonpath: "$.results[*].name",
		runtime:  r,
	}
}

func newGitHubReleasesProvider(spec GitHubReleases, r *Tracker) *httpJsonPathProvider {
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
	command string
	args    []string

	runtime *Tracker
}

var _ ReleaseProvider = &execProvider{}

func (p *execProvider) All() ([]*Release, error) {
	return p.runtime.releasesFromExec(p.command, p.args)
}

type shellProvider struct {
	script string

	runtime *Tracker
}

var _ ReleaseProvider = &shellProvider{}

func (p *shellProvider) All() ([]*Release, error) {
	return p.runtime.releasesFromShell(p.script)
}

type getterJsonPathProvider struct {
	spec GetterJSONPath

	runtime *Tracker
}

var _ ReleaseProvider = &getterJsonPathProvider{}

func (p *getterJsonPathProvider) All() ([]*Release, error) {
	return p.runtime.releasesFromGetterJsonPath(p.spec)
}

type httpJsonPathProvider struct {
	url, jsonpath string

	runtime *Tracker
}

var _ ReleaseProvider = &httpJsonPathProvider{}

func (p *httpJsonPathProvider) All() ([]*Release, error) {
	return p.runtime.releasesFromHttpJsonPath(p.url, p.jsonpath)
}

func (p *Tracker) execScript(cmd string) ([]string, error) {
	return p.exec("sh", []string{"-c", cmd})
}

func (p *Tracker) exec(cmd string, args []string) ([]string, error) {
	stdout, stderr, err := p.cmdSite.CaptureStrings(cmd, args)
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

func (p *Tracker) releasesFromExec(cmd string, args []string) ([]*Release, error) {
	vs, err := p.exec(cmd, args)
	if err != nil {
		return nil, err
	}

	return p.versionsToReleases(vs)
}

func (p *Tracker) releasesFromShell(cmd string) ([]*Release, error) {
	vs, err := p.execScript(cmd)
	if err != nil {
		return nil, err
	}

	return p.versionsToReleases(vs)
}

func (p *Tracker) releasesFromGetterJsonPath(spec GetterJSONPath) ([]*Release, error) {
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

func (p *Tracker) releasesFromHttpJsonPath(url string, jpath string) ([]*Release, error) {
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

func (p *Tracker) extractVersions(tmp interface{}, jpath string) ([]*Release, error) {
	vs, err := p.extractVersionStrings(tmp, jpath)
	if err != nil {
		return nil, err
	}

	return p.versionsToReleases(vs)
}

func (p *Tracker) versionsToReleases(vs []string) ([]*Release, error) {
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
		Semver:  ver,
		Version: ver.String(),
	}
}

func (p *Tracker) extractVersionStrings(tmp interface{}, jpath string) ([]string, error) {
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

func (p *Tracker) versionStringsToSemvers(vs []string) ([]*semver.Version, error) {
	vss := []*semver.Version{}
	for i, s := range vs {
		v, err := semver.NewVersion(s)
		if err != nil {
			e := fmt.Errorf("parsing version: index %d: %q: %v", i, s, err)
			p.Logger.V(1).Info("ignoring error", "err", e)
		}
		if v != nil{
			vss = append(vss, v)
		}
	}

	sort.Sort(semver.Collection(vss))

	return vss, nil
}

func (p *Tracker) GetProvider() (ReleaseProvider, error) {
	versionsFrom := p.Spec.VersionsFrom

	if versionsFrom.JSONPath.Source != "" {
		return newGetterProvider(versionsFrom.JSONPath, p), nil
	} else if versionsFrom.Exec.Command != "" {
		return newExecProvider(versionsFrom.Exec.Command, versionsFrom.Exec.Args, p), nil
	} else if versionsFrom.DockerImageTags.Source != "" {
		return newDockerHubImageTagsProvider(versionsFrom.DockerImageTags, p), nil
	} else if versionsFrom.GitTags.Source != "" {
		cmd := fmt.Sprintf("git ls-remote --tags git://%s.git | grep -v { | awk '{ print $2 }' | cut -d'/' -f 3", versionsFrom.GitTags.Source)
		return newShellProvider(cmd, p), nil
	} else if versionsFrom.GitHubReleases.Source != "" {
		return newGitHubReleasesProvider(versionsFrom.GitHubReleases, p), nil
	}
	return nil, fmt.Errorf("no versions provider specified")
}

func (p *Tracker) GetReleases() ([]*Release, error) {
	pp, err := p.GetProvider()
	if err != nil {
		return nil, err
	}

	return pp.All()
}
