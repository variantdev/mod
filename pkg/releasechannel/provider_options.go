package releasechannel

import (
	"github.com/go-logr/logr"
	"github.com/twpayne/go-vfs"
	"github.com/variantdev/mod/pkg/cmdsite"
	"github.com/variantdev/mod/pkg/vhttpget"
)

func Logger(logger logr.Logger) Option {
	return &loggerOption{l: logger}
}

type loggerOption struct {
	l logr.Logger
}

func (s *loggerOption) SetOption(r *Provider) error {
	r.Logger = s.l
	return nil
}

func FS(fs vfs.FS) Option {
	return &fsOption{f: fs}
}

type fsOption struct {
	f vfs.FS
}

func (s *fsOption) SetOption(r *Provider) error {
	r.fs = s.f
	return nil
}

func WD(wd string) Option {
	return &wdOption{d: wd}
}

type wdOption struct {
	d string
}

func (s *wdOption) SetOption(r *Provider) error {
	r.AbsWorkDir = s.d
	return nil
}

func GoGetterWD(wd string) Option {
	return &goGetterWdOption{d: wd}
}

type goGetterWdOption struct {
	d string
}

func (s *goGetterWdOption) SetOption(r *Provider) error {
	r.GoGetterAbsWorkDir = s.d
	return nil
}

func HttpGetter(g vhttpget.Getter) Option {
	return &httpGetterOption{g: g}
}

type httpGetterOption struct {
	g vhttpget.Getter
}

func (o *httpGetterOption) SetOption(r *Provider) error {
	r.httpGetter = o.g
	return nil
}

func Commander(rc cmdsite.RunCommand) Option {
	return &commanderOption{rc: rc}
}

type commanderOption struct {
	rc cmdsite.RunCommand
}

func (o *commanderOption) SetOption(r *Provider) error {
	r.cmdSite.RunCmd = o.rc
	return nil
}
