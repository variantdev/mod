package deploycoordinator

import (
	"fmt"
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

func Parse(config, state string) (*Single, error) {
	var coord Single

	var spec SingleSpec

	if err := yaml.Unmarshal([]byte(config), &spec); err != nil {
		return nil, fmt.Errorf("unmarshalling yaml: %w", err)
	}

	s, err := ParseSingleState(state)
	if err != nil {
		return nil, fmt.Errorf("parsing state: %w", err)
	}

	coord.SingleSpec = &spec
	coord.State = s

	return &coord, nil
}
