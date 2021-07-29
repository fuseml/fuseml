#!/bin/bash

set -e
set -u
set -o pipefail

CODESETS="sklearn tensorflow"
WORKFLOW="mlflow-e2e"

print_bold() {
    echo
    echo -e "\033[1m$1\033[0m"
}

wait_for_run() {
    retries=$1
    codeset=$2
    run_name=$(./fuseml workflow list-runs --name ${WORKFLOW} --codeset-name ${codeset} --format json | jq -r ".[0].name")
    for i in $(seq ${retries}); do
        if [[ "${i}" == ${retries} ]]; then
            print_bold "❌  Timeout waiting for the workflow run to finish."
            kubectl get pipelineruns ${run_name} -n fuseml-workloads -o json | jq ".status"
            exit 1
        fi

        run_status=$(./fuseml workflow list-runs --name ${WORKFLOW} --codeset-name ${codeset} --format json | jq -r ".[0].status")
        echo "  [${codeset}] status: ${run_status} (${i}/$((retries-1)))"

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

for cs in ${CODESETS}; do
    print_bold "➤ Register Codeset: ${cs}"
    ./fuseml codeset register --name ${cs} --project mlflow fuseml-examples/codesets/mlflow/${cs}
done

export ACCESS=$(kubectl get secret -n fuseml-workloads mlflow-minio -o json | jq -r '.["data"]["accesskey"]' | base64 -d)
export SECRET=$(kubectl get secret -n fuseml-workloads mlflow-minio -o json | jq -r '.["data"]["secretkey"]' | base64 -d)

sed -i -e "/AWS_ACCESS_KEY_ID/{N;s/value: [^ \t]*/value: $ACCESS/}" fuseml-examples/workflows/${WORKFLOW}.yaml
sed -i -e "/AWS_SECRET_ACCESS_KEY/{N;s/value: [^ \t]*/value: $SECRET/}" fuseml-examples/workflows/${WORKFLOW}.yaml

print_bold "➤ Create Workflow: ${WORKFLOW}"
# Delete workflow if already exists
if ./fuseml workflow get -n ${WORKFLOW} &> /dev/null; then
    ./fuseml workflow delete -n ${WORKFLOW}
fi
./fuseml workflow create fuseml-examples/workflows/${WORKFLOW}.yaml

for cs in $CODESETS; do
    print_bold "➤ Assign Workflow to Codeset: ${cs}"
    ./fuseml workflow assign --name ${WORKFLOW} --codeset-name ${cs} --codeset-project mlflow
    ./fuseml workflow list-runs --name ${WORKFLOW}
done

retries=121
print_bold "⏱  Waiting $(((retries-1)*15/60))m for workflow runs to finish..."
for cs in ${CODESETS}; do
    wait_for_run ${retries} ${cs} &
done
wait

print_bold "➤ List Applications:"
./fuseml application list

for cs in ${CODESETS}; do
    PREDICTION_URL=$(./fuseml application get -n mlflow-${cs} --format json | jq -r ".url")
    print_bold "⛓  [${cs}] Prediction URL: ${PREDICTION_URL}"

    print_bold "➤ Perform prediction:"
    data="fuseml-examples/prediction/data-${cs}.json"
    curl -sd @${data} ${PREDICTION_URL} | jq

    prediction=$(curl -sd @${data} $PREDICTION_URL)

    case ${cs} in
        sklearn)
            result=$(jq -r ".outputs[0].data[0]" <<< ${prediction})
            expected_result="6.486344809506676"
            ;;
        tensorflow)
            result=$(jq -r ".predictions[0].all_classes[0]" <<< ${prediction})
            expected_result="0"
            ;;
    esac

    if [[ "$result" != "${expected_result}" ]]; then
        print_bold "❌  Prediction result expected ${expected_result} but got ${result}"
        exit 1
    fi
done