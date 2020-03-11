package variantmod

import (
	"fmt"
	"github.com/go-logr/logr"
	"github.com/k-kinzal/aliases/pkg/aliases/yaml"
	"github.com/twpayne/go-vfs"
	"github.com/variantdev/mod/pkg/cmdsite"
	"github.com/variantdev/mod/pkg/config/confapi"
	"github.com/variantdev/mod/pkg/depresolver"
	"github.com/variantdev/mod/pkg/execversionmanager"
	"github.com/variantdev/mod/pkg/releasetracker"
	"regexp"
	"strings"
)

func NewLoaderFromManager(man *ModuleManager) *ModuleLoader {
	return &ModuleLoader{
		FS:                 man.fs,
		RunCommand:         man.cmdr,
		Logger:             man.Logger,
		AbsWorkDir:         man.AbsWorkDir,
		GoGetterAbsWorkDir: man.goGetterAbsWorkDir,
		dep:                man.dep,
	}
}

type ModuleLoader struct {
	FS         vfs.FS
	RunCommand cmdsite.RunCommand
	Logger     logr.Logger

	AbsWorkDir         string
	GoGetterAbsWorkDir string

	dep *depresolver.Resolver
}

func (m *ModuleLoader) LoadModule(params confapi.ModuleParams) (mod *Module, err error) {
	defer func() {
		if err != nil {
			m.Logger.Error(err, "LoadModule", "params", params)
		}
	}()

	var conf *confapi.Module

	if params.Module != nil {
		conf = params.Module
	} else {
		matched := strings.HasSuffix(params.Source, ".variantmod")
		if matched {
			conf, err = m.loadHclModule(params)
		} else {
			conf, err = m.loadYamlModule(params)
		}
		if err != nil {
			return nil, err
		}
	}

	return m.InitModule(params, *conf)
}

func (m *ModuleLoader) InitModule(params confapi.ModuleParams, mod confapi.Module) (*Module, error) {
	vals := mergeByOverwrite(Values{}, mod.Defaults, params.Arguments, params.LockedVersions.ToMap())

	verLock := params.LockedVersions

	trackers := map[string]*releasetracker.Tracker{}

	for alias, dep := range mod.Releases {
		var r releasetracker.Spec

		var err error
		r.VersionsFrom.Exec.Args = dep.VersionsFrom.Exec.Args
		r.VersionsFrom.Exec.Command = dep.VersionsFrom.Exec.Command
		if dep.VersionsFrom.DockerImageTags.Source != nil {
			r.VersionsFrom.DockerImageTags.Source, err = dep.VersionsFrom.DockerImageTags.Source(vals)
			if err != nil {
				return nil, err
			}
		}
		if dep.VersionsFrom.GitHubReleases.Source != nil {
			r.VersionsFrom.GitHubReleases.Source, err = dep.VersionsFrom.GitHubReleases.Source(vals)
			if err != nil {
				return nil, err
			}
			r.VersionsFrom.GitHubReleases.Host = dep.VersionsFrom.GitHubReleases.Host
		}
		if dep.VersionsFrom.GitHubTags.Source != nil {
			r.VersionsFrom.GitHubTags.Source, err = dep.VersionsFrom.GitHubTags.Source(vals)
			if err != nil {
				return nil, err
			}
			r.VersionsFrom.GitHubTags.Host = dep.VersionsFrom.GitHubTags.Host
		}
		if dep.VersionsFrom.GitTags.Source != nil {
			r.VersionsFrom.GitTags.Source, err = dep.VersionsFrom.GitTags.Source(vals)
			if err != nil {
				return nil, err
			}
		}
		if dep.VersionsFrom.JSONPath.Source != nil {
			r.VersionsFrom.JSONPath.Source, err = dep.VersionsFrom.JSONPath.Source(vals)
			if err != nil {
				return nil, err
			}
			r.VersionsFrom.JSONPath.Description = dep.VersionsFrom.JSONPath.Description
			r.VersionsFrom.JSONPath.Versions = dep.VersionsFrom.JSONPath.Versions
		}

		if dep.VersionsFrom.ValidVersionPattern != "" {
			validVerPattern, err := regexp.Compile(dep.VersionsFrom.ValidVersionPattern)
			if err != nil {
				return nil, err
			}
			r.VersionsFrom.ValidVersionPattern = validVerPattern
		}

		var rc *releasetracker.Tracker
		rc, err = releasetracker.New(
			r,
			releasetracker.WD(m.AbsWorkDir),
			releasetracker.GoGetterWD(m.GoGetterAbsWorkDir),
			releasetracker.FS(m.FS),
			releasetracker.Commander(m.RunCommand),
		)
		if err != nil {
			return nil, err
		}

		trackers[alias] = rc
	}

	submods := map[string]*Module{}

	// Resolve versions of dependencies
	for alias, dep := range mod.Dependencies {
		if dep.Kind == "Module" {
			continue
		}

		preUp, ok := verLock.Dependencies[alias]
		if ok {
			if params.ForceUpdate {
				m.Logger.V(2).Info("finding tracker", "alias", alias, "trackers", trackers)
				tracker, ok := trackers[alias]
				if ok {
					m.Logger.V(2).Info("tracker found", "alias", alias)
					rel, err := tracker.Latest(dep.VersionConstraint)
					if err != nil {
						return nil, fmt.Errorf("resolving dependency %q: %w", alias, err)
					}

					if preUp.Version == rel.Version {
						m.Logger.V(2).Info("No update found", "alias", alias)
						continue
					}

					prev := verLock.Dependencies[alias].Version
					vals[alias] = Values{"version": rel.Version, "previousVersion": prev}

					verLock.Dependencies[alias] = confapi.DepVersionLock{
						Version:         rel.Version,
						PreviousVersion: prev,
					}
				} else {
					m.Logger.V(2).Info("no tracker found", "alias", alias)
				}
			} else {
				m.Logger.V(2).Info("tracker unused. lock version exists", "alias", alias, "verLock", verLock)
			}
		} else {
			m.Logger.V(2).Info("finding tracker", "alias", alias, "trackers", trackers)
			tracker, ok := trackers[alias]
			if ok {
				m.Logger.V(2).Info("tracker found", "alias", alias)
				rel, err := tracker.Latest(dep.VersionConstraint)
				if err != nil {
					return nil, fmt.Errorf("updating locked dependency %q: %w", alias, err)
				}
				vals[alias] = Values{"version": rel.Version}

				verLock.Dependencies[alias] = confapi.DepVersionLock{Version: rel.Version}
			} else {
				m.Logger.V(2).Info("no tracker found", "alias", alias)
			}
		}
	}

	// Regenerate template parameters from the up-to-date versions of dependencies
	vals = mergeByOverwrite(Values{}, mod.Defaults, params.Arguments, verLock.ToDepsMap(), verLock.ToMap())

	// Load sub-modules
	for alias, dep := range mod.Dependencies {
		if dep.Kind != "Module" {
			continue
		}

		dep.Alias = alias

		if dep.LockedVersions.Dependencies == nil {
			dep.LockedVersions.Dependencies = map[string]confapi.DepVersionLock{}
		}

		args, err := dep.Arguments(vals)
		if err != nil {
			m.Logger.V(2).Info("renderargs failed with values", "vals", vals)
			return nil, err
		}
		m.Logger.V(2).Info("loading dependency", "alias", alias, "dep", dep)
		ps := confapi.ModuleParams{
			Source:         dep.Source,
			Arguments:      args,
			Alias:          dep.Alias,
			LockedVersions: dep.LockedVersions,
			ForceUpdate:    dep.ForceUpdate,
		}
		submod, err := m.LoadModule(ps)
		if err != nil {
			return nil, err
		}
		submods[alias] = submod

		vals = mergeByOverwrite(Values{}, vals, map[string]interface{}{alias: submod.Values})
		//vals[alias] = submod.Values

		m.Logger.V(1).Info("loaded dependency", "alias", alias, "vals", vals)
	}

	execs := map[string]execversionmanager.Executable{}
	for k, v := range mod.Executables {
		var e execversionmanager.Executable
		for _, p := range v.Platforms {
			var src string
			if p.Source != nil {
				s, err := p.Source(vals)
				if err != nil {
					return nil, err
				}
				src = s
			}

			var docker yaml.OptionSpec
			if p.Docker != nil {
				d, err := p.Docker(vals)
				if err != nil {
					return nil, err
				}
				docker = *d
			}

			e.Platforms = append(e.Platforms, execversionmanager.Platform{
				Source: src,
				Docker: docker,
				Selector: execversionmanager.Selector{MatchLabels: execversionmanager.MatchLabels{
					OS:   p.Selector.MatchLabels.OS,
					Arch: p.Selector.MatchLabels.Arch,
				}},
			})
		}
		execs[k] = e
	}
	execset, err := execversionmanager.New(
		&execversionmanager.Config{
			Executables: execs,
		},
		execversionmanager.Values(vals),
		execversionmanager.WD(m.AbsWorkDir),
		execversionmanager.GoGetterWD(m.GoGetterAbsWorkDir),
		execversionmanager.FS(m.FS),
	)
	if err != nil {
		return nil, err
	}

	return &Module{
		Alias:           mod.Name,
		Values:          vals,
		ValuesSchema:    mod.ValuesSchema,
		Files:           mod.Files,
		Directories:     mod.Directories,
		RegexpReplaces:  mod.RegexpReplaces,
		TextReplaces:    mod.TextReplaces,
		Yamls:           mod.Yamls,
		Executable:      execset,
		Submodules:      submods,
		ReleaseTrackers: trackers,
		VersionLock:     verLock,
	}, nil
}
