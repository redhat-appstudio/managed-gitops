
The `modified-for-kcp` folder is a modified version of the Argo CD install YAML, specifically to work on KCP.

The `2.4.10` folder is the unmodified version of that same Argo CD YAML. This allows us to compare the two for differences (until we have kustomization.yaml to generated the KCP modified version for us.)

The individual manifest files were split using [kubernetes-split-yaml](https://github.com/mogensen/kubernetes-split-yaml/)
