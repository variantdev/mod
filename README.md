# mod

See examples to get started:

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

## Templates

The following template functions are available for use within template provisioners:

- `{{ hasKey .Foo.Bar "mykey" }}` returns `true` if `.Foo.Bar` is a `map[string]interface{}` and the value for the key `mykey` is set.
- `{{ trimSpace .Str }}` removes spaces, tabs, and new-lines from `.Str`
