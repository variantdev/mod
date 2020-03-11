package releasetracker

import "regexp"

type Config struct {
	ReleaseChannel Spec `yaml:"releaseChannel"`
}

type Spec struct {
	VersionsFrom VersionsFrom `yaml:"versionsFrom"`
}

type VersionsFrom struct {
	Exec            Exec            `yaml:"exec"`
	JSONPath        GetterJSONPath  `yaml:"jsonPath"`
	GitTags         GitTags         `yaml:"gitTags"`
	GitHubTags      GitHubTags      `yaml:"githubTags"`
	GitHubReleases  GitHubReleases  `yaml:"githubReleases"`
	DockerImageTags DockerImageTags `yaml:"dockerImageTags"`

	ValidVersionPattern *regexp.Regexp
}

type Exec struct {
	Command string   `yaml:"command"`
	Args    []string `yaml:"args"`
}

type GetterJSONPath struct {
	Source      string `yaml:"source"`
	Versions    string `yaml:"versions"`
	Description string `yaml:"description"`
}

type GitTags struct {
	Source string `yaml:"source"`
}

type GitHubTags struct {
	Host   string `yaml:"host"`
	Source string `yaml:"source"`
}

type GitHubReleases struct {
	Host   string `yaml:"host"`
	Source string `yaml:"source"`
}

type DockerImageTags struct {
	Source string `yaml:"source"`
}
