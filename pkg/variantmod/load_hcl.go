package variantmod

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/k-kinzal/aliases/pkg/aliases/yaml"
	"github.com/variantdev/mod/pkg/config/confapi"
	"github.com/variantdev/mod/pkg/config/hclconf"
	"github.com/zclconf/go-cty/cty"
)

func (m *ModuleLoader) loadHclModule(params confapi.ModuleParams) (*confapi.Module, error) {
	hclApp, err := hclconf.NewLoader(m.FS.ReadFile).LoadFile(params.Source)
	if err != nil {
		return nil, err
	}
	return appToModule(hclApp)
}

func appToModule(app *hclconf.App) (*confapi.Module, error) {
	numModules := len(app.Config.Modules)
	if numModules == 0 {
		return nil, fmt.Errorf("one or more modules must be present")
	} else if numModules > 1 {
		return nil, fmt.Errorf("unsupported number of modules: only one module is currently supported: got %d", numModules)
	}

	return HCLModuleAsConfModule(app.Config.Modules[0])
}

func HCLModuleAsConfModule(mod hclconf.Module) (*confapi.Module, error) {
	rels := map[string]confapi.Release{}

	deps := map[string]confapi.Dependency{}
	for i := range mod.Dependencies {
		d := mod.Dependencies[i]
		provider := confapi.VersionsFrom{}
		switch d.Type {
		case "exec":
			var e hclconf.ExecDependency
			if err := gohcl.DecodeBody(d.BodyForType, &hcl.EvalContext{}, &e); err != nil {
				return nil, err
			}
			provider.Exec = confapi.Exec{
				Command: e.Command,
				Args:    e.Args,
			}
		case "git_tag":
			var e hclconf.GitTags
			if err := gohcl.DecodeBody(d.BodyForType, &hcl.EvalContext{}, &e); err != nil {
				return nil, err
			}
			provider.GitTags = confapi.GitTags{
				Source: func(_ map[string]interface{}) (string, error) {
					return e.Source, nil
				},
			}
		case "github_tag":
			var e hclconf.GitHubTags
			if err := gohcl.DecodeBody(d.BodyForType, &hcl.EvalContext{}, &e); err != nil {
				return nil, err
			}
			var host string
			if e.Host != nil {
				host = *e.Host
			}
			provider.GitHubTags = confapi.GitHubTags{
				Host: host,
				Source: func(_ map[string]interface{}) (string, error) {
					return e.Source, nil
				},
			}
		case "github_release":
			var e hclconf.GitHubReleases
			if err := gohcl.DecodeBody(d.BodyForType, &hcl.EvalContext{}, &e); err != nil {
				return nil, err
			}
			var host string
			if e.Host != nil {
				host = *e.Host
			}
			provider.GitHubReleases = confapi.GitHubReleases{
				Host: host,
				Source: func(_ map[string]interface{}) (string, error) {
					return e.Source, nil
				},
			}
		case "docker_tag":
			var e hclconf.DockerImageTags
			if err := gohcl.DecodeBody(d.BodyForType, &hcl.EvalContext{}, &e); err != nil {
				return nil, err
			}
			provider.DockerImageTags = confapi.DockerImageTags{
				Source: func(_ map[string]interface{}) (string, error) {
					return e.Source, nil
				},
			}
		case "json_path":
			var e hclconf.JSONPath
			if err := gohcl.DecodeBody(d.BodyForType, &hcl.EvalContext{}, &e); err != nil {
				return nil, err
			}
			provider.JSONPath = confapi.GetterJSONPath{
				Source: func(_ map[string]interface{}) (string, error) {
					return e.Source, nil
				},
				Versions:    e.Versions,
				Description: e.Description,
			}
		default:
			return nil, fmt.Errorf("dependency of type %q not implemented yet", d.Type)
		}

		rels[d.Name] = confapi.Release{VersionsFrom: provider}
		deps[d.Name] = confapi.Dependency{
			ReleasesFrom:      provider,
			Source:            "",
			Kind:              "",
			VersionConstraint: d.Version,
			Arguments: func(v map[string]interface{}) (map[string]interface{}, error) {
				m := map[string]interface{}{}
				return m, nil
			},
			Alias:          d.Name,
			LockedVersions: confapi.ModVersionLock{},
			ForceUpdate:    false,
		}
	}

	vToVars := func(v map[string]interface{}) (map[string]cty.Value, error) {
		deps := map[string]cty.Value{}
		orig, ok := v["Dependencies"]
		if !ok {
			return nil, fmt.Errorf("map doesnt contain Dependencies: %v", v)
		}
		origMap, ok := orig.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("unexpected type of orig: %T", orig)
		}
		for k, info := range origMap {
			infoMap, ok := info.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("unexpected type of info: %T", info)
			}
			v, ok := infoMap["version"]
			if !ok {
				return nil, fmt.Errorf("map doesnt contain version: %v", info)
			}
			deps[k] = cty.MapVal(map[string]cty.Value{
				"version": cty.StringVal(fmt.Sprintf("%s", v)),
			})
		}
		return map[string]cty.Value{
			"dep": cty.MapVal(deps),
		}, nil
	}

	regexpReplaces := []confapi.RegexpReplace{}
	for i := range mod.RegexpReplaces {
		r := mod.RegexpReplaces[i]
		var rr confapi.RegexpReplace
		rr.Path = func(v map[string]interface{}) (string, error) {
			return r.Name, nil
		}
		rr.From = r.From
		rr.To = func(v map[string]interface{}) (string, error) {
			vars, err := vToVars(v)
			if err != nil {
				return "", err
			}

			var str cty.Value
			if err := gohcl.DecodeExpression(r.To, &hcl.EvalContext{
				Variables: vars,
			}, &str); err != nil {
				return "", err
			}
			return str.AsString(), nil
		}
		regexpReplaces = append(regexpReplaces, rr)
	}

	files := []confapi.File{}
	for i := range mod.Files {
		f := mod.Files[i]
		var ff confapi.File
		ff.Path = f.Name
		ff.Source = func(v map[string]interface{}) (string, error) {
			return f.Source, nil
		}
		ff.Args = func(v map[string]interface{}) (map[string]interface{}, error) {
			vars, err := vToVars(v)
			if err != nil {
				return nil, err
			}
			m := map[string]string{}
			if err := gohcl.DecodeExpression(f.Args, &hcl.EvalContext{
				Variables: vars,
			}, &m); err != nil {
				return nil, err
			}
			mi := map[string]interface{}{}
			for k, v := range m {
				mi[k] = v
			}
			return mi, nil
		}
		files = append(files, ff)
	}

	dirs := []confapi.Directory{}
	for i := range mod.Directories {
		d := mod.Directories[i]

		var dd confapi.Directory

		dd.Path = d.Name
		dd.Source = func(v map[string]interface{}) (string, error) {
			return d.Source, nil
		}

		for j := range d.Templates {
			tmpl := d.Templates[j]

			dd.Templates = append(dd.Templates, confapi.Template{
				SourcePattern: tmpl.PathPattern,
				Args: func(v map[string]interface{}) (map[string]interface{}, error) {
					vars, err := vToVars(v)
					if err != nil {
						return nil, err
					}
					m := map[string]string{}
					if err := gohcl.DecodeExpression(tmpl.Args, &hcl.EvalContext{
						Variables: vars,
					}, &m); err != nil {
						return nil, err
					}
					mi := map[string]interface{}{}
					for k, v := range m {
						mi[k] = v
					}
					return mi, nil
				},
			})
		}

		dirs = append(dirs, dd)
	}

	execs := map[string]confapi.Executable{}
	for k := range mod.Executables {
		e := mod.Executables[k]
		n := e.Name
		var ee confapi.Executable
		ee.Platforms = []confapi.Platform{}

		for i := range e.Platfoms {
			p := e.Platfoms[i]

			var os, arch string
			if p.OS != nil {
				os = *p.OS
			}
			if p.Arch != nil {
				arch = *p.Arch
			}
			pp := confapi.Platform{
				Selector: confapi.Selector{
					MatchLabels: confapi.MatchLabels{
						OS:   os,
						Arch: arch,
					},
				},
			}

			if p.Source.Range().Start != p.Source.Range().End {
				pp.Source = func(v map[string]interface{}) (string, error) {
					vars, err := vToVars(v)
					if err != nil {
						return "", err
					}
					var src string
					if err := gohcl.DecodeExpression(p.Source, &hcl.EvalContext{
						Variables: vars,
					}, &src); err != nil {
						return "", err
					}
					return src, nil
				}
			}

			if p.Docker != nil {
				pp.Docker = func(v map[string]interface{}) (*yaml.OptionSpec, error) {
					var d yaml.OptionSpec

					d.Command = p.Docker.Command
					d.Image = p.Docker.Image
					d.Workdir = &p.Docker.WorkDir

					vars, err := vToVars(v)
					if err != nil {
						return nil, err
					}

					var tag string
					if err := gohcl.DecodeExpression(p.Docker.Tag, &hcl.EvalContext{
						Variables: vars,
					}, &tag); err != nil {
						return nil, err
					}

					d.Tag = tag

					var vols []string
					if err := gohcl.DecodeExpression(p.Docker.Volumes, &hcl.EvalContext{
						Variables: vars,
					}, &vols); err != nil {
						return nil, err
					}

					d.Volume = vols

					env := map[string]string{}

					if err := gohcl.DecodeExpression(p.Docker.Env, &hcl.EvalContext{
						Variables: vars,
					}, &env); err != nil {
						return nil, err
					}

					d.Env = env

					return &d, nil
				}
			}

			ee.Platforms = append(ee.Platforms, pp)
		}

		execs[n] = ee
	}

	return &confapi.Module{
		Name:           mod.Name,
		Defaults:       map[string]interface{}{},
		ValuesSchema:   map[string]interface{}{},
		Dependencies:   deps,
		Releases:       rels,
		Executables:    execs,
		Files:          files,
		Directories:    dirs,
		RegexpReplaces: regexpReplaces,
		TextReplaces:   []confapi.TextReplace{},
		Yamls:          []confapi.YamlPatch{},
	}, nil
}

func appToManager(hclApp *hclconf.App, opts ...Option) (Interface, error) {
	numModules := len(hclApp.Config.Modules)
	if numModules == 0 {
		return nil, fmt.Errorf("one or more modules must be present")
	} else if numModules > 1 {
		return nil, fmt.Errorf("unsupported number of modules: only one module is currently supported: got %d", numModules)
	}

	modConf := hclApp.Config.Modules[0]

	man := &ModuleManager{
		LockFile: modConf.Name + ".lock",
	}

	var err error
	man, err = initModuleManager(man, opts...)
	if err != nil {
		return nil, err
	}

	man.load = func(lock confapi.ModVersionLock) (*Module, error) {
		mod := &Module{
			Alias: modConf.Name,
		}

		return mod, nil
	}

	return man, nil
}
