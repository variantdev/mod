# mod

[![CircleCI](https://circleci.com/gh/variantdev/mod.svg?style=svg)](https://circleci.com/gh/variantdev/mod)

`mod` is a universal package manager complements task runners and build tools like `make` and [variant](https://github.com/mumoshu/variant).

It turns any set of files in Git/S3/GCS/HTTP as a reusable module with managed, versioned dependencies.

Think of it as a `vgo`, `npm`, `bundle` alternative, but for any project.

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

- [Learn about use-cases](#use-cases)
  - [Using variantmod as a staged gitops deployment manager](#staged-gitops-deployment-manager)
- [See examples](#examples)
- [Read API reference](#api-reference)

## Use-cases

- Automate Any-Dependency Updates with GitHub Actions v2
  - Use [mod-action, a GitHub action for running mod](https://github.com/variantdev/mod-action) to periodically run `mod` and submit a pull request to update everything managed via `mod` automatically
- Automating container image updates
- Initializing repository manually created from a [GitHub Repository Template](https://help.github.com/en/articles/creating-a-repository-from-a-template)
  - Configure your CI to run `mod up --build --pull-request --title "Initialize this repository"` in response to "repository created" webhook and submit a PR for initialization
- Create and initialize repository from a [GitHub Repository Template](https://help.github.com/en/articles/creating-a-repository-from-a-template)
  - Run `mod create myorg/mytemplate-repo myorg/mynew-repo --build --pull-request`
- Automatically update container image tags in git-managed CRDs (See [flux#1194](https://github.com/fluxcd/flux/issues/1194) for the motivation

### Boilerplate project generator

`mod` is basically the package manager for any make/[variant](https://github.com/mumoshu/variant)-based project with git support. mod create creates a new project from a template repo by rendering all the [gomplate](https://github.com/hairyhenderson/gomplate)-like template on init.

`mod up` updates dependencies of the project originally created from the template repo, re-rendering required files.

## Staged gitops deployment manager

You can define one or more `stages` so that `mod` can update stages one by one promoting `revisions` tested with previous stages to later stages.

Let's say you want three stages, `development`, `staging` and `production`, where `development` stage has `test` envirnoment associated, and `staging` stage has `manual` and `stress` environemnts, and `production` stage has `prod` environment associated.

```
stages:
- name: development
  environments:
  - test
- name: staging
  environments:
  - qa
  - stress
- name: production
  environments:
  - prod

provisioners:
  files:
    helmfile:
      path: states/helmfile.{{ .stage.environment }}.yaml
      source: templates/helmfile.yaml.tpl
      arguments:
        MYAPP_VERSION: "{{ .stage.dependencies.myapp.version }}"

dependencies:
  myapp:
    releasesFrom:
      dockerImageTags:
        source: example.com/myorg/myapp
    version: "> 0.1.0"
```

When there's any update in dependencies, you will firstly deploy it to the `test` environment by running:

```
$ mod up development --build --pull-request
```

This command updates the list of dependencies, and updates the `development` stage if and only if there were one or more new versions of dependencies.

`mod` delegates the actual update for the `development` stage to your CD system. That's done by `mod` generating the desired state of the `development` stage by executing the file provider defined in the config.

In the above example, the desired state is rendered from the template file located at `states/helmfile.test.yaml`, whose content is generated by rendering the go template `templates/helmfile.yaml.tpl`, as defined in the config.

The environment name listed in stage's `environments` is available for use in file provisioner's `arguments`, so that you can generate the desired state depending on the environment included in the stage.

For `development` stage, `path: states/helmfile.{{ .stage.environment }}.yaml` is evaluated to `states/helmfile.test.yaml`, which is why `mod up development --build` updates `states/helmfile.test.yaml`.

In addition to generating the desired state, due to that the `--pull-request` flag is provided, `mod` automatically creates a pull request to update a gitops config repo, which is then reviewed and merged by your team.

Once the pull request has been merged and deployed, you can run the below command to promote the previous deployment passed the `test` stage to the next `staging` stage:

```
$ mod up staging --build --pull-request
```

For the above example, updating `staging` stage results in updating `states/helmfile.qa.yaml` and `states/helmfile.stress.yaml`.
For example, merging the pull request updating those two files should result in updating `qa` and `stress` Kubernetes clusters, so that the new revision can be tested by respective teams. 

After your QA team finished testing it in `qa` and your peformance team finished testing it in the `stress` environments, you can finally ship it to `production`:

```
$ mod up production --build --pull-request
```

As you might have guessed, this updates `states/helmfile.qa.yaml`, and merging the pull request updating it should trigger a production deployment.

## Examples

Navigate to the following examples to see practical usage of `mod`:

- [examples/eks-k8s-vers](https://github.com/variantdev/mod/blob/master/examples/eks-k8s-vers) for updating your [eksctl] cluster on new K8s release
- [examples/image-tag-in-dockerfile-and-config](https://github.com/variantdev/mod/tree/master/examples/image-tag-in-dockerfile-and-config) for updating your `Dockerfile` and `.circleci/config.yml` on new Golang release

## API Reference

## `regexpReplace` provisioner

`regexpReplace` updates any text file like Dockerfile with regular expressions.

Let's say you want to automate updating the base image of the below Dockerfile:

```
FROM helmfile:0.94.0

RUN echo hello
```

You can write a `variant.mod` file like the below so that `mod` knows where is the image tag to be updated:

```yaml
provisioners:
  regexpReplace:
    Dockerfile:
      from: "(FROM helmfile:)(\\S+)(\\s+)"
      to: "${1}{{.Dependencies.helmfile.version}}${3}"

dependencies:
  helmfile:
    releasesFrom:
      dockerImageTags:
        source: quay.io/roboll/helmfile
    version: "> 0.94.0"
```

### `docker` executable provisioner

Setting `provisioners.executables.NAME.platforms[].docker` allows you to run `mod exec -- NAME $args` where the executable is backed by a docker image which is managed by `mod`.

`variant.mod`:

```
parameters:
  defaults:
    version: "1.12.6"

provisioners:
  executables:
    dockergo:
      platforms:
        # Adds $VARIANT_MOD_PATH/mod/cache/CACHE_KEY/dockergo to $PATH
        # Or its shim at $VARIANT_MOD_PATH/MODULE_NAME/shims
      - docker:
          command: go
          image: golang
          tag: '{{.version}}'
          volume:
          - $PWD:/work
          workdir: /work
```

```console
$ go version
go version go1.12.7 darwin/amd64

$ mod exec -- dockergo version
go version go1.12.6 linux/amd64
```

### Template Functions

The following template functions are available for use within template provisioners:

- `{{ hasKey .Foo.Bar "mykey" }}` returns `true` if `.Foo.Bar` is a `map[string]interface{}` and the value for the key `mykey` is set.
- `{{ trimSpace .Str }}` removes spaces, tabs, and new-lines from `.Str`
``

# Similar Projects

- https://github.com/dailymotion/octopilot
