package variantmod

import (
	"fmt"
	"github.com/twpayne/go-vfs/vfst"
	"github.com/variantdev/mod/pkg/execversionmanager"
	"github.com/variantdev/mod/pkg/loginfra"
	"k8s.io/klog"
	"k8s.io/klog/klogr"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func init() {
	// See https://groups.google.com/forum/#!topic/Golang-nuts/uSFM8jG7yn4 for why this needs to be in init()
	fs := loginfra.NewFlagSet()
	fs = loginfra.AddKlogFlags(fs)
	fs.Set("v", "2")
	loginfra.Parse(fs)
}

func TestModule(t *testing.T) {
	mod := &Module{
		ValuesSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"foo": map[string]interface{}{
					"type": "string",
				},
			},
			"required": []string{
				"foo",
			},
		},
		Values: map[string]interface{}{
			"foo": "FOO",
		},
		Files: []File{
			{
				Path:   "test.yaml",
				Source: "git::https://github.com/cloudposse/helmfiles.git@releases/kiam.yaml?ref=0.40.0",
			},
		},
		Dependencies: map[string]*Module{},
	}

	fs, clean, err := vfst.NewTestFS(map[string]interface{}{})
	if err != nil {
		t.Fatal(err)
	}

	defer clean()
	log := klogr.New()
	klog.SetOutput(os.Stderr)
	// We don't use the testfs itself i.e. set FS(fs). Instead we just point the workdir to the tempdir managed by the testfs
	// So that all the cache files are created there and we're still able to clean up files easily.
	man, err := New(Logger(log), WD(fs.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	if err := man.run(mod); err != nil {
		t.Fatal(err)
	}
}

func TestModuleFile_Simple(t *testing.T) {
	files := map[string]interface{}{
		"/path/to/variant.mod": `
schema:
  properties:
    foo:
      type: string
  required:
  - foo

values:
  foo: FOO

files:
  dst.yaml:
    source: src.yaml.tpl
    arguments:
      foo: FOO2
      arg1: "{{.foo}}_BAR"
`,
		"/path/to/src.yaml.tpl": `{{.foo}}_{{.arg1}}`,
	}
	fs, clean, err := vfst.NewTestFS(files)
	if err != nil {
		t.Fatal(err)
	}
	defer clean()
	log := klogr.New()
	klog.SetOutput(os.Stderr)
	man, err := New(Logger(log), FS(fs), WD("/path/to"))
	if err != nil {
		t.Fatal(err)
	}
	if err := man.Run(); err != nil {
		t.Fatal(err)
	}

	actual, err := fs.ReadFile("/path/to/dst.yaml")
	if err != nil {
		t.Fatal(err)
	}

	if string(actual) != "FOO2_FOO_BAR" {
		t.Errorf("assertion failed: expected=%s, got=%s", "FOO2_FOO_BAR", string(actual))
	}
}

func TestModuleFile_Dependencies(t *testing.T) {
	if testing.Verbose() {
	}

	files := map[string]interface{}{
		"/path/to/variant.mod": `
name: myapp

schema:
  properties:
    foo:
      type: string
  required:
  - foo

values:
  foo: FOO

files:
  dst.yaml:
    source: src.yaml.tpl
    arguments:
      foo: FOO2
      arg1: "{{.foo}}_BAR_{{.coreos.version}}"
  myapp.txt:
    source: myapp.txt.tpl
    arguments:
      go: "{{.go.version}}"
      coreos: "{{.coreos.version}}"

dependencies:
  coreos:
    source: ./modules/coreos
    releaseChannel: stable
  go:
    source: ./modules/go
`,
		"/path/to/src.yaml.tpl":   `{{.foo}}_{{.arg1}}`,
		"/path/to/coreos.txt.tpl": `{{.ver}}`,
		"/path/to/myapp.txt.tpl":  `{{.go}}_{{.coreos}}`,
		"/path/to/modules/coreos/variant.mod": `name: coreos

files:
  coreos.txt:
    source: coreos.txt.tpl
    arguments:
      ver: "{{.version}}"

releaseChannels:
  stable:
    source: https://coreos.com/releases/releases-stable.json
    versions: "$"
    type: semver
    description: "$['{{.version}}'].release_notes"
`,
		"/path/to/modules/go/variant.mod": `name: go

values:
  version: "1.12.6"

executables:
  go:
    platforms:
      # Adds $VARIANT_MOD_PATH/mod/cache/CACHE_KEY/go/bin/go to $PATH
      # Or its shim at $VARIANT_MOD_PATH/MODULE_NAME/shims
    - source: https://dl.google.com/go/go{{.version}}.darwin-amd64.tar.gz@go/bin/go
      selector:
        matchLabels:
          os: darwin
          arch: amd64
    - source: https://dl.google.com/go/go{{.version}}.linux-amd64.tar.gz@go/bin/go
      selector:
        matchLabels:
          os: linux
          arch: amd64
`,
	}
	fs, clean, err := vfst.NewTestFS(files)
	if err != nil {
		t.Fatal(err)
	}
	defer clean()
	log := klogr.New()
	klog.SetOutput(os.Stderr)
	klog.V(2).Info(fmt.Sprintf("temp dir: %v", fs.TempDir()))
	man, err := New(Logger(log), FS(fs), WD("/path/to"), GoGetterWD(filepath.Join(fs.TempDir(), "path", "to")))
	if err != nil {
		t.Fatal(err)
	}

	mod, err := man.Load()
	if err != nil {
		t.Fatal(err)
	}

	if err := man.run(mod); err != nil {
		t.Fatal(err)
	}

	dstActual, err := fs.ReadFile("/path/to/dst.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if string(dstActual) != "FOO2_FOO_BAR_2079.6.0" {
		t.Errorf("assertion failed: expected=%s, got=%s", "FOO2_FOO_BAR_2079.6.0", string(dstActual))
	}

	coreosTxtActual, err := fs.ReadFile("/path/to/coreos.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(coreosTxtActual) != "2079.6.0" {
		t.Errorf("assertion failed: expected=%s, got=%s", "2079.6.0", string(coreosTxtActual))
	}

	myappTxtActual, err := fs.ReadFile("/path/to/myapp.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(myappTxtActual) != "1.12.6_2079.6.0" {
		t.Errorf("assertion failed: expected=%s, got=%s", "1.12.6_2079.6.0", string(myappTxtActual))
	}

	if _, err := fs.ReadFile("/path/to/variant.lock"); err == nil {
		t.Fatal("expected error not occurred")
	}

	if err := man.lock(mod); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lockActual, err := fs.ReadFile("/path/to/variant.lock")
	if err != nil {
		t.Fatal(err)
	}
	lockExpected := `coreos:
    version: 2079.6.0
`
	if string(lockActual) != lockExpected {
		t.Errorf("assertion failed: expected=%s, got=%s", lockExpected, string(lockActual))
	}

	sh, err := mod.Shell()
	if err != nil {
		t.Fatal(err)
	}

	stdout, _, err := sh.CaptureStrings("sh", []string{"-c", "go version"})
	if err != nil {
		t.Fatal(err)
	}

	actual := strings.TrimSpace(string(stdout))
	os, arch := execversionmanager.OsArch()
	if err != nil {
		t.Fatal(err)
	}
	expected := fmt.Sprintf("go version go1.12.6 %s/%s", os, arch)

	if actual != expected {
		t.Errorf("unexpected go version output: expected=\"%s\", got=\"%s\"", expected, actual)
	}
}

func TestModuleFile_DependenciesLocking(t *testing.T) {
	if testing.Verbose() {
	}

	files := map[string]interface{}{
		"/path/to/variant.mod": `
name: myapp

schema:
  properties:
    foo:
      type: string
  required:
  - foo

values:
  foo: FOO

files:
  dst.yaml:
    source: src.yaml.tpl
    arguments:
      foo: FOO2
      arg1: "{{.foo}}_BAR_{{.coreos.version}}"
  myapp.txt:
    source: myapp.txt.tpl
    arguments:
      go: "{{.go.version}}"
      coreos: "{{.coreos.version}}"

dependencies:
  coreos:
    source: ./modules/coreos
    releaseChannel: stable
  go:
    source: ./modules/go
`,
		"/path/to/src.yaml.tpl":   `{{.foo}}_{{.arg1}}`,
		"/path/to/coreos.txt.tpl": `{{.ver}}`,
		"/path/to/myapp.txt.tpl":  `{{.go}}_{{.coreos}}`,
		"/path/to/modules/coreos/variant.mod": `name: coreos

files:
  coreos.txt:
    source: coreos.txt.tpl
    arguments:
      ver: "{{.version}}"

releaseChannels:
  stable:
    source: https://coreos.com/releases/releases-stable.json
    versions: "$"
    type: semver
    description: "$['{{.version}}'].release_notes"
`,
		"/path/to/modules/go/variant.mod": `name: go

values:
  version: "1.12.6"

executables:
  go:
    platforms:
      # Adds $VARIANT_MOD_PATH/mod/cache/CACHE_KEY/go/bin/go to $PATH
      # Or its shim at $VARIANT_MOD_PATH/MODULE_NAME/shims
    - source: https://dl.google.com/go/go{{.version}}.darwin-amd64.tar.gz@go/bin/go
      selector:
        matchLabels:
          os: darwin
          arch: amd64
    - source: https://dl.google.com/go/go{{.version}}.linux-amd64.tar.gz@go/bin/go
      selector:
        matchLabels:
          os: linux
          arch: amd64
`,
		"/path/to/variant.lock": `
coreos:
  version: "2079.5.0"
`,
	}
	fs, clean, err := vfst.NewTestFS(files)
	if err != nil {
		t.Fatal(err)
	}
	defer clean()
	log := klogr.New()
	klog.SetOutput(os.Stderr)
	klog.V(2).Info(fmt.Sprintf("temp dir: %v", fs.TempDir()))
	man, err := New(Logger(log), FS(fs), WD("/path/to"), GoGetterWD(filepath.Join(fs.TempDir(), "path", "to")))
	if err != nil {
		t.Fatal(err)
	}

	mod, err := man.Load()
	if err != nil {
		t.Fatal(err)
	}

	if err := man.run(mod); err != nil {
		t.Fatal(err)
	}

	dstActual, err := fs.ReadFile("/path/to/dst.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if string(dstActual) != "FOO2_FOO_BAR_2079.5.0" {
		t.Errorf("assertion failed: expected=%s, got=%s", "FOO2_FOO_BAR_2079.5.0", string(dstActual))
	}

	coreosTxtActual, err := fs.ReadFile("/path/to/coreos.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(coreosTxtActual) != "2079.5.0" {
		t.Errorf("assertion failed: expected=%s, got=%s", "2079.5.0", string(coreosTxtActual))
	}

	myappTxtActual, err := fs.ReadFile("/path/to/myapp.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(myappTxtActual) != "1.12.6_2079.5.0" {
		t.Errorf("assertion failed: expected=%s, got=%s", "1.12.6_2079.5.0", string(myappTxtActual))
	}

	sh, err := mod.Shell()
	if err != nil {
		t.Fatal(err)
	}

	stdout, _, err := sh.CaptureStrings("sh", []string{"-c", "go version"})
	if err != nil {
		t.Fatal(err)
	}

	actual := strings.TrimSpace(string(stdout))
	os, arch := execversionmanager.OsArch()
	if err != nil {
		t.Fatal(err)
	}
	expected := fmt.Sprintf("go version go1.12.6 %s/%s", os, arch)

	if actual != expected {
		t.Errorf("unexpected go version output: expected=\"%s\", got=\"%s\"", expected, actual)
	}

	upMod, err := man.Up()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := man.lock(upMod); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lockActual, err := fs.ReadFile("/path/to/variant.lock")
	if err != nil {
		t.Fatal(err)
	}
	lockExpected := `coreos:
    version: 2079.6.0
`
	if string(lockActual) != lockExpected {
		t.Errorf("assertion failed: expected=%s, got=%s", lockExpected, string(lockActual))
	}
}
