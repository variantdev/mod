package variantmod

import (
	"fmt"
	"github.com/variantdev/mod/pkg/cmdsite"
	"github.com/variantdev/mod/pkg/execversionmanager"
	"github.com/variantdev/mod/pkg/releasechannel"
	"io"
)

type Values map[string]interface{}

type Module struct {
	Alias string

	Values       Values
	ValuesSchema Values
	Files        []File

	ReleaseChannel *releasechannel.Provider
	Executable     *execversionmanager.ExecVM

	Dependencies              map[string]*Module
	DependencyReleaseChannels map[string]*releasechannel.Provider

	VersionLock map[string]interface{}
}

type File struct {
	Path      string
	Source    string
	Arguments map[string]interface{}
}

func merge(src, dst map[string]struct{}) {
	for k, v := range src {
		dst[k] = v
	}
}

func (m *Module) Walk(f func(*Module) error) error {
	for _, dep := range m.Dependencies {
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

func (m *Module) ListVersions(out io.Writer) error {
	releases, err := m.ReleaseChannel.GetVersions()
	if err != nil {
		return err
	}

	for _, r := range releases {
		fmt.Fprintf(out, "%s\t%s\n", r.Version, r.Description)
	}

	return nil
}

func (m *Module) ListDependencyVersions(name string, out io.Writer) error {
	dep, ok := m.DependencyReleaseChannels[name]
	if !ok {
		return fmt.Errorf("local module is unversioned: %s", name)
	}
	releases, err := dep.GetVersions()
	if err != nil {
		return err
	}

	for _, r := range releases {
		fmt.Fprintf(out, "%s\t%s\n", r.Version, r.Description)
	}

	return nil
}
