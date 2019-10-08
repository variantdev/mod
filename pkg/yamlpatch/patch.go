package yamlpatch

import (
	"encoding/json"
	jsonpatch "github.com/evanphx/json-patch"
	"github.com/google/go-cmp/cmp"
	"gopkg.in/yaml.v3"
)

func Patch(y *yaml.Node, patchJSON string) error {
	var v1 interface{}
	if err := y.Decode(&v1); err != nil {
		return err
	}

	origJson, err := json.Marshal(v1)
	if err != nil {
		return err
	}

	patch, err := jsonpatch.DecodePatch([]byte(patchJSON))
	if err != nil {
		return err
	}

	modified, err := patch.Apply(origJson)
	if err != nil {
		return err
	}

	var v2 interface{}
	if err := json.Unmarshal(modified, &v2); err != nil {
		return err
	}

	t := &Traversal{stack:[]interface{}{}}
	t.pushState(y, nil, "$")
	cmp.Equal(v1, v2, cmp.Reporter(t))

	return nil
}