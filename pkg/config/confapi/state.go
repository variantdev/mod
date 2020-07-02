package confapi

type State struct {
	Stages       []StageState               `yaml:"stages,omitempty"`
	Revisions    []Revision                 `yaml:"revisions,omitempty"`
	Dependencies map[string]DependencyState `yaml:"dependencies"`
	Meta         StateMeta                  `yaml:"meta,omitempty"`
	RawLock      string                     `yaml:"-"`
}

type StateMeta struct {
	Dependencies map[string]VersionedDependencyStateMeta `yaml:"dependencies,omitempty"`
}

//func (s *State) GetStage(stageName string) (*StageStateSummary, error) {
//	return getStageStateSummary(s.Stages, s.Revisions, stageName)
//}
//
