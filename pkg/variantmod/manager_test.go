package variantmod

import (
	"github.com/twpayne/go-vfs/vfst"
	"github.com/variantdev/mod/pkg/loginfra"
	"k8s.io/klog"
	"k8s.io/klog/klogr"
	"os"
	"testing"
)

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
	}

	fs, clean, err := vfst.NewTestFS(map[string]interface{}{})
	if err != nil {
		t.Fatal(err)
	}

	defer clean()
	log := klogr.New()
	loginfra.Init()
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
	loginfra.Init()
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
