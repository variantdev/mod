# Example: Automated updates of container image tags in Dockerfile and CI config file

This example demonstrates a basic usage of `mod` to stream-line container image version updates for Dockerfile and CI config files.

## Problem

How should we automate updating container images tags?

A container image tag is usually appear in your `Dockerfile`:

```text
FROM golang:1.11
#snip
``` 

And in your CI config file like `.circleci/config.yml`:

```yaml
version: 2
jobs:
  build:
    docker:
      - image: circleci/golang:1.11
    steps:
      - checkout
      - run: build .
```

Updating the `golang` container image tag(`1.11`) requires the following steps:

- Get the available versions of the container image by [browsing DockerHub](https://hub.docker.com/_/golang?tab=tags)
- Change `golang:1.11` to e.g. `golang:1.12.7` in `Dockerfile` and `.circleci/config.yml`
- Run `docker build` and CI build

Repeating these steps gets cumbersome when you have many projects to maintain.

`mod` allows you to automate the first 2 steps.

## Solution

Create a go template of your `Dockerfile` and `.circleci/config.yml`:

`Dockerfile.tpl`:

```
FROM golang:{{.version}}

RUN echo test
```

`circleci.config.yml.tpl`:

```yaml
version: 2
jobs:
  build:
    docker:
      - image: circleci/golang:{{.version}}
    steps:
      - checkout
      - run: build .
```

`version` is the container image tag to be used.

Create `variant.mod` contains the module definition:

```yaml
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
```

Run `mod up` to fetch the available container image tags from DockerHub's via Registry v2 API and save the latest tag that matches the version constraint `> 1.10`.

```console
$ for f in Dockerfile circleci.config.yml; do cp $f{,.bak}; done

$ mod up

$ cat variant.lock
dependencies:
  go:
    version: 1.13.7
```

Run `mod provision` to update `Dockerfile` and `config.yml` with the latest tag.

```console
$ mod provision
```

```console
$ diff --unified Dockerfile{.bak,}
```

```patch
--- Dockerfile.bak	2019-07-26 15:15:51.000000000 +0900
+++ Dockerfile	2019-07-26 15:16:17.000000000 +0900
@@ -1,3 +1,3 @@
-FROM golang:1.11
+FROM golang:1.12.7

 RUN echo test
```

```console
$ diff --unified circleci.config.yml{.bak,}
```

```patch
--- circleci.config.yml.bak	2019-07-26 15:15:51.000000000 +0900
+++ circleci.config.yml	2019-07-26 15:16:17.000000000 +0900
@@ -2,7 +2,7 @@
 jobs:
   build:
     docker:
-      - image: circleci/golang:1.11
+      - image: circleci/golang:1.12.7
     steps:
       - checkout
       - run: build .
```

Push the updated files to your CI:

```console
$ git add variant.lock Dockerfile cirleci.config.yml
$ git commit -m 'Update golang'
$ git push origin update-golang
$ hub pull-request
```
