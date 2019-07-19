package releasetracker

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
	GitHubReleases  GitHubReleases  `yaml:"githubReleases"`
	DockerImageTags DockerImageTags `yaml:"dockerImageTags"`
}

func (f VersionsFrom) IsDefined() bool {
	return f.Exec.Command != "" ||
		f.JSONPath.Source != "" ||
		f.GitTags.Source != "" ||
		f.GitHubReleases.Source != "" ||
		f.DockerImageTags.Source != ""
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

type GitHubReleases struct {
	Host   string `yaml:"host"`
	Source string `yaml:"source"`
}

type DockerImageTags struct {
	Source string `yaml:"source"`
}
