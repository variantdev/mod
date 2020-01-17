package hclconf

import (
	hcl2 "github.com/hashicorp/hcl/v2"
)

type Config struct {
	Modules []Module `hcl:"module,block"`
}

type Module struct {
	Name string `hcl:"name,label"`

	Dependencies   []Dependency    `hcl:"dependency,block"`
	Files          []File          `hcl:"file,block"`
	RegexpReplaces []RegexpReplace `hcl:"regexp_replace,block"`
	Executables    []Executable    `hcl:"executable,block"`
}

type Dependency struct {
	Type string `hcl:"type,label"`
	Name string `hcl:"name,label"`

	Version string `hcl:"version,attr"`

	BodyForType hcl2.Body `hcl:",remain"`
}

type ExecDependency struct {
	Command string   `hcl:"command,attr"`
	Args    []string `hcl:"args,attr"`
}

type JSONPath struct {
	Source      string `hcl:"source,attr"`
	Versions    string `hcl:"versions,attr"`
	Description string `hcl:"description,attr"`
}

type GitTags struct {
	Source string `hcl:"source,attr"`
}

type GitHubTags struct {
	Host   *string `hcl:"host,attr"`
	Source string  `hcl:"source,attr"`
}

type GitHubReleases struct {
	Host   *string `hcl:"host,attr"`
	Source string  `hcl:"source,attr"`
}

type DockerImageTags struct {
	Source string `hcl:"source,attr"`
}

type File struct {
	Name string `hcl:"name,label"`

	Source string          `hcl:"source,attr"`
	Args   hcl2.Expression `hcl:"args,attr"`
}

type RegexpReplace struct {
	Name string `hcl:"name,label"`

	From string          `hcl:"from,attr"`
	To   hcl2.Expression `hcl:"to,attr"`
}

type Executable struct {
	Name string `hcl:"name,label"`

	Platfoms []Platform `hcl:"platform,block"`
}

type Platform struct {
	Source hcl2.Expression `hcl:"source,attr"`
	Docker *Docker         `hcl:"docker,block"`
	OS     *string         `hcl:"os,attr"`
	Arch   *string         `hcl:"arch,attr"`
}

type Docker struct {
	Command *string         `hcl:"command,attr"`
	Image   string          `hcl:"image,attr"`
	Tag     hcl2.Expression `hcl:"tag,attr"`
	Volumes hcl2.Expression `hcl:"volumes,attr"`
	WorkDir string          `hcl:"workdir,attr"`
}
