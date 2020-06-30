package deploycoordinator

type Spec struct {
	Deployments []*DeploymentSpec `yaml:"deployments"`
}

type DeploymentSpec struct {
	Name         string                     `yaml:"name"`
	Stages       []DeploymentStageSpec      `yaml:"stages"`
	Dependencies []DeploymentDependencySpec `yaml:"dependencies"`
}

type DeploymentDependencySpec struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
}

type DeploymentStageSpec struct {
	Name         string   `yaml:"name"`
	Environments []string `yaml:"environments"`
}

type MultiStateFile struct {
	State MultiState `yaml:"state"`
}

type StageStateSummary struct {
	Versions map[string]string
}

type DeploymentState struct {
	Revisions []Revision   `yaml:"revisions"`
	Stages    []StageState `yaml:"stages"`
}

type Revision struct {
	ID       int               `yaml:"id"`
	Versions map[string]string `yaml:"versions"`
}

type StageState struct {
	Name                    string `yaml:"name"`
	DependencySetRevisionID int    `yaml:"dependencySetRevisionID"`
}

type Dependency struct {
	Versions []string `yaml:"versions"`
}

