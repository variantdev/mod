package execversionmanager

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/k-kinzal/aliases/pkg/aliases/script"
	"github.com/variantdev/mod/pkg/tmpl"
	"gopkg.in/yaml.v3"

	"github.com/k-kinzal/aliases/pkg/aliases/context"
	aliasesyaml "github.com/k-kinzal/aliases/pkg/aliases/yaml"
	"github.com/k-kinzal/aliases/pkg/docker"
)

func (m *ExecVM) getDockerAlias(name string, platform Platform) (string, error) {
	if strings.Contains(name, "-") {
		return "", fmt.Errorf("executable.name containing hyphens(-) is not supported by the docker executable provider")
	}

	dockerRunConf := platform.Docker

	var err error

	dockerRunConf.Image, err = tmpl.Render("docker.image", dockerRunConf.Image, m.Values)
	if err != nil {
		return "", err
	}

	dockerRunConf.Tag, err = tmpl.Render("docker.tag", dockerRunConf.Tag, m.Values)
	if err != nil {
		return "", err
	}

	dockerRunConf.EnvPrefix = append(dockerRunConf.EnvPrefix, strings.ToUpper(name)+"_")

	aliasesConfMap := map[string]interface{}{
		name: dockerRunConf,
	}
	aliasesConfBytes, err := yaml.Marshal(aliasesConfMap)
	if err != nil {
		return "", err
	}

	exportDir := filepath.Join(m.GoGetterCacheDir, name)

	if err := context.ChangeHomePath(exportDir); err != nil {
		return "", err
	}

	if err := context.ChangeExportPath(exportDir); err != nil {
		return "", err
	}

	client, err := docker.NewClient()
	if err != nil {
		return "", fmt.Errorf("docker: new client: %v", err)
	}

	conf, err := aliasesyaml.Unmarshal(aliasesConfBytes)
	if err != nil {
		return "", err
	}

	for _, opt := range *conf {
		if err := script.NewScript(*opt).Write(client); err != nil {
			return "", err
		}
	}

	m.Logger.V(2).Info("docker: "+name, "exportDir", exportDir, "data", string(aliasesConfBytes))

	return filepath.Join(exportDir, name), nil
}
