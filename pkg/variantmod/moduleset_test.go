package variantmod

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/twpayne/go-vfs/vfst"
	"github.com/variantdev/mod/pkg/cmdsite"
	"k8s.io/klog"
	"k8s.io/klog/klogr"
)

func TestDependencyLockinge_Dockerfile_Examples_Hcl(t *testing.T) {
	if testing.Verbose() {
	}

	testcases := []struct {
		in string
	}{
		{
			in: `
module "myapp" {
  dependency "exec" "helmfile" {
    command = "go"
    args = ["run", "main.go"]
    version = "> 0.94.0"
  }

  regexp_replace "Dockerfile" {
    from = "(FROM helmfile:)(\\S+)(\\s+)"
    to = "$${1}${dep.helmfile.version}$${3}"
  }
}
`,
		},
		{
			in: `
module "myapp" {
  dependency "exec" "helmfile" {
    command = "go"
    args = ["run", "main.go"]
    version = "> 0.94.0"
  }

  file "Dockerfile" {
    source = "Dockerfile.tpl"
    args = {
      version = "${dep.helmfile.version}"
    }
  }
}
`,
		},
	}

	for i := range testcases {
		tc := testcases[i]

		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			files := map[string]interface{}{
				"/path/to/myapp.variantmod": tc.in,
				"/path/to/Dockerfile": `FROM helmfile:0.94.0

RUN echo hello
`,
				"/path/to/Dockerfile.tpl": `FROM helmfile:{{.version}}

RUN echo hello
`,
				"/path/to/myapp.variantmod.lock": `
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

			man, err := New(ModuleFile("myapp.variantmod"), LockFile("myapp.variantmod.lock"), Logger(log), FS(fs), WD("/path/to"), GoGetterWD(filepath.Join(fs.TempDir(), "path", "to")), Commander(cmdr))
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

			lockActual, err := fs.ReadFile("/path/to/myapp.variantmod.lock")
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
		})
	}
}
