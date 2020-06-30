package deploycoordinator

import (
	"fmt"
	"gopkg.in/yaml.v3"
)

func (s *MultiState) GetStage(deploymentName string, stageName string) (*StageStateSummary, error) {
	p, ok := s.Deployments[deploymentName]
	if !ok {
		return nil, fmt.Errorf("getting deployment: %q not found", deploymentName)
	}

	return getStageStateSummary(p.Stages, p.Revisions, stageName)
}

type MultiState struct {
	Deployments  map[string]*DeploymentState `yaml:"deployments`
	Dependencies map[string]*Dependency      `yaml:"dependencies"`
}

func (s *MultiState) GetRevisions(deploymentName string) ([]Revision, error) {
	pl, ok := s.Deployments[deploymentName]
	if !ok {
		return nil, fmt.Errorf("getting dependency set revision: %s not found", deploymentName)
	}

	return pl.Revisions, nil
}

func (s *MultiState) GetCurrentRevision(deploymentName string) (*Revision, error) {
	revs, err := s.GetRevisions(deploymentName)
	if err != nil {
		return nil, fmt.Errorf("getting latest dependency set revision: %w", err)
	}

	if len(revs) == 0 {
		return nil, fmt.Errorf("getting latest dependency set revision: not found: %w", err)
	}

	return &revs[len(revs)-1], nil
}

func (s *MultiState) DeploymentUpdateDependencies(deploymentName string, depPattern string, requiredDepToConstraint map[string]string) error {
	p, ok := s.Deployments[deploymentName]
	if !ok {
		return fmt.Errorf("getting deployment: %q not found", deploymentName)
	}

	current, err := s.GetCurrentRevision(deploymentName)
	if err != nil {
		return fmt.Errorf("getting latest dependency set revision: %w", err)
	}

	updated, err := updateRevisions(s.Dependencies, current, p.Revisions, depPattern, requiredDepToConstraint)
	if err != nil {
		return err
	}

	p.Revisions = updated

	return nil
}

func (s *MultiState) AddDependencyUpdate(name, version string) error {
	return addDependencyUpdate(s.Dependencies, name, version)
}

func (s *MultiState) UpdateDependencies(f func(depName string, current string) (string, error)) error {
	return updateDependencies(s.Dependencies, f)
}

func ParseMultiState(doc string) (*MultiState, error) {
	var statefile MultiStateFile

	if err := yaml.Unmarshal([]byte(doc), &statefile); err != nil {
		return nil, fmt.Errorf("unmarshalling yaml: %w", err)
	}

	return &statefile.State, nil
}

func (s *MultiState) UpdateStage(deployName string, stageName string) error {
	p, ok := s.Deployments[deployName]
	if !ok {
		return fmt.Errorf("getting deployment: %q not found", deployName)
	}

	latest, err := s.GetCurrentRevision(deployName)
	if err != nil {
		return fmt.Errorf("getting latest dependency set revision: %w", err)
	}

	return updateStage(p.Stages, latest, stageName)
}
