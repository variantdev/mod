package deploycoordinator

import (
	"fmt"
	"github.com/variantdev/mod/pkg/config/confapi"
	"gopkg.in/yaml.v3"
)

func ParseSingleState(doc string) (*confapi.State, error) {
	var state confapi.State

	state.Meta.Dependencies = map[string]confapi.VersionedDependencyStateMeta{}

	if err := yaml.Unmarshal([]byte(doc), &state); err != nil {
		return nil, fmt.Errorf("unmarshalling yaml: %w", err)
	}

	return &state, nil
}

