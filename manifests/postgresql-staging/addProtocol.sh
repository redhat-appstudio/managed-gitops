#!/usr/bin/env bash

# KCP till release-0.7 has no knowledge of protocol by default, and hence we need to
# add these protocols in our yamls explicitly. Here, we are defining explicitly TCP.

yq  e -i 'select(.kind == "Service").spec.ports[] += {"protocol": "TCP"}' postgresql-staging.yaml
yq  e -i 'select(.kind == "StatefulSet").spec.template.spec.containers[].ports[] += {"protocol": "TCP"}' postgresql-staging.yaml
