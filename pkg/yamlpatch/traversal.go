package yamlpatch

import (
	"encoding/json"
	"github.com/google/go-cmp/cmp"
	"gopkg.in/yaml.v3"
	"strconv"
)

type TraversalState struct {
	yml *yaml.Node
	ps  cmp.PathStep
	key string
}

type Traversal struct {
	stack []interface{}
}

func (t *Traversal) pushState(yml *yaml.Node, ps cmp.PathStep, key string) {
	t.stack = append(t.stack, TraversalState{
		yml: yml,
		ps:  ps,
		key: key,
	})
}

func (t *Traversal) popState() (yml *yaml.Node, ps cmp.PathStep, key string) {
	v := t.stack[len(t.stack)-1].(TraversalState)
	t.stack = t.stack[:len(t.stack)-1]
	return v.yml, v.ps, v.key
}

func (t *Traversal) state() (yml *yaml.Node, ps cmp.PathStep, key string) {
	v := t.stack[len(t.stack)-1].(TraversalState)
	return v.yml, v.ps, v.key
}

func (t *Traversal) parentState() (yml *yaml.Node, ps cmp.PathStep, key string) {
	l := len(t.stack) - 2
	if l < 0 {
		return nil, nil, ""
	}
	v := t.stack[len(t.stack)-2].(TraversalState)
	return v.yml, v.ps, v.key
}

func (t *Traversal) PushStep(ps cmp.PathStep) {
	yml, pss, _ := t.state()

	switch p := ps.(type) {
	case cmp.SliceIndex:
		index := p.Key()
		if 0 <= index && index < len(yml.Content) {
			t.pushState(yml.Content[index], ps, strconv.Itoa(index))
		} else {
			_, ykey := p.SplitKeys()
			t.pushState(yml, ps, strconv.Itoa(ykey))
		}
	case cmp.MapIndex:
		index := -1
		for i, n := range yml.Content {
			if i%2 > 0 {
				continue
			}
			if n.Value == p.Key().String() {
				index = i + 1
				break
			}
		}
		if index == -1 {
			t.pushState(yml, ps, p.Key().String())
		} else {
			t.pushState(yml.Content[index], ps, p.Key().String())
		}
	case cmp.TypeAssertion:
		t.pushState(yml, pss, "_")
	case cmp.PathStep:
		t.pushState(yml.Content[0], ps, "$")
	}
}

func (t *Traversal) Report(rs cmp.Result) {
	yml, ps, key := t.state()
	parent, _, _ := t.parentState()
	vx, vy := ps.Values()

	if vx.IsValid() && vy.IsValid() {
		// modify
		out, _ := json.Marshal(vy.Interface())
		var node yaml.Node
		yaml.Unmarshal(out, &node)
		yml.Kind = node.Content[0].Kind
		yml.Style = node.Content[0].Style
		yml.Tag = node.Content[0].Tag
		yml.Value = node.Content[0].Value
		yml.Content = node.Content[0].Content

	} else if vx.IsValid() && !vy.IsValid() {
		// remove
		switch parent.Kind {
		case yaml.DocumentNode:
			parent.Content = []*yaml.Node{}
		case yaml.SequenceNode:
			// Always delete the last element
			parent.Content = parent.Content[:len(parent.Content)-1]
		case yaml.MappingNode:
			for i, n := range parent.Content {
				if i%2 > 0 {
					continue
				}
				if n.Value == key {
					var nodes []*yaml.Node
					if i > 0 {
						nodes = append(nodes, parent.Content[:i]...)
					}
					if (i + 1) < (len(parent.Content) - 1) {
						nodes = append(nodes, parent.Content[i+2:]...)
					}
					parent.Content = nodes
					break
				}
			}
		}

	} else if !vx.IsValid() && vy.IsValid() {
		// add
		out, _ := yaml.Marshal(vy.Interface())
		var node yaml.Node
		yaml.Unmarshal(out, &node)

		switch parent.Kind {
		case yaml.DocumentNode:
			parent.Content = []*yaml.Node{node.Content[0]}
		case yaml.SequenceNode:
			var nodes []*yaml.Node
			index, _ := strconv.Atoi(key)
			if index > 0 {
				nodes = append(nodes, parent.Content[:index]...)
			}
			nodes = append(nodes, node.Content...)
			if index < len(parent.Content) {
				nodes = append(nodes, parent.Content[index:]...)
			}
			parent.Content = nodes
		case yaml.MappingNode:
			keyNode := yaml.Node{
				Kind:  yaml.ScalarNode,
				Tag:   "!!str",
				Value: key,
			}
			parent.Content = append(parent.Content, &keyNode, node.Content[0])
		}

	}
}

func (t *Traversal) PopStep() {
	t.popState()
}
