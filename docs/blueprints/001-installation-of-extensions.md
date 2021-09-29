## Installation of ML extensions

## Overview

Epic: [Installer: add official support to install extensions](https://github.com/fuseml/fuseml/issues/161)

Our installer needs to be updated so that it provides a better UX regarding the installation of 3rd party tools. Currently, it installs MLFlow by default, but it has no coverage for KFServing at all, which makes for an awkward installation experience for those who want to follow our (currently solely available) tutorial on end-to-end serving with MLFlow and KFServing.

The installer should make a clear distinction between basic mandatory components and extensions for 3rd party tools, but at the same time continue to provide the "all-in-one" experience of installing everything in one shot. This is a summary of needed installer changes:

* installer command line arguments that the user can supply to indicate which optional extensions need to be installed (e.g. fuseml-install --extension mlflow --extension kfserving)
* make it possible to install/uninstall extensions in follow-up installer invocations, separately from the main installation
* MLFlow no longer installed by default
* add support to install KFServing and KNative.

## Current state

`fuseml-installer` has two ways of installing components

* `fuseml-installer install` - installs all necessary components (but actually _also_ MLFlow, which should be an extension)
* `make` targets. Specific targets defined in the `Makefile` describe the installation of other components, like KFServing

## Proposed solution

### Makefile

Get rid of specific installation targets that install something for production usage (e.g. not used by testing/development).

### CLI

1. Implement new option for `install` command

  `fuseml-installer install [--extension <name>] [--extension-repository <path>]`

  This command installs all necessary components AND extensions selected by `--extension` option. Optionally uses path specified
  by `--extension-repository` option as a source for extensions that should be installed. Path could be local directory on URL.


2. Implement new command for managing extensions. This could be useful for adding an extension to already configured system.

  `fuseml-installer extension`

  This should have several sub commands, like

  * `fuseml-installer extension list` - list installed extensions
  * `fuseml-installer extension available [--extension-repository <path>]` = list extensions available for installation
  * `fuseml-installer extension add <name> [--extension-repository <path>]` - install selected extension
  * `fuseml-installer extension remove <name> [--extension-repository <path>]` - ubinstalls selected extension

3. Add information about extensions into the fuseml-installer config file

### Description files

Extension repository is a directory (local or remote) containing description files of extensions. Each description file provides 
description of operations that could be done with the extension.

Each extension has its own subdirectory that matches the extension name. Mandatory component under each extension subdirectory is the
description file, in `yaml` format.

Directory structure:

```
extensions
 - installer
    - mlflow
      - description.yaml
      - values.yaml
    - knative
      - description.yaml
      - install.sh
      - uninstall.sh
```

Possible installation types for extensions:

* `helm` - helm chart. For helm chart type, no extra install/uninstall commands are necessary
* `manifest` - Kubernetes manifest, to be installaded using `kubectl`. No extra install/uninstall commands are necessary. All information (including namespace for example) are expected to be present in the manifest file.
* `kustomize` - directory with Kustomize files. It has to be full URL (that works as an input for `kubectl -k`) or a (absolute or relative) path to local directory
* `script` - shell script. Specific `install` and `uninstall` actions need to be provided by way of referencing specific scripts.

`script` type seems like a least secure one, and we should aim for replacing it with the other types in the future.

Location arguments:

`location`: could be either URL or a local path relative to the extension directory. If used together with a helm chart, `location` needs to point to tarball with the chart.

`repo`, `chart`: specific to helm charts only. `repo` is the chart repository from which the chart with `chart` name should be installed. To use this combination `location` field must be empty.

`values` pointing to values.yaml file of a helm chart - just like `location`, could be URL or a local path

Namespace:

Each step could have it's own namespace. If it is missing, namespace of extension is used. If this is missing or empty fuseml-installer will not use any namespace during the step operation (this indeed might be the correct scenario for example when installing CRDs).

Version:

Version of the helm chart to use. Applies only for `helm` installation step type.

#### Examples of extension files

```yaml
name: mlflow 
description: |
  MLFlow is an open source platform specialized in tracking ML experiments, and packaging and deploying ML models.
namespace: 
install:
  - type: helm
    location: https://github.com/fuseml/extensions/raw/charts/mlflow-0.0.1.tgz
    values: values.yaml
    version: 1.2.3
  - type: script
    namespace: fuseml-workloads
    location: post-install.yaml
    waitfor:
      - kind: pod
        namespace: fuseml-workloads
        selector: all
        condition: ready
        timeout: 300
uninstall:
  - type: helm
gateways:
  - name: mlflow
    servicehost: mlflow
    port: 80
  - name: minio
    servicehost: mlflow-minio
    port: 9000
```

```yaml
name: knative 
description: |
  Kubernetes-based platform to deploy and manage modern serverless workloads.
namespace: knative-serving
requires:
  - cert-manager
install:
  - type: script
    location: install.sh
uninstall:
  - type: script
    location: uninstall.sh
```

```yaml
name: seldon-core
namespace: seldon-system
install:
  - type: helm
    chart: seldon-core-operator
    repo: https://storage.googleapis.com/seldon-charts
    values: values.yaml
```

### Gateways

If `gateways` field is specified in the desctription, fuseml installer will create istio gateway(s) for a component. See the `mlflow` example for the syntax.


### Wait For

After the instruction from installation step are executed, it would be wise to wait until certain condition is true to make sure the installer may continue with the next step.
`waitfor` may indicate specific condition the installer should wait for. It takes the argument that could generally be passed to `kubectl wait` command.
Currently supported arguments are `kind` (if missing, defaults to `pod`), `namespace`, `condition` (defaults to `ready`) `timeout` (in seconds; defaults to 300) and `selector`. If the value of `selector` is `all`, it is gonna wait for all resources of given kind to reach the condition. Otherwise the selector's value is treated like the value for `--selector` option of `kubectl wait` command.

### Location

Default location of extension description is github repository owned by FuseML project.
We can use https://github.com/fuseml/extensions, however this repository currently hosts Docker images for extensions used during the FuseML workflow.
To distinguish from instalation description files, we just need to pick proper directory structure:

`extensions/installer` for description files used by the installer

`extensions/images` for docker images

`extensions/charts` for Helm charts

### Dependencies

Extensions can depend one on another. If a description file contains `requires` field, the value is expected to be a list of names of other extensions that are considered requirements for current one.

Fuseml-installer will take care that such required extensions get installed in the right order, so they do not need to be explicitly listed on the command line.
