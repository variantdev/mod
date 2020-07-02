package deploycoordinator

import (
	"bytes"
	"fmt"
	"github.com/variantdev/mod/pkg/config/confapi"
	"gopkg.in/yaml.v3"
)

type Single struct {
	Spec *StageSpec

	State struct {
		Stages []confapi.StageState
	}

	RevisionManager   *RevisionManager
	DependencyManager *DependencyManager
}

type StageSpec struct {
	Stages       []confapi.Stage
	Dependencies []DeploymentDependencySpec
}

func (c *Single) DeploymentDependencies() map[string]string {
	r := map[string]string{}

	for _, dep := range c.Spec.Dependencies {
		r[dep.Name] = dep.Version
	}

	return r
}

func (s *Single) GetStage(stageName string) (*StageSummary, error) {
	return getStageSummary(s.Spec.Stages, s.State.Stages, s.RevisionManager.Revisions, s.DependencyManager.StateMeta, stageName)
}

func (c *Single) Marshal() (string, error) {
	var buf bytes.Buffer

	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)

	state := &confapi.State{
		Stages:       c.State.Stages,
		Revisions:    c.RevisionManager.Revisions,
		Dependencies: c.DependencyManager.State,
		Meta: confapi.StateMeta{
			Dependencies: c.DependencyManager.StateMeta,
		},
	}

	if err := enc.Encode(state); err != nil {
		return "", fmt.Errorf("encoding state: %w", err)
	}

	got := buf.String()

	return got, nil
}

func (s *Single) GetRevisions() ([]confapi.Revision, error) {
	return s.RevisionManager.GetRevisions()
}

func (s *Single) GetCurrentRevision() (*confapi.Revision, error) {
	return s.RevisionManager.GetCurrentRevision()
}

func (s *Single) UpdateRevisions(depPattern string, requiredDepToConstraint map[string]string) error {
	return s.RevisionManager.UpdateRevisions(s.DependencyManager.State, depPattern, requiredDepToConstraint)
}

func (s *Single) UpdateDependencies(deps []string, f func(depName string) ([]DependencyEntry, error)) error {
	return s.DependencyManager.UpdateDependencies(deps, f)
}

func (s *Single) RequiresRevisionUpdate(stageName string) bool {
	specIdx := GetStageIndexForName(s.Spec.Stages, stageName)

	return specIdx == 0
}

func (s *Single) UpdateStage(stageName string) error {
	specIdx := GetStageIndexForName(s.Spec.Stages, stageName)

	var latest *confapi.Revision

	if specIdx == 0 {
		var err error

		latest, err = s.GetCurrentRevision()
		if err != nil {
			return fmt.Errorf("updating stage %q: %w", stageName, err)
		}
	}

	updated, err := updateStage(s.Spec.Stages, s.State.Stages, latest, stageName)
	if err != nil {
		return err
	}

	s.State.Stages = updated

	return nil
}
