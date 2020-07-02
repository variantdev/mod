package deploycoordinator

import (
	"fmt"
	"github.com/google/go-cmp/cmp"
	"testing"
)

func TestCoordinator(t *testing.T) {
	testcases := []struct {
		spec  string
		state string

		stateAfter string
	}{
		{
			spec: `
name: example

stages:
- name: first
  environments: ["staging"]
- name: second
  environments: ["prod"]

dependencies:
- name: exampleMaster
  version: ">= 1.0.0"
- name: myappLatest
  version: ">= 1.0.0"
`,
			state: `
revisions:
- id: 1
  versions:
    exampleMaster: v1.0.0
    myappLatest: v2.0.0
- id: 2
  versions:
    exampleMaster: v1.0.1
    myappLatest: v2.0.0

stages:
- name: first
  revision: 1
- name: second
  revision: 1
  prodDeploy:

dependencies:
  exampleMaster:
    versions:
    - v1.0.0
    - v1.0.1
    - v1.1.0
  example:
    versions:
    - v1.0.0
    - v1.0.1
    - v1.1.0
  myappLatest:
    versions:
    - v2.0.0
    - v2.0.1

meta:
  dependencies:
    exampleMaster:
      v1.0.0:
        foo: FOO
      v1.0.1:
        bar: BAR
`,
			stateAfter: `stages:
- name: first
  revision: 4
- name: second
  revision: 2
revisions:
- id: 1
  versions:
    exampleMaster: v1.0.0
    myappLatest: v2.0.0
- id: 2
  versions:
    exampleMaster: v1.0.1
    myappLatest: v2.0.0
- id: 3
  versions:
    example: v1.1.0
    exampleMaster: v1.1.0
    myappLatest: v2.0.1
- id: 4
  versions:
    exampleMaster: v1.2.0
    myappLatest: v2.1.0
dependencies:
  example:
    versions:
    - v1.0.0
    - v1.0.1
    - v1.1.0
  exampleMaster:
    versions:
    - v1.0.0
    - v1.0.1
    - v1.1.0
    - v1.2.0
  myappLatest:
    versions:
    - v2.0.0
    - v2.0.1
    - v2.1.0
meta:
  dependencies:
    exampleMaster:
      v1.0.0:
        foo: FOO
      v1.0.1:
        bar: BAR
`,
		},
	}

	for i := range testcases {
		tc := testcases[i]

		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			t.Parallel()

			c, err := Parse(tc.spec, tc.state)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Should be noop and stick to revision 1 as the first stage isn't updated to revision 2 yet
			{
				c.UpdateStage("second")
				got, err := c.GetStage("second")
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got.Versions["exampleMaster"] != "v1.0.0" {
					t.Errorf("unexpected exampleMaster: %v", got.Versions["exampleMaster"])
				}
				if got.Versions["myappLatest"] != "v2.0.0" {
					t.Errorf("unexpected myappLatest: %v", got.Versions["myappLatest"])
				}
				if got.Environments[0] != "prod" {
					t.Errorf("unexpected environment: %v", got.Environments[0])
				}
				if v := got.Deps["exampleMaster"].Version; v != "v1.0.0" {
					t.Errorf("unexpected myapplatest Version: %v", v)
				}
				if v := got.Deps["exampleMaster"].Meta["foo"]; v != "FOO" {
					t.Errorf("unexpected myapplatest meta.foo: %v", v)
				}
			}

			// first should be updated to revision 1
			{
				{
					err := c.UpdateStage("first")
					if err != nil {
						t.Fatalf("unexpected error: %v", err)
					}

					got, err := c.GetStage("first")
					if err != nil {
						t.Errorf("unexpected error: %v", err)
					}
					if got.Versions["exampleMaster"] != "v1.0.1" {
						t.Errorf("unexpected exampleMaster: %v", got.Versions["exampleMaster"])
					}
					if got.Versions["myappLatest"] != "v2.0.0" {
						t.Errorf("unexpected myappLatest: %v", got.Versions["myappLatest"])
					}
				}
			}

			// second should be updated to revision 2
			{
				c.UpdateStage("second")
				got, err := c.GetStage("second")
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got.Versions["exampleMaster"] != "v1.0.1" {
					t.Errorf("unexpected exampleMaster: %v", got.Versions["exampleMaster"])
				}
				if got.Versions["myappLatest"] != "v2.0.0" {
					t.Errorf("unexpected myappLatest: %v", got.Versions["myappLatest"])
				}
				if got.Environments[0] != "prod" {
					t.Errorf("unexpected environment: %v", got.Environments[0])
				}
			}

			{
				err := c.UpdateRevisions("*", c.DeploymentDependencies())
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}

				{
					r, err := c.GetRevisions()
					if err != nil {
						t.Errorf("unexpected error: %v", err)
					}
					if len(r) != 3 {
						t.Errorf("unexpeced result: expected size 3, got %d", len(r))
					}
				}

				r, err := c.GetCurrentRevision()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if r.ID != 3 {
					t.Errorf("unexpected id: want 3, got %d", r.ID)
				}
				if ver := r.Versions["exampleMaster"]; ver != "v1.1.0" {
					t.Errorf("unexpected exampleMaster version: want v1.1.0, got %s", ver)
				}
				if ver := r.Versions["myappLatest"]; ver != "v2.0.1" {
					t.Errorf("unexpected myappLatest version: want v2.0.1, got %s", ver)
				}
			}

			{
				err := c.UpdateStage("first")
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}

				got, err := c.GetStage("first")
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if got.Versions["exampleMaster"] != "v1.1.0" {
					t.Errorf("unexpected exampleMaster: %v", got.Versions["exampleMaster"])
				}
				if got.Versions["myappLatest"] != "v2.0.1" {
					t.Errorf("unexpected myappLatest: %v", got.Versions["myappLatest"])
				}
			}

			deps := []string{"exampleMaster", "myappLatest", "example"}

			{
				err := c.UpdateDependencies(deps, func(depName string) ([]DependencyEntry, error) {
					switch depName {
					case "exampleMaster":
						return []DependencyEntry{{Version: "v1.2.0"}}, nil
					case "myappLatest":
						return []DependencyEntry{{Version:"v2.1.0"}}, nil
					case "example":
						// no update
						return nil, nil
					}

					return nil, fmt.Errorf("updating dependencies: %s unsupported", depName)
				})
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}

				if err := c.UpdateRevisions("*", c.DeploymentDependencies()); err != nil {
					t.Errorf("unexpected error: %v", err)
				}

				{
					err := c.UpdateStage("first")
					if err != nil {
						t.Fatalf("unexpected error: %v", err)
					}
				}

				got, err := c.GetStage("first")
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if got.Versions["exampleMaster"] != "v1.2.0" {
					t.Errorf("unexpected exampleMaster: %v", got.Versions["exampleMaster"])
				}
				if got.Versions["myappLatest"] != "v2.1.0" {
					t.Errorf("unexpected myappLatest: %v", got.Versions["myappLatest"])
				}
				if got.Environments[0] != "staging" {
					t.Errorf("unexpected environment: %v", got.Environments[0])
				}
			}

			{
				got, err := c.Marshal()
				if err != nil {
					t.Fatalf("marshalling coordinator: %v", err)
				}

				if diff := cmp.Diff(tc.stateAfter, got); diff != "" {
					t.Errorf("unexpected diff:\n%s", diff)
				}
			}

		})
	}
}
