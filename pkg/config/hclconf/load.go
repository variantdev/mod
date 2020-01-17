package hclconf

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	hcl2 "github.com/hashicorp/hcl/v2"
	gohcl2 "github.com/hashicorp/hcl/v2/gohcl"
	hcl2parse "github.com/hashicorp/hcl/v2/hclparse"
	"github.com/zclconf/go-cty/cty"
)

// FindFilesWithExt walks the given path and returns the files ending whose ext is .<tpe>
// Also, it returns the path if the path is just a file and a HCL file
func FindFilesWithExt(path string, tpe string) ([]string, error) {
	var (
		files []string
		err   error
	)
	fi, err := os.Stat(path)
	if err != nil {
		return files, err
	}
	ext := "." + tpe
	if fi.IsDir() {
		return filepath.Glob(filepath.Join(path, "*"+ext+"*"))
	}
	switch filepath.Ext(path) {
	case ext:
	case ext + ".json":
		files = append(files, path)
	}
	return files, err
}

type Loader struct {
	readFile func(string) ([]byte, error)
	Parser   *hcl2parse.Parser
}

type configurable struct {
	Body hcl2.Body
}

func (t *configurable) HCL2Config() (*Config, error) {
	config := &Config{}

	ctx := &hcl2.EvalContext{
		Variables: map[string]cty.Value{
			"name": cty.StringVal("Ermintrude"),
			"age":  cty.NumberIntVal(32),
			"path": cty.ObjectVal(map[string]cty.Value{
				"root":    cty.StringVal("rootDir"),
				"module":  cty.StringVal("moduleDir"),
				"current": cty.StringVal("currentDir"),
			}),
		},
	}

	diags := gohcl2.DecodeBody(t.Body, ctx, config)
	if diags.HasErrors() {
		// We return the diags as an implementation of error, which the
		// caller than then type-assert if desired to recover the individual
		// diagnostics.
		// FIXME: The current API gives us no way to return warnings in the
		// absence of any errors.
		return config, diags
	}

	return config, nil
}

func (l Loader) loadFile(filenames ...string) (*configurable, []*hcl2.File, error) {
	var files []*hcl2.File
	var diags hcl2.Diagnostics

	for _, filename := range filenames {
		var f *hcl2.File
		var ds hcl2.Diagnostics
		src, err := l.readFile(filename)
		if err != nil {
			return nil, nil, err
		}
		if strings.HasSuffix(filename, ".json") {
			f, ds = l.Parser.ParseJSON(src, filename)
		} else {
			f, ds = l.Parser.ParseHCL(src, filename)
		}
		files = append(files, f)
		diags = append(diags, ds...)
	}

	if diags.HasErrors() {
		return nil, files, diags
	}

	body := hcl2.MergeFiles(files)

	return &configurable{
		Body: body,
	}, files, nil
}

type App struct {
	Files  map[string]*hcl2.File
	Config *Config
}

func NewLoader(readFile func(string)([]byte, error)) *Loader {
	if readFile == nil {
		readFile = ioutil.ReadFile
	}
	l := &Loader{
		Parser: hcl2parse.NewParser(),
		readFile: readFile,
	}
	return l
}

func (l *Loader) LoadFile(file string) (*App, error) {
	return l.loadFiles([]string{file})
}

func (l *Loader) LoadDirectory(dir string) (*App, error) {
	files, err := FindFilesWithExt(dir, "variantmod")
	if err != nil {
		return nil, fmt.Errorf("failed to get .variantmod files: %v", err)
	}

	return l.loadFiles(files)
}

func (l *Loader) loadFiles(files []string) (*App, error) {
	c, hclFiles, err := l.loadFile(files...)
	nameToFiles := map[string]*hcl2.File{}
	for i := range files {
		nameToFiles[files[i]] = hclFiles[i]
	}

	app := &App{
		Files: nameToFiles,
	}
	if err != nil {
		return app, err
	}

	cc, err := c.HCL2Config()
	if err != nil {
		return app, err
	}

	moduleByName := map[string]Module{}
	for _, j := range cc.Modules {
		moduleByName[j.Name] = j
	}

	app.Config = cc

	return app, nil
}
