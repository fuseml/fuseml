## Reusing the training image

## Overview

Issue: [E2e MLFlow use-case: don't rebuild container images if content doesn't change](https://github.com/fuseml/fuseml/issues/162)

In the end-to-end workflow tutorial used as our current example, the container image used as a runtime environment for training the MLFlow model is re-built every time a codeset update is published. This is time/storage consuming and only necessary if the conda.yaml requirements change. We should devise a way to allow the step (re)building the environment to be skipped and allow the training step to reuse the container image matching the current code contents.

## Current state

Before building the image, the `builder-prep` task needs to be executed. That task will provide the Dockerfile used by the `builder` task. The end-to-end workflow tutorial uses a Dockerfile that builds an image based
on the `ghcr.io/fuseml/mlflow:1.14.1` image, plus the requirements from the codeset `conda.yaml` file.

The image is built using the kaniko task as in the [Tekton Catalog](https://github.com/tektoncd/catalog/blob/main/task/kaniko/0.4/kaniko.yaml), with minimal changes. The inputs for that task are the Dockerfile (provided by the `builder-prep`) task and the image name in the format: `<registry>/<repository>:<tag>` where `registry` is a local registry (registry.fuseml-registry), repository is `mlflow-builder/<codeset-name>` and tag is the codeset revision fetched from the git repository where the codeset is stored.

With the builder task successfully ran, the expected outcome is an image that includes all dependencies from the `conda.yaml` of the codeset stored on the local FuseML registry.

## Proposed solution

### MLflow builder image

Create a new container image that can be used to build the mlflow trainer image. That new image should provide a Dockerfile for building the trainer image and, build and push the image to an OCI registry. That image can also be customized to perform operations related to the image building/pushing, such as checking if building the image is required or can be skipped based on its inputs. That image partially implements https://github.com/fuseml/fuseml/issues/59.

For checking if building the image is required, instead of tagging the image based on the codeset revision, tag it using a hash generated from the dependencies listed on the `conda.yaml` file. With that tag, it is possible to use a tool, such as [docker-ls](https://github.com/mayflower/docker-ls), to check if there is already an image with the same dependencies already present on the registry and build it only when such an image does not exist.

### Affects

1. **Example `mlflow-sklearn-e2e` workflow**
   - The builder step should use the new mlflow-builder image and its output will be the image location from where it can be pulled, including the tag. This means that the
     builder step will not rely on the `kaniko` and `builder-prep` tasks anymore.

    ```
    --- a/workflows/mlflow-sklearn-e2e.yaml
    +++ b/workflows/mlflow-sklearn-e2e.yaml
    @@ -17,17 +17,15 @@ outputs:
        type: string
    steps:
      - name: builder
    -    image: ghcr.io/fuseml/mlflow-dockerfile:0.1
    +    image: ghcr.io/fuseml/mlflow-builder:v0.1
        inputs:
          - codeset:
              name: '{{ inputs.mlflow-codeset }}'
              path: /project
        outputs:
    -      - name: mlflow-env
    -        image:
    -          name: 'registry.fuseml-registry/mlflow-builder/{{ inputs.mlflow-codeset.name }}:{{ inputs.mlflow-codeset.version }}'
    +      - name: image
      - name: trainer
    -    image: '{{ steps.builder.outputs.mlflow-env }}'
    +    image: '{{ steps.builder.outputs.image }}'
        inputs:
          - codeset:
              name: '{{ inputs.mlflow-codeset }}'
    ```


### Future Considerations

#### Reusing the image between codesets

The proposed solution can also be extended to allow different codesets to reuse the same image in case they share the same requirements. To achieve that, the image must be stored on a common repository on the same registry. In that case, might make sense to have the repository name based on the built image, for the example workflow case it could be for example: `mlflow-trainer/envs`. With that, changing the FuseML workflow to receive only the registry, instead of registry/repository/tag, might also make sense.
