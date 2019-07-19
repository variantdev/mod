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
