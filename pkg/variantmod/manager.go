package variantmod

import (
	"fmt"
	"github.com/go-logr/logr"
	"github.com/twpayne/go-vfs"
	"github.com/variantdev/mod/pkg/depresolver"
	"github.com/variantdev/mod/pkg/execversionmanager"
	"github.com/variantdev/mod/pkg/maputil"
	"github.com/variantdev/mod/pkg/releasechannel"
	"github.com/variantdev/mod/pkg/tmpl"
	"github.com/xeipuuv/gojsonschema"
	"gopkg.in/yaml.v3"
	"k8s.io/klog"
	"k8s.io/klog/klogr"
	"os"
	"path/filepath"
	"strings"
)

type ModuleSpec struct {
	Name string `yaml:"name"`

	Values map[string]interface{} `yaml:"values"`
	Schema map[string]interface{} `yaml:"schema"`
	Files  map[string]FileSpec    `yaml:"files"`

	Dependencies   map[string]DependencySpec `yaml:"dependencies"`
	ReleaseChannel releasechannel.Config     `yaml:",inline"`
	Executable     execversionmanager.Config `yaml:",inline"`
}

type FileSpec struct {
	Source    string                 `yaml:"source"`
	Arguments map[string]interface{} `yaml:"arguments"`
}

type DependencySpec struct {
	Source         string                 `yaml:"source"`
	Arguments      map[string]interface{} `yaml:"arguments"`
	ReleaseChannel string                 `yaml:"releaseChannel"`
	// ModuleVersionRange is the version range for this dependency. Works only for modules hosted on Git or GitHub
	ModuleVersionRange string `yaml:"version"`
	Alias              string

	VersionLock map[string]interface{}
}

type ModuleManager struct {
	fs vfs.FS

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

func (m *ModuleManager) Load() (*Module, error) {
	bytes, err := m.fs.ReadFile(filepath.Join(m.AbsWorkDir, "variant.lock"))
	if err != nil {
		m.Logger.V(2).Info("load.readfile", "err", err.Error())
		if !strings.HasSuffix(err.Error(), "no such file or directory") {
			return nil, err
		}
	}

	versionLock := Values{}
	if bytes != nil {
		m.Logger.V(2).Info("load.yaml.unmarshal.begin", "bytes", string(bytes))
		if err := yaml.Unmarshal(bytes, &versionLock); err != nil {
			return nil, err
		}

		m.Logger.V(2).Info("load.yaml.unmarshal.end", "data", versionLock)

		versionLock, err = maputil.CastKeysToStrings(map[string]interface{}(versionLock))
		if err != nil {
			return nil, err
		}

		m.Logger.V(2).Info("load.yaml.castkeys", "data", versionLock)
	}

	spec := DependencySpec{
		Source:         filepath.Join(m.AbsWorkDir, "variant.mod"),
		Arguments:      map[string]interface{}{},
		ReleaseChannel: "stable",
		VersionLock:    versionLock,
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
		Name:   "variant",
		Schema: map[string]interface{}{},
		Values: map[string]interface{}{},
	}
	if err := yaml.Unmarshal(bytes, spec); err != nil {
		return nil, err
	}
	m.Logger.V(2).Info("load", "alias", depspec.Alias, "module", spec, "dep", depspec)

	var vals map[string]interface{}
	vals, err = maputil.CastKeysToStrings(spec.Values)
	if err != nil {
		return nil, err
	}

	vals = mergeByOverwrite(Values{}, vals, depspec.Arguments, depspec.VersionLock)

	var rel *releasechannel.Provider
	if len(spec.ReleaseChannel.ReleaseChannels) > 0 && depspec.VersionLock["version"] == nil {

		rel, err = releasechannel.New(
			&spec.ReleaseChannel,
			"stable",
			releasechannel.WD(m.AbsWorkDir),
			releasechannel.GoGetterWD(m.goGetterAbsWorkDir),
			releasechannel.FS(m.fs),
		)
		if err != nil {
			return nil, err
		}

		alias := depspec.Alias

		rel.Spec.VersionsFrom.JSONPath.Source, err = tmpl.Render("releaseChannel.source", rel.Spec.VersionsFrom.JSONPath.Source, vals)
		if err != nil {
			return nil, err
		}

		rel.Spec.VersionsFrom.GitTags.Source, err = tmpl.Render("releaseChannel.source", rel.Spec.VersionsFrom.GitTags.Source, vals)
		if err != nil {
			return nil, err
		}

		rel.Spec.VersionsFrom.GitHubReleases.Source, err = tmpl.Render("releaseChannel.source", rel.Spec.VersionsFrom.GitHubReleases.Source, vals)
		if err != nil {
			return nil, err
		}

		rel.Spec.VersionsFrom.DockerImageTags.Source, err = tmpl.Render("releaseChannel.source", rel.Spec.VersionsFrom.DockerImageTags.Source, vals)
		if err != nil {
			return nil, err
		}

		rel, err := rel.Latest()
		if err != nil {
			return nil, err
		}
		mm := Values{}
		m, ok := vals[alias]
		if ok && m != nil {
			mm = m.(Values)
		}
		mm["version"] = rel.Version
		vals[alias] = mm
	}

	var verLock Values
	if depspec.VersionLock != nil {
		verLock = depspec.VersionLock
	}
	if vals[depspec.Alias] != nil {
		switch t := vals[depspec.Alias].(type) {
		case map[string]interface{}:
			verLock = t
		case Values:
			verLock = map[string]interface{}(t)
		default:
			return nil, fmt.Errorf("unexpected type: value=%v, type=%T", t, t)
		}
	}

	deps := map[string]*Module{}
	depchs := map[string]*releasechannel.Provider{}
	for name, dspec := range spec.Dependencies {
		dspec.Alias = name
		if lock, ok := depspec.VersionLock[name]; ok {
			switch t := lock.(type) {
			case map[string]interface{}:
				dspec.VersionLock = t
			case Values:
				dspec.VersionLock = map[string]interface{}(t)
			default:
				return nil, fmt.Errorf("unexpected error: value=%v, type=%T", t, t)
			}
		}
		args, err := maputil.CastKeysToStrings(dspec.Arguments)
		if err != nil {
			return nil, err
		}
		dspec.Arguments, err = tmpl.RenderArgs(args, vals)
		if err != nil {
			return nil, err
		}
		depmod, err := m.load(dspec)
		if err != nil {
			return nil, err
		}
		deps[name] = depmod

		vals[dspec.Alias] = depmod.Values

		// release channel
		if !strings.Contains(dspec.Source, "github.com") {
			continue
		}

		src, err := depresolver.Parse(dspec.Source)
		if err != nil {
			return nil, err
		}

		orgRepo := strings.Split(src.Dir, "/")[:2]
		rel, err = releasechannel.New(
			&releasechannel.Config{
				ReleaseChannels: map[string]releasechannel.Spec{
					"stable": {
						VersionsFrom: releasechannel.VersionsFrom{
							GitHubReleases: releasechannel.GitHubReleases{
								Source: strings.Join(orgRepo, "/"),
							},
						},
					},
				},
			},
			"stable",
			releasechannel.WD(m.AbsWorkDir),
			releasechannel.GoGetterWD(m.goGetterAbsWorkDir),
			releasechannel.FS(m.fs),
		)
		if err != nil {
			return nil, err
		}
		depchs[name] = rel
	}

	files := []File{}
	for path, fspec := range spec.Files {
		f := File{
			Path:      path,
			Source:    fspec.Source,
			Arguments: fspec.Arguments,
		}
		files = append(files, f)
	}

	spec.Schema["type"] = "object"

	execset, err := execversionmanager.New(
		&spec.Executable,
		execversionmanager.Values(vals),
		execversionmanager.WD(m.AbsWorkDir),
		execversionmanager.GoGetterWD(m.goGetterAbsWorkDir),
		execversionmanager.FS(m.fs),
	)
	if err != nil {
		return nil, err
	}

	schema, err := maputil.CastKeysToStrings(spec.Schema)
	if err != nil {
		return nil, err
	}

	if vals[depspec.Alias] != nil {
		vs, ok := vals[depspec.Alias].(Values)
		if ok {
			v, set := vs["version"]
			if set {
				vals["version"] = v
			}
		}
	}

	mod = &Module{
		Alias:                     spec.Name,
		Values:                    vals,
		ValuesSchema:              schema,
		Files:                     files,
		Executable:                execset,
		ReleaseChannel:            rel,
		Dependencies:              deps,
		DependencyReleaseChannels: depchs,
		VersionLock:               verLock,
	}

	return mod, nil
}

func (m *ModuleManager) Run() error {
	mod, err := m.Load()
	if err != nil {
		return err
	}

	return m.run(mod)
}

func (m *ModuleManager) Up() (*Module, error) {
	spec := DependencySpec{
		Source:         filepath.Join(m.AbsWorkDir, "variant.mod"),
		Arguments:      map[string]interface{}{},
		ReleaseChannel: "stable",
		VersionLock:    map[string]interface{}{},
	}

	mod, err := m.load(spec)
	if err != nil {
		return nil, err
	}

	fmt.Fprintf(os.Stderr, "%+v\n", mod)
	return mod, nil
}

func (m *ModuleManager) lock(mod *Module) error {
	vals := Values{}
	if err := m.breadthFirstWalk(mod, []string{}, &vals, func(path []string, ctx *Values, dep *Module) error {
		m.Logger.V(2).Info("lock", "path", path, "ctx", ctx, "lock", dep.VersionLock)
		if dep.VersionLock != nil {
			maputil.Set(*ctx, path, dep.VersionLock)
		}
		return nil
	}); err != nil {
		return err
	}

	bytes, err := yaml.Marshal(vals)
	if err != nil {
		return err
	}

	writeTo := filepath.Join(m.AbsWorkDir, "variant.lock")

	m.Logger.V(2).Info("lock.write", "path", writeTo, "data", string(bytes))

	return m.fs.WriteFile(writeTo, bytes, 0644)
}

func (m *ModuleManager) breadthFirstWalk(cur *Module, path []string, vals *Values, f func([]string, *Values, *Module) error) error {
	if cur.Dependencies != nil {
		for i := range cur.Dependencies {
			dep := cur.Dependencies[i]
			if err := f(append(append([]string{}, path...), dep.Alias), vals, dep); err != nil {
				return err
			}
		}
		for i := range cur.Dependencies {
			dep := cur.Dependencies[i]
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
			switch t := v.(type) {
			case map[string]interface{}:
				switch tt := res[k].(type) {
				case map[string]interface{}:
					res[k] = mergeByOverwrite(tt, t)
				default:
					res[k] = tt
				}
			default:
				res[k] = v
			}
		}
	}
	return res
}

func (m *ModuleManager) run(mod *Module) error {
	return mod.Walk(func(dep *Module) error {
		return m.runSingle(dep)
	})
}

func (m *ModuleManager) runSingle(mod *Module) (err error) {
	defer func() {
		if err != nil {
			m.Logger.V(0).Info("run", "error", err.Error())
		}
	}()

	schemaLoader := gojsonschema.NewGoLoader(mod.ValuesSchema)
	values := mergeByOverwrite(Values{}, mod.Values)
	jsonLoader := gojsonschema.NewGoLoader(values)
	result, err := gojsonschema.Validate(schemaLoader, jsonLoader)
	if err != nil {
		return fmt.Errorf("validate: %v", err)
	}
	for i, err := range result.Errors() {
		m.Logger.V(1).Info("err", "index", i, "err", err.String())
	}

	for _, f := range mod.Files {
		u, err := tmpl.Render("source", f.Source, values)
		if err != nil {
			m.Logger.V(1).Info(err.Error())
			return err
		}

		yours, err := m.dep.Resolve(u)
		if err != nil {
			m.Logger.V(1).Info(err.Error())
			return err
		}

		var target string

		mine := filepath.Join(m.AbsWorkDir, filepath.Base(yours))
		m.Logger.V(1).Info("resolved", "input", u, "modulefile", yours, "localfile", mine)
		contents, err := m.fs.ReadFile(mine)
		if err != nil {
			contents, err = m.fs.ReadFile(yours)
			if err != nil {
				m.Logger.V(1).Info(err.Error())
				return err
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
				return err
			}
			res, err := tmpl.Render("source file", string(contents), args)
			if err != nil {
				m.Logger.V(1).Info(err.Error())
				return err
			}
			contents = []byte(res)
		}

		if err := m.fs.WriteFile(filepath.Join(m.AbsWorkDir, f.Path), contents, 0644); err != nil {
			m.Logger.V(1).Info(err.Error())
			return err
		}
	}

	return nil
}
