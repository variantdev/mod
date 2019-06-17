executables:
  ruby:
    source: https://github.com/rbenv/rbenv.git
    install: "{{.localCopy}}/bin/rbenv install {{.version}}"
    shims:
      ruby: |
        exec {{.localCopy}}/bin/shims/ruby "$@"
