package deploycoordinator

import (
	"bytes"
	"fmt"
	"gopkg.in/yaml.v3"
)

type Single struct {
	*SingleSpec

	*State
}

type SingleSpec struct {
	Stages       []DeploymentStageSpec
	Dependencies []DeploymentDependencySpec
}

func (c *Single) DeploymentDependencies() map[string]string {
	r := map[string]string{}

	for _, dep := range c.SingleSpec.Dependencies {
		r[dep.Name] = dep.Version
	}

	return r
}

func (s *Single) GetStage(stageName string) (*StageSummary, error) {
	return getStageSummary(s.SingleSpec.Stages, s.State.Stages, s.Revisions, stageName)
}

func (c *Single) Marshal() (string, error) {
	var buf bytes.Buffer

	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)

	if err := enc.Encode(c.State); err != nil {
		return "", fmt.Errorf("encoding state: %w", err)
	}

	got := buf.String()

	return got, nil
}
