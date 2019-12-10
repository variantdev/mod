package variantmod

import (
	"fmt"
	"github.com/variantdev/mod/pkg/cmdsite"
	"github.com/variantdev/mod/pkg/execversionmanager"
	"github.com/variantdev/mod/pkg/releasetracker"
	"io"
)

type Values map[string]interface{}

type Module struct {
	Alias string

	Values       Values
	ValuesSchema Values
	Files        []File
	TextReplaces []TextReplace
	Yamls        []YamlPatch

	ReleaseChannel *releasetracker.Tracker
	Executable     *execversionmanager.ExecVM

	Submodules      map[string]*Module
	ReleaseTrackers map[string]*releasetracker.Tracker

	VersionLock ModVersionLock
}

type ModVersionLock struct {
	Dependencies map[string]DepVersionLock `yaml:"dependencies"`
	RawLock string `yaml:"-"`
}

type DepVersionLock struct {
	Version         string `yaml:"version"`
	PreviousVersion string `yaml:"previousVersion,omitempty"`
}

func (l ModVersionLock) ToMap() map[string]interface{} {
	return map[string]interface{}{"Dependencies": l.ToDepsMap(), "RawLock": l.RawLock}
}

func (l ModVersionLock) ToDepsMap() map[string]interface{} {
	deps := map[string]interface{}{}
	for k, v := range l.Dependencies {
		m := map[string]interface{}{"version": v.Version}
		if v.PreviousVersion != "" {
			m["previousVersion"] = v.PreviousVersion
		}
		deps[k] = m
	}
	return deps
}

type File struct {
	Path      string
	Source    string
	Arguments map[string]interface{}
}

type TextReplace struct {
	Path     string
	From, To string
}

type YamlPatch struct {
	Path    string
	Patches []Patch
}

type Patch struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value"`
	From  string      `json:"from"`
}

func merge(src, dst map[string]struct{}) {
	for k, v := range src {
		dst[k] = v
	}
}

func (m *Module) Walk(f func(*Module) error) error {
	for _, dep := range m.Submodules {
		if err := dep.Walk(f); err != nil {
			return err
		}
	}
	return f(m)
}

func (m *Module) Dirs() (map[string]struct{}, error) {
	dirs := map[string]struct{}{}

	if err := m.Walk(func(dep *Module) error {
		subdirs, err := dep.Executable.Dirs()
		if err != nil {
			return err
		}
		merge(subdirs, dirs)
		return nil
	}); err != nil {
		return nil, err
	}

	return dirs, nil
}

func (m *Module) Shell() (*cmdsite.CommandSite, error) {
	dirs, err := m.Dirs()
	if err != nil {
		return nil, err
	}

	return m.Executable.ShellFromDirs(dirs), nil
}

func (m *Module) ListVersions(depName string, out io.Writer) error {
	dep, ok := m.ReleaseTrackers[depName]
	if !ok {
		return fmt.Errorf("local module is unversioned: %s", depName)
	}
	releases, err := dep.GetReleases()
	if err != nil {
		return err
	}

	for _, r := range releases {
		fmt.Fprintf(out, "%s\t%s\n", r.Version, r.Description)
	}

	return nil
}
