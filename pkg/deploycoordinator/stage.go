package deploycoordinator

import (
	"fmt"
	"github.com/variantdev/mod/pkg/config/confapi"
)

func GetStageIndexForName(specs []confapi.Stage, stageName string) int {
	specIdx := -1

	for i, stg := range specs {
		if stg.Name == stageName {
			specIdx = i
			break
		}
	}

	return specIdx
}

func updateStage(specs []confapi.Stage, states []confapi.StageState, latest *confapi.Revision, stageName string) ([]confapi.StageState, error) {
	specIdx := GetStageIndexForName(specs, stageName)

	if specIdx == -1 {
		return nil, fmt.Errorf("getting stage: %q not found", stageName)
	}

	spec := specs[specIdx]

	if len(states) <= specIdx {
		if len(states) != len(specs)-1 {
			return nil, fmt.Errorf("stage %q must be updated before %q", specs[specIdx-1].Name, specs[specIdx].Name)
		}
		states = append(states, confapi.StageState{
			Name:     spec.Name,
			Revision: -1,
		})
	}

	var revisionID int

	if specIdx > 0 {
		revisionID = states[specIdx-1].Revision
	} else {
		revisionID = latest.ID
	}

	if revisionID > states[specIdx].Revision {
		states[specIdx].Revision = revisionID
	}

	return states, nil
}

type StageSummary struct {
	*StageStateSummary
	*confapi.Stage
	Deps map[string]DependencyEntry
}

type Deployment struct {
	Environment string
	Values      map[string]interface{}
}

func (s StageSummary) GetDeployments() []Deployment {
	var r []Deployment

	for _, env := range s.Environments {
		ds := map[string]interface{}{}

		for name, d := range s.Deps {
			x := map[string]interface{}{}

			x["version"] = d.Version

			for k, v := range d.Meta {
				x[k] = v
			}

			ds[name] = x
		}

		values := map[string]interface{}{
			"stage": map[string]interface{}{
				"environment":  env,
				"dependencies": ds,
			},
		}

		r = append(r, Deployment{
			Environment: env,
			Values:      values,
		})
	}

	return r
}

func getStageSummary(stages []confapi.Stage, states []confapi.StageState, revisions []confapi.Revision, meta map[string]confapi.VersionedDependencyStateMeta, stageName string) (*StageSummary, error) {
	stateSummary, err := getStageStateSummary(states, revisions, stageName)
	if err != nil {
		return nil, err
	}

	stageIdx := -1

	for i, stg := range stages {
		if stg.Name == stageName {
			stageIdx = i
			break
		}
	}

	if stageIdx == -1 {
		return nil, fmt.Errorf("getting stage: %q not found", stageName)
	}

	stg := stages[stageIdx]

	deps := map[string]DependencyEntry{}

	for d, ver := range stateSummary.Versions {
		meta := meta[d][ver]

		deps[d] = DependencyEntry{Version: ver, Meta: meta}
	}

	return &StageSummary{
		StageStateSummary: stateSummary,
		Stage:             &stg,
		Deps:              deps,
	}, nil
}

func getStageStateSummary(stages []confapi.StageState, revisions []confapi.Revision, stageName string) (*StageStateSummary, error) {
	stageIdx := -1

	for i, stg := range stages {
		if stg.Name == stageName {
			stageIdx = i
			break
		}
	}

	if stageIdx == -1 {
		return nil, fmt.Errorf("getting stage: %q not found", stageName)
	}

	stg := stages[stageIdx]

	if len(revisions) == 0 {
		return nil, fmt.Errorf("[bug] no revisions found for stage %q", stg.Name)
	}

	var found *confapi.Revision

	for i, rev := range revisions {
		if stg.Revision == rev.ID {
			found = &revisions[i]
		}
	}

	if found == nil {
		return nil, fmt.Errorf("[invalid state] getting revision: stage %s's revision %d not found in %+v", stg.Name, stg.Revision, revisions)
	}

	summary := StageStateSummary{
		Versions: found.Versions,
	}

	return &summary, nil
}
