# Example: eks-k8s-vers

This example demonstrates a basic usage of `mod` to stream-line K8s version updates on EKS with `eksctl`.

## Problem

How should we automate updating K8s versions of EKS clusters?

With `eksctl`, creating an EKS cluster is a matter of writing a `cluster.yaml` like:

```
apiVersion: eksctl.io/v1alpha5
kind: ClusterConfig
metadata:
  name: k8s1
  region: ap-northeast-1
  version: 1.12.6
nodeGroups:
- name: ng1
  instanceType: m5.xlarge
  desiredCapacity: 1
```

However, updating the K8s version used by this cluster requires the following steps:

- Get the available versions of K8s on EKS and decide the target version
- Change `version: 1.12.6` to e.g. `version: 1.13.7`
- Run `eksctl update`

Repeating these steps gets cumbersome when you have many clusters.

`mod` allows you to automate the first 2 steps.

## Solution

Firstly, we need a script that collects all the available K8s versions on EKS.

We use `main.go` in this directory as the script. The script should print a text where each line contains one release version:

```console
$ go run main.go
1.13.7
1.12.6
1.11.8
1.10.13
```

Now create a go template of your eksctl `cluster.yaml` that looks like:

```yaml
apiVersion: eksctl.io/v1alpha5
kind: ClusterConfig
metadata:
  name: {{.name}}
  region: {{.region}}
  version: {{.version}}
nodeGroups:
- name: ng1
  instanceType: m5.xlarge
  desiredCapacity: 1
  volumeSize: 100
  volumeType: gp2
  volumeEncrypted: true
```

`version` is the K8s version number to be used by eksctl.

Create `variant.mod` contains the module definition:

```yaml
provisioners:
  files:
    cluster.yaml:
      source: cluster.yaml.tpl
      arguments:
        name: k8s1
        region: ap-northeast-1
        version: "{{.k8s.version}}"

dependencies:
  k8s:
    releasesFrom:
      exec:
        command: go
        args:
        - run
        - main.go
    version: "> 1.10"
```

Run `mod up` to fetch the available K8s version from `main.go` and save the latest version that matches the version constraint `> 1.10`.

```console
$ cp cluster.yaml{,.bak}

$ mod up

$ cat variant.lock
dependencies:
  k8s:
    version: 1.13.7
```

Run `mod provision` to update `cluster.yaml` with the latest K8s version.

```console
$ mod provision
```

```console
$ diff --unified cluster.yaml{.bak,}
```

```patch
--- cluster.yaml.bak	2019-07-19 15:54:07.000000000 +0900
+++ cluster.yaml	2019-07-19 15:54:15.000000000 +0900
@@ -3,8 +3,11 @@
 metadata:
   name: k8s1
   region: ap-northeast-1
-  version: 1.12.6
+  version: 1.13.7
 nodeGroups:
 - name: ng1
   instanceType: m5.xlarge
   desiredCapacity: 1
```

Finally run `eksctl update` to actually update the cluster with the new K8s version:

```console
$ eksctl update -f cluster.yaml
```

Or defer it to your GitOps pipeline by git-commit/pushing it to your GitOps repo:

```console
$ git add variant.lock cluster.yaml
$ git commit -m 'Update K8s'
$ git push origin update-k8s
$ hub pull-request
```
