#!/bin/bash

if helm list --namespace seldon-system -q | grep -q seldon-core ; then
    echo "seldon-core already installed under 'seldon-system' namespace"
    exit
fi

helm install seldon-core seldon-core-operator --create-namespace --repo https://storage.googleapis.com/seldon-charts --set usageMetrics.enabled=true --namespace seldon-system --set istio.enabled=true --set istio.gateway="seldon-system/seldon-gateway"

# Note the namespace, it has to match the path provided to the helm chart for istio.gateway value
cat <<EOF | kubectl apply -f -
apiVersion: networking.istio.io/v1alpha3
kind: Gateway
metadata:
  name: seldon-gateway
  namespace: seldon-system
spec:
  selector:
    istio: ingressgateway
  servers:
  - port:
      number: 80
      name: http
      protocol: HTTP
    hosts:
    - "*"
EOF
