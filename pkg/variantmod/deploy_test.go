package variantmod

import (
	"fmt"
	"github.com/google/go-cmp/cmp"
	"os"
	"path/filepath"
	"testing"

	"github.com/twpayne/go-vfs/vfst"
	"github.com/variantdev/mod/pkg/cmdsite"
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

func TestDeploy(t *testing.T) {
	if testing.Verbose() {
	}

	files := map[string]interface{}{
		"/path/to/variant.mod": `
name: myapp

stages:
- name: dev
  environments:
  - dev1
- name: prod
  environments:
  - prod1

provisioners:
  files:
    cluster.yaml:
      path: environments/{{ .stage.environment }}/values.yaml
      source: values.yaml.tpl
      arguments:
        myappVersion: "{{ .stage.dependencies.myapp.version }}"

dependencies:
  myapp:
    releasesFrom:
      exec:
        command: go
        args:
        - run
        - main.go
    version: "> 1.10"
`,
		"/path/to/values.yaml.tpl": `myapp:
  version: {{.myappVersion}}
`,
		"/path/to/variant.lock": `
revisions:
- id: 1
  versions:
    myapp: 1.10.13
stages:
- name: dev
  revision: 1
dependencies:
  myapp:
    versions:
    - "1.10.13"
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

	{
		_, err = man.Build("dev")
		if err != nil {
			t.Fatal(err)
		}

		dev1ValuesExpected := `myapp:
  version: 1.10.13
`
		dev1ValuesActual, err := fs.ReadFile("/path/to/environments/dev1/values.yaml")
		if err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff(string(dev1ValuesActual), dev1ValuesExpected); diff != "" {
			t.Errorf("unexpected dev1 values:\n%s", diff)
		}
	}

	{
		err = man.Up("prod")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		lockActual, err := fs.ReadFile("/path/to/variant.lock")
		if err != nil {
			t.Fatal(err)
		}
		lockExpected := `stages:
- name: dev
  revision: 1
- name: prod
  revision: 1
revisions:
- id: 1
  versions:
    myapp: 1.10.13
dependencies:
  myapp:
    versions:
    - 1.10.13
`
		if diff := cmp.Diff(lockExpected, string(lockActual)); diff != "" {
			t.Errorf("unexpected state:\n%s", diff)
		}

		_, err = man.Build("prod")
		if err != nil {
			t.Fatal(err)
		}

		prod1ValuesExpected := `myapp:
  version: 1.10.13
`
		prod1ValuesActual, err := fs.ReadFile("/path/to/environments/prod1/values.yaml")
		if err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff(string(prod1ValuesActual), prod1ValuesExpected); diff != "" {
			t.Errorf("unexpected prod1 values:\n%s", diff)
		}
	}

	{
		err = man.Up("dev")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		lockActual, err := fs.ReadFile("/path/to/variant.lock")
		if err != nil {
			t.Fatal(err)
		}
		lockExpected := `stages:
- name: dev
  revision: 2
- name: prod
  revision: 1
revisions:
- id: 1
  versions:
    myapp: 1.10.13
- id: 2
  versions:
    myapp: 1.13.7
dependencies:
  myapp:
    version: 1.13.7
    versions:
    - 1.10.13
    - 1.13.7
`
		if diff := cmp.Diff(lockExpected, string(lockActual)); diff != "" {
			t.Errorf("unexpected state:\n%s", diff)
		}
	}

	{
		_, err = man.Build()
		if err != nil {
			t.Fatal(err)
		}

		dev1ValuesExpected := `myapp:
  version: 1.13.7
`
		dev1ValuesActual, err := fs.ReadFile("/path/to/environments/dev1/values.yaml")
		if err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff(string(dev1ValuesActual), dev1ValuesExpected); diff != "" {
			t.Errorf("unexpected dev1 values:\n%s", diff)
		}
	}
}
