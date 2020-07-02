package deploycoordinator

import "github.com/variantdev/mod/pkg/config/confapi"

type Spec struct {
	Deployments []*DeploymentSpec `yaml:"deployments"`
}

type DeploymentSpec struct {
	Name         string                     `yaml:"name"`
	Stages       []confapi.Stage            `yaml:"stages"`
	Dependencies []DeploymentDependencySpec `yaml:"dependencies"`
}

type DeploymentDependencySpec struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
}

type MultiStateFile struct {
	State MultiState `yaml:"state"`
}

type StageStateSummary struct {
	Versions map[string]string
}

type DeploymentState struct {
	Revisions []confapi.Revision   `yaml:"revisions"`
	Stages    []confapi.StageState `yaml:"stages"`
}

type DependencyEntry struct {
	Version string
	Meta    confapi.DependencyStateMeta
}
