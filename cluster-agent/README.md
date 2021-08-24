
# Managed GitOps Cluster Agent



Bootstrapped using:
```
go mod init github.com/jgwest/managed-gitops/cluster-agent

operator-sdk-v1.11 init --domain redhat.com

operator-sdk-v1.11 create api --group managed-gitops --version v1alpha1 --kind Operation --resource --controller

```


