#!/bin/bash

cd kustomize

kustomize build --load-restrictor LoadRestrictionsNone > ../dbschema-config-map.yaml

