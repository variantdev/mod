package deploycoordinator

import (
	"fmt"
	"github.com/Masterminds/semver"
)

func addDependencyUpdate(deps map[string]*Dependency, name, version string) error {
	dep, ok := deps[name]
	if !ok {
		return fmt.Errorf("getting dependency: %q not found", name)
	}

	latest := dep.Versions[len(dep.Versions)-1]

	latestV, err := semver.NewVersion(latest)
	if err != nil {
		return fmt.Errorf("parsing %q as semver: %w", latest, err)
	}

	newV, err := semver.NewVersion(version)
	if err != nil {
		return fmt.Errorf("parsing %q as semver: %w", version, err)
	}

	if newV.GreaterThan(latestV) {
		dep.Versions = append(dep.Versions, version)
	}

	return nil
}

func updateDependencies(deps map[string]*Dependency, f func(depName string, current string) (string, error)) error {
	for k := range deps {
		v := deps[k]

		newVer, err := f(k, v.Versions[len(v.Versions)-1])
		if err != nil {
			return fmt.Errorf("udpating dependencies: %w", err)
		}

		if newVer != "" {
			v.Versions = append(v.Versions, newVer)
		}
	}

	return nil
}

func updateRevisions(fetchedDeps map[string]*Dependency, current *Revision, revs []Revision, depPattern string, requiredDepToConstraint map[string]string) ([]Revision, error) {
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
		newRev := Revision{
			ID:       current.ID + 1,
			Versions: vers,
		}

		revs = append(revs, newRev)

		return revs, nil
	}

	return nil, nil
}

