package variantmod

import (
	"fmt"
	"io"

	"github.com/variantdev/mod/pkg/cmdsite"
	"github.com/variantdev/mod/pkg/config/confapi"
	"github.com/variantdev/mod/pkg/execversionmanager"
	"github.com/variantdev/mod/pkg/releasetracker"
)

type Values map[string]interface{}

type Module struct {
	Alias string

	Values         Values
	ValuesSchema   Values
	Files          []confapi.File
	TextReplaces   []confapi.TextReplace
	RegexpReplaces []confapi.RegexpReplace
	Yamls          []confapi.YamlPatch

	ReleaseChannel *releasetracker.Tracker
	Executable     *execversionmanager.ExecVM

	Submodules      map[string]*Module
	ReleaseTrackers map[string]*releasetracker.Tracker

	VersionLock confapi.ModVersionLock
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

func (m *Module) getDirs() (map[string]struct{}, error) {
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
	dirs, err := m.getDirs()
	if err != nil {
		return nil, err
	}

	return m.Executable.ShellFromDirs(dirs), nil
}

func (m *Module) executableDirs() ([]string, error) {
	dm, err := m.getDirs()
	if err != nil {
		return nil, err
	}

	ds := []string{}
	for k := range dm {
		ds = append(ds, k)
	}
	return ds, nil
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
