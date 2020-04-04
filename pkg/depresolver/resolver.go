package depresolver

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-logr/logr"
	"github.com/hashicorp/go-getter"
	"github.com/hashicorp/go-getter/helper/url"
	"github.com/twpayne/go-vfs"
	"gopkg.in/yaml.v3"
	"k8s.io/klog/klogr"
	"path/filepath"
	"strings"
)

// Resolver is the caching dependency resolver for variantmod that
// resolves a string of either an go-getter URL to a local dir containing the assets hosted on the URL.
type Resolver struct {
	Logger logr.Logger

	// Home is the home directory for helmfile. Usually this points to $HOME of the user running helmfile.
	// variantmod saves fetched remote files into .variant/mod/cache under home
	Home string

	// GoGetterHome is the working directory to be used by go-getter for downloading the dependency
	// This differs from Home only when testing with go-vfs/vtfs
	GoGetterHome string

	// Getter is the underlying implementation of getter used for fetching remote files
	Getter Getter

	// ReadFile is the implementation of the file reader that reads a local file from the specified path.
	// Inject any implementation of your choice, like an im-memory impl for testing, ioutil.ReadFile for the real-world use.
	ReadFile   func(string) ([]byte, error)
	DirExists  func(string) bool
	FileExists func(string) bool

	fs vfs.FS
}

type Option interface {
	SetOption(*Resolver) error
}

func Home(dir string) Option {
	return &homeOption{d: dir}
}

type homeOption struct {
	d string
}

func (s *homeOption) SetOption(r *Resolver) error {
	r.Home = s.d
	return nil
}

func GoGetterHome(dir string) Option {
	return &goGetterHomeOption{d: dir}
}

type goGetterHomeOption struct {
	d string
}

func (s *goGetterHomeOption) SetOption(r *Resolver) error {
	r.GoGetterHome = s.d
	return nil
}

func Logger(logger logr.Logger) Option {
	return &loggerOption{l: logger}
}

type loggerOption struct {
	l logr.Logger
}

func (s *loggerOption) SetOption(r *Resolver) error {
	r.Logger = s.l
	return nil
}

func FS(fs vfs.FS) Option {
	return &fsOption{f: fs}
}

type fsOption struct {
	f vfs.FS
}

func (s *fsOption) SetOption(r *Resolver) error {
	r.fs = s.f
	return nil
}

func New(opts ...Option) (*Resolver, error) {
	r := &Resolver{}

	for _, o := range opts {
		if err := o.SetOption(r); err != nil {
			return nil, err
		}
	}

	if r.GoGetterHome == "" {
		r.GoGetterHome = r.Home
	}

	if r.Logger == nil {
		r.Logger = klogr.New()
	}

	if r.fs == nil {
		r.fs = vfs.HostOSFS
	}

	if r.FileExists == nil {
		r.FileExists = func(path string) bool {
			s, err := r.fs.Stat(path)
			return err == nil && s != nil && !s.IsDir()
		}
	}

	if r.DirExists == nil {
		r.DirExists = func(path string) bool {
			s, err := r.fs.Stat(path)
			return err == nil && s != nil && s.IsDir()
		}
	}

	if r.ReadFile == nil {
		r.ReadFile = r.fs.ReadFile
	}

	if r.Getter == nil {
		r.Getter = &GoGetter{Logger: r.Logger}
	}

	return r, nil
}

func (r *Resolver) Unmarshal(src string, dst interface{}) error {
	bytes, err := r.GetBytes(src)
	if err != nil {
		return err
	}

	strs := strings.Split(src, "/")
	file := strs[len(strs)-1]
	ext := filepath.Ext(file)

	{
		r.Logger.V(1).Info("unmarshalling", "bytes", string(bytes))

		var err error
		switch ext {
		case "json":
			err = json.Unmarshal(bytes, dst)
		default:
			err = yaml.Unmarshal(bytes, dst)
		}

		r.Logger.V(1).Info("unmarshalled", "dst", dst)

		if err != nil {
			return err
		}
	}

	return nil
}

func (r *Resolver) GetBytes(goGetterSrc string) ([]byte, error) {
	f, err := r.FetchFile(goGetterSrc)
	if err != nil {
		return nil, err
	}

	bytes, err := r.ReadFile(f)
	if err != nil {
		return nil, fmt.Errorf("read file: %v", err)
	}

	return bytes, nil
}

// ResolveFile takes an URL to a remote file or a path to a local file.
//
// If the argument was an URL, it fetches the remote directory contained within the URL,
// and returns the path to the file in the fetched directory
func (r *Resolver) ResolveFile(urlOrPath string) (string, error) {
	fetched, err := r.FetchFile(urlOrPath)
	if err != nil {
		switch err.(type) {
		case InvalidURLError:
			return urlOrPath, nil
		}
		return "", err
	}
	return fetched, nil
}

// ResolveDir takes an URL to a remote directory or a path to a local directory.
//
// If the argument was an URL, it fetches the remote directory contained within the URL,
// and returns the path to the the fetched directory
func (r *Resolver) ResolveDir(urlOrPath string) (string, error) {
	fetched, err := r.FetchDir(urlOrPath)
	if err != nil {
		switch err.(type) {
		case InvalidURLError:
			return urlOrPath, nil
		}
		return "", err
	}
	return fetched, nil
}

type InvalidURLError struct {
	err string
}

func (e InvalidURLError) Error() string {
	return e.err
}

type Source struct {
	Getter, Scheme, User, Host, Dir, File, RawQuery string
	IsFileMode                                      bool
}

func IsRemote(goGetterSrc string) bool {
	if _, err := Parse(goGetterSrc); err != nil {
		return false
	}
	return true
}

func Parse(goGetterSrc string) (*Source, error) {
	items := strings.Split(goGetterSrc, "::")
	var getter string
	switch len(items) {
	case 2:
		getter = items[0]
		goGetterSrc = items[1]
	}

	u, err := url.Parse(goGetterSrc)
	if err != nil {
		return nil, InvalidURLError{err: fmt.Sprintf("parse url: %v", err)}
	}

	if u.Scheme == "" {
		return nil, InvalidURLError{err: fmt.Sprintf("parse url: missing scheme - probably this is a local file path? %s", goGetterSrc)}
	}

	var dir, file string
	var filemode bool
	pathComponents := strings.Split(u.Path, "@")
	if len(pathComponents) == 1 {
		dir = u.Path
		file = filepath.Base(u.Path)
		filemode = true
	} else if len(pathComponents) == 2 {
		dir = pathComponents[0]
		file = pathComponents[1]
	} else {
		return nil, fmt.Errorf("invalid src format: it must be `[<getter>::]<scheme>://<host>/<path/to/dir>@<path/to/file>?key1=val1&key2=val2: got %s", goGetterSrc)
	}

	return &Source{
		Getter:     getter,
		Scheme:     u.Scheme,
		User:       u.User.String(),
		Host:       u.Host,
		Dir:        dir,
		File:       file,
		RawQuery:   u.RawQuery,
		IsFileMode: filemode,
	}, nil
}

func (r *Resolver) FetchFile(goGetterSrc string) (string, error) {
	u, vfsLocalCopyDir, err := r.fetchSource(goGetterSrc)

	if err != nil {
		return "", err
	}

	file := u.File

	return filepath.Join(vfsLocalCopyDir, file), nil
}

func (r *Resolver) FetchDir(goGetterSrc string) (string, error) {
	_, vfsLocalCopyDir, err := r.fetchSource(goGetterSrc)

	if err != nil {
		return "", err
	}

	return vfsLocalCopyDir, nil
}

func (r *Resolver) fetchSource(goGetterSrc string) (*Source, string, error) {
	u, err := Parse(goGetterSrc)
	if err != nil {
		return nil, "", err
	}

	query := u.RawQuery

	var getterSrc string

	if u.User == "" {
		getterSrc = fmt.Sprintf("%s://%s%s", u.Scheme, u.Host, u.Dir)
	} else {
		getterSrc = fmt.Sprintf("%s://%s@%s%s", u.Scheme, u.User, u.Host, u.Dir)
	}

	if len(query) != 0 {
		getterSrc = strings.Join([]string{getterSrc, query}, "?")
	}

	r.Logger.V(1).Info("fetching", "getter", u.Getter, "scheme", u.Scheme, "host", u.Host, "dir", u.Dir, "file", u.File)

	// This should be shared across variant commands, so that they can share cache for the shared imports

	replacer := strings.NewReplacer(":", "", "//", "_", "/", "_", ".", "_", "&", "_", "?", ".")
	getterDstDir := replacer.Replace(getterSrc)

	cached := false

	vfsLocalCopyDir := filepath.Join(r.Home, getterDstDir)

	r.Logger.V(1).Info("fetching", "home", r.Home, "dst", getterDstDir, "cache-dir", vfsLocalCopyDir)

	{
		if r.FileExists(vfsLocalCopyDir) {
			return nil, "", fmt.Errorf("%s is not directory. please remove it so that variant could use it for dependency caching", getterDstDir)
		}

		if r.DirExists(vfsLocalCopyDir) {
			cached = true
		}
	}

	if !cached {
		if u.Getter != "" {
			getterSrc = u.Getter + "::" + getterSrc
		}

		r.Logger.V(1).Info("downloading", "src", getterSrc, "dir", r.Home, "dst", getterDstDir)
		r.Logger.V(1).Info("creating directories", "path", vfsLocalCopyDir)

		// go-getter silently fails when the destination directory already exists.
		// So we create directories down to the parent directory of the target.
		if err := vfs.MkdirAll(r.fs, filepath.Dir(vfsLocalCopyDir), 0755); err != nil {
			return nil, "", err
		}

		var getterDst string
		var fileMode bool
		if u.IsFileMode {
			fileMode = true
			getterDst = filepath.Join(getterDstDir, u.File)
		} else {
			getterDst = getterDstDir
		}

		r.Logger.V(1).Info("mkdirall succeeded", "dir", vfsLocalCopyDir)

		if err := r.Getter.Get(r.GoGetterHome, getterSrc, getterDst, fileMode); err != nil {
			if err2 := r.fs.RemoveAll(vfsLocalCopyDir); err2 != nil {
				return nil, "", err2
			}
			return nil, "", err
		}
	}

	return u, vfsLocalCopyDir, nil
}

type Getter interface {
	Get(wd, src, dst string, fileMode bool) error
}

type GoGetter struct {
	Logger logr.Logger
}

func (g *GoGetter) Get(wd, src, dst string, fileMode bool) error {
	ctx := context.Background()

	get := &getter.Client{
		Ctx:     ctx,
		Src:     src,
		Dst:     filepath.Join(wd, dst),
		Pwd:     wd,
		Mode:    getter.ClientModeDir,
		Options: []getter.ClientOption{},
	}

	if fileMode {
		get.Mode = getter.ClientModeFile
	}

	g.Logger.V(1).Info("get", "client", *get, "wd", wd, "src", src, "dst", dst, "filemode", fileMode)

	if err := get.Get(); err != nil {
		return fmt.Errorf("get: %v", err)
	}

	return nil
}
