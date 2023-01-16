#!/bin/bash

# -----------------

# Using OpenShift's Prometheus instance requires us to enable user workload monitoring via the openshift-monitoring ConfigMap:
# https://docs.openshift.com/container-platform/4.11/monitoring/enabling-monitoring-for-user-defined-projects.html)
# - BUT, the way I'm doing it here is a brittle hack :P
kubectl apply -f prometheus/enable-user-workload-monitoring.yaml

# Allow Prometheus process to view the openshift-operators/gitops namespaces
kubectl apply -f prometheus/prometheus-roles-for-openshift-operators.yaml -n openshift-operators
kubectl apply -f prometheus/prometheus-roles-for-gitops-ns.yaml -n gitops

# -----------------
echo 
echo "* Kick off an OLM install of Grafana, and wait for it to complete"

kubectl create ns grafana > /dev/null 2>&1

kubectl apply -f grafana/grafana-operator-group.yaml -n grafana
kubectl apply -f grafana/grafana-subscription.yaml -n grafana

echo
echo "* Waiting for Grafana CRDs to exist"

while : ; do
  kubectl get customresourcedefinition/grafanas.integreatly.org  > /dev/null 2>&1 && break
  sleep 1
done

# -----------------

echo "* Waiting to acquire cluster host name"
while : ; do
  kubectl get route/thanos-querier -n openshift-monitoring -o yaml  > /dev/null 2>&1 && break
  sleep 1
done

# We determine the cluster hostname by looking at the thanos-queroer route, which should already exist
# on the cluster by default, and already have a hostname in its Route.

HOSTNAME=`kubectl get route/thanos-querier -n openshift-monitoring -o yaml  | grep "    host:" | cut -c11-`
HOSTNAME=`echo $HOSTNAME | sed 's/thanos-querier-openshift-monitoring/grafana/g'`


export ADMIN_SECRET_VALUE=`openssl rand -hex 10`

echo
echo "* Grafana route is: https://$HOSTNAME"
echo "  Username: user"
echo "  Password: $ADMIN_SECRET_VALUE"
echo

TMP_DIR=`mktemp -d`

# Substitute the cluster domain into the Grafana Ingress CR
cp -f grafana/grafana-cr.yaml $TMP_DIR/grafana-cr-resolved.yaml
sed -i.bak 's/HOSTNAME/'$HOSTNAME'/g' $TMP_DIR/grafana-cr-resolved.yaml
sed -i.bak 's/ADMIN_SECRET_VALUE/'$ADMIN_SECRET_VALUE'/g' $TMP_DIR/grafana-cr-resolved.yaml

kubectl apply -f $TMP_DIR/grafana-cr-resolved.yaml -n grafana

rm -f "$TMP_DIR/grafana-cr-resolved.yaml"

# The kubectl equivalent to: 'oc adm policy add-cluster-role-to-user cluster-monitoring-view -z grafana-serviceaccount'
kubectl apply -f grafana/grafana-cluster-role-binding.yaml -n grafana

# -----------------
echo
echo "* Waiting for Grafana service account token secret to exist"
while : ; do
  kubectl get secrets -n grafana | grep "grafana-serviceaccount-token"  > /dev/null 2>&1 && break
  sleep 1
done

echo
echo "* Applying GrafanaDataSource, using Grafana Service Account Token"

GRAFANA_SECRET=`kubectl -n grafana get secrets | grep "grafana-serviceaccount-token" |  cut -d ' ' -f 1`

GRAFANA_SA_TOKEN=`kubectl -n grafana get secret $GRAFANA_SECRET -o jsonpath={.data.token} | base64 -d`

cp -f grafana/grafana-data-source.yaml  $TMP_DIR/grafana-data-source-resolved.yaml

sed -i.bak 's/GRAFANA_SA_TOKEN/'$GRAFANA_SA_TOKEN'/g' $TMP_DIR/grafana-data-source-resolved.yaml

kubectl apply -f $TMP_DIR/grafana-data-source-resolved.yaml

rm -f "$TMP_DIR/grafana-data-source-resolved.yaml"

# This section was based on https://www.redhat.com/en/blog/custom-grafana-dashboards-red-hat-openshift-container-platform-4

# -----------------
echo
echo "* Create Argo CD dashboards"

# Argo CD dashboard is based on example Grafana dashboard from upstream Argo CD docs
kubectl apply -f dashboards/argo-cd/argo-grafana-dashboard-cm.yaml
kubectl apply -f dashboards/argo-cd/grafana-argo-dashboard.yaml

# Create a ServiceMonitor for GitOps Operator, in the openshift-gitops namespace
# - This SHOULD instead be created in the openshift-operators namespace (which is where the gitops-operator lives),
#   but the prometheus-operator process has a hardcoded list of namespaces that it checks, openshift-operators
#   is not on it.

echo "* Create GitOps Operator ServiceMonitor"

kubectl apply -f prometheus/openshift-operators-service-monitor.yaml -n openshift-gitops

# -----------------



