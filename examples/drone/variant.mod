files:
  mytest.yaml:
    source: git::https://github.com/cloudposse/helmfiles.git@releases/kiam.yaml?ref={{.version}}
    arguments:
      foo: FOO
