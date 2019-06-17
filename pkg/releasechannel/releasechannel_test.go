package releasechannel

import (
	"gopkg.in/yaml.v3"
	"testing"
)

func TestReleaseChannel(t *testing.T) {
	input := `releaseChannels:
  stable:
    source: https://coreos.com/releases/releases-stable.json
    versions: "$"
    type: semver
    description: "$['{{.version}}'].release_notes"
`

	conf := &Config{}
	if err := yaml.Unmarshal([]byte(input), conf); err != nil {
		t.Fatal(err)
	}

	stable, err := New(conf, "stable")
	if err != nil {
		t.Fatal(err)
	}

	latest, err := stable.Latest()
	if err != nil {
		t.Fatal(err)
	}

	if latest.Version != "2079.5.1" {
		t.Errorf("unexpected version: expected=%v, got=%v", "2079.5.1", latest.Version)
	}
}
