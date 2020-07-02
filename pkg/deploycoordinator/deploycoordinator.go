package deploycoordinator

import (
	"fmt"
	"github.com/variantdev/mod/pkg/config/confapi"
	"gopkg.in/yaml.v3"
)

func ParseMulti(config, state string) (*Multi, error) {
	var coord Multi

	var spec Spec

	if err := yaml.Unmarshal([]byte(config), &spec); err != nil {
		return nil, fmt.Errorf("unmarshalling yaml: %w", err)
	}

	s, err := ParseMultiState(state)
	if err != nil {
		return nil, fmt.Errorf("parsing state: %w", err)
	}

	coord.Spec = spec
	coord.MultiState = s

	return &coord, nil
}

func New(stages []confapi.Stage, state string) (*Single, error) {
	var coord Single

	var spec StageSpec

	spec.Stages = stages

	s, err := ParseSingleState(state)
	if err != nil {
		return nil, fmt.Errorf("parsing state: %w", err)
	}

	coord.Spec = &spec
	coord.State.Stages = s.Stages

	coord.DependencyManager = &DependencyManager{
		State:     s.Dependencies,
		StateMeta: s.Meta.Dependencies,
	}

	coord.RevisionManager = &RevisionManager{
		Revisions: s.Revisions,
	}

	return &coord, nil
}

func Parse(config, state string) (*Single, error) {
	var spec StageSpec

	if err := yaml.Unmarshal([]byte(config), &spec); err != nil {
		return nil, fmt.Errorf("unmarshalling yaml: %w", err)
	}

	coord, err := New(spec.Stages, state)
	if err != nil {
		return nil, fmt.Errorf("initializing sate: %w", err)
	}

	coord.Spec = &spec

	return coord, nil
}
