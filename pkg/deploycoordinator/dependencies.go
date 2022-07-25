package deploycoordinator

import (
	"fmt"

	"github.com/variantdev/mod/pkg/config/confapi"
	"github.com/variantdev/mod/pkg/semver"
)

type DependencyManager struct {
	State map[string]confapi.DependencyState

	StateMeta map[string]confapi.VersionedDependencyStateMeta
}

func (s *DependencyManager) AddDependencyUpdate(name, version string) error {
	return addDependencyUpdate(s.State, name, version)
}

func (s *DependencyManager) UpdateDependencies(deps []string, f func(depName string) ([]DependencyEntry, error)) error {
	return updateDependencies(deps, s.State, s.StateMeta, f)
}

func addDependencyUpdate(existingDeps map[string]confapi.DependencyState, name, version string) error {
	dep, ok := existingDeps[name]
	if !ok {
		return fmt.Errorf("getting dependency: %q not found", name)
	}

	latest := dep.Versions[len(dep.Versions)-1]

	latestV, err := semver.Parse(latest)
	if err != nil {
		return fmt.Errorf("parsing %q as semver: %w", latest, err)
	}

	newV, err := semver.Parse(version)
	if err != nil {
		return fmt.Errorf("parsing %q as semver: %w", version, err)
	}

	if newV.GreaterThan(latestV) {
		dep.Versions = append(dep.Versions, version)
	}

	return nil
}

func updateDependencies(deps []string, existingDeps map[string]confapi.DependencyState, meta map[string]confapi.VersionedDependencyStateMeta, f func(depName string) ([]DependencyEntry, error)) error {
	for _, k := range deps {
		dep := existingDeps[k]

		fetchedDeps, err := f(k)
		if err != nil {
			return fmt.Errorf("udpating dependencies: %w", err)
		}

		var latestV *semver.Version

		if len(dep.Versions) > 0 {
			latest := dep.Versions[len(dep.Versions)-1]

			latestV, err = semver.Parse(latest)
			if err != nil {
				return fmt.Errorf("parsing %q as semver: %w", latest, err)
			}
		}

		var updated bool

		for _, d := range fetchedDeps {
			if version := d.Version; version != "" {
				newV, err := semver.Parse(version)
				if err != nil {
					return fmt.Errorf("parsing %q as semver: %w", version, err)
				}

				if latestV == nil || newV.GreaterThan(latestV) {
					updated = true

					dep.Versions = append(dep.Versions, version)

					if len(d.Meta) > 0 {
						if _, ok := meta[k]; !ok {
							meta[k] = confapi.VersionedDependencyStateMeta{}
						}
						meta[k][version] = d.Meta
					}
				}
			}
		}

		if updated {
			existingDeps[k] = dep
		}
	}

	return nil
}
