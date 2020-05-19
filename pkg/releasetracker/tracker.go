package releasetracker

import (
	"fmt"
	"github.com/Masterminds/semver"
	"github.com/PaesslerAG/jsonpath"
	"github.com/go-logr/logr"
	"github.com/heroku/docker-registry-client/registry"
	"github.com/twpayne/go-vfs"
	"github.com/variantdev/mod/pkg/cmdsite"
	"github.com/variantdev/mod/pkg/depresolver"
	"github.com/variantdev/mod/pkg/maputil"
	"github.com/variantdev/mod/pkg/vhttpget"
	"gopkg.in/yaml.v3"
	"io/ioutil"
	"k8s.io/klog/klogr"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type Release struct {
	// Semver is the semantic version of the release.
	//
	// For "1.2.3" this is a semver object of "1.2.3", and for "1.2.3.123" it is a semver of "1.2.3-123" so that we can
	// even handle versions that are not precisely semver-compatible.
	Semver *semver.Version

	// Version is mostly the original version string obtained from a release provider, with the "v" prefix removed
	Version string

	Description string

	// Meta is the provider-specific metadata composed of arbitrary kv pairs
	Meta map[string]interface{}
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

func debug(msg string, v ...interface{}) {
	if os.Getenv("DEBUG") != "" {
		fmt.Fprintf(os.Stderr, msg+"\n", v...)
	}
}

func (p *Tracker) Latest(constraint string) (*Release, error) {
	all, err := p.GetReleases()
	if err != nil {
		return nil, err
	}

	return getLatest(constraint, all)
}

func getLatest(constraint string, all []*Release) (*Release, error) {
	if constraint == "" {
		constraint = "> 0.0.0-0"
	}

	cons, err := semver.NewConstraint(constraint)
	if err != nil {
		return nil, err
	}

	debug("releases: %+v", all)

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

func newDockerHubImageTagsProvider(spec DockerImageTags, r *Tracker) *dockerImageTagsProvider {
	return &dockerImageTagsProvider{
		source:  spec.Source,
		runtime: r,
	}
}

func newGitHubReleasesProvider(spec GitHubReleases, r *Tracker) *httpJsonPathProvider {
	host := spec.Host
	if host == "" {
		host = "api.github.com"
	}
	url := fmt.Sprintf("https://%s/repos/%s/releases", host, spec.Source)

	return &httpJsonPathProvider{
		url:         url,
		jsonpath:    "$[*].tag_name",
		metaKey:     "githubRelease",
		objectPath:  "$[*]",
		versionPath: "tag_name",
		runtime:     r,
	}
}

func newGitHubTagsProvider(spec GitHubTags, r *Tracker) *httpJsonPathProvider {
	host := spec.Host
	if host == "" {
		host = "api.github.com"
	}
	url := fmt.Sprintf("https://%s/repos/%s/tags", host, spec.Source)

	return &httpJsonPathProvider{
		url:      url,
		jsonpath: "$[*].name",
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

type dockerImageTagsProvider struct {
	source   string
	username string
	password string

	runtime *Tracker
}

func (p *dockerImageTagsProvider) All() ([]*Release, error) {
	if p.username == "" {
		p.username = os.Getenv("DOCKER_USERNAME")
	}
	if p.password == "" {
		p.password = os.Getenv("DOCKER_PASSWORD")
	}
	w := log.Writer()
	log.SetOutput(ioutil.Discard)
	defer log.SetOutput(w)
	hub, err := registry.New("https://registry.hub.docker.com/", p.username, p.password)
	if err != nil {
		return nil, err
	}

	tags, err := hub.Tags(p.source)
	if err != nil {
		return nil, err
	}

	releases, err := p.runtime.versionsToReleases(tags)
	if err != nil {
		return nil, err
	}

	return releases, nil
}

type httpJsonPathProvider struct {
	url, jsonpath string
	nextpagePath  string
	params        map[string]string

	metaKey     string
	objectPath  string
	versionPath string

	runtime *Tracker
}

var _ ReleaseProvider = &httpJsonPathProvider{}

func (p *httpJsonPathProvider) All() ([]*Release, error) {
	return p.runtime.releasesFromHttpJsonPath(p)
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
	localCopy, err := p.dep.ResolveFile(spec.Source)
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

func (p *Tracker) releasesFromHttpJsonPath(pp *httpJsonPathProvider) ([]*Release, error) {
	url := pp.url
	jpath := pp.jsonpath
	nextpagePath := pp.nextpagePath
	params := pp.params

	query := ""
	for k, v := range params {
		if query != "" {
			query += "&"
		}
		query += k + "=" + v
	}

	var releases []*Release
	for url != "" {
		var u string
		if strings.Contains(url, query) {
			u = url
		} else {
			if strings.Contains(url, "?") {
				u = fmt.Sprintf("%s%s", url, query)
			} else {
				u = fmt.Sprintf("%s?%s", url, query)
			}
		}
		debug("http get: %s", u)

		res, err := p.httpGetter.DoRequest(u)
		if err != nil {
			return nil, err
		}

		tmp := interface{}(nil)
		if err := yaml.Unmarshal([]byte(res), &tmp); err != nil {
			return nil, err
		}

		debug("http response: %v", res)

		if pp.objectPath != "" && pp.versionPath != "" && pp.metaKey != "" {
			page, err := p.extractObjects(tmp, pp.objectPath, pp.versionPath, pp.metaKey)
			if err != nil {
				return nil, err
			}

			releases = append(releases, page...)
		} else {
			page, err := p.extractVersions(tmp, jpath)
			if err != nil {
				return nil, err
			}

			releases = append(releases, page...)
		}

		if nextpagePath == "" {
			break
		}

		nextUrl, err := p.extractString(tmp, nextpagePath)
		if err != nil {
			return nil, err
		}

		url = nextUrl
	}

	return releases, nil
}

func (p *Tracker) extractObjects(tmp interface{}, objPath, verPath, metaKey string) ([]*Release, error) {
	v, err := maputil.RecursivelyCastKeysToStrings(tmp)
	if err != nil {
		return nil, err
	}

	got, err := jsonpath.Get(objPath, v)
	if err != nil {
		return nil, err
	}

	var rs []*Release

	switch typed := got.(type) {
	case []interface{}:
		for _, obj := range typed {
			raw, err := jsonpath.Get(verPath, obj)
			if err != nil {
				return nil, err
			}

			s, ok := raw.(string)
			if !ok {
				return nil, fmt.Errorf("unexpected type of value: want string, got %T, value is %v", raw, raw)
			}

			v, err := p.parseVersion(s)
			if err != nil {
				return nil, err
			}

			meta := map[string]interface{}{
				metaKey: obj,
			}

			rs = append(rs, &Release{
				Semver: v,
				Version: strings.TrimPrefix(s, "v"),
				Meta: meta,
			})
		}
	}

	sort.Slice(rs, func(i, j int) bool {
		return rs[i].Semver.LessThan(rs[j].Semver)
	})

	return rs, nil
}

func (p *Tracker) extractVersions(tmp interface{}, jpath string) ([]*Release, error) {
	vs, err := p.extractVersionStrings(tmp, jpath)
	if err != nil {
		return nil, err
	}

	return p.versionsToReleases(vs)
}

func (p *Tracker) extractString(tmp interface{}, path string) (string, error) {
	v, err := maputil.RecursivelyCastKeysToStrings(tmp)
	if err != nil {
		return "", err
	}

	got, err := jsonpath.Get(path, v)
	if err != nil {
		return "", err
	}

	if got == nil {
		return "", nil
	}

	return fmt.Sprintf("%v", got), nil
}

func (p *Tracker) versionsToReleases(vs []string) ([]*Release, error) {
	rs, err := p.versionStringsToReleases(vs)
	if err != nil {
		return nil, err
	}

	return rs, nil
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

var versionRegex *regexp.Regexp

func init() {
	versionRegex = regexp.MustCompile(`v?([0-9]+)(\.[0-9]+)?(\.[0-9]+)?` + `(.*)`)
}

func nonSemverWorkaround(s string) string {
	matches := versionRegex.FindStringSubmatch(s)

	var preLike string

	if len(matches) > 3 {
		preLike = matches[4]
	}

	if preLike != "" && preLike[0] == '.' {
		s = ""
		ss := matches[1:4]
		for i := range ss {
			if ss[i] != "" {
				s += ss[i]
			}
		}

		s += "-" + preLike[1:]
	}

	return s
}

func (p *Tracker) parseVersion(s string) (*semver.Version, error) {
	fixedS := nonSemverWorkaround(strings.TrimSpace(s))

	return semver.NewVersion(fixedS)
}

func (p *Tracker) versionStringsToReleases(vs []string) ([]*Release, error) {
	rs := []*Release{}
	for i, s := range vs {
		v, err := p.parseVersion(s)
		if err != nil {
			e := fmt.Errorf("parsing version: index %d: %q: %v", i, s, err)
			p.Logger.V(1).Info("ignoring error", "err", e)
		}

		if v != nil {
			rs = append(rs, &Release{
				Semver:  v,
				Version: strings.TrimPrefix(s, "v"),
			})
		}
	}

	sort.Slice(rs, func(i, j int) bool {
		return rs[i].Semver.LessThan(rs[j].Semver)
	})

	return rs, nil
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
	} else if versionsFrom.GitHubTags.Source != "" {
		return newGitHubTagsProvider(versionsFrom.GitHubTags, p), nil
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

	all, err := pp.All()
	if err != nil {
		return nil, err
	}

	if p.Spec.VersionsFrom.ValidVersionPattern == nil {
		return all, err
	}

	var filtered []*Release

	for i := range all {
		r := all[i]

		if p.Spec.VersionsFrom.ValidVersionPattern.MatchString(r.Version) {
			filtered = append(filtered, r)
		}
	}

	return filtered, nil
}
