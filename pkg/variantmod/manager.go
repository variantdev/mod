package variantmod

import (
	"bytes"
	"fmt"
	"github.com/ghodss/yaml"
	"github.com/go-logr/logr"
	"github.com/twpayne/go-vfs"
	"github.com/variantdev/mod/pkg/depresolver"
	"github.com/xeipuuv/gojsonschema"
	"k8s.io/klog/klogr"
	"os"
	"path/filepath"
	"text/template"
)

type ModuleSpec struct {
	Values map[string]interface{} `yaml:"values"`
	Schema map[string]interface{} `yaml:"schema"`
	Files  map[string]FileSpec    `yaml:"files"`
}

type FileSpec struct {
	Source    string                 `yaml:"source"`
	Arguments map[string]interface{} `yaml:"arguments"`
}

type ModuleManager struct {
	fs vfs.FS

	Logger logr.Logger

	AbsWorkDir     string
	CacheDirectory string

	dep *depresolver.Resolver
}

type Values map[string]interface{}

type Module struct {
	Values       Values
	ValuesSchema Values
	Files        []File
}

type Parameter struct {
	Name     string
	Default  interface{}
	Type     string
	Required []string
}

type File struct {
	Path      string
	Source    string
	Arguments map[string]interface{}
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

	if mod.CacheDirectory == "" {
		mod.CacheDirectory = ".variant/mod/cache"
	}

	abs := filepath.IsAbs(mod.CacheDirectory)
	if !abs {
		mod.CacheDirectory = filepath.Join(mod.AbsWorkDir, mod.CacheDirectory)
	}

	mod.Logger.V(1).Info("init", "workdir", mod.AbsWorkDir, "cachedir", mod.CacheDirectory)

	dep, err := depresolver.New(
		depresolver.Home(mod.CacheDirectory),
		depresolver.Logger(mod.Logger),
	)
	if err != nil {
		return nil, err
	}

	mod.dep = dep

	return mod, nil
}
func (m *ModuleManager) Run() error {
	bytes, err := m.fs.ReadFile(filepath.Join(m.AbsWorkDir, "variant.mod"))
	if err != nil {
		return err
	}

	spec := &ModuleSpec{}
	if err := yaml.Unmarshal(bytes, &spec); err != nil {
		return err
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

	mod := &Module{
		Values:       spec.Values,
		ValuesSchema: spec.Schema,
		Files:        files,
	}

	return m.run(mod)
}

func (m *ModuleManager) run(mod *Module) error {
	schemaLoader := gojsonschema.NewGoLoader(mod.ValuesSchema)
	jsonLoader := gojsonschema.NewGoLoader(mod.Values)
	result, err := gojsonschema.Validate(schemaLoader, jsonLoader)
	if err != nil {
		return err
	}
	for i, err := range result.Errors() {
		m.Logger.V(1).Info("err", "index", i, "err", err.String())
	}

	for _, f := range mod.Files {
		u, err := render("source", f.Source, mod.Values)
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
			args, err := renderArgs(f.Arguments, mod.Values)
			if err != nil {
				m.Logger.V(1).Info(err.Error())
				return err
			}
			res, err := render("source file", string(contents), args)
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

func render(name, text string, data interface{}) (string, error) {
	funcs := map[string]interface{}{}
	tpl := template.New(name).Option("missingkey=error").Funcs(funcs)
	tpl, err := tpl.Parse(text)
	if err != nil {
		return "", err
	}
	buf := &bytes.Buffer{}
	if err := tpl.Execute(buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func renderArgs(args map[string]interface{}, data map[string]interface{}) (map[string]interface{}, error) {
	res := map[string]interface{}{}

	for k, v := range args {
		switch t := v.(type) {
		case map[string]interface{}:
			r, err := renderArgs(t, data)
			if err != nil {
				return nil, err
			}
			res[k] = r
		case string:
			r, err := render(fmt.Sprintf("arg:%s", t), t, data)
			if err != nil {
				return nil, err
			}
			res[k] = r
		case int, bool:
			res[k] = t
		default:
			return nil, fmt.Errorf("unsupported type: value=%v, type=%T", t, t)
		}
	}

	return res, nil
}
