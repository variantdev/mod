# mod

`mod` is an universal package manager complements task runners and build tools like `make` and [variant](https://github.com/mumoshu/variant).

## Getting started

Let's assume you have a `Dockerfile` to build a Docker image containing specific version of `helm`:

`Dockerfile`:

```
FROM alpine:3.9

ARG HELM_VERSION=2.14.2

ADD http://storage.googleapis.com/kubernetes-helm/${HELM_FILE_NAME} /tmp
RUN tar -zxvf /tmp/${HELM_FILE_NAME} -C /tmp \
  && mv /tmp/linux-amd64/helm /bin/helm \
  && rm -rf /tmp/* \
  && /bin/helm init --client-only
```

With `mod`, you can automate the process of occasionally updating the version number in `HELM_VERSION`.

Replace the hard-coded version number `2.14.2` with a template expression `{{ .helm_version }}`, that refers to the latest helm version which is tracked by `mod`:

`Dockerfile.tpl`:

```
FROM alpine:3.9

ARG HELM_VERSION={{ .helm_version }}

ADD http://storage.googleapis.com/kubernetes-helm/${HELM_FILE_NAME} /tmp
RUN tar -zxvf /tmp/${HELM_FILE_NAME} -C /tmp \
  && mv /tmp/linux-amd64/helm /bin/helm \
  && rm -rf /tmp/* \
  && /bin/helm init --client-only
```

Create a `variant.mod` that defines:

- How you want to fetch available version numbers of `helm`(`dependencies.helm`)
- Which file to update/how to update it according to which variable managed by `mod`:

`variant.mod`:

```
provisioners:
  files:
    Dockerfile:
      source: Dockerfile.tpl
      arguments:
        helm_version: "{{ .helm.version }}"

helm:
    releasesFrom:
      githubReleases:
        source: helm/helm
    version: "> 1.0.0"
```

`Dockerfile`(Updated automatically by `mod`:

```
FROM alpine:3.9

ARG HELM_VERSION=2.14.3

ADD http://storage.googleapis.com/kubernetes-helm/${HELM_FILE_NAME} /tmp
RUN tar -zxvf /tmp/${HELM_FILE_NAME} -C /tmp \
  && mv /tmp/linux-amd64/helm /bin/helm \
  && rm -rf /tmp/* \
  && /bin/helm init --client-only
```

Run `mod build` and see `mod` retrieves the latest version number of `helm` that satisfies the semver constraint `> 1.0.0` and updates your `Dockerfile` by rendering `Dockerfile.tpl` accordingly.

## Next steps

### Automate Dependency Updates

Use [mod-action, a GitHub action for running mod](https://github.com/variantdev/mod-action) to periodically run `mod` and submit a pull request to update everything managed via `mod` automatically

## Examples

Navigate to the following examples to see practical usage of `mod`:

- [examples/eks-k8s-vers](https://github.com/variantdev/mod/blob/master/examples/eks-k8s-vers) for updating your [eksctl] cluster on new K8s release
- [examples/image-tag-in-dockerfile-and-config](https://github.com/variantdev/mod/tree/master/examples/image-tag-in-dockerfile-and-config) for updating your `Dockerfile` and `.circleci/config.yml` on new Golang release

## Use-cases

- Automating container image updates
- Initializing repository manually created from a [GitHub Repository Template](https://help.github.com/en/articles/creating-a-repository-from-a-template)
  - Configure your CI to run `mod up --build --pull-request --title "Initialize this repository"` in response to "repository created" webhook and submit a PR for initialization
- Create and initialize repository from a [GitHub Repository Template](https://help.github.com/en/articles/creating-a-repository-from-a-template)
  - Run `mod create myorg/mytemplate-repo myorg/mynew-repo --build --pull-request`
- Automatically update container image tags in git-managed CRDs (See [flux#1194](https://github.com/fluxcd/flux/issues/1194) for the motivation

### Boilerplate project generator

`mod` is basically the package manager for any make/[variant](https://github.com/mumoshu/variant)-based project with git support. mod create creates a new project from a template repo by rendering all the [gomplate](https://github.com/hairyhenderson/gomplate)-like template on init.

`mod up` updates dependencies of the project originally created from the template repo, re-rendering required files.

## API Reference

### Template Functions

The following template functions are available for use within template provisioners:

- `{{ hasKey .Foo.Bar "mykey" }}` returns `true` if `.Foo.Bar` is a `map[string]interface{}` and the value for the key `mykey` is set.
- `{{ trimSpace .Str }}` removes spaces, tabs, and new-lines from `.Str`
``