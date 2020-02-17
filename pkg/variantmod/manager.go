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
	"github.com/variantdev/mod/pkg/gitops"
	"github.com/variantdev/mod/pkg/gitrepo"
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
	Module     *confapi.Module

	load func(lock confapi.ModVersionLock) (*Module, error)

	fs   vfs.FS
	cmdr cmdsite.RunCommand

	Logger logr.Logger

	AbsWorkDir string
	cacheDir   string

	goGetterAbsWorkDir string
	goGetterCacheDir   string

	dep    *depresolver.Resolver
	loader *ModuleLoader
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
		spec := man.newModuleParams(confapi.ModuleParams{
			Source:         filepath.Join(man.AbsWorkDir, man.ModuleFile),
			Arguments:      map[string]interface{}{},
			LockedVersions: lock,
		})

		return man.loader.LoadModule(spec)
	}

	return man, nil
}

func (man *ModuleManager) newModuleParams(params confapi.ModuleParams) confapi.ModuleParams {
	spec := params
	if man.Module != nil {
		spec.Module = man.Module
	}
	return spec
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

	loader := NewLoaderFromManager(mod)
	mod.loader = loader

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

func (m *ModuleManager) ExecutableDirs() ([]string, error) {
	mod, err := m.loadLockAndModule()
	if err != nil {
		return nil, err
	}
	return mod.executableDirs()
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
	spec := m.newModuleParams(confapi.ModuleParams{
		Source:    filepath.Join(m.AbsWorkDir, m.ModuleFile),
		Arguments: map[string]interface{}{},
		//LockedVersions: ModVersionLock{Dependencies: map[string]DepVersionLock{}},
		LockedVersions: *lockContents,
		ForceUpdate:    true,
	})

	mod, err := m.loader.LoadModule(spec)
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

	for _, d := range mod.Directories {
		src, err := d.Source(values)
		if err != nil {
			m.Logger.V(1).Info(err.Error())
			return nil, err
		}

		yours, err := m.dep.ResolveDir(src)
		if err != nil {
			m.Logger.V(1).Info(err.Error())
			return nil, err
		}

		tmpls := map[string]struct{
			pat *regexp.Regexp
			tmpl confapi.Template
		}{}

		for _, v := range d.Templates {
			tmpls[v.SourcePattern] =  struct{
				pat *regexp.Regexp
				tmpl confapi.Template
			}{
				pat: regexp.MustCompile(v.SourcePattern),
				tmpl: v,
			}
		}

		var srcDir string

		var dstDir string

		mine := filepath.Join(m.AbsWorkDir, yours)
		m.Logger.V(1).Info("resolved", "modulefile", yours, "localfile", mine)

		if _, err := m.fs.ReadDir(mine); err != nil {
			if _, err = m.fs.ReadDir(yours); err != nil {
				m.Logger.V(1).Info(err.Error())
				return nil, err
			}
			srcDir = yours
			dstDir = d.Path
		} else {
			srcDir = mine
			dstDir = filepath.Join(m.AbsWorkDir, d.Path)
		}

		err = vfs.Walk(m.fs, srcDir, func(path string, info os.FileInfo, err error) error {
		 	if err != nil {
		 		return fmt.Errorf("%s: %v", path, err)
			}

		 	if info.IsDir() {
		 		return nil
			}

			for _, v:= range tmpls {
				matches := v.pat.FindStringSubmatch(path)

				contents, err := m.fs.ReadFile(path)
				if err != nil {
					return err
				}

				var dst string

				if len(matches) > 1 {
					dst = strings.ReplaceAll(matches[1], srcDir, dstDir)

					args, err := v.tmpl.Args(values)
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

					// write(dst, result)
				} else {
					dst = strings.ReplaceAll(path, srcDir, dstDir)
				}

				dstDirToWrite := filepath.Dir(dst)

				if err := vfs.MkdirAll(m.fs, dstDirToWrite, 0755); err != nil {
					return fmt.Errorf("mkdirall on %q: %w", dstDirToWrite, err)
				}

				if err := m.fs.WriteFile(dst, contents, info.Mode()); err != nil {
					m.Logger.V(1).Info(err.Error())
					return err
				}

				r.Files = append(r.Files, dst)
			}

			return nil
		})

		if err != nil {
			return nil, err
		}
	}

	for _, f := range mod.Files {
		u, err := f.Source(values)
		if err != nil {
			m.Logger.V(1).Info(err.Error())
			return nil, err
		}

		yours, err := m.dep.ResolveFile(u)
		if err != nil {
			m.Logger.V(1).Info(err.Error())
			return nil, err
		}

		var target string

		mine := filepath.Join(m.AbsWorkDir, yours)
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

		dstFile := filepath.Join(m.AbsWorkDir, f.Path)

		dstDir := filepath.Dir(dstFile)

		if err := vfs.MkdirAll(m.fs, dstDir, 0755); err != nil {
			return nil, fmt.Errorf("mkdirall on %q: %w", dstDir, err)
		}

		if err := m.fs.WriteFile(dstFile, contents, 0644); err != nil {
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
