schema:
  properties:
    foo:
     type: string
  required:
  - foo

values:
  foo: bar
  version: "0.40.0"

files:
  mytest.yaml:
    source: git::https://github.com/cloudposse/helmfiles.git@releases/kiam.yaml?ref={{.version}}
    arguments:
      foo: FOO
