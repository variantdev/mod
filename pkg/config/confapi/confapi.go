package confapi

import (
	"github.com/k-kinzal/aliases/pkg/aliases/yaml"
)

type Module struct {
	Name           string
	Defaults       map[string]interface{}
	ValuesSchema   map[string]interface{}
	Dependencies   map[string]Dependency
	Executables    map[string]Executable
	Releases       map[string]Release
	Files          []File
	Directories    []Directory
	TextReplaces   []TextReplace
	RegexpReplaces []RegexpReplace
	Yamls          []YamlPatch
	Stages         []Stage
}

type File struct {
	Path   func(map[string]interface{}) (string, error)
	Source func(map[string]interface{}) (string, error)
	Args   func(map[string]interface{}) (map[string]interface{}, error)
}

type Directory struct {
	Path      string
	Source    func(map[string]interface{}) (string, error)
	Templates []Template
}

type Template struct {
	SourcePattern string
	Args          func(map[string]interface{}) (map[string]interface{}, error)
}

type ModuleParams struct {
	Source         string
	Arguments      map[string]interface{}
	Alias          string
	LockedVersions State
	ForceUpdate    bool
	Module         *Module
}

type TextReplace struct {
	Path, From, To func(map[string]interface{}) (string, error)
}

type RegexpReplace struct {
	Path, To func(map[string]interface{}) (string, error)
	From     string
}

type YamlPatch struct {
	Path  func(map[string]interface{}) (string, error)
	Patch func(map[string]interface{}) (string, error)
}

type Patch struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value"`
	From  string      `json:"from"`
}

type Dependency struct {
	ReleasesFrom VersionsFrom

	Source string
	Kind   string
	// VersionConstraint is the version range for this dependency. Works only for modules hosted on Git or GitHub
	VersionConstraint string

	Arguments func(map[string]interface{}) (map[string]interface{}, error)

	Alias          string
	LockedVersions State

	ForceUpdate bool
}

type Meta struct {
	Dependencies map[string]map[string]map[string]interface{} `yaml:"dependencies,omitempty"`
}

func (l State) ToMap() map[string]interface{} {
	deps := l.ToDepsMap()
	m := map[string]interface{}{"Dependencies": deps, "RawLock": l.RawLock}

	for k, v := range deps {
		m[k] = v
	}

	return m
}

func (l State) ToDepsMap() map[string]interface{} {
	deps := map[string]interface{}{}
	for k, v := range l.Dependencies {
		m := map[string]interface{}{"version": v.Version}
		if v.PreviousVersion != "" {
			m["previousVersion"] = v.PreviousVersion
		}
		for kk, vv := range v.Meta {
			m[kk] = vv
		}
		deps[k] = m
	}
	return deps
}

type Executable struct {
	Platforms []Platform
}

type Platform struct {
	Source   func(map[string]interface{}) (string, error)
	Docker   func(map[string]interface{}) (*yaml.OptionSpec, error)
	Selector Selector
}

type Selector struct {
	MatchLabels MatchLabels
}

type MatchLabels struct {
	OS   string
	Arch string
}

type Release struct {
	VersionsFrom VersionsFrom
}

type VersionsFrom struct {
	Exec            Exec
	JSONPath        GetterJSONPath
	GitTags         GitTags
	GitHubTags      GitHubTags
	GitHubReleases  GitHubReleases
	DockerImageTags DockerImageTags

	// ValidVersionPattern is the regular expression that should match only against valid version numbers for this dependency.
	// Used for filtering out unnecessary, unexpected or invalid version numbers from being used for dependency updates.
	ValidVersionPattern string
}

type Exec struct {
	Command string
	Args    []string
}

type GetterJSONPath struct {
	Source      func(map[string]interface{}) (string, error)
	Versions    string
	Description string
}

type GitTags struct {
	Source func(map[string]interface{}) (string, error)
}

type GitHubTags struct {
	Host   string
	Source func(map[string]interface{}) (string, error)
}

type GitHubReleases struct {
	Host   string
	Source func(map[string]interface{}) (string, error)
}

type DockerImageTags struct {
	Source func(map[string]interface{}) (string, error)
}

type Stage struct {
	Name         string
	Environments []string
}
