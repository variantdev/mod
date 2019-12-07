package tmpl

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"text/template"
)

func Render(name, text string, data interface{}) (string, error) {
	funcs := map[string]interface{}{
		"hasKey": func(m interface{}, key string) (bool, error) {
			switch m := m.(type) {
			case map[string]interface{}:
				_, ok := m[key]
				return ok, nil
			default:
				return false, fmt.Errorf("map[string]interface expected, but got: value=%v, type=%T", m, m)
			}
		},
		"trimSpace": func(s string) string {
			return strings.TrimSpace(s)
		},
		"sha256": func(s string) string {
			b := sha256.Sum256([]byte(s))
			return hex.EncodeToString(b[:])
		},
	}
	tpl := template.New(name).Option("missingkey=error").Funcs(funcs)
	tpl, err := tpl.Parse(text)
	if err != nil {
		return "", err
	}
	buf := &bytes.Buffer{}
	if err := tpl.Execute(buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func RenderArgs(args map[string]interface{}, data map[string]interface{}) (map[string]interface{}, error) {
	res := map[string]interface{}{}

	for k, v := range args {
		switch t := v.(type) {
		case map[string]interface{}:
			r, err := RenderArgs(t, data)
			if err != nil {
				return nil, err
			}
			res[k] = r
		case string:
			r, err := Render(fmt.Sprintf("%s: \"%s\"", k, t), t, data)
			if err != nil {
				return nil, err
			}
			res[k] = r
		case int, bool:
			res[k] = t
		default:
			return nil, fmt.Errorf("unsupported type: value=%v, type=%T", t, t)
		}
	}

	return res, nil
}
