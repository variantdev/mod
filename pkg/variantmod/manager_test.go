package variantmod

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twpayne/go-vfs/vfst"
	"github.com/variantdev/mod/pkg/cmdsite"
	"github.com/variantdev/mod/pkg/config/confapi"
	"github.com/variantdev/mod/pkg/execversionmanager"
	"github.com/variantdev/mod/pkg/loginfra"
	"k8s.io/klog"
	"k8s.io/klog/klogr"
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
		Files: []confapi.File{
			{
				Path: "test.yaml",
				Source: func(_ map[string]interface{}) (string, error) {
					return "git::https://github.com/cloudposse/helmfiles.git@releases/kiam.yaml?ref=0.40.0", nil
				},
			},
		},
		Submodules: map[string]*Module{},
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
	if _, err := man.doBuild(mod); err != nil {
		t.Fatal(err)
	}
}

func TestModuleFile_Simple(t *testing.T) {
	files := map[string]interface{}{
		"/path/to/variant.mod": `
parameters:
  schema:
    properties:
      foo:
        type: string
    required:
    - foo
  defaults:
    foo: FOO

provisioners:
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
	if _, err := man.Build(); err != nil {
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

parameters:
  schema:
    properties:
      foo:
        type: string
    required:
    - foo
  defaults:
    foo: FOO

provisioners:
  files:
    dst.yaml:
      source: src.yaml.tpl
      arguments:
        foo: FOO2
        arg1: "{{.foo}}_BAR_{{.coreos.coreos.version}}"
    myapp.txt:
      source: myapp.txt.tpl
      arguments:
        go: "{{.go.version}}"
        coreos: "{{.coreos.coreos.version}}"

dependencies:
  coreos:
    kind: Module
    source: ./modules/coreos
  go:
    kind: Module
    source: ./modules/go
`,
		"/path/to/src.yaml.tpl":   `{{.foo}}_{{.arg1}}`,
		"/path/to/coreos.txt.tpl": `{{.ver}}`,
		"/path/to/myapp.txt.tpl":  `{{.go}}_{{.coreos}}`,
		"/path/to/modules/coreos/variant.mod": `name: coreos

provisioners:
  files:
    coreos.txt:
      source: coreos.txt.tpl
      arguments:
        ver: "{{.coreos.version}}"

releases:
  coreos:
    versionsFrom:
      jsonPath:
        source: https://coreos.com/releases/releases-stable.json
        versions: "$"
        type: semver
        description: "$['{{.version}}'].release_notes"

dependencies:
  coreos:
    version: 2135.5.0
`,
		"/path/to/modules/go/variant.mod": `name: go

parameters:
  defaults:
    version: "1.12.6"

provisioners:
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
    dockergo:
      platforms:
        # Adds $VARIANT_MOD_PATH/mod/cache/CACHE_KEY/dockergo to $PATH
        # Or its shim at $VARIANT_MOD_PATH/MODULE_NAME/shims
      - docker:
          command: go
          image: golang
          tag: '{{.version}}'
          volume:
          - $PWD:/work
          workdir: /work
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

	_, err = man.Build()
	if err != nil {
		t.Fatal(err)
	}

	dstActual, err := fs.ReadFile("/path/to/dst.yaml")
	if err != nil {
		t.Fatal(err)
	}
	dstExpected := "FOO2_FOO_BAR_2135.5.0"
	if string(dstActual) != dstExpected {
		t.Errorf("assertion failed: expected=%s, got=%s", dstExpected, string(dstActual))
	}

	coreosTxtActual, err := fs.ReadFile("/path/to/coreos.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(coreosTxtActual) != "2135.5.0" {
		t.Errorf("assertion failed: expected=%s, got=%s", "2135.5.0", string(coreosTxtActual))
	}

	myappTxtActual, err := fs.ReadFile("/path/to/myapp.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(myappTxtActual) != "1.12.6_2135.5.0" {
		t.Errorf("assertion failed: expected=%s, got=%s", "1.12.6_2135.5.0", string(myappTxtActual))
	}

	if _, err := fs.ReadFile("/path/to/variant.lock"); err == nil {
		t.Fatal("expected error not occurred")
	}

	if err := fs.WriteFile("/path/to/variant.lock", []byte("dependencies: {}\n"), 0755); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lockActual, err := fs.ReadFile("/path/to/variant.lock")
	if err != nil {
		t.Fatal(err)
	}
	lockExpected := `dependencies: {}
`
	if string(lockActual) != lockExpected {
		t.Errorf("assertion failed: expected=%s, got=%s", lockExpected, string(lockActual))
	}

	sh, err := man.Shell()
	if err != nil {
		t.Fatal(err)
	}

	{
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

	{
		stdout, _, err := sh.CaptureStrings("sh", []string{"-c", "dockergo version"})
		if err != nil {
			t.Fatal(err)
		}

		actual := strings.TrimSpace(string(stdout))
		expected := fmt.Sprintf("go version go1.12.6 %s/%s", "linux", "amd64")

		if actual != expected {
			t.Errorf("unexpected go version output: expected=\"%s\", got=\"%s\"", expected, actual)
		}
	}
}

func TestModuleFile_DependenciesLocking(t *testing.T) {
	if testing.Verbose() {
	}

	files := map[string]interface{}{
		"/path/to/variant.mod": `
name: myapp

parameters:
  schema:
    properties:
      foo:
        type: string
    required:
    - foo
  defaults:
    foo: FOO

provisioners:
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
    kind: Module
    source: ./modules/coreos
  go:
    kind: Module
    source: ./modules/go
`,
		"/path/to/src.yaml.tpl":   `{{.foo}}_{{.arg1}}`,
		"/path/to/coreos.txt.tpl": `{{.ver}}`,
		"/path/to/myapp.txt.tpl":  `{{.go}}_{{.coreos}}`,
		"/path/to/modules/coreos/variant.mod": `name: coreos

provisioners:
  files:
    coreos.txt:
      source: coreos.txt.tpl
      arguments:
        ver: "{{.coreos.version}}"

releases:
  coreos:
    versionsFrom:
      jsonPath:
        source: https://coreos.com/releases/releases-stable.json
        versions: "$"
        type: semver
        description: "$['{{.version}}'].release_notes"

dependencies:
  coreos:
    version: ">= 2135.5.0, < 2135.5.1"
`,
		"/path/to/modules/go/variant.mod": `name: go

parameters:
  defaults:
    version: "1.12.6"

provisioners:
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
dependencies:
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

	_, err = man.Build()
	if err != nil {
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
	if string(coreosTxtActual) != "2135.5.0" {
		t.Errorf("assertion failed: expected=%s, got=%s", "2135.5.0", string(coreosTxtActual))
	}

	myappTxtActual, err := fs.ReadFile("/path/to/myapp.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(myappTxtActual) != "1.12.6_2079.5.0" {
		t.Errorf("assertion failed: expected=%s, got=%s", "1.12.6_2079.5.0", string(myappTxtActual))
	}

	sh, err := man.Shell()
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

	err = man.Up()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lockActual, err := fs.ReadFile("/path/to/variant.lock")
	if err != nil {
		t.Fatal(err)
	}
	lockExpected := `dependencies:
  coreos:
    version: 2079.5.0
`
	if string(lockActual) != lockExpected {
		t.Errorf("assertion failed: expected=%s, got=%s", lockExpected, string(lockActual))
	}
}

func TestDependencyLockinge_EksK8s(t *testing.T) {
	if testing.Verbose() {
	}

	files := map[string]interface{}{
		"/path/to/variant.mod": `
name: myapp

provisioners:
  files:
    cluster.yaml:
      source: cluster.yaml.tpl
      arguments:
        name: k8s1
        region: ap-northeast-1
        version: "{{.Dependencies.k8s.version}}"
        prev: |
          {{if hasKey .Dependencies.k8s "previousVersion"}}{{.Dependencies.k8s.previousVersion}}{{end}}

dependencies:
  k8s:
    releasesFrom:
      exec:
        command: go
        args:
        - run
        - main.go
    version: "> 1.10"
`,
		"/path/to/cluster.yaml.tpl": `apiVersion: eksctl.io/v1alpha5
kind: ClusterConfig
metadata:
  name: {{.name}}
  region: {{.region}}
  version: {{.version}}
{{ $prev := trimSpace .prev -}}
{{ if ne $prev ""}}  prev: {{$prev}}
{{ end -}}
nodeGroups:
- name: ng1
  instanceType: m5.xlarge
  desiredCapacity: 1
  volumeSize: 100
  volumeType: gp2
  volumeEncrypted: true
`,
		"/path/to/variant.lock": `
dependencies:
  k8s:
    version: "1.10.13"
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

	expectedInput := cmdsite.NewInput("go", []string{"run", "main.go"}, map[string]string{})
	expectedStdout := `1.13.7
1.12.6
1.11.8
1.10.13
`
	cmdr := cmdsite.NewTester(map[cmdsite.CommandInput]cmdsite.CommandOutput{
		expectedInput: {Stdout: expectedStdout},
	})

	man, err := New(Logger(log), FS(fs), WD("/path/to"), GoGetterWD(filepath.Join(fs.TempDir(), "path", "to")), Commander(cmdr))
	if err != nil {
		t.Fatal(err)
	}

	_, err = man.Build()
	if err != nil {
		t.Fatal(err)
	}

	clusterYaml1Expected := `apiVersion: eksctl.io/v1alpha5
kind: ClusterConfig
metadata:
  name: k8s1
  region: ap-northeast-1
  version: 1.10.13
nodeGroups:
- name: ng1
  instanceType: m5.xlarge
  desiredCapacity: 1
  volumeSize: 100
  volumeType: gp2
  volumeEncrypted: true
`
	clusterYaml1Actual, err := fs.ReadFile("/path/to/cluster.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if string(clusterYaml1Actual) != clusterYaml1Expected {
		t.Errorf("assertion failed: expected=%s, got=%s", clusterYaml1Expected, string(clusterYaml1Actual))
	}

	err = man.Up()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lockActual, err := fs.ReadFile("/path/to/variant.lock")
	if err != nil {
		t.Fatal(err)
	}
	lockExpected := `dependencies:
  k8s:
    version: 1.13.7
    previousVersion: 1.10.13
`
	if string(lockActual) != lockExpected {
		t.Errorf("assertion failed: expected=%s, got=%s", lockExpected, string(lockActual))
	}

	_, err = man.Build()
	if err != nil {
		t.Fatal(err)
	}

	clusterYaml2Expected := `apiVersion: eksctl.io/v1alpha5
kind: ClusterConfig
metadata:
  name: k8s1
  region: ap-northeast-1
  version: 1.13.7
  prev: 1.10.13
nodeGroups:
- name: ng1
  instanceType: m5.xlarge
  desiredCapacity: 1
  volumeSize: 100
  volumeType: gp2
  volumeEncrypted: true
`
	clusterYaml2Actual, err := fs.ReadFile("/path/to/cluster.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if string(clusterYaml2Actual) != clusterYaml2Expected {
		t.Errorf("assertion failed: expected=%s, got=%s", clusterYaml2Expected, string(clusterYaml2Actual))
	}

}

func TestDependencyLockinge_EksK8s_TextReplace(t *testing.T) {
	if testing.Verbose() {
	}

	files := map[string]interface{}{
		"/path/to/variant.mod": `
name: myapp

provisioners:
  textReplace:
    cluster.yaml:
      from: |
        {{if hasKey .Dependencies.k8s "previousVersion"}}{{.Dependencies.k8s.previousVersion}}{{end}}
      to: "{{.Dependencies.k8s.version}}"

dependencies:
  k8s:
    releasesFrom:
      exec:
        command: go
        args:
        - run
        - main.go
    version: "> 1.10"
`,
		"/path/to/cluster.yaml": `apiVersion: eksctl.io/v1alpha5
kind: ClusterConfig
metadata:
  name: k8s1
  region: ap-northeast-1
  version: 1.10.13
nodeGroups:
- name: ng1
  instanceType: m5.xlarge
  desiredCapacity: 1
  volumeSize: 100
  volumeType: gp2
  volumeEncrypted: true
`,
		"/path/to/variant.lock": `
dependencies:
  k8s:
    version: "1.10.13"
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

	expectedInput := cmdsite.NewInput("go", []string{"run", "main.go"}, map[string]string{})
	expectedStdout := `1.13.7
1.12.6
1.11.8
1.10.13
`
	cmdr := cmdsite.NewTester(map[cmdsite.CommandInput]cmdsite.CommandOutput{
		expectedInput: {Stdout: expectedStdout},
	})

	man, err := New(Logger(log), FS(fs), WD("/path/to"), GoGetterWD(filepath.Join(fs.TempDir(), "path", "to")), Commander(cmdr))
	if err != nil {
		t.Fatal(err)
	}

	_, err = man.Build()
	if err != nil {
		t.Fatal(err)
	}

	clusterYaml1Expected := `apiVersion: eksctl.io/v1alpha5
kind: ClusterConfig
metadata:
  name: k8s1
  region: ap-northeast-1
  version: 1.10.13
nodeGroups:
- name: ng1
  instanceType: m5.xlarge
  desiredCapacity: 1
  volumeSize: 100
  volumeType: gp2
  volumeEncrypted: true
`
	clusterYaml1Actual, err := fs.ReadFile("/path/to/cluster.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if string(clusterYaml1Actual) != clusterYaml1Expected {
		t.Errorf("assertion failed: expected=%s, got=%s", clusterYaml1Expected, string(clusterYaml1Actual))
	}

	err = man.Up()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lockActual, err := fs.ReadFile("/path/to/variant.lock")
	if err != nil {
		t.Fatal(err)
	}
	lockExpected := `dependencies:
  k8s:
    version: 1.13.7
    previousVersion: 1.10.13
`
	if string(lockActual) != lockExpected {
		t.Errorf("assertion failed: expected=%s, got=%s", lockExpected, string(lockActual))
	}

	_, err = man.Build()
	if err != nil {
		t.Fatal(err)
	}

	clusterYaml2Expected := `apiVersion: eksctl.io/v1alpha5
kind: ClusterConfig
metadata:
  name: k8s1
  region: ap-northeast-1
  version: 1.13.7
nodeGroups:
- name: ng1
  instanceType: m5.xlarge
  desiredCapacity: 1
  volumeSize: 100
  volumeType: gp2
  volumeEncrypted: true
`
	clusterYaml2Actual, err := fs.ReadFile("/path/to/cluster.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if string(clusterYaml2Actual) != clusterYaml2Expected {
		t.Errorf("assertion failed: expected=%s, got=%s", clusterYaml2Expected, string(clusterYaml2Actual))
	}

}

func TestDependencyLockinge_Dockerfile_RegexpReplace(t *testing.T) {
	if testing.Verbose() {
	}

	files := map[string]interface{}{
		"/path/to/variant.mod": `
name: myapp

provisioners:
  regexpReplace:
    Dockerfile:
      from: "(FROM helmfile:)(\\S+)(\\s+)"
      to: "${1}{{.Dependencies.helmfile.version}}${3}"

dependencies:
  helmfile:
    releasesFrom:
      exec:
        command: go
        args:
        - run
        - main.go
    version: "> 0.94.0"
`,
		"/path/to/Dockerfile": `FROM helmfile:0.94.0

RUN echo hello
`,
		"/path/to/variant.lock": `
dependencies:
  helmfile:
    version: "0.94.1"
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

	expectedInput := cmdsite.NewInput("go", []string{"run", "main.go"}, map[string]string{})
	expectedStdout := `0.94.1
0.95.0
`
	cmdr := cmdsite.NewTester(map[cmdsite.CommandInput]cmdsite.CommandOutput{
		expectedInput: {Stdout: expectedStdout},
	})

	man, err := New(Logger(log), FS(fs), WD("/path/to"), GoGetterWD(filepath.Join(fs.TempDir(), "path", "to")), Commander(cmdr))
	if err != nil {
		t.Fatal(err)
	}

	_, err = man.Build()
	if err != nil {
		t.Fatal(err)
	}

	dockerfile1Expected := `FROM helmfile:0.94.1

RUN echo hello
`
	dockerfile1Actual, err := fs.ReadFile("/path/to/Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	if string(dockerfile1Actual) != dockerfile1Expected {
		t.Errorf("assertion failed: expected=%s, got=%s", dockerfile1Expected, string(dockerfile1Actual))
	}

	err = man.Up()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lockActual, err := fs.ReadFile("/path/to/variant.lock")
	if err != nil {
		t.Fatal(err)
	}
	lockExpected := `dependencies:
  helmfile:
    version: 0.95.0
    previousVersion: 0.94.1
`
	if string(lockActual) != lockExpected {
		t.Errorf("assertion failed: expected=%s, got=%s", lockExpected, string(lockActual))
	}

	_, err = man.Build()
	if err != nil {
		t.Fatal(err)
	}

	dockerfile2Expected := `FROM helmfile:0.95.0

RUN echo hello
`
	dockerfile2Actual, err := fs.ReadFile("/path/to/Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	if string(dockerfile2Actual) != dockerfile2Expected {
		t.Errorf("assertion failed: expected=%s, got=%s", dockerfile2Expected, string(dockerfile2Actual))
	}

}
