package confapi

type VersionedDependencyStateMeta map[string]DependencyStateMeta

type DependencyStateMeta map[string]interface{}

type DependencyState struct {
	Version         string                 `yaml:"version,omitempty"`
	PreviousVersion string                 `yaml:"previousVersion,omitempty"`
	Meta            map[string]interface{} `yaml:",inline"`

	Versions []string `yaml:"versions,omitempty"`
}

