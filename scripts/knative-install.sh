#!/bin/bash

if kubectl get svc controller -n knative-serving >/dev/null 2>&1 ; then
  echo "knative-serving already installed"
  exit
fi

export KNATIVE_VERSION=v0.24.0

# Install Knative
kubectl apply --filename https://github.com/knative/serving/releases/download/${KNATIVE_VERSION}/serving-crds.yaml
kubectl apply --filename https://github.com/knative/serving/releases/download/${KNATIVE_VERSION}/serving-core.yaml
kubectl wait --for=condition=available --timeout=600s deployment --all -n knative-serving

# Configure Istio as ingress provider for Knative
kubectl apply --filename https://github.com/knative/net-istio/releases/download/${KNATIVE_VERSION}/release.yaml
kubectl wait --for=condition=available --timeout=600s deployment -l networking.knative.dev/ingress-provider=istio -n knative-serving


# Patch the KNative domain configuration with the domain pointing to the Istio ingress gateway.
# This is computed as follows:
# * if a custom domain was used to install FuseML and FuseML is also using Istio, use that domain
# * otherwise, compute the domain as an nip.io subdomain from the Istio ingress gateway load balancer IP address
ISTIO_DOMAIN=$(kubectl get virtualservice fuseml-core -n fuseml-core -o=jsonpath='{.spec.hosts[0]}' | sed -e "s/fuseml-core\.//")
ISTIO_LB_IP=$(kubectl -n istio-system get service istio-ingressgateway -o=jsonpath='{.status.loadBalancer.ingress[0].ip}')
if [ -z "$ISTIO_DOMAIN" ]; then
  if [ ! -z "$ISTIO_LB_IP" ]; then
    ISTIO_DOMAIN="$ISTIO_LB_IP.nip.io"
  else
    echo "WARNING: could not update the KNative domain configuration with a domain matching the Istio ingress gateway !"
    exit
  fi
fi

kubectl -n knative-serving patch configmap config-domain --type merge \
  -p "{\"data\":{\"$ISTIO_DOMAIN\":\"\"}}"
