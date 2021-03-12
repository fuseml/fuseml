````
export HELM_EXPERIMENTAL_OCI=1
helm chart pull private-registry.prv.suse.net/framalho/mlflow-helm:0.0.1
helm chart export private-registry.prv.suse.net/framalho/mlflow-helm:0.0.1
helm install mlflow ./mlflow --namespace mlflow --create-namespace --dependency-update \
  --set ingress.hosts='{mlflow.10.160.5.140.nip.io}',minio.ingress.hosts='{minio.10.160.5.140.nip.io}'
````
