package deploycoordinator

import "fmt"

func updateStage(stages []StageState, latest *Revision, stageName string) error {
	stageIdx := -1

	for i, stg := range stages {
		if stg.Name == stageName {
			stageIdx = i
			break
		}
	}

	if stageIdx == -1 {
		return fmt.Errorf("getting stage: %q not found", stageName)
	}

	var revisionID int

	if stageIdx > 0 {
		revisionID = stages[stageIdx-1].DependencySetRevisionID
	} else {
		revisionID = latest.ID
	}

	if revisionID > stages[stageIdx].DependencySetRevisionID {
		stages[stageIdx].DependencySetRevisionID = revisionID
	}

	return nil
}

type StageSummary struct {
	*StageStateSummary
	*DeploymentStageSpec
}

func getStageSummary(stages []DeploymentStageSpec, states []StageState, revisions []Revision, stageName string) (*StageSummary, error) {
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

	return &StageSummary{
		StageStateSummary:   stateSummary,
		DeploymentStageSpec: &stg,
	}, nil
}

func getStageStateSummary(stages []StageState, revisions []Revision, stageName string) (*StageStateSummary, error) {
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

	var found *Revision

	for i, rev := range revisions {
		if stg.DependencySetRevisionID == rev.ID {
			found = &revisions[i]
		}
	}

	summary := StageStateSummary{
		Versions: found.Versions,
	}

	return &summary, nil
}
