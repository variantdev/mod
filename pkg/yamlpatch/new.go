package yamlpatch

import (
	"gopkg.in/yaml.v3"
)

type YAMLPatch struct {
	node *yaml.Node
	indent int
}

func getIndent(node *yaml.Node) int {
	if node.Kind == yaml.ScalarNode {
		return 0
	}
	if node.Column != 1 {
		if node.Kind == yaml.SequenceNode {
			return (node.Column - 1) * 2
		}
		return node.Column - 1
	}
	for _, content := range node.Content {
		indent := getIndent(content)
		if indent > 1 {
			return indent
		}
	}
	return 0
}

func New(b []byte) (*YAMLPatch, error) {
	var yml yaml.Node
	if err := yaml.Unmarshal(b, &yml); err != nil {
		return nil, err
	}

	return &YAMLPatch{
		node: &yml,
		indent: getIndent(&yml),
	}, nil
}