package execversionmanager

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"path/filepath"
	"testing"
)

func TestReleaseChannel(t *testing.T) {
	input := `executables:
  go:
    platforms:
      # Adds $VARIANT_MOD_PATH/mod/cache/CACHE_KEY/go/bin/go to $PATH
      # Or its shim at $VARIANT_MOD_PATH/MODULE_NAME/shims
    - source: https://dl.google.com/go/go1.12.6.darwin-amd64.tar.gz@go/bin/go
      selector:
        matchLabels:
          os: darwin
          arch: amd64
    - source: https://dl.google.com/go/go1.12.6.linux-amd64.tar.gz@go/bin/go
      selector:
        matchLabels:
          os: linux
          arch: amd64
`

	conf := &Config{}
	if err := yaml.Unmarshal([]byte(input), conf); err != nil {
		t.Fatal(err)
	}

	execset, err := New(conf)
	if err != nil {
		t.Fatal(err)
	}

	bin, err := execset.Locate("go")
	if err != nil {
		t.Fatal(err)
	}

	if filepath.Base(bin.Path) != "go" {
		t.Errorf("unexpected bin path: expected=go, got=%v", bin.Path)
	}
}

func TestReleaseChannel_Shell(t *testing.T) {
	input := `executables:
  go:
    platforms:
      # Adds $VARIANT_MOD_PATH/mod/cache/CACHE_KEY/go/bin/go to $PATH
      # Or its shim at $VARIANT_MOD_PATH/MODULE_NAME/shims
    - source: https://dl.google.com/go/go1.12.6.darwin-amd64.tar.gz@go/bin/go
      selector:
        matchLabels:
          os: darwin
          arch: amd64
    - source: https://dl.google.com/go/go1.12.6.linux-amd64.tar.gz@go/bin/go
      selector:
        matchLabels:
          os: linux
          arch: amd64
`

	conf := &Config{}
	if err := yaml.Unmarshal([]byte(input), conf); err != nil {
		t.Fatal(err)
	}

	execset, err := New(conf)
	if err != nil {
		t.Fatal(err)
	}

	sh, err := execset.Shell()
	if err != nil {
		t.Fatal(err)
	}

	stdout, _, err := sh.CaptureStrings("sh", []string{"-c", "go version"})
	if err != nil {
		t.Fatal(err)
	}

	actual := string(stdout)
	os, arch := osArch()
	if err != nil {
		t.Fatal(err)
	}
	expected := fmt.Sprintf("go version go1.12.6 %s/%s", os, arch)

	if actual != expected {
		t.Errorf("unexpected go version output: expected=%s, got=%s", expected, actual)
	}
}
