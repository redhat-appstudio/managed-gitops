kubectl patch configmap argocd-cm -n gitops-service-argocd --type=json -p='[{"op": "remove", "path": "/data/resource.customizations.health.apps_Deployment"}]'