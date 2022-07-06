#!/usr/bin/env bash

ARGO_CD_VERSION="${ARGO_CD_VERSION:-stable}"
ARGO_CD_NAMESPACE="${ARGO_CD_NAMESPACE:-gitops-service-argocd}"
ARGO_CD_PORT="${ARGO_CD_PORT:-4000}"

# This script installs ArgoCD vanilla according to https://argo-cd.readthedocs.io/en/stable/getting_started/

# Checks if a binary is present on the local system
exit_if_binary_not_installed() {
    for binary in "$@"; do
        command -v "$binary" >/dev/null 2>&1 || {
            echo >&2 "Script requires '$binary' command-line utility to be installed on your local machine. Aborting..."
            exit 1
        }
    done
}

# Install 'ArgoCD Web UI' in your Kubernetes cluster
if [ "$1" = "install" ]; then
    exit_if_binary_not_installed "kubectl"

    if kubectl -n "$ARGO_CD_NAMESPACE" get pods | grep argocd-server | grep '1/1' | grep 'Running' &>/dev/null; then
        echo "ArgoCD is already running..."
        echo "Skipping ArgoCD setup."
        exit 1
    fi

    # Apply the argo-cd-route manifest
    kubectl create namespace "$ARGO_CD_NAMESPACE" || true
    kubectl apply -n "$ARGO_CD_NAMESPACE" -f https://raw.githubusercontent.com/argoproj/argo-cd/$ARGO_CD_VERSION/manifests/install.yaml

    # Get the secret
    counter=0
    until kubectl -n "$ARGO_CD_NAMESPACE" get secret | grep argocd-initial-admin-secret; do
        ((counter++))
        sleep 1
        if [ "$counter" -gt 60 ]; then
            echo " --> Error: Cannot find argocd-initial-admin-secret secret."
            exit 1
        fi
    done
    echo " * argocd-initial-admin-secret secret has been created."

    # Wait until argocd-server pod is running
    echo " * Wait until argocd-server pod is running"
    counter=0
    until kubectl -n "$ARGO_CD_NAMESPACE" get pods | grep argocd-server | grep '1/1' | grep 'Running' &>/dev/null; do
        ((counter++))
        sleep 1
        if [ "$counter" -gt 60 ]; then
            echo " --> Error: argocd-server pod cannot start. Quitting ..."
            exit 1
        fi
    done
    echo " * argocd-server Pod is running."

    # Port forward the ArgoCD service locally, so we can access it
    kubectl port-forward svc/argocd-server -n "$ARGO_CD_NAMESPACE" "$ARGO_CD_PORT":443 &
    KUBE_PID=$!

    # Checks if ARGO_CD_PORT is occupied
    counter=0
    until lsof -i:"$ARGO_CD_PORT" | grep LISTEN; do
        sleep 1
        if [ "$counter" -gt 10 ]; then
            echo ".. retry $counter ..."
            echo " --> Error: Port-forwarding takes too long. Quiting ..."
            if ! kill $KUBE_PID; then
                echo " --> Error: Cannot kill the background port-forward, do it yourself."
            fi
            exit 1
        fi
    done
    echo " * Port-Forwarding worked"

    # Decode the password from the secret
    ARGOCD_PASSWORD=$(kubectl -n "$ARGO_CD_NAMESPACE" get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" | base64 --decode)

    # Stop port-forwarding
    if ! kill $KUBE_PID; then
        echo " --> Error: Cannot kill the background port-forward, do it yourself."
        exit 1
    else
        echo " * Port-forwarding has been successfully stopped"
    fi

    if lsof -i:"$ARGO_CD_PORT" | grep LISTEN; then
        echo " * Port forwarding is still active for some reason. Investigate further ..."
    fi

    # Exit now, do not continue with the rest of the bash script
    echo
    echo " ------------------------------"
    echo "| To access the ArgoCD Web UI |"
    echo " ------------------------------"
    echo
    echo "  - Run:            kubectl port-forward svc/argocd-server -n "$ARGO_CD_NAMESPACE" "$ARGO_CD_PORT":443 &"
    echo "  - Credentials:    HOST=localhost:"$ARGO_CD_PORT"  USERNAME=admin  PASSWORD=$ARGOCD_PASSWORD"
    echo
    exit 0
fi

# Remove 'ArgoCD Web UI' from your Kubernetes cluster
if [ "$1" = "remove" ]; then
    exit_if_binary_not_installed "kubectl"
    kubectl delete -n "$ARGO_CD_NAMESPACE" -f https://raw.githubusercontent.com/argoproj/argo-cd/$ARGO_CD_VERSION/manifests/install.yaml
fi
