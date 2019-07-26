provisioners:
  files:
    circleci.config.yml:
      source: circleci.config.yml.tpl
      arguments:
        version: "{{.go.version}}"
    Dockerfile:
      source: Dockerfile.tpl
      arguments:
        version: "{{.go.version}}"

dependencies:
  go:
    releasesFrom:
      dockerImageTags:
        source: library/golang
    version: "> 1.10"
