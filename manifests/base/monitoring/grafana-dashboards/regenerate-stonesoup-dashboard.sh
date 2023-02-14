#!/bin/bash

# See README.md for the purpose of this script

cp managed-gitops/gitops-dashboard.json stonesoup/gitops-dashboard.json

sed -i.bak -e 's/PBFA97CFB590B2093/PF224BEF3374A25F8/g' stonesoup/gitops-dashboard.json

rm stonesoup/*.bak

