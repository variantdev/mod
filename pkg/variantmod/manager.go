package variantmod

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-logr/logr"
	"github.com/google/go-github/v27/github"
	"github.com/twpayne/go-vfs"
	"github.com/variantdev/mod/pkg/cmdsite"
	"github.com/variantdev/mod/pkg/depresolver"
	"github.com/variantdev/mod/pkg/execversionmanager"
	"github.com/variantdev/mod/pkg/gitops"
	"github.com/variantdev/mod/pkg/maputil"
	"github.com/variantdev/mod/pkg/releasetracker"
	"github.com/variantdev/mod/pkg/tmpl"
	"github.com/variantdev/mod/pkg/yamlpatch"
	"github.com/xeipuuv/gojsonschema"
	"golang.org/x/oauth2"
	"gopkg.in/yaml.v3"
	"k8s.io/klog"
	"k8s.io/klog/klogr"
	"os"
	"path/filepath"
	"strings"
)

type ModuleSpec struct {
	Name string `yaml:"name"`

	Parameters   ParametersSpec                 `yaml:"parameters"`
	Provisioners ProvisionersSpec               `yaml:"provisioners"`
	Dependencies map[string]DependencySpec      `yaml:"dependencies"`
	Releases     map[string]releasetracker.Spec `yaml:"releases"`
}

type ParametersSpec struct {
	Schema   map[string]interface{} `yaml:"schema"`
	Defaults map[string]interface{} `yaml:"defaults"`
}

type ProvisionersSpec struct {
	Files       map[string]FileSpec        `yaml:"files"`
	Executables execversionmanager.Config  `yaml:",inline"`
	TextReplace map[string]TextReplaceSpec `yaml:"textReplace"`
	YamlPatch   map[string][]YamlPatchSpec `yaml:"yamlPatch"`
}

type FileSpec struct {
	Source    string                 `yaml:"source"`
	Arguments map[string]interface{} `yaml:"arguments"`
}

type TextReplaceSpec struct {
	From string `yaml:"from"`
	To   string `yaml:"to"`
}

type YamlPatchSpec struct {
	Op string `yaml:"op"`
	Path string `yaml:"path"`
	Value interface{} `yaml:"value"`
	From string `yaml:"string"`
}

type DependencySpec struct {
	ReleasesFrom releasetracker.VersionsFrom `yaml:"releasesFrom""`

	Source string `yaml:"source"`
	Kind   string `yaml:"kind"`
	// VersionConstraint is the version range for this dependency. Works only for modules hosted on Git or GitHub
	VersionConstraint string                 `yaml:"version"`
	Arguments         map[string]interface{} `yaml:"arguments"`

	Alias          string
	LockedVersions ModVersionLock

	ForceUpdate bool
}

type ModuleManager struct {
	fs   vfs.FS
	cmdr cmdsite.RunCommand

	Logger logr.Logger

	AbsWorkDir string
	cacheDir   string

	goGetterAbsWorkDir string
	goGetterCacheDir   string

	dep *depresolver.Resolver
}

type Parameter struct {
	Name     string
	Default  interface{}
	Type     string
	Required []string
}

type Option interface {
	SetOption(r *ModuleManager) error
}

func Logger(logger logr.Logger) Option {
	return &loggerOption{l: logger}
}

type loggerOption struct {
	l logr.Logger
}

func (s *loggerOption) SetOption(r *ModuleManager) error {
	r.Logger = s.l
	return nil
}

func FS(fs vfs.FS) Option {
	return &fsOption{f: fs}
}

type fsOption struct {
	f vfs.FS
}

func (s *fsOption) SetOption(r *ModuleManager) error {
	r.fs = s.f
	return nil
}

func Commander(cmdr cmdsite.RunCommand) Option {
	return &cmdrOption{cmdr: cmdr}
}

type cmdrOption struct {
	cmdr cmdsite.RunCommand
}

func (s *cmdrOption) SetOption(r *ModuleManager) error {
	r.cmdr = s.cmdr
	return nil
}

func WD(wd string) Option {
	return &wdOption{d: wd}
}

type wdOption struct {
	d string
}

func (s *wdOption) SetOption(r *ModuleManager) error {
	r.AbsWorkDir = s.d
	return nil
}

func GoGetterWD(goGetterWD string) Option {
	return &goGetterWDOption{d: goGetterWD}
}

type goGetterWDOption struct {
	d string
}

func (s *goGetterWDOption) SetOption(r *ModuleManager) error {
	r.goGetterAbsWorkDir = s.d
	return nil
}

func New(opts ...Option) (*ModuleManager, error) {
	mod := &ModuleManager{}

	for _, o := range opts {
		if err := o.SetOption(mod); err != nil {
			return nil, err
		}
	}

	if mod.Logger == nil {
		mod.Logger = klogr.New()
	}

	if mod.fs == nil {
		mod.fs = vfs.HostOSFS
	}

	if mod.AbsWorkDir == "" {
		path, err := os.Getwd()
		if err != nil {
			return nil, err
		}

		abs, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		mod.AbsWorkDir = abs
	}

	if mod.goGetterAbsWorkDir == "" {
		mod.goGetterAbsWorkDir = mod.AbsWorkDir
	}

	if mod.cacheDir == "" {
		mod.cacheDir = ".variant/mod/cache"
	}

	if mod.goGetterCacheDir == "" {
		mod.goGetterCacheDir = mod.cacheDir
	}

	abs := filepath.IsAbs(mod.cacheDir)
	if !abs {
		mod.cacheDir = filepath.Join(mod.AbsWorkDir, mod.cacheDir)
	}

	abs = filepath.IsAbs(mod.goGetterCacheDir)
	if !abs {
		mod.goGetterCacheDir = filepath.Join(mod.goGetterAbsWorkDir, mod.goGetterCacheDir)
	}

	mod.Logger.V(1).Info("init", "workdir", mod.AbsWorkDir, "cachedir", mod.cacheDir)

	dep, err := depresolver.New(
		depresolver.Home(mod.cacheDir),
		depresolver.GoGetterHome(mod.goGetterCacheDir),
		depresolver.Logger(mod.Logger),
	)
	if err != nil {
		return nil, err
	}
	mod.dep = dep

	return mod, nil
}

func (m *ModuleManager) LoadLockFile() (*ModVersionLock, error) {
	bytes, err := m.fs.ReadFile(filepath.Join(m.AbsWorkDir, "variant.lock"))
	if err != nil {
		m.Logger.V(2).Info("load.readfile", "err", err.Error())
		if !strings.HasSuffix(err.Error(), "no such file or directory") {
			return nil, err
		}
	}

	lockContents := ModVersionLock{Dependencies: map[string]DepVersionLock{}}
	if bytes != nil {
		m.Logger.V(2).Info("load.yaml.unmarshal.begin", "bytes", string(bytes))
		if err := yaml.Unmarshal(bytes, &lockContents); err != nil {
			return nil, err
		}

		m.Logger.V(2).Info("load.yaml.unmarshal.end", "data", lockContents)
	}

	return &lockContents, nil
}

func (m *ModuleManager) Load() (*Module, error) {
	lockContents, err := m.LoadLockFile()
	if err != nil {
		return nil, err
	}

	spec := DependencySpec{
		Source:         filepath.Join(m.AbsWorkDir, "variant.mod"),
		Arguments:      map[string]interface{}{},
		LockedVersions: *lockContents,
	}

	m.Logger.V(2).Info("load.begin", "spec", spec)

	mod, err := m.load(spec)
	if err != nil {
		return nil, err
	}

	m.Logger.V(2).Info("load.end", "mod", fmt.Sprintf("%+v", mod))

	return mod, nil
}

func (m *ModuleManager) load(depspec DependencySpec) (mod *Module, err error) {
	defer func() {
		if err != nil {
			m.Logger.Error(err, "load", "depspec", depspec)
		}
	}()

	resolved, err := m.dep.Resolve(depspec.Source)
	if err != nil {
		return nil, err
	}

	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(m.AbsWorkDir, resolved)
	}

	if filepath.Base(resolved) != "variant.mod" {
		resolved = filepath.Join(resolved, "variant.mod")
	}

	bytes, err := m.fs.ReadFile(resolved)
	if err != nil {
		m.Logger.Error(err, "read file", "resolved", resolved, "depspec", depspec)
		var err2 error
		bytes, err2 = m.fs.ReadFile(resolved)
		if err2 != nil {
			return nil, err2
		}
	}

	spec := &ModuleSpec{
		Name: "variant",
		Parameters: ParametersSpec{
			Schema:   map[string]interface{}{},
			Defaults: map[string]interface{}{},
		},
		Releases: map[string]releasetracker.Spec{},
	}
	if err := yaml.Unmarshal(bytes, spec); err != nil {
		return nil, err
	}
	m.Logger.V(2).Info("load", "alias", depspec.Alias, "module", spec, "dep", depspec)

	defaults, err := maputil.CastKeysToStrings(spec.Parameters.Defaults)
	if err != nil {
		return nil, err
	}

	vals := mergeByOverwrite(Values{}, defaults, depspec.Arguments, depspec.LockedVersions.ToMap())

	trackers := map[string]*releasetracker.Tracker{}

	for n, dep := range spec.Dependencies {
		if dep.ReleasesFrom.IsDefined() {
			_, conflicted := spec.Releases[n]
			if conflicted {
				return nil, fmt.Errorf("conflicting dependency %q", n)
			}
			spec.Releases[n] = releasetracker.Spec{VersionsFrom: dep.ReleasesFrom}
		}
	}

	for alias, dep := range spec.Releases {
		var rc *releasetracker.Tracker
		rc, err = releasetracker.New(
			dep,
			releasetracker.WD(m.AbsWorkDir),
			releasetracker.GoGetterWD(m.goGetterAbsWorkDir),
			releasetracker.FS(m.fs),
			releasetracker.Commander(m.cmdr),
		)
		if err != nil {
			return nil, err
		}

		rc.Spec.VersionsFrom.JSONPath.Source, err = tmpl.Render("releaseChannel.source", rc.Spec.VersionsFrom.JSONPath.Source, vals)
		if err != nil {
			return nil, err
		}

		rc.Spec.VersionsFrom.GitTags.Source, err = tmpl.Render("releaseChannel.source", rc.Spec.VersionsFrom.GitTags.Source, vals)
		if err != nil {
			return nil, err
		}

		rc.Spec.VersionsFrom.GitHubReleases.Source, err = tmpl.Render("releaseChannel.source", rc.Spec.VersionsFrom.GitHubReleases.Source, vals)
		if err != nil {
			return nil, err
		}

		rc.Spec.VersionsFrom.DockerImageTags.Source, err = tmpl.Render("releaseChannel.source", rc.Spec.VersionsFrom.DockerImageTags.Source, vals)
		if err != nil {
			return nil, err
		}

		trackers[alias] = rc
	}

	verLock := depspec.LockedVersions

	submods := map[string]*Module{}

	// Resolve versions of dependencies
	for alias, dep := range spec.Dependencies {
		preUp, ok := verLock.Dependencies[alias]
		if ok {
			if depspec.ForceUpdate {
				m.Logger.V(2).Info("finding tracker", "alias", alias, "trackers", trackers)
				tracker, ok := trackers[alias]
				if ok {
					m.Logger.V(2).Info("tracker found", "alias", alias)
					rel, err := tracker.Latest(dep.VersionConstraint)
					if err != nil {
						return nil, err
					}

					if preUp.Version == rel.Version {
						m.Logger.V(2).Info("No update found", "alias", alias)
						continue
					}

					prev := verLock.Dependencies[alias].Version
					vals[alias] = Values{"version": rel.Version, "previousVersion": prev}

					verLock.Dependencies[alias] = DepVersionLock{
						Version:         rel.Version,
						PreviousVersion: prev,
					}
				} else {
					m.Logger.V(2).Info("no tracker found", "alias", alias)
				}
			} else {
				m.Logger.V(2).Info("tracker unused. lock version exists", "alias", alias, "verLock", verLock)
			}
		} else {
			m.Logger.V(2).Info("finding tracker", "alias", alias, "trackers", trackers)
			tracker, ok := trackers[alias]
			if ok {
				m.Logger.V(2).Info("tracker found", "alias", alias)
				rel, err := tracker.Latest(dep.VersionConstraint)
				if err != nil {
					return nil, err
				}
				vals[alias] = Values{"version": rel.Version}

				verLock.Dependencies[alias] = DepVersionLock{Version: rel.Version}
			} else {
				m.Logger.V(2).Info("no tracker found", "alias", alias)
			}
		}
	}

	// Regenerate template parameters from the up-to-date versions of dependencies
	vals = mergeByOverwrite(Values{}, defaults, depspec.Arguments, verLock.ToDepsMap(), verLock.ToMap())

	// Load sub-modules
	for alias, dep := range spec.Dependencies {
		if dep.Kind != "Module" {
			continue
		}

		dep.Alias = alias

		if dep.LockedVersions.Dependencies == nil {
			dep.LockedVersions.Dependencies = map[string]DepVersionLock{}
		}

		args, err := maputil.CastKeysToStrings(dep.Arguments)
		if err != nil {
			return nil, err
		}
		dep.Arguments, err = tmpl.RenderArgs(args, vals)
		if err != nil {
			m.Logger.V(2).Info("renderargs failed with values", "vals", vals)
			return nil, err
		}
		m.Logger.V(2).Info("loading dependency", "alias", alias, "dep", dep)
		submod, err := m.load(dep)
		if err != nil {
			return nil, err
		}
		submods[alias] = submod

		vals = mergeByOverwrite(Values{}, vals, map[string]interface{}{alias: submod.Values})
		//vals[alias] = submod.Values

		m.Logger.V(1).Info("loaded dependency", "alias", alias, "vals", vals)
	}

	files := []File{}
	for path, fspec := range spec.Provisioners.Files {
		f := File{
			Path:      path,
			Source:    fspec.Source,
			Arguments: fspec.Arguments,
		}
		files = append(files, f)
	}

	textReplaces := []TextReplace{}
	for path, tspec := range spec.Provisioners.TextReplace {
		t := TextReplace{
			Path: path,
			From: tspec.From,
			To:   tspec.To,
		}
		textReplaces = append(textReplaces, t)
	}

	yamls := []YamlPatch{}
	for path, yspec := range spec.Provisioners.YamlPatch {
		patches := []Patch{}
		for _, v := range yspec {
			p := Patch{
				Op: v.Op,
				Path: v.Path,
				Value: v.Value,
				From: v.From,
			}
			patches = append(patches, p)
		}
		y := YamlPatch{
			Path: path,
			Patches: patches,
		}
		yamls = append(yamls, y)
	}

	spec.Parameters.Schema["type"] = "object"

	execset, err := execversionmanager.New(
		&spec.Provisioners.Executables,
		execversionmanager.Values(vals),
		execversionmanager.WD(m.AbsWorkDir),
		execversionmanager.GoGetterWD(m.goGetterAbsWorkDir),
		execversionmanager.FS(m.fs),
	)
	if err != nil {
		return nil, err
	}

	schema, err := maputil.CastKeysToStrings(spec.Parameters.Schema)
	if err != nil {
		return nil, err
	}

	//if vals[depspec.Alias] != nil {
	//	vs, ok := vals[depspec.Alias].(Values)
	//	if ok {
	//		v, set := vs["version"]
	//		if set {
	//			vals["version"] = v
	//		}
	//	}
	//}
	//
	mod = &Module{
		Alias:           spec.Name,
		Values:          vals,
		ValuesSchema:    schema,
		Files:           files,
		TextReplaces:    textReplaces,
		Yamls:           yamls,
		Executable:      execset,
		Submodules:      submods,
		ReleaseTrackers: trackers,
		VersionLock:     verLock,
	}

	return mod, nil
}

type BuildResult struct {
	Files []string
}

func (m *ModuleManager) Build() (*BuildResult, error) {
	mod, err := m.Load()
	if err != nil {
		return nil, err
	}

	return m.doBuild(mod)
}

func (m *ModuleManager) Up() error {
	mod, err := m.doUp()
	if err != nil {
		return err
	}

	return m.lock(mod)
}

func (m *ModuleManager) Checkout(branch string) error {
	g := gitops.New(
		gitops.WD(m.AbsWorkDir),
		gitops.Commander(m.cmdr),
	)
	return g.Checkout(branch)
}

func (m *ModuleManager) Push(files []string, branch string) (bool, error) {
	g := gitops.New(
		gitops.WD(m.AbsWorkDir),
		gitops.Commander(m.cmdr),
	)
	if err := g.Add(files...); err != nil {
		return false, err
	}
	diffExists := g.DiffExists()
	if diffExists {
		if err := g.Commit("Automated update"); err != nil {
			return false, err
		}
		if err := g.Push(branch); err != nil {
			return false, err
		}
		return true, nil
	}
	// No changes
	return false, nil
}

func (m *ModuleManager) PullRequest(title, base, head string) error {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: os.Getenv("GITHUB_TOKEN")},
	)
	tc := oauth2.NewClient(ctx, ts)

	gc := github.NewClient(tc)

	body := ``
	newPr := github.NewPullRequest{
		Title: &title,
		Head:  &head,
		Base:  &base,
		Body:  &body,
	}

	g := gitops.New(
		gitops.WD(m.AbsWorkDir),
		gitops.Commander(m.cmdr),
	)
	push, err := g.GetPushURL("origin")
	if err != nil {
		return err
	}

	p := strings.TrimSpace(push)
	p = strings.TrimSuffix(p, ".git")
	p = strings.TrimPrefix(p, "git@github.com:")
	p = strings.TrimPrefix(p, "https://github.com/")
	ownerRepo := strings.Split(p, "/")
	if len(ownerRepo) != 2 {
		return fmt.Errorf("unexpected format of remote: %s", push)
	}
	owner := ownerRepo[0]
	repo := ownerRepo[1]

	pr, res, err := gc.PullRequests.Create(ctx, owner, repo, &newPr)
	if err != nil {
		klog.V(1).Infof("create pull request: %v", res)
		return fmt.Errorf("create pull request: %v", err)
	}

	klog.V(2).Infof("pull request created: %+v", pr)

	return nil
}

func (m *ModuleManager) Create(templateRepo, newRepo string, public bool) error {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: os.Getenv("GITHUB_TOKEN")},
	)
	tc := oauth2.NewClient(ctx, ts)

	gc := github.NewClient(tc)

	g := gitops.New(
		gitops.WD(m.AbsWorkDir),
		gitops.Commander(m.cmdr),
	)

	t := strings.TrimSuffix(templateRepo, ".git")
	t = strings.TrimPrefix(t, "git@github.com:")
	ownerRepo := strings.Split(t, "/")
	if len(ownerRepo) != 2 {
		return fmt.Errorf("unexpected format of template repo: %s", templateRepo)
	}
	tOwner := ownerRepo[0]
	tRepo := ownerRepo[1]

	n := strings.TrimSuffix(newRepo, ".git")
	n = strings.TrimPrefix(n, "git@github.com:")
	nOwnerRepo := strings.Split(n, "/")
	if len(nOwnerRepo) != 2 {
		return fmt.Errorf("unexpected format of template repo: %s", templateRepo)
	}
	nOwner := nOwnerRepo[0]
	nRepo := nOwnerRepo[1]

	private := !public

	req := github.TemplateRepoRequest{
		Name:    &nRepo,
		Owner:   &nOwner,
		Private: &private,
	}

	createdRepo, res, err := gc.Repositories.CreateFromTemplate(ctx, tOwner, tRepo, &req)
	if err != nil {
		klog.V(1).Infof("create repository from template: %v", res)
		return fmt.Errorf("create repository from template: %v", err)
	}

	klog.V(2).Infof("repository created: %+v", createdRepo)

	if err := g.Clone("git@github.com:" + newRepo); err != nil {
		return err
	}

	if err := os.Chdir(nRepo); err != nil {
		return err
	}

	return nil
}

func (m *ModuleManager) doUp() (*Module, error) {
	lockContents, err := m.LoadLockFile()
	if err != nil {
		return nil, err
	}

	m.Logger.V(2).Info("running up")
	spec := DependencySpec{
		Source:    filepath.Join(m.AbsWorkDir, "variant.mod"),
		Arguments: map[string]interface{}{},
		//LockedVersions: ModVersionLock{Dependencies: map[string]DepVersionLock{}},
		LockedVersions: *lockContents,
		ForceUpdate:    true,
	}

	mod, err := m.load(spec)
	if err != nil {
		return nil, err
	}

	fmt.Fprintf(os.Stderr, "up: %+v\n", mod)
	return mod, nil
}

func (m *ModuleManager) lock(mod *Module) error {
	buf := &bytes.Buffer{}
	enc := yaml.NewEncoder(buf)
	enc.SetIndent(2)
	err := enc.Encode(mod.VersionLock)
	if err != nil {
		return err
	}
	bytes := buf.Bytes()

	writeTo := filepath.Join(m.AbsWorkDir, "variant.lock")

	m.Logger.V(2).Info("lock.write", "path", writeTo, "data", string(bytes))

	return m.fs.WriteFile(writeTo, bytes, 0644)
}

func (m *ModuleManager) breadthFirstWalk(cur *Module, path []string, vals *Values, f func([]string, *Values, *Module) error) error {
	if cur.Submodules != nil {
		for i := range cur.Submodules {
			dep := cur.Submodules[i]
			if err := f(append(append([]string{}, path...), dep.Alias), vals, dep); err != nil {
				return err
			}
		}
		for i := range cur.Submodules {
			dep := cur.Submodules[i]
			if err := m.breadthFirstWalk(dep, append(append([]string{}, path...), dep.Alias), vals, f); err != nil {
				return err
			}
		}
	}
	return nil
}

func mergeByOverwrite(src ...Values) (res Values) {
	res = Values{}
	defer func() {
		if res != nil {
			klog.V(0).Infof("mergeByOverwrite: src=%v, res=%v", src, res)
		}
	}()
	for _, s := range src {
		for k, v := range s {
			klog.V(0).Infof("mergeByOverwrite: k=%v, v=%v(%T)", k, v, v)
			switch typedV := v.(type) {
			case map[string]interface{}, Values:
				_, ok := res[k]
				if ok {
					switch typedDestV := res[k].(type) {
					case map[string]interface{}:
						klog.V(0).Infof("mergeByOverwrite: map[string]interface{}: %v", typedDestV)
						res[k] = mergeByOverwrite(typedDestV, typedV.(Values))
					case Values:
						klog.V(0).Infof("mergeByOverwrite: Values: %v", typedDestV)
						res[k] = mergeByOverwrite(typedDestV, typedV.(Values))
					default:
						klog.V(0).Infof("mergeByOverwrite: default: %v", typedDestV)
						res[k] = typedDestV
					}
				} else {
					res[k] = typedV
				}
			default:
				res[k] = v
			}
		}
	}
	return res
}

func (m *ModuleManager) doBuild(mod *Module) (*BuildResult, error) {
	r := BuildResult{}
	err := mod.Walk(func(dep *Module) error {
		rr, err := m.doBuildSingle(dep)
		if err != nil {
			return err
		}

		r.Files = append(r.Files, rr.Files...)

		return nil
	})
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (m *ModuleManager) doBuildSingle(mod *Module) (r *BuildResult, err error) {
	defer func() {
		if err != nil {
			m.Logger.V(0).Info("doBuild", "error", err.Error())
		}
	}()

	r = &BuildResult{
		Files: []string{},
	}

	schemaLoader := gojsonschema.NewGoLoader(mod.ValuesSchema)
	values := mergeByOverwrite(Values{}, mod.Values)
	jsonLoader := gojsonschema.NewGoLoader(values)
	result, err := gojsonschema.Validate(schemaLoader, jsonLoader)
	if err != nil {
		return nil, fmt.Errorf("validate: %v", err)
	}
	for i, err := range result.Errors() {
		m.Logger.V(1).Info("err", "index", i, "err", err.String())
	}

	for _, f := range mod.Files {
		u, err := tmpl.Render("source", f.Source, values)
		if err != nil {
			m.Logger.V(1).Info(err.Error())
			return nil, err
		}

		yours, err := m.dep.Resolve(u)
		if err != nil {
			m.Logger.V(1).Info(err.Error())
			return nil, err
		}

		var target string

		mine := filepath.Join(m.AbsWorkDir, filepath.Base(yours))
		m.Logger.V(1).Info("resolved", "input", u, "modulefile", yours, "localfile", mine)
		contents, err := m.fs.ReadFile(mine)
		if err != nil {
			contents, err = m.fs.ReadFile(yours)
			if err != nil {
				m.Logger.V(1).Info(err.Error())
				return nil, err
			}
			target = yours
		} else {
			target = mine
		}

		ext := filepath.Ext(target)
		if ext == ".tpl" || ext == ".tmpl" || ext == ".gotmpl" {
			args, err := tmpl.RenderArgs(f.Arguments, values)
			if err != nil {
				m.Logger.V(1).Info(err.Error())
				return nil, err
			}
			res, err := tmpl.Render("source file", string(contents), args)
			if err != nil {
				m.Logger.V(1).Info(err.Error())
				return nil, err
			}
			contents = []byte(res)
		}

		if err := m.fs.WriteFile(filepath.Join(m.AbsWorkDir, f.Path), contents, 0644); err != nil {
			m.Logger.V(1).Info(err.Error())
			return nil, err
		}

		r.Files = append(r.Files, f.Path)
	}

	for _, t := range mod.TextReplaces {
		from, err := tmpl.Render("from", t.From, values)
		if err != nil {
			m.Logger.V(1).Info(err.Error())
			return nil, err
		}

		from = strings.TrimSpace(from)

		if from == "" {
			continue
		}

		to, err := tmpl.Render("to", t.To, values)
		if err != nil {
			m.Logger.V(1).Info(err.Error())
			return nil, err
		}

		to = strings.TrimSpace(to)

		path, err := tmpl.Render("path", t.Path, values)
		if err != nil {
			m.Logger.V(1).Info(err.Error())
			return nil, err
		}

		target := filepath.Join(m.AbsWorkDir, path)
		m.Logger.V(1).Info("textReplace", "path", target, "from", from, "to", to)
		contents, err := m.fs.ReadFile(target)
		if err != nil {
			m.Logger.V(1).Info(err.Error())
			return nil, err
		}

		str := strings.ReplaceAll(string(contents), from, to)

		if err := m.fs.WriteFile(target, []byte(str), 0644); err != nil {
			m.Logger.V(1).Info(err.Error())
			return nil, err
		}

		r.Files = append(r.Files, t.Path)
	}

	for _, y := range mod.Yamls {
		path, err := tmpl.Render("path", y.Path, values)
		if err != nil {
			m.Logger.V(1).Info(err.Error())
			return nil, err
		}

		target := filepath.Join(m.AbsWorkDir, path)
		contents, err := m.fs.ReadFile(target)
		if err != nil {
			m.Logger.V(1).Info(err.Error())
			return nil, err
		}

		out, err := json.Marshal(y.Patches)
		if err != nil {
			m.Logger.V(1).Info(err.Error())
			return nil, err
		}
		patchJSON, err := tmpl.Render("patches", string(out), values)
		if err != nil {
			m.Logger.V(1).Info(err.Error())
			return nil, err
		}

		var yml yaml.Node
		if err := yaml.Unmarshal(contents, &yml); err != nil {
			m.Logger.V(1).Info(err.Error())
			return nil, err
		}

		var v interface{}
		if err := yml.Decode(&v); err != nil {
			m.Logger.V(1).Info(err.Error())
			return nil, err
		}
		yamlpatch.Patch(&yml, patchJSON)

		s, err := yaml.Marshal(yml.Content[0])
		if err != nil {
			m.Logger.V(1).Info(err.Error())
			return nil, err
		}

		if err := m.fs.WriteFile(target, s, 0644); err != nil {
			m.Logger.V(1).Info(err.Error())
			return nil, err
		}
		r.Files = append(r.Files, y.Path)
	}

	return r, nil
}
