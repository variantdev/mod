values:
  version: 1.0.0

modules:
  drone:
    source: git::https://github.com/variantdev/modules.git@drone/variant.mod?ref={{.version}}
    arguments:
      foo: FOO
