package yamlconf

import (
	"encoding/json"

	"github.com/variantdev/mod/pkg/config/confapi"
	"github.com/variantdev/mod/pkg/execversionmanager"
	"github.com/variantdev/mod/pkg/maputil"
	"github.com/variantdev/mod/pkg/tmpl"
)

type ModuleSpec struct {
	Name string `yaml:"name"`

	Parameters   ParametersSpec            `yaml:"parameters"`
	Provisioners ProvisionersSpec          `yaml:"provisioners"`
	Dependencies map[string]DependencySpec `yaml:"dependencies"`
	Releases     map[string]ReleaseSpec    `yaml:"releases"`
	Stages       []Stage                   `yaml:"stages,omitempty"`
}

type ReleaseSpec struct {
	VersionsFrom VersionsFrom `yaml:"versionsFrom"`
}

type VersionsFrom struct {
	Exec            Exec            `yaml:"exec"`
	JSONPath        GetterJSONPath  `yaml:"jsonPath"`
	GitTags         GitTags         `yaml:"gitTags"`
	GitHubTags      GitHubTags      `yaml:"githubTags"`
	GitHubReleases  GitHubReleases  `yaml:"githubReleases"`
	DockerImageTags DockerImageTags `yaml:"dockerImageTags"`

	ValidVersionPattern string `yaml:"validVersionPattern"`
}

func (f VersionsFrom) IsDefined() bool {
	return f.Exec.Command != "" ||
		f.JSONPath.Source != "" ||
		f.GitTags.Source != "" ||
		f.GitHubReleases.Source != "" ||
		f.DockerImageTags.Source != ""
}

func ToVersionsFrom(v VersionsFrom) confapi.VersionsFrom {
	var r confapi.VersionsFrom
	r.Exec.Args = v.Exec.Args
	r.Exec.Command = v.Exec.Command
	r.DockerImageTags.Source = NewRender("dockerimageTags.source", v.DockerImageTags.Source)
	r.GitHubReleases.Source = NewRender("githubReleases.source", v.GitHubReleases.Source)
	r.GitHubReleases.Host = v.GitHubReleases.Host
	r.GitHubTags.Source = NewRender("githubTags.source", v.GitHubTags.Source)
	r.GitHubTags.Host = v.GitHubTags.Host
	r.GitTags.Source = NewRender("gitTags.source", v.GitTags.Source)
	r.JSONPath.Source = NewRender("jsonPath.source", v.JSONPath.Source)
	r.JSONPath.Description = v.JSONPath.Description
	r.JSONPath.Versions = v.JSONPath.Versions
	r.ValidVersionPattern = v.ValidVersionPattern
	return r
}

type Exec struct {
	Command string   `yaml:"command"`
	Args    []string `yaml:"args"`
}

type GetterJSONPath struct {
	Source      string `yaml:"source"`
	Versions    string `yaml:"versions"`
	Description string `yaml:"description"`
}

type GitTags struct {
	Source string `yaml:"source"`
}

type GitHubTags struct {
	Host   string `yaml:"host"`
	Source string `yaml:"source"`
}

type GitHubReleases struct {
	Host   string `yaml:"host"`
	Source string `yaml:"source"`
}

type DockerImageTags struct {
	Source string `yaml:"source"`
}

type ParametersSpec struct {
	Schema   map[string]interface{} `yaml:"schema"`
	Defaults map[string]interface{} `yaml:"defaults"`
}

type ProvisionersSpec struct {
	Files         map[string]FileSpec          `yaml:"files"`
	Directories   map[string]DirectorySpec     `yaml:"directories"`
	Executables   execversionmanager.Config    `yaml:",inline"`
	TextReplace   map[string]TextReplaceSpec   `yaml:"textReplace"`
	RegexpReplace map[string]RegexpReplaceSpec `yaml:"regexpReplace"`
	YamlPatch     map[string][]YamlPatchSpec   `yaml:"yamlPatch"`
}

type FileSpec struct {
	Source    string                 `yaml:"source"`
	Path      string                 `yaml:"path"`
	Arguments map[string]interface{} `yaml:"arguments"`
}

type DirectorySpec struct {
	Source    string                  `yaml:"source"`
	Arguments map[string]interface{}  `yaml:"arguments"`
	Templates map[string]TemplateSpec `yaml:"templates"`
}

type TemplateSpec struct {
	Arguments map[string]interface{} `yaml:"arguments"`
}

type Stage struct {
	Name         string   `yaml:"name"`
	Environments []string `yaml:"environments"`
}

func NewRenderArgs(args map[string]interface{}) func(map[string]interface{}) (map[string]interface{}, error) {
	return func(vals map[string]interface{}) (map[string]interface{}, error) {
		a, err := maputil.CastKeysToStrings(args)
		if err != nil {
			return nil, err
		}
		return tmpl.RenderArgs(a, vals)
	}
}

func NewRender(name, str string) func(map[string]interface{}) (string, error) {
	return func(vals map[string]interface{}) (string, error) {
		return tmpl.Render(name, str, vals)
	}
}

func ToFile(path string, spec FileSpec) confapi.File {
	if spec.Path != "" {
		path = spec.Path
	}

	return confapi.File{
		Path:   NewRender("file.path", path),
		Source: NewRender("file.sourc", spec.Source),
		Args:   NewRenderArgs(spec.Arguments),
	}
}

func ToDirectory(path string, spec DirectorySpec) confapi.Directory {
	var tmpls []confapi.Template

	for pat := range spec.Templates {
		tmplSpec := spec.Templates[pat]

		tmpls = append(tmpls, confapi.Template{
			SourcePattern: pat,
			Args:          NewRenderArgs(tmplSpec.Arguments),
		})
	}

	return confapi.Directory{
		Path:      path,
		Source:    NewRender("directory.sourc", spec.Source),
		Templates: tmpls,
	}
}

func ToTextReplace(path string, spec TextReplaceSpec) confapi.TextReplace {
	return confapi.TextReplace{
		Path: NewRender("textReplace.path", path),
		From: NewRender("textReplace.from", spec.From),
		To:   NewRender("textReplace.to", spec.To),
	}
}

func ToRegexpReplace(path string, spec RegexpReplaceSpec) confapi.RegexpReplace {
	return confapi.RegexpReplace{
		Path: NewRender("textReplace.path", path),
		From: spec.From,
		To:   NewRender("textReplace.to", spec.To),
	}
}

func ToYamlPatch(path string, spec []YamlPatchSpec) confapi.YamlPatch {
	patches := []confapi.Patch{}
	for _, v := range spec {
		p := confapi.Patch{
			Op:    v.Op,
			Path:  v.Path,
			Value: v.Value,
			From:  v.From,
		}
		patches = append(patches, p)
	}
	y := confapi.YamlPatch{
		Path: NewRender("yamlPatch.path", path),
		Patch: func(values map[string]interface{}) (string, error) {
			out, err := json.Marshal(patches)
			if err != nil {
				return "", err
			}
			return tmpl.Render("yamlPatch.patches", string(out), values)
		},
	}
	return y
}

type TextReplaceSpec struct {
	From string `yaml:"from"`
	To   string `yaml:"to"`
}

type RegexpReplaceSpec struct {
	From string `yaml:"from"`
	To   string `yaml:"to"`
}

type YamlPatchSpec struct {
	Op    string      `yaml:"op"`
	Path  string      `yaml:"path"`
	Value interface{} `yaml:"value"`
	From  string      `yaml:"string"`
}

type DependencySpec struct {
	ReleasesFrom VersionsFrom `yaml:"releasesFrom""`

	Source string `yaml:"source"`
	Kind   string `yaml:"kind"`
	// VersionConstraint is the version range for this dependency. Works only for modules hosted on Git or GitHub
	VersionConstraint string                 `yaml:"version"`
	Arguments         map[string]interface{} `yaml:"arguments"`

	Alias          string
	LockedVersions confapi.State

	ForceUpdate bool
}

func (d DependencySpec) RenderArgs(vals map[string]interface{}) (map[string]interface{}, error) {
	args, err := maputil.CastKeysToStrings(d.Arguments)
	if err != nil {
		return nil, err
	}
	return tmpl.RenderArgs(args, vals)
}
