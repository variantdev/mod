# on `variant mod up`, it fetches `source`, extract and semver-sort versions, and stores the latest version in `variant.lock` so that it looks:
# versions:
#   coreos: 2079.5.1
# This `version` is different from `module version`, which is used to version the module i.e. `variant.mod` in a versioned-source.
releaseChannels:
  stable:
    source: https://coreos.com/releases/releases-stable.json
    versions: "$"
    type: semver
    description: "$['{{.version}}'].release_notes"
#    # channel specific versions
#    channels:
#      stable:
#        source: https://coreos.com/releases/releases-stable.json
#        versions: "$.*~"
#        description: "$['{{.version}}'].release_notes"
