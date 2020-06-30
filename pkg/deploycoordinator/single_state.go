package deploycoordinator

import (
	"fmt"
	"gopkg.in/yaml.v3"
)

func (s *State) GetStage(stageName string) (*StageStateSummary, error) {
	return getStageStateSummary(s.Stages, s.Revisions, stageName)
}

type State struct {
	Stages       []StageState           `yaml:"stages`
	Revisions    []Revision             `yaml:"revisions"`
	Dependencies map[string]*Dependency `yaml:"dependencies"`
}

func (s *State) GetRevisions() ([]Revision, error) {
	return s.Revisions, nil
}

func (s *State) GetCurrentRevision() (*Revision, error) {
	revs, err := s.GetRevisions()
	if err != nil {
		return nil, fmt.Errorf("getting latest dependency set revision: %w", err)
	}

	if len(revs) == 0 {
		return nil, fmt.Errorf("getting latest dependency set revision: not found: %w", err)
	}

	return &revs[len(revs)-1], nil
}

func (s *State) UpdateRevisions(depPattern string, requiredDepToConstraint map[string]string) error {
	current, err := s.GetCurrentRevision()
	if err != nil {
		return fmt.Errorf("getting latest dependency set revision: %w", err)
	}

	updated, err := updateRevisions(s.Dependencies, current, s.Revisions, depPattern, requiredDepToConstraint)
	if err != nil {
		return err
	}

	s.Revisions = updated

	return nil
}

func (s *State) AddDependencyUpdate(name, version string) error {
	return addDependencyUpdate(s.Dependencies, name, version)
}

func (s *State) UpdateDependencies(f func(depName string, current string) (string, error)) error {
	return updateDependencies(s.Dependencies, f)
}

func ParseSingleState(doc string) (*State, error) {
	var state State

	if err := yaml.Unmarshal([]byte(doc), &state); err != nil {
		return nil, fmt.Errorf("unmarshalling yaml: %w", err)
	}

	return &state, nil
}

func (s *State) UpdateStage(stageName string) error {
	latest, err := s.GetCurrentRevision()
	if err != nil {
		return fmt.Errorf("getting latest dependency set revision: %w", err)
	}

	return updateStage(s.Stages, latest, stageName)
}
