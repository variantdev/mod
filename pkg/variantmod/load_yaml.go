package variantmod

import (
	"fmt"
	"path/filepath"

	aliases "github.com/k-kinzal/aliases/pkg/aliases/yaml"
	"github.com/variantdev/mod/pkg/config/confapi"
	"github.com/variantdev/mod/pkg/config/yamlconf"
	"github.com/variantdev/mod/pkg/maputil"
	"github.com/variantdev/mod/pkg/tmpl"
	"gopkg.in/yaml.v3"
)

func (m *ModuleLoader) loadYamlModule(params confapi.ModuleParams) (*confapi.Module, error) {
	resolved, err := m.dep.ResolveFile(params.Source)
	if err != nil {
		return nil, err
	}

	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(m.AbsWorkDir, resolved)
	}

	if filepath.Base(resolved) != "variant.mod" {
		resolved = filepath.Join(resolved, "variant.mod")
	}

	bytes, err := m.FS.ReadFile(resolved)
	if err != nil {
		m.Logger.Error(err, "read file", "resolved", resolved, "depspec", params)
		var err2 error
		bytes, err2 = m.FS.ReadFile(resolved)
		if err2 != nil {
			return nil, err2
		}
	}

	spec := &yamlconf.ModuleSpec{
		Name: "variant",
		Parameters: yamlconf.ParametersSpec{
			Schema:   map[string]interface{}{},
			Defaults: map[string]interface{}{},
		},
		Releases: map[string]yamlconf.ReleaseSpec{},
	}
	if err := yaml.Unmarshal(bytes, spec); err != nil {
		return nil, err
	}
	m.Logger.V(2).Info("load", "alias", params.Alias, "module", spec, "dep", params)

	for n, dep := range spec.Dependencies {
		if dep.ReleasesFrom.IsDefined() {
			_, conflicted := spec.Releases[n]
			if conflicted {
				return nil, fmt.Errorf("conflicting dependency %q", n)
			}
			spec.Releases[n] = yamlconf.ReleaseSpec{VersionsFrom: dep.ReleasesFrom}
		}
	}

	releases := map[string]confapi.Release{}

	for alias, dep := range spec.Releases {
		var r confapi.Release

		r.VersionsFrom = yamlconf.ToVersionsFrom(dep.VersionsFrom)

		releases[alias] = r
	}

	dependencies := map[string]confapi.Dependency{}
	for alias, dep := range spec.Dependencies {
		releaseFrom := yamlconf.ToVersionsFrom(dep.ReleasesFrom)

		dependencies[alias] = confapi.Dependency{
			ReleasesFrom:      releaseFrom,
			Source:            dep.Source,
			Kind:              dep.Kind,
			VersionConstraint: dep.VersionConstraint,
			Arguments:         yamlconf.NewRenderArgs(dep.Arguments),
			Alias:             dep.Alias,
			LockedVersions:    dep.LockedVersions,
			ForceUpdate:       dep.ForceUpdate,
		}

		releases[alias] = confapi.Release{VersionsFrom: releaseFrom}
	}

	execs := map[string]confapi.Executable{}
	for k, v := range spec.Provisioners.Executables.Executables {
		var e confapi.Executable
		for _, p := range v.Platforms {
			e.Platforms = append(e.Platforms, confapi.Platform{
				Source: yamlconf.NewRender("platform.source", p.Source),
				Docker: func(v map[string]interface{}) (*aliases.OptionSpec, error) {
					d := p.Docker
					var err error
					d.Tag, err = tmpl.Render("platform.docker.tag", d.Tag, v)
					if err != nil {
						return nil, err
					}
					return &d, nil
				},
				Selector: confapi.Selector{MatchLabels: confapi.MatchLabels{
					OS:   p.Selector.MatchLabels.OS,
					Arch: p.Selector.MatchLabels.Arch,
				}},
			})
		}
		execs[k] = e
	}

	files := []confapi.File{}
	for path, fspec := range spec.Provisioners.Files {
		files = append(files, yamlconf.ToFile(path, fspec))
	}

	dirs := []confapi.Directory{}
	for path, dspec := range spec.Provisioners.Directories {
		dirs = append(dirs, yamlconf.ToDirectory(path, dspec))
	}

	regexpReplaces := []confapi.RegexpReplace{}
	for path, rspec := range spec.Provisioners.RegexpReplace {
		regexpReplaces = append(regexpReplaces, yamlconf.ToRegexpReplace(path, rspec))
	}

	textReplaces := []confapi.TextReplace{}
	for path, tspec := range spec.Provisioners.TextReplace {
		textReplaces = append(textReplaces, yamlconf.ToTextReplace(path, tspec))
	}

	yamls := []confapi.YamlPatch{}
	for path, yspec := range spec.Provisioners.YamlPatch {
		yamls = append(yamls, yamlconf.ToYamlPatch(path, yspec))
	}

	spec.Parameters.Schema["type"] = "object"

	schema, err := maputil.CastKeysToStrings(spec.Parameters.Schema)
	if err != nil {
		return nil, err
	}

	defaults, err := maputil.CastKeysToStrings(spec.Parameters.Defaults)
	if err != nil {
		return nil, err
	}

	return &confapi.Module{
		Name:           spec.Name,
		Defaults:       defaults,
		ValuesSchema:   schema,
		Dependencies:   dependencies,
		Releases:       releases,
		Executables:    execs,
		Files:          files,
		Directories:    dirs,
		RegexpReplaces: regexpReplaces,
		TextReplaces:   textReplaces,
		Yamls:          yamls,
	}, nil
}
