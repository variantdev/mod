package deploycoordinator

import (
	"errors"
	"fmt"
	"github.com/Masterminds/semver"
	"github.com/variantdev/mod/pkg/config/confapi"
)

type RevisionManager struct {
	Revisions []confapi.Revision `yaml:"revisions"`
}

func (s *RevisionManager) GetRevisions() ([]confapi.Revision, error) {
	return s.Revisions, nil
}

func (s *RevisionManager) GetCurrentRevision() (*confapi.Revision, error) {
	revs, err := s.GetRevisions()
	if err != nil {
		return nil, fmt.Errorf("getting latest dependency set revision: %w", err)
	}

	if len(revs) == 0 {
		return nil, errors.New("getting latest dependency set revision: not found")
	}

	return &revs[len(revs)-1], nil
}

func (s *RevisionManager) UpdateRevisions(deps map[string]confapi.DependencyState, depPattern string, requiredDepToConstraint map[string]string) error {
	current, err := s.GetCurrentRevision()
	if err != nil {
		return fmt.Errorf("updating revisions: %w", err)
	}

	updated, err := updateRevisions(deps, current, s.Revisions, depPattern, requiredDepToConstraint)
	if err != nil {
		return err
	}

	s.Revisions = updated

	return nil
}

func updateRevisions(fetchedDeps map[string]confapi.DependencyState, current *confapi.Revision, revs []confapi.Revision, depPattern string, requiredDepToConstraint map[string]string) ([]confapi.Revision, error) {
	vers := map[string]string{}

	var anyNew bool

	var err error

	for dep, constraintStr := range requiredDepToConstraint {
		var constraints *semver.Constraints

		if constraintStr != "" {
			constraints, err = semver.NewConstraint(constraintStr)
			if err != nil {
				return nil, fmt.Errorf("parsing version constraint %q: %w", constraintStr, err)
			}
		}

		var curV *semver.Version

		curVer, ok := current.Versions[dep]
		if !ok {
			curV = semver.MustParse("v0.0.0")
		} else {
			curV, err = semver.NewVersion(curVer)
			if err != nil {
				return nil, fmt.Errorf("getting curV: %w", err)
			}
		}

		depHistory, ok := fetchedDeps[dep]
		if !ok {
			return nil, fmt.Errorf("no dependencies found for %q: please retrieve latest versions for %q", dep, dep)
		}

		var verToUse string

		if depPattern != "*" && depPattern != dep {
			verToUse = curVer
		} else {
			for i := len(depHistory.Versions) - 1; i >= 0; i-- {
				newerVer := fetchedDeps[dep].Versions[i]

				if newerVer == "" {
					return nil, fmt.Errorf("invalid state: empty version number found for %s's version at index %d", dep, i)
				}

				newerSemver, err := semver.NewVersion(newerVer)
				if err != nil {
					return nil, fmt.Errorf("getting newerSemver: %w", err)
				}

				if !newerSemver.GreaterThan(curV) {
					verToUse = curVer

					break
				}

				if constraints == nil || constraints.Check(newerSemver) {
					anyNew = true
					verToUse = newerVer

					break
				}
			}
		}

		if verToUse == "" {
			return nil, fmt.Errorf("unresolved dependency %q: no dependency satisfying %q", dep, constraintStr)
		}

		vers[dep] = verToUse
	}

	for name, dep := range fetchedDeps {
		_, ok := current.Versions[name]
		if !ok {
			anyNew = true
			vers[name] = dep.Versions[len(dep.Versions)-1]
		}
	}

	if anyNew {
		newRev := confapi.Revision{
			ID:       current.ID + 1,
			Versions: vers,
		}

		revs = append(revs, newRev)
	}

	return revs, nil
}
