package deploycoordinator

import (
	"bytes"
	"fmt"
	"gopkg.in/yaml.v3"
)

type Multi struct {
	Spec

	*MultiState
}

func (c *Multi) DeploymentDependencies(deployName string) map[string]string {
	r := map[string]string{}

	var deploy *DeploymentSpec

	for i := range c.Spec.Deployments {
		p := c.Spec.Deployments[i]

		if p.Name == deployName {
			deploy = p

			break
		}
	}

	if deploy == nil {
		panic(fmt.Errorf("deployment %q not found", deployName))
	}

	for _, dep := range deploy.Dependencies {
		r[dep.Name] = dep.Version
	}

	return r
}

func (c *Multi) Marshal() (string, error) {
	var buf bytes.Buffer

	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)

	type out struct {
		State *MultiState `yaml:"state"`
	}

	if err := enc.Encode(out{State: c.MultiState}); err != nil {
		return "", fmt.Errorf("encoding state: %w", err)
	}

	got := buf.String()

	return got, nil
}


func (s *Multi) UpdateStage(deployName string, stageName string) error {
	var p *DeploymentSpec

	for _, d := range s.Spec.Deployments {
		if d.Name == deployName {
			p = d
			break
		}
	}

	if p == nil {
		return fmt.Errorf("getting deployment: %q not found", deployName)
	}

	state, ok := s.MultiState.Deployments[deployName]
	if !ok {
		return fmt.Errorf("getting deployment state: %q not found", deployName)
	}

	latest, err := s.GetCurrentRevision(deployName)
	if err != nil {
		return fmt.Errorf("getting latest dependency set revision: %w", err)
	}

	updated, err := updateStage(p.Stages, state.Stages, latest, stageName)
	if err != nil {
		return err
	}

	s.MultiState.Deployments[deployName].Stages = updated

	return nil
}
