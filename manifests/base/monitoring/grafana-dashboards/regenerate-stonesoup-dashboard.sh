#!/bin/bash

# See README.md for the purpose of this script

cp managed-gitops/gitops-dashboard.json stonesoup/gitops-dashboard.json
cp managed-gitops/gitops-argocd-dashboard.json stonesoup/gitops-argocd-dashboard.json

sed -i.bak -e 's/PBFA97CFB590B2093/PF224BEF3374A25F8/g' stonesoup/gitops-dashboard.json
sed -i.bak -e 's/PBFA97CFB590B2093/PF224BEF3374A25F8/g' stonesoup/gitops-argocd-dashboard.json

rm stonesoup/*.bak
