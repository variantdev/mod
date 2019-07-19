provisioners:
  files:
    cluster.yaml:
      source: cluster.yaml.tpl
      arguments:
        name: k8s1
        region: ap-northeast-1
        version: "{{.k8s.version}}"

dependencies:
  k8s:
    releasesFrom:
      exec:
        command: go
        args:
        - run
        - main.go
    version: "> 1.10"
