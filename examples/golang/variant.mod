executables:
  go:
    source: default_source
    platforms:
      # Adds $VARIANT_MOD_PATH/mod/cache/CACHE_KEY/go/bin/go to $PATH
      # Or its shim at $VARIANT_MOD_PATH/MODULE_NAME/shims
    - source: https://dl.google.com/go/go1.12.6.darwin-amd64.tar.gz@go/bin/go
      install: "bin/rbenv install {{.version}}"
      shims:
        ruby: |
          exec {{.localCopy}}/bin/shims/ruby "$@"
      #implies:
      #bin: go/bin/go
      selector:
        matchLabels:
          os: darwin
          arch: amd64
    - source: https://dl.google.com/go/go1.12.6.linux-amd64.tar.gz@go/bin/go
      selector:
        matchLabels:
          os: linux
          arch: amd64
    fallback:
    - source: https://dl.google.com/go/go1.12.6.{{.os}}-{{.arch}}.tar.gz@go/bin/go
