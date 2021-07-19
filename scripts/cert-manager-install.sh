#!/bin/bash

if kubectl get svc cert-manager -n cert-manager >/dev/null 2>&1 ; then
  echo "cert-manager already installed"
  exit
fi

export CERT_MANAGER_VERSION=v1.4.0

# Install Cert Manager
kubectl apply --validate=false -f https://github.com/jetstack/cert-manager/releases/download/${CERT_MANAGER_VERSION}/cert-manager.yaml
kubectl wait --for=condition=available --timeout=600s deployment/cert-manager-webhook -n cert-manager