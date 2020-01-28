package variantmod

import (
	"fmt"
	"github.com/variantdev/mod/pkg/config/confapi"
	"path/filepath"
	"strings"

	"github.com/go-logr/logr"
	"github.com/twpayne/go-vfs"
	"github.com/variantdev/mod/pkg/cmdsite"
)

type Option interface {
	SetOption(r *ModuleManager) error
}

func Logger(logger logr.Logger) Option {
	return &loggerOption{l: logger}
}

type loggerOption struct {
	l logr.Logger
}

func (s *loggerOption) SetOption(r *ModuleManager) error {
	r.Logger = s.l
	return nil
}

func FS(fs vfs.FS) Option {
	return &fsOption{f: fs}
}

type fsOption struct {
	f vfs.FS
}

func (s *fsOption) SetOption(r *ModuleManager) error {
	r.fs = s.f
	return nil
}

func Commander(cmdr cmdsite.RunCommand) Option {
	return &cmdrOption{cmdr: cmdr}
}

type cmdrOption struct {
	cmdr cmdsite.RunCommand
}

func (s *cmdrOption) SetOption(r *ModuleManager) error {
	r.cmdr = s.cmdr
	return nil
}

func File(path string) Option {
	return &fileOption{p: path}
}

type fileOption struct {
	p string
}

func (f *fileOption) SetOption(r *ModuleManager) error {
	var lockFile string
	switch f.p {
	case ModuleFileName:
		lockFile = LockFileName
	default:
		dotExt := filepath.Ext(f.p)
		switch dotExt {
		case ".variantmod":
			lockFile = f.p + ".lock"
		case ".mod":
			lockFile = strings.TrimSuffix(f.p, ".mod") + ".lock"
		default:
			return fmt.Errorf("unsupported file extension %q: file is %q", dotExt, f.p)
		}
	}
	mf := &moduleFileOption{p: f.p}
	if err := mf.SetOption(r); err != nil {
		return err
	}
	lf := &lfOption{p: lockFile}
	return lf.SetOption(r)
}

func ModuleFile(path string) Option {
	return &moduleFileOption{p: path}
}

type moduleFileOption struct {
	p string
}

func (o *moduleFileOption) SetOption(m *ModuleManager) error {
	m.ModuleFile = o.p
	return nil
}

func LockFile(path string) Option {
	return &lfOption{p: path}
}

type lfOption struct {
	p string
}

func (o *lfOption) SetOption(m *ModuleManager) error {
	m.LockFile = o.p
	return nil
}

func WD(wd string) Option {
	return &wdOption{d: wd}
}

type wdOption struct {
	d string
}

func (s *wdOption) SetOption(r *ModuleManager) error {
	r.AbsWorkDir = s.d
	return nil
}

func GoGetterWD(goGetterWD string) Option {
	return &goGetterWDOption{d: goGetterWD}
}

type goGetterWDOption struct {
	d string
}

func (s *goGetterWDOption) SetOption(r *ModuleManager) error {
	r.goGetterAbsWorkDir = s.d
	return nil
}

type moduleOption struct {
	mod confapi.Module
}

func InMemoryModule(mod confapi.Module) Option {
	return &moduleOption{mod: mod}
}

func (m *moduleOption) SetOption(r *ModuleManager) error {
	r.Module = &m.mod
	return nil
}
