package yamlpatch_test

import (
	"github.com/kylelemons/godebug/diff"
	_ "github.com/kylelemons/godebug/diff"
	"github.com/variantdev/mod/pkg/yamlpatch"
	"gopkg.in/yaml.v3"
	"strings"
	"testing"
)

func Patch(yml string, jsonPatch string, expected string) string {
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(yml), &node); err != nil {
		panic(err)
	}

	if err := yamlpatch.Patch(&node, jsonPatch); err != nil {
		panic(err)
	}

	out, err := yaml.Marshal(node.Content[0])
	if err != nil {
		panic(err)
	}

	if strings.TrimSpace(expected) != strings.TrimSpace(string(out)) {
		return diff.Diff(strings.TrimSpace(expected), strings.TrimSpace(string(out)))
	}

	return ""
}

func TestPatch_AddScalarToSeqIndex(t *testing.T) {
	yml := `
foo:
  - 1
  - 2
  - 3
`
	patch := `[{"op": "add", "path": "/foo/1", "value": 4}]`
	expected := `
foo:
  - 1
  - 4
  - 2
  - 3
`
	if d := Patch(yml, patch, expected); d != "" {
		t.Fatalf("\n%s", d)
	}
}

func TestPatch_AddSeqToSeqIndex(t *testing.T) {
	yml := `
foo:
  - 1
  - 2
  - 3
`
	patch := `[{"op": "add", "path": "/foo/1", "value": [1, 2, 3]}]`
	expected := `
foo:
  - 1
  - [1, 2, 3]
  - 2
  - 3
`
	if d := Patch(yml, patch, expected); d != "" {
		t.Fatalf("\n%s", d)
	}
}

func TestPatch_AddMapToSeqIndex(t *testing.T) {
	yml := `
foo:
  - 1
  - 2
  - 3
`
	patch := `[{"op": "add", "path": "/foo/1", "value": {"a": 1, "b": 2, "c": 3}}]`
	expected := `
foo:
  - 1
  - {"a": 1, "b": 2, "c": 3}
  - 2
  - 3
`
	if d := Patch(yml, patch, expected); d != "" {
		t.Fatalf("\n%s", d)
	}
}

func TestPatch_AddScalarToEmpty(t *testing.T) {
	yml := `foo: 1`
	patch := `[{"op": "add", "path": "/bar", "value": 2}]`
	expected := `
foo: 1
bar: 2
`
	if d := Patch(yml, patch, expected); d != "" {
		t.Fatalf("\n%s", d)
	}
}

func TestPatch_AddSeqToEmpty(t *testing.T) {
	yml := `foo: 1`
	patch := `[{"op": "add", "path": "/bar", "value": [1, 2, 3]}]`
	expected := `
foo: 1
bar:
  - 1
  - 2
  - 3
`
	if d := Patch(yml, patch, expected); d != "" {
		t.Fatalf("\n%s", d)
	}
}

func TestPatch_AddMapToEmpty(t *testing.T) {
	yml := `foo: 1`
	patch := `[{"op": "add", "path": "/bar", "value": {"a": 1, "b": 2, "c": 3}}]`
	expected := `
foo: 1
bar:
    a: 1
    b: 2
    c: 3
`
	if d := Patch(yml, patch, expected); d != "" {
		t.Fatalf("\n%s", d)
	}
}

func TestPatch_AddScalarToScalar(t *testing.T) {
	yml := `foo: 1`
	patch := `[{"op": "add", "path": "/foo", "value": 2}]`
	expected := `foo: 2`
	if d := Patch(yml, patch, expected); d != "" {
		t.Fatalf("\n%s", d)
	}
}

func TestPatch_AddSecToScalar(t *testing.T) {
	yml := `foo: 1`
	patch := `[{"op": "add", "path": "/foo", "value": [1, 2, 3]}]`
	expected := `foo: [1, 2, 3]`
	if d := Patch(yml, patch, expected); d != "" {
		t.Fatalf("\n%s", d)
	}
}

func TestPatch_AddMapToScalar(t *testing.T) {
	yml := `foo: 1`
	patch := `[{"op": "add", "path": "/foo", "value": {"a": 1, "b": 2, "c": 3}}]`
	expected := `foo: {"a": 1, "b": 2, "c": 3}`
	if d := Patch(yml, patch, expected); d != "" {
		t.Fatalf("\n%s", d)
	}
}

func TestPatch_RemoveScalar(t *testing.T) {
	yml := `
foo: 1
bar: 2
`
	patch := `[{"op": "remove", "path": "/foo"}]`
	expected := `bar: 2`
	if d := Patch(yml, patch, expected); d != "" {
		t.Fatalf("\n%s", d)
	}
}

func TestPatch_RemoveSeq(t *testing.T) {
	yml := `
foo:
  - 1
  - 2
  - 3
bar: 2
`
	patch := `[{"op": "remove", "path": "/foo"}]`
	expected := `bar: 2`
	if d := Patch(yml, patch, expected); d != "" {
		t.Fatalf("\n%s", d)
	}
}

func TestPatch_RemoveSeqIndex(t *testing.T) {
	yml := `
foo:
  - 1
  - 2
  - 3
bar: 2
`
	patch := `[{"op": "remove", "path": "/foo/0"}]`
	expected := `
foo:
  - 2
  - 3
bar: 2
`
	if d := Patch(yml, patch, expected); d != "" {
		t.Fatalf("\n%s", d)
	}
}

func TestPatch_RemoveMap(t *testing.T) {
	yml := `
foo:
    a: 1
    b: 2
    c: 3
bar: 2
`
	patch := `[{"op": "remove", "path": "/foo"}]`
	expected := `bar: 2`
	if d := Patch(yml, patch, expected); d != "" {
		t.Fatalf("\n%s", d)
	}
}

func TestPatch_RemoveKey(t *testing.T) {
	yml := `
foo:
    a: 1
    b: 2
    c: 3
bar: 2
`
	patch := `[{"op": "remove", "path": "/foo/b"}]`
	expected := `
foo:
    a: 1
    c: 3
bar: 2`
	if d := Patch(yml, patch, expected); d != "" {
		t.Fatalf("\n%s", d)
	}
}

func TestPatch_ReplaceScalarToScalar(t *testing.T) {
	yml := `foo: 1`
	patch := `[{"op": "replace", "path": "/foo", "value": 1.5}]`
	expected := `foo: 1.5`
	if d := Patch(yml, patch, expected); d != "" {
		t.Fatalf("\n%s", d)
	}
}

func TestPatch_ReplaceSeqToScalar(t *testing.T) {
	yml := `foo: 1`
	patch := `[{"op": "replace", "path": "/foo", "value": [1, 2, 3]}]`
	expected := `foo: [1, 2, 3]`
	if d := Patch(yml, patch, expected); d != "" {
		t.Fatalf("\n%s", d)
	}
}

func TestPatch_ReplaceMapToScalar(t *testing.T) {
	yml := `foo: 1`
	patch := `[{"op": "replace", "path": "/foo", "value": {"a": 1, "b": 2, "c": 3}}]`
	expected := `foo: {"a": 1, "b": 2, "c": 3}`
	if d := Patch(yml, patch, expected); d != "" {
		t.Fatalf("\n%s", d)
	}
}

func TestPatch_ReplaceScalarToSeq(t *testing.T) {
	yml := `foo: [1, 2, 3]`
	patch := `[{"op": "replace", "path": "/foo", "value": 1.5}]`
	expected := `foo: 1.5`
	if d := Patch(yml, patch, expected); d != "" {
		t.Fatalf("\n%s", d)
	}
}

func TestPatch_ReplaceSeqToSeq(t *testing.T) {
	yml := `foo: [1, 2, 3]`
	patch := `[{"op": "replace", "path": "/foo", "value": [3, 4, 5]}]`
	expected := `foo: [3, 4, 5]`
	if d := Patch(yml, patch, expected); d != "" {
		t.Fatalf("\n%s", d)
	}
}

func TestPatch_ReplaceMapToSeq(t *testing.T) {
	yml := `foo: [1, 2, 3]`
	patch := `[{"op": "replace", "path": "/foo", "value": {"a": 1, "b": 2, "c": 3}}]`
	expected := `foo: {"a": 1, "b": 2, "c": 3}`
	if d := Patch(yml, patch, expected); d != "" {
		t.Fatalf("\n%s", d)
	}
}

func TestPatch_ReplaceScalarToSeqIndex(t *testing.T) {
	yml := `foo: [1, 2, 3]`
	patch := `[{"op": "replace", "path": "/foo/0", "value": 1.5}]`
	expected := `foo: [1.5, 2, 3]`
	if d := Patch(yml, patch, expected); d != "" {
		t.Fatalf("\n%s", d)
	}
}

func TestPatch_ReplaceSeqToSeqIndex(t *testing.T) {
	yml := `foo: [1, 2, 3]`
	patch := `[{"op": "replace", "path": "/foo/0", "value": [3, 4, 5]}]`
	expected := `foo: [[3, 4, 5], 2, 3]`
	if d := Patch(yml, patch, expected); d != "" {
		t.Fatalf("\n%s", d)
	}
}

func TestPatch_ReplaceMapToSeqIndex(t *testing.T) {
	yml := `foo: [1, 2, 3]`
	patch := `[{"op": "replace", "path": "/foo/0", "value": {"a": 1, "b": 2, "c": 3}}]`
	expected := `foo: [{"a": 1, "b": 2, "c": 3}, 2, 3]`
	if d := Patch(yml, patch, expected); d != "" {
		t.Fatalf("\n%s", d)
	}
}

func TestPatch_ReplaceScalarToMap(t *testing.T) {
	yml := `foo: {"a": 1, "b": 2, "c": 3}`
	patch := `[{"op": "replace", "path": "/foo", "value": 1.5}]`
	expected := `foo: 1.5`
	if d := Patch(yml, patch, expected); d != "" {
		t.Fatalf("\n%s", d)
	}
}

func TestPatch_ReplaceSeqToMap(t *testing.T) {
	yml := `foo: {"a": 1, "b": 2, "c": 3}`
	patch := `[{"op": "replace", "path": "/foo", "value": [3, 4, 5]}]`
	expected := `foo: [3, 4, 5]`
	if d := Patch(yml, patch, expected); d != "" {
		t.Fatalf("\n%s", d)
	}
}

func TestPatch_ReplaceMapToMap(t *testing.T) {
	yml := `foo: {"a": 1, "b": 2, "c": 3}`
	patch := `[{"op": "replace", "path": "/foo", "value": {"a": 1, "b": 2, "c": 3}}]`
	expected := `foo: {"a": 1, "b": 2, "c": 3}`
	if d := Patch(yml, patch, expected); d != "" {
		t.Fatalf("\n%s", d)
	}
}

func TestPatch_NotRemoveComment(t *testing.T) {
	yml := `
# FOO
foo: 1
`
	patch := `[{"op": "add", "path": "/foo", "value": 2}]`
	expected := `
# FOO
foo: 2
`
	if d := Patch(yml, patch, expected); d != "" {
		t.Fatalf("\n%s", d)
	}
}