package depresolver

import (
	"fmt"
	"github.com/twpayne/go-vfs/vfst"
	"k8s.io/klog"
	"k8s.io/klog/klogr"
	"os"
	"testing"
)

func TestRemote(t *testing.T) {
	cleanfs := map[string]string{}
	cachefs := map[string]string{
		"/path/to/home/https_github_com_cloudposse_helmfiles_git.ref=0.40.0/releases/kiam.yaml": "foo: bar",
	}

	type testcase struct {
		files          map[string]string
		expectCacheHit bool
	}

	testcases := []testcase{
		{files: cleanfs, expectCacheHit: false},
		{files: cachefs, expectCacheHit: true},
	}

	for i := range testcases {
		testcase := testcases[i]

		t.Run(fmt.Sprintf("case %d", i), func(t *testing.T) {
			testfs, cleanup, err := vfst.NewTestFS(testcase.files)
			if err != nil {
				t.Fatal(err)
			}
			defer cleanup()

			hit := true

			get := func(wd, src, dst string) error {
				if wd != "/path/to/home" {
					return fmt.Errorf("unexpected wd: %s", wd)
				}
				if src != "git::https://github.com/cloudposse/helmfiles.git?ref=0.40.0" {
					return fmt.Errorf("unexpected src: %s", src)
				}

				hit = false

				return nil
			}

			getter := &testGetter{
				get: get,
			}
			klog.SetOutput(os.Stderr)
			remote, err := New(Logger(klogr.New()), Home("/path/to/home"), FS(testfs))
			if err != nil {
				t.Fatal(err)
			}
			remote.Getter = getter

			// FYI, go-getter in the `dir` mode accepts URL like the below. So helmfile expects URLs similar to it:
			//   go-getter -mode dir git::https://github.com/cloudposse/helmfiles.git?ref=0.40.0 gettertest1/b

			// We use `@` to separate dir and the file path. This is a good idea borrowed from helm-git:
			//   https://github.com/aslafy-z/helm-git

			url := "git::https://github.com/cloudposse/helmfiles.git@releases/kiam.yaml?ref=0.40.0"
			file, err := remote.Resolve(url)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if file != "/path/to/home/https_github_com_cloudposse_helmfiles_git.ref=0.40.0/releases/kiam.yaml" {
				t.Errorf("unexpected file located: %s", file)
			}

			if testcase.expectCacheHit && !hit {
				t.Errorf("unexpected result: unexpected cache miss: expected=%v", testcase.expectCacheHit)
			}
			if !testcase.expectCacheHit && hit {
				t.Errorf("unexpected result: unexpected cache hit: expected=%v", testcase.expectCacheHit)
			}
		})
	}
}

type testGetter struct {
	get func(wd, src, dst string) error
}

func (t *testGetter) Get(wd, src, dst string) error {
	return t.get(wd, src, dst)
}
