package yamlpatch

import (
	"bytes"
	"gopkg.in/yaml.v3"
)

func (p *YAMLPatch) Marshal() ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(p.indent)
	if err := enc.Encode(p.node); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}