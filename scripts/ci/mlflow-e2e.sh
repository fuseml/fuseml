#!/bin/bash

set -e
set -u
set -o pipefail

print_bold() {
    echo
    echo -e "\033[1m$1\033[0m"
}

if ! git describe --tags --exact-match &> /dev/null; then
    print_bold "➤ Build FuseML client:"
    if [ -d "fuseml-core" ]; then
        cd fuseml-core
        git fetch origin
        git reset --hard origin/main
        git clean -f -d
    else
        git clone https://github.com/fuseml/fuseml-core.git
        cd fuseml-core
    fi
    make deps generate build_client
    mv bin/fuseml ../fuseml
    cd ..
    rm -rf fuseml-core
fi


export FUSEML_SERVER_URL=http://$(kubectl get VirtualService -n fuseml-core fuseml-core -o jsonpath="{.spec.hosts[0]}")
print_bold "⛓  FuseML URL: ${FUSEML_SERVER_URL}"
./fuseml version

print_bold "➤ Checkout examples repository:"
if [ -d "fuseml-examples" ]; then
    cd fuseml-examples
    git fetch origin
    git reset --hard origin/main
    git clean -f -d
    cd ..
else
    git clone --depth 1 https://github.com/fuseml/examples.git fuseml-examples
fi

print_bold "➤ Register Codeset:"
./fuseml codeset register --name "mlflow-wines" --project "mlflow-project-01" "fuseml-examples/codesets/mlflow-wines"

export ACCESS=$(kubectl get secret -n fuseml-workloads mlflow-minio -o json | jq -r '.["data"]["accesskey"]' | base64 -d)
export SECRET=$(kubectl get secret -n fuseml-workloads mlflow-minio -o json | jq -r '.["data"]["secretkey"]' | base64 -d)

sed -i -e "/AWS_ACCESS_KEY_ID/{N;s/value: [^ \t]*/value: $ACCESS/}" fuseml-examples/workflows/mlflow-sklearn-e2e.yaml
sed -i -e "/AWS_SECRET_ACCESS_KEY/{N;s/value: [^ \t]*/value: $SECRET/}" fuseml-examples/workflows/mlflow-sklearn-e2e.yaml

print_bold "➤ Create Workflow:"
# Delete workflow if already exists
if ./fuseml workflow get -n mlflow-sklearn-e2e &> /dev/null; then
    ./fuseml workflow delete -n mlflow-sklearn-e2e
fi

./fuseml workflow create fuseml-examples/workflows/mlflow-sklearn-e2e.yaml

print_bold "➤ Assign Workflow:"
./fuseml workflow assign --name mlflow-sklearn-e2e --codeset-name mlflow-wines --codeset-project mlflow-project-01
./fuseml workflow list-runs --name mlflow-sklearn-e2e

retries=121
print_bold "⏱  Waiting $(((retries-1)*15/60))m for the workflow run to finish..."
run_name=$(./fuseml workflow list-runs --name mlflow-sklearn-e2e --format json | jq -r ".[0].name")
for i in $(seq ${retries}); do
    if [[ "${i}" == ${retries} ]]; then
        print_bold "❌  Timeout waiting for the workflow run to finish."
        kubectl get pipelineruns ${run_name} -n fuseml-workloads -o json | jq ".status"
        exit 1
    fi

    run_status=$(./fuseml workflow list-runs --name mlflow-sklearn-e2e --format json | jq -r ".[0].status")
    echo "  status: ${run_status} (${i}/$((retries-1)))"

    case ${run_status} in
        Unknown|Started|Running) sleep 15 ;;
        Failed)
            print_bold "❌  Workflow run failed: "
            kubectl get pipelineruns ${run_name} -n fuseml-workloads -o json | jq ".status"
            exit 1
            ;;
        *) break ;;
    esac
done

print_bold "➤ List Applications:"
./fuseml application list

PREDICTION_URL=$(./fuseml application get -n mlflow-project-01-mlflow-wines --format json | jq -r ".url")
print_bold "⛓  Prediction URL: ${PREDICTION_URL}"

print_bold "➤ Perform prediction:"
data="fuseml-examples/prediction/data-wines-kfserving.json"
curl -sd @${data} ${PREDICTION_URL} | jq

result=$(curl -sd @${data} $PREDICTION_URL | jq -r ".outputs[0].data[0]")

pred_expected_result="6.486344809506676"
if [[ "$result" != "${pred_expected_result}" ]]; then
    print_bold "❌  Prediction result expected ${pred_expected_result} but got ${result}"
    exit 1
fi