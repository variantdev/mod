package confapi

type Revision struct {
	ID       int               `yaml:"id"`
	Versions map[string]string `yaml:"versions"`
}

