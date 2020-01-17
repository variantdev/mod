package variantmod

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/go-logr/logr"
	"github.com/twpayne/go-vfs"
	"github.com/variantdev/mod/pkg/cmdsite"
	"github.com/variantdev/mod/pkg/config/confapi"
	"github.com/variantdev/mod/pkg/depresolver"
	"github.com/variantdev/mod/pkg/execversionmanager"
	"github.com/variantdev/mod/pkg/gitops"
	"github.com/variantdev/mod/pkg/gitrepo"
	"github.com/variantdev/mod/pkg/releasetracker"
	"github.com/variantdev/mod/pkg/tmpl"
	"github.com/variantdev/mod/pkg/yamlpatch"
	"github.com/xeipuuv/gojsonschema"
	"gopkg.in/yaml.v3"
	"k8s.io/klog"
	"k8s.io/klog/klogr"
)

type ModuleManager struct {
	ModuleFile string
	LockFile   string

	load func(lock confapi.ModVersionLock) (*Module, error)

	fs   vfs.FS
	cmdr cmdsite.RunCommand

	Logger logr.Logger

	AbsWorkDir string
	cacheDir   string

	goGetterAbsWorkDir string
	goGetterCacheDir   string

	dep *depresolver.Resolver
}

const (
	ModuleFileName = "variant.mod"
	LockFileName   = "variant.lock"
)

func New(opts ...Option) (*ModuleManager, error) {
	man := &ModuleManager{
	}

	man, err := initModuleManager(man, opts...)
	if err != nil {
		return nil, err
	}

	man.load = func(lock confapi.ModVersionLock) (*Module, error) {
		spec := confapi.ModuleParams{
			Source:         filepath.Join(man.AbsWorkDir, man.ModuleFile),
			Arguments:      map[string]interface{}{},
			LockedVersions: lock,
		}

		return man.loadModule(spec)
	}

	return man, nil
}

func initModuleManager(mod *ModuleManager, opts ...Option) (*ModuleManager, error) {
	for _, o := range opts {
		if err := o.SetOption(mod); err != nil {
			return nil, err
		}
	}

	if mod.ModuleFile == "" {
		mod.ModuleFile = ModuleFileName
	}

	if mod.LockFile == "" {
		mod.LockFile = LockFileName
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

func (m *ModuleManager) loadLockFile(path string) (*confapi.ModVersionLock, error) {
	bytes, err := m.fs.ReadFile(filepath.Join(m.AbsWorkDir, path))
	if err != nil {
		m.Logger.V(2).Info("load.readfile", "err", err.Error())
		if !strings.HasSuffix(err.Error(), "no such file or directory") {
			return nil, err
		}
	}

	lockContents := confapi.ModVersionLock{Dependencies: map[string]confapi.DepVersionLock{}, RawLock: string(bytes)}
	if bytes != nil {
		m.Logger.V(2).Info("load.yaml.unmarshal.begin", "bytes", string(bytes))
		if err := yaml.Unmarshal(bytes, &lockContents); err != nil {
			return nil, err
		}
		lockContents.RawLock = string(bytes)

		m.Logger.V(2).Info("load.yaml.unmarshal.end", "data", lockContents)
	}

	return &lockContents, nil
}

func (m *ModuleManager) Enabled() bool {
	_, err := m.load(confapi.ModVersionLock{
		Dependencies: map[string]confapi.DepVersionLock{},
		RawLock:      "",
	})
	return err == nil
}

func (m *ModuleManager) ListVersions(depName string, out io.Writer) error {
	mod, err := m.loadLockAndModule()
	if err != nil {
		return err
	}
	return mod.ListVersions(depName, out)
}

func (m *ModuleManager) Shell() (*cmdsite.CommandSite, error) {
	mod, err := m.loadLockAndModule()
	if err != nil {
		return nil, err
	}
	return mod.Shell()
}

func (m *ModuleManager) loadLockAndModule() (*Module, error) {
	lockContents, err := m.loadLockFile(m.LockFile)
	if err != nil {
		return nil, err
	}

	m.Logger.V(2).Info("load.begin")

	mod, err := m.load(*lockContents)
	if err != nil {
		return nil, err
	}

	m.Logger.V(2).Info("load.end", "mod", fmt.Sprintf("%+v", mod))

	return mod, nil
}

func (m *ModuleManager) loadModule(params confapi.ModuleParams) (mod *Module, err error) {
	defer func() {
		if err != nil {
			m.Logger.Error(err, "loadModule", "params", params)
		}
	}()

	matched := strings.HasSuffix(params.Source, ".variantmod")
	var conf *confapi.Module
	if matched {
		conf, err = m.loadHclModule(params)
	} else {
		conf, err = m.loadYamlModule(params)
	}
	if err != nil {
		return nil, err
	}

	return m.initModule(params, *conf)
}

func (m *ModuleManager) initModule(params confapi.ModuleParams, mod confapi.Module) (*Module, error) {
	vals := mergeByOverwrite(Values{}, mod.Defaults, params.Arguments, params.LockedVersions.ToMap())

	verLock := params.LockedVersions

	trackers := map[string]*releasetracker.Tracker{}

	for alias, dep := range mod.Releases {
		var r releasetracker.Spec

		var err error
		r.VersionsFrom.Exec.Args = dep.VersionsFrom.Exec.Args
		r.VersionsFrom.Exec.Command = dep.VersionsFrom.Exec.Command
		if dep.VersionsFrom.DockerImageTags.Source != nil {
			r.VersionsFrom.DockerImageTags.Source, err = dep.VersionsFrom.DockerImageTags.Source(vals)
			if err != nil {
				return nil, err
			}
		}
		if dep.VersionsFrom.GitHubReleases.Source != nil {
			r.VersionsFrom.GitHubReleases.Source, err = dep.VersionsFrom.GitHubReleases.Source(vals)
			if err != nil {
				return nil, err
			}
			r.VersionsFrom.GitHubReleases.Host = dep.VersionsFrom.GitHubReleases.Host
		}
		if dep.VersionsFrom.GitHubTags.Source != nil {
			r.VersionsFrom.GitHubTags.Source, err = dep.VersionsFrom.GitHubTags.Source(vals)
			if err != nil {
				return nil, err
			}
			r.VersionsFrom.GitHubTags.Host = dep.VersionsFrom.GitHubTags.Host
		}
		if dep.VersionsFrom.GitTags.Source != nil {
			r.VersionsFrom.GitTags.Source, err = dep.VersionsFrom.GitTags.Source(vals)
			if err != nil {
				return nil, err
			}
		}
		if dep.VersionsFrom.JSONPath.Source != nil {
			r.VersionsFrom.JSONPath.Source, err = dep.VersionsFrom.JSONPath.Source(vals)
			if err != nil {
				return nil, err
			}
			r.VersionsFrom.JSONPath.Description = dep.VersionsFrom.JSONPath.Description
			r.VersionsFrom.JSONPath.Versions = dep.VersionsFrom.JSONPath.Versions
		}
		var rc *releasetracker.Tracker
		rc, err = releasetracker.New(
			r,
			releasetracker.WD(m.AbsWorkDir),
			releasetracker.GoGetterWD(m.goGetterAbsWorkDir),
			releasetracker.FS(m.fs),
			releasetracker.Commander(m.cmdr),
		)
		if err != nil {
			return nil, err
		}

		trackers[alias] = rc
	}

	submods := map[string]*Module{}

	// Resolve versions of dependencies
	for alias, dep := range mod.Dependencies {
		preUp, ok := verLock.Dependencies[alias]
		if ok {
			if params.ForceUpdate {
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

					verLock.Dependencies[alias] = confapi.DepVersionLock{
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

				verLock.Dependencies[alias] = confapi.DepVersionLock{Version: rel.Version}
			} else {
				m.Logger.V(2).Info("no tracker found", "alias", alias)
			}
		}
	}

	// Regenerate template parameters from the up-to-date versions of dependencies
	vals = mergeByOverwrite(Values{}, mod.Defaults, params.Arguments, verLock.ToDepsMap(), verLock.ToMap())

	// Load sub-modules
	for alias, dep := range mod.Dependencies {
		if dep.Kind != "Module" {
			continue
		}

		dep.Alias = alias

		if dep.LockedVersions.Dependencies == nil {
			dep.LockedVersions.Dependencies = map[string]confapi.DepVersionLock{}
		}

		args, err := dep.Arguments(vals)
		if err != nil {
			m.Logger.V(2).Info("renderargs failed with values", "vals", vals)
			return nil, err
		}
		m.Logger.V(2).Info("loading dependency", "alias", alias, "dep", dep)
		ps := confapi.ModuleParams{
			Source:         dep.Source,
			Arguments:      args,
			Alias:          dep.Alias,
			LockedVersions: dep.LockedVersions,
			ForceUpdate:    dep.ForceUpdate,
		}
		submod, err := m.loadModule(ps)
		if err != nil {
			return nil, err
		}
		submods[alias] = submod

		vals = mergeByOverwrite(Values{}, vals, map[string]interface{}{alias: submod.Values})
		//vals[alias] = submod.Values

		m.Logger.V(1).Info("loaded dependency", "alias", alias, "vals", vals)
	}

	execs := map[string]execversionmanager.Executable{}
	for k, v := range mod.Executables {
		var e execversionmanager.Executable
		for _, p := range v.Platforms {
			src, err := p.Source(vals)
			if err != nil{
				return nil, err
			}
			docker, err := p.Docker(vals)
			if err != nil {
				return nil, err
			}
			e.Platforms = append(e.Platforms, execversionmanager.Platform{
				Source: src,
				Docker: *docker,
				Selector: execversionmanager.Selector{MatchLabels: execversionmanager.MatchLabels{
					OS:   p.Selector.MatchLabels.OS,
					Arch: p.Selector.MatchLabels.Arch,
				}},
			})
		}
		execs[k] = e
	}
	execset, err := execversionmanager.New(
		&execversionmanager.Config{
			Executables: execs,
		},
		execversionmanager.Values(vals),
		execversionmanager.WD(m.AbsWorkDir),
		execversionmanager.GoGetterWD(m.goGetterAbsWorkDir),
		execversionmanager.FS(m.fs),
	)
	if err != nil {
		return nil, err
	}

	return &Module{
		Alias:           mod.Name,
		Values:          vals,
		ValuesSchema:    mod.ValuesSchema,
		Files:           mod.Files,
		RegexpReplaces:  mod.RegexpReplaces,
		TextReplaces:    mod.TextReplaces,
		Yamls:           mod.Yamls,
		Executable:      execset,
		Submodules:      submods,
		ReleaseTrackers: trackers,
		VersionLock:     verLock,
	}, nil
}

type BuildResult struct {
	Files []string
}

func (m *ModuleManager) GetShellIfEnabled() (*cmdsite.CommandSite, error) {
	if m.Enabled() {
		if _, err := m.Build(); err != nil {
			return nil, err
		}

		mod, err := m.loadLockAndModule()
		if err != nil {
			return nil, err
		}

		return mod.Shell()
	}

	return nil, nil
}

func (m *ModuleManager) Build() (*BuildResult, error) {
	mod, err := m.loadLockAndModule()
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
	b, err := g.GetCurrentBranch()
	if err != nil {
		return err
	}
	if branch == b {
		return nil
	}

	has, err := g.HasBranch(branch)
	if err != nil {
		return err
	}

	return g.Checkout(branch, has)
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

func (m *ModuleManager) PullRequest(title, body, base, head string, skipDuplicatePRBody, skipDuplicatePRTitle bool) error {
	mod, err := m.loadLockAndModule()
	if err != nil {
		return err
	}
	ctx := context.Background()
	gc := gitrepo.NewClient(ctx)

	g := gitops.New(
		gitops.WD(m.AbsWorkDir),
		gitops.Commander(m.cmdr),
	)
	r, err := g.Repo()
	if err != nil {
		return err
	}
	ownerRepo := strings.Split(r, "/")
	if len(ownerRepo) != 2 {
		return fmt.Errorf("unexpected format of remote: %s", r)
	}
	owner := ownerRepo[0]
	repo := ownerRepo[1]

	b, err := tmpl.Render("body", body, mod.Values)
	if err != nil {
		return err
	}
	t, err := tmpl.Render("title", title, mod.Values)
	if err != nil {
		return err
	}

	if skipDuplicatePRBody || skipDuplicatePRTitle {
		query := &gitrepo.Query{State: "open"}
		if skipDuplicatePRBody {
			query.Body = b
		}
		if skipDuplicatePRTitle {
			query.Title = t
		}
		issues, err := gc.SearchIssues(ctx, owner, repo, query)
		if err != nil {
			return err
		}
		if len(issues) > 0 {
			klog.V(0).Infof("skipped due to duplicate pull request: #%d", issues[0].GetNumber())
			return nil
		}
	}

	newPr := gitrepo.NewPullRequestOptions{
		Title: t,
		Head:  head,
		Base:  base,
		Body:  b,
	}
	pr, err := gc.NewPullRequest(ctx, owner, repo, &newPr)
	if err != nil {
		return fmt.Errorf("create pull request: %v", err)
	}

	klog.V(2).Infof("pull request created: %+v", pr)

	return nil
}

func (m *ModuleManager) Create(templateRepo, newRepo string, public bool) error {
	ctx := context.Background()

	gc := gitrepo.NewClient(ctx)

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

	opt := &gitrepo.NewRepositoryOption{
		Private:       private,
		TemplateOwner: tOwner,
		TemplateRepo:  tRepo,
	}
	createdRepo, err := gc.NewRepository(ctx, nOwner, nRepo, opt)
	if err != nil {
		return fmt.Errorf("create repository from template: %v", err)
	}

	klog.V(2).Infof("repository created: %+v", createdRepo)

	g := gitops.New(
		gitops.WD(m.AbsWorkDir),
		gitops.Commander(m.cmdr),
	)
	if err := g.Clone("git@github.com:" + newRepo); err != nil {
		return err
	}

	if err := os.Chdir(nRepo); err != nil {
		return err
	}

	return nil
}

func (m *ModuleManager) doUp() (*Module, error) {
	lockContents, err := m.loadLockFile(m.LockFile)
	if err != nil {
		return nil, err
	}

	m.Logger.V(2).Info("running up")
	spec := confapi.ModuleParams{
		Source:    filepath.Join(m.AbsWorkDir, m.ModuleFile),
		Arguments: map[string]interface{}{},
		//LockedVersions: ModVersionLock{Dependencies: map[string]DepVersionLock{}},
		LockedVersions: *lockContents,
		ForceUpdate:    true,
	}

	mod, err := m.loadModule(spec)
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

	writeTo := filepath.Join(m.AbsWorkDir, m.LockFile)

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
		rr, err := m.buildModule(dep)
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

func (m *ModuleManager) buildModule(mod *Module) (r *BuildResult, err error) {
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
		u, err := f.Source(values)
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
			args, err := f.Args(values)
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
		from, err := t.From(values)
		if err != nil {
			m.Logger.V(1).Info(err.Error())
			return nil, err
		}

		from = strings.TrimSpace(from)

		if from == "" {
			continue
		}

		to, err := t.To(values)
		if err != nil {
			m.Logger.V(1).Info(err.Error())
			return nil, err
		}

		to = strings.TrimSpace(to)

		path, err := t.Path(values)
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

		r.Files = append(r.Files, path)
	}

	for _, t := range mod.RegexpReplaces {
		from, err := regexp.Compile(t.From)
		if err != nil {
			m.Logger.V(1).Info(err.Error())
			return nil, err
		}

		to, err := t.To(values)
		if err != nil {
			m.Logger.V(1).Info(err.Error())
			return nil, err
		}

		to = strings.TrimSpace(to)

		path, err := t.Path(values)
		if err != nil {
			m.Logger.V(1).Info(err.Error())
			return nil, err
		}

		target := filepath.Join(m.AbsWorkDir, path)
		m.Logger.V(1).Info("regexpReplace", "path", target, "from", from, "to", to)
		contents, err := m.fs.ReadFile(target)
		if err != nil {
			m.Logger.V(1).Info(err.Error())
			return nil, err
		}

		res, err := regexpReplace(contents, from, to)
		if err != nil {
			m.Logger.V(1).Info(err.Error())
			return nil, err
		}

		if err := m.fs.WriteFile(target, res, 0644); err != nil {
			m.Logger.V(1).Info(err.Error())
			return nil, err
		}

		r.Files = append(r.Files, path)
	}

	for _, y := range mod.Yamls {
		path, err := y.Path(values)
		if err != nil {
			m.Logger.V(1).Info(err.Error())
			return nil, err
		}
		abspath := filepath.Join(m.AbsWorkDir, path)
		origYAML, err := m.fs.ReadFile(abspath)
		if err != nil {
			m.Logger.V(1).Info(err.Error())
			return nil, err
		}

		patchJSON, err := y.Patch(values)
		if err != nil {
			m.Logger.V(1).Info(err.Error())
			return nil, err
		}

		p, err := yamlpatch.New(origYAML)
		if err != nil {
			m.Logger.V(1).Info(err.Error())
			return nil, err
		}
		if err := p.Patch(patchJSON); err != nil {
			m.Logger.V(1).Info(err.Error())
			return nil, err
		}
		modifiedYAML, err := p.Marshal()
		if err != nil {
			m.Logger.V(1).Info(err.Error())
			return nil, err
		}

		if err := m.fs.WriteFile(abspath, modifiedYAML, 0644); err != nil {
			m.Logger.V(1).Info(err.Error())
			return nil, err
		}
		r.Files = append(r.Files, path)
	}

	return r, nil
}
