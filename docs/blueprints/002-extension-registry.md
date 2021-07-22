# Extension Registry

## Overview

Epic: [Installer: add official support to install extensions](https://github.com/fuseml/fuseml/issues/161)
Story: [Feature: extension registry](https://github.com/fuseml/fuseml/issues/74)

The FuseML core service should manage a central registry where it keeps information about the available extensions and
3rd party tools that it integrates with. This information can be interactively consumed by users, but more importantly,
is available as input for workflows and for future control plane features that we'll add to the FuseML core service.

The immediate application of an extension registry is to provide a place where endpoints, URLs, credentials and other
data required to access the 3rd party tools like MLFlow can be stored and accessed by the container images implementing
workflow steps.

## Current state

There is currently no dedicated means of dynamically storing information on the FuseML server describing where and how to access
3rd party tools and services. To compensate for this limitation, information such as 3rd party tool endpoint URLs and credentials
is instead provided using other extensibility mechanisms:
* in workflow definitions (as is currently the case with MLFlow)
* hardcoded inside the container images implemeting workflow steps
* hardcoded inside codesets
* even hardcoded directly into the FuseML server itself

This has significant negative impact on flexibiity and reusability, on user experience in general.

For 3rd party tools implemented as k8s controllers, such as KNative, this is less of a problem, because pods running as
pipeline steps are by default given access to create and manage k8s resources in the workloads namespace through a custom
service account. However, this will be more difficult to achieve when adding multi-cluster capabilities (see https://github.com/fuseml/fuseml/issues/146).

## Feature Requirements

The extension registry feature implements the following mandatory requirements:

1. as an Ops engineer (MLOps/DevOps/ITOps), I need a way to configure my FuseML instance with the parameters required to dynamically integrate with 3rd party AI/ML tools (e.g. URLs, endpoints, credentials, other tool specific configuration attributes). For non-production environments, this requirement can also come from ML engineers or even Data Scientists that are looking to quickly set up FuseML for experimentation purposes.

    Examples:

    * DS: I have an MLFlow tracking service already set up by me that I use for my ML experiments and I want to reuse it for FuseML automated workflows. I will configure FuseML with the information it needs to access the MLFlow tracking service (the tracker service URL and the hostname, username and keys for the storage backend). The information is stored in a central registry where FuseML workflows can access it.
    * DS: I'm using my Google cloud storage account to store datasets or ML models that I use in my local experiments. I want my FuseML automated workflows to upload/download artifacts using that same storage account, but I don't want to expose my credentials in the workflow definition or in the code I'm pushing to FuseML. I'll store those credentials in the FuseML extension registry and access them from my FuseML workflow steps.
    * Ops: I have an S3 storage service set up for my organization and I want to use that as a storage backend for FuseML artifacts (e.g. models, datasets).
    I will manage buckets, accounts and credentials and add them as extension records in the FuseML extension registry. ML engineers and DSs can then write FuseML workflows that have access to the S3 storage service without having to deal with these operational details.


2. as a ML engineer or Data Scientist, I need a list of the 3rd party tools that my FuseML instance is integrated with, to help me make decisions about how I implement and run my ML experiments and how I design my FuseML workflows

3. as a ML engineer or Data Scientist, I want to design FuseML workflows consisting of steps that interact with my AI/ML toolstack of choice, independently of how those tools are deployed. This makes my workflows generic and reusable:
* FuseML workflow definitions don't need to be updated when there are changes in the configuration of the 3rd party tools (e.g. upgrade, migration) or in the way they are integrated with FuseML (e.g. change of accounts, change in access permissions or credentials)
* configuring and integrating the tools with FuseML and configuring the FuseML workflows are independent operations and can be done by different roles requiring minimum interaction
* a FuseML workflow, once defined, can be executed across multiple FuseML instances and 3rd party tools deployment configurations, as long as the same set of tools are involved

    Examples:

    * DS: I'm writing a FuseML workflow to automate the end-to-end lifecycle of my ML model. I wrote my ML code using MLFlow during the experimentation phase so I want to also use MLFlow as a tracking service and model store for my workflow. I also want to use Seldon Core as an inference service. These tools (MLFlow and Seldon Core) have already been installed by DevOps and set up as entries in the FuseML extension registry. All I need to do is specify in the definition of my FuseML workflows which step requires which extension, and the FuseML orchestrator will automatically make that information available to the container images implementing those steps. This way, I don't need to concern myself with endpoints, accounts or credentials.
    * Ops: I need to migrate the on-prem S3 storage service used by my organization to a new location. I'm also using this storage service for a range of FuseML service instances in use by various AI/ML teams. All I need to do to re-configure the FuseML services is to update the entries I previously configured in their extension registries to point to the new URL. Future workflow executions will automatically pick-up the new values.
    * ML engineer: I'm using a staged approach to automating the deployment of my ML model in production. I have a development environment, a staging/testing environment and a production environment. I can write a complex FuseML workflow that I can reuse across these environments with minimal changes. The workflow definition is independent of how the 3rd party tools are deployed and set up for access by FuseML in these environments.


4. as an Ops engineer, when I install a FuseML 3rd party tool through the installer, I want to also be able to quickly register it as a FuseML extension

In addition, there are optional requirements to be considered at this time, some of which might not be fully implemented by this feature alone, but the implementation can at least be extensible enough to facilitate later implementation:

5. as an Ops engineer, I run multiple instances of the same AI/ML tool or I have several accounts configured that can be used to access the same tool using different permissions and resources (e.g. one for dev, one for testing, one for production). I want to be able to configure several instances of the same 3rd party tool in FuseML or different accounts to be used to access the same instance.

6. (derived from 4.) as an Ops engineer, I want to have more fine-tuned control over who in my organization has access to the AI/ML tools integrated with FuseML. E.g. I want to associate an extension with a FuseML group, project or user and only give access to the workflows running under that project/group/user to use the extension.

7. as a ML engineer or Data Scientist, I want to design FuseML workflows that are portable and reusable across different but equivalent AI/ML toolstacks

    Examples:

    * ML engineer: in one FuseML instance, I use Seldon Core for prediction serving, in another one I use KFServing. I can write a single FuseML workflow that is valid to both FuseML instances. FuseML automatically chooses the prediction server that is available and the prediction step specified in the workflow definition is compatible with both Seldon Core and KFServing.

## Notes and questions

* how do we associate access credentials to an entry in the extension registry:
  * global, or per-project
  * different uses (some creds for upload, others for download)

* in a multi-instance scenario, how does FuseML decide which instance of the same extension to use when required by a workflow step ?
  * one (and only one) instance could be marked as default in the config
  * instances could be global or project-scoped

* should we add the built-in services (gitea, tekton, trow) to the list of extensions, e.g. as immutable entries ?

## Proposed solution

Implementing an extension registry involves the following high-level changes:

* FuseML core updates:
  * implementation of an extension registry component
  * support to manage the extension registry through the FuseML REST API
  * extend the workflow capabilities:
    * add support in the workflow DSL to be able to reference an extension as a step requirement
    * implement a convention and mechanism to automatically resolve the extension requirement and provides the extension information as a step input

* installer updates:
  * support to "register"/"update"/"unregister" the details (endpoints, credentials etc) of 3rd party tools into the FuseML extension registry
  * 3rd party tools registered as extensions don't need to be running in the same k8s cluster as FuseML or even be k8s applications at all. We need to support all installation scenarios:
    * tool is installed/uninstalled using the FuseML installer, either at the same time as FuseML or at a later time. For tools installed/uninstalled with the FuseML installer, if feasible, registration/deregistration as an extension should be done automatically
    * tool is installed/uninstalled using another means, independent of FuseML
  * support to list registered extensions (TBD if sensitive data such as passwords should be included)

* CLI:
  * support to list registered extensions (TBD if sensitive data such as passwords should be included)

### Domain Model

This section covers the feature definition in business domain model terms, independent of I/O details such as state storage or REST APIs. A minimalistic approach is described first, followed by a more extensive one.

#### Minimalistic Proposal

The simplest form the extension registry record can take is a list of configuration parameters associated with a name and version, e.g.:

```yaml
name: mlflow-global
description: MLFlow tracking service - global instance
version: "1.19.0"
configuration:
  - name: MLFLOW_TRACKING_URI
    value: http://mlflow
  - name: MLFLOW_S3_ENDPOINT_URL
    value: http://mlflow-minio:9000
  - name: AWS_ACCESS_KEY_ID
    value: 24oT0SfbJPEu6kUbUKsH
  - name: AWS_SECRET_ACCESS_KEY
    value: cMGiZff8KqS5xWQ4eagRujh1tDcbQyRP0bEJSBOf
```

```yaml
name: kfserving-local-cluster
description: KFServing prediction service
version: "0.6.0"
configuration:
  - name: KFSERVING_UI_URL
    value: http://kfserving.10.120.130.140.nip.io/
```

Each entry represents a single tool deployment, embedding configuration information about all services that the tool provides and how they are exposed and can be accessed through endpoints. Multiple records need to be configured to represent multiple instances of the same tool. The configuration data is unstructured (i.e. FuseML can't tell which parameter is the URL, which contain sensitive information such as passwords, keys etc.).

For tools and services implemented as k8s controllers (KFServing, Seldon Core etc.) and installed in the same k8s cluster where FuseML workflows are executed, we generally don't need to include any configuration parameters pertaining to service access or authentication/authorization. We rely on the default service account and namespace configured for FuseML workloads to access these services. However, the extension registry implementation should account for the following FuseML features and improvements:

* multi-cluster access: the k8s cluster where a workflow step is executed may be different then the one where the tool is installed. In this case, the credentials needed to authenticate against the remote k8s cluster need to be provided explicitly to FuseML. We also need to refactor the container image implementing the workflow step to allow custom k8s credentials to be supplied instead of mounted from a service account secret.
* multi-tenant access: even in the context of the same k8s cluster, the k8s admin might prefer to set up one or more customized namespaces (e.g. to configure quotas, traffic rules and so on) to host the FuseML workloads associated with a particular tool and/or project, instead of reusing a single namespace for all FuseML workloads. Different namespaces require different credentials.

Multi-cluster and multi-tenant are both aspects that should be facilitated through FuseML features implemented explicitly for that purpose. For example, when a FuseML project is created, the FuseML server could automatically create a namespace dedicated to that project and set up the credentials for it. For multi-cluster access, the FuseML architecture might need to be expanded to include agent services running in the context of each cluster, or a centralized registry of k8s clusters and associated credentials could be implemented.

For the time being, we can settle with modelling the extension registry record in a way that doesn't make it difficult to expand FuseML with these features later.

Modifying the workflow domain model to reference the extension configuration data as a step input can be as simple as using the extension `name` field as a foreign key and having FuseML automatically convert all the extension configuration entries into environment variables, e.g.:

```yaml
[...]
steps:
[...]
  - name: trainer
    image: '{{ steps.builder.outputs.mlflow-env }}'
    inputs:
      - codeset:
          name: '{{ inputs.mlflow-codeset }}'
          path: '/project'
      - extension:
          name: mlflow-global
    outputs:
      - name: mlflow-model-url
    # no longer needed because they can be extracted from the extension record and generated automatically 
    #env:
    #  - name: MLFLOW_TRACKING_URI
    #    value: "http://mlflow"
    #  - name: MLFLOW_S3_ENDPOINT_URL
    #    value: "http://mlflow-minio:9000"
    #  - name: AWS_ACCESS_KEY_ID
    #    value: 24oT0SfbJPEu6kUbUKsH
    #  - name: AWS_SECRET_ACCESS_KEY
    #    value: cMGiZff8KqS5xWQ4eagRujh1tDcbQyRP0bEJSBOf
  - name: predictor
    image: ghcr.io/fuseml/kfserving-predictor:0.1
    inputs:
      - name: model
        value: '{{ steps.trainer.outputs.mlflow-model-url }}'
      - name: predictor
        value: '{{ inputs.predictor }}'
      - codeset:
          name: '{{ inputs.mlflow-codeset }}'
          path: '/project'
      - extension:
          name: kfserving-local-cluster
    outputs:
      - name: prediction-url
    # no longer needed because they can be extracted from the extension record and generated automatically
    #env:
    #  - name: AWS_ACCESS_KEY_ID
    #    value: 24oT0SfbJPEu6kUbUKsH
    #  - name: AWS_SECRET_ACCESS_KEY
    #    value: cMGiZff8KqS5xWQ4eagRujh1tDcbQyRP0bEJSBOf
```

Implementation detail: to further protect sensitive data from being leaked, FuseML could store all the configuration data associated with an extension record into a secret and reference that secret in the generated Tekton pipeline, instead of adding them as inline environment variable values.  

There are some disadvantages with this minimalistic model, addressed in the second proposal:

* extension configuration data is unstructured and opaque, we can't implement any logic in the FuseML core to take advantage of it. For example, we don't know which entries contain sensitive information that we need to hide from regular FuseML users and which contain information that regular FuseML users might benefit from (e.g. public, general-purpose URLs such as UIs)
* extension records themselves are unstructured. Aside from the name, there is nothing that FuseML can use to make better decision when resolving extension references automatically, such as being able to dynamically choose between multiple instances of the same service, or multiple services of the same category, based on version, location or accounting constraints. 
* organizing the extension information better would also improve the UX of accessing and consuming that information (e.g. listing extensions of a certain type).
* the workflow step environment variable names are mapped one-to-one to the extension configuration entry names. This can create conflict situations. It also limits the reusability across FuseML instances of container images implementing workflow steps, because they need to be custom tailored to the environment variable names used within a single FuseML instance.

#### Extended Proposal

The information captured in the extension record can be organized to have a more structured form, and to reflect common patterns extracted from the data used to describe known tools and services and their installations. The following hierarchy of elements is better suited to represent this data:

* the root _extension_ model element represents a single framework/platform/service/product developed and released or hosted under a unique name and operated as a single cohesive unit. We'll keep the list of possible values for the top-level element open and unregulated for the time being, but in the future, as we add extensions, we should also consider maintaining a pre-populated list of tools and services as a common reference that is valid across FuseML installations.
* one _extension_ element groups together several _extension instances_ - different installations of the same extension, with different or similar versions and configurations. This differentiation is needed to support requirements related to multi-instance.
* several individual _services_, which can be consumed separately, can be provided by the same _extension instance_. For example, an MLFlow instance is composed of an experiment tracking service/API, a model store service and a UI. A _service_ is represented by a single API or UI. We could try to model services as children immediately under the _extension_ top-level element, but it works better to use the instance as a parent, given that we're more interested in service _instances_. For extensions implemented as cloud-native applications, a _service_ is the equivalent of a k8s service that is used to expose a public API or UI. _Services_ should also be classified into known categories (e.g. s3, git), to make it easier to support portable workflows (see optional requirements), where a workflow step lists a service category as a requirement, and FuseML automatically resolves that to whatever particular service instance is available at runtime.
* a _service_ is exposed through several individual _endpoints_. Having a list of _endpoints_ associated with a single _service_ is particularly important for representing k8s services, which can be exposed both internally (cluster IP) and externally (e.g. ingress). Depending on the consumer location, FuseML can choose the endpoint that is accessible to and closer to the consumer. All _endpoints_ grouped under the same _service_ must be equivalent in the sense that they are backed by the same API and/or protocol.
* _services_ under the same _instance_ can be accessed using one of several _profiles_. A _profile_ can be generally used to embed information pertaining to the authentication and authorization features supported by a service or a group of services. This element allows administrators and operators of 3rd party tools integrated with FuseML to configure different accounts and credentials (tokens, certificates, passwords) to be associated with different FuseML organization entities (users, projects, groups etc.). All information embedded in a _profile_ is treated as sensitive information. In the future, we could further specialize this element to model a predefined list of supported standard authentication and authorization schemes. Each _profile_ has an associated scope that controls who has access to this information (e.g. global, project, user, workflow). This is the equivalent of a k8s secret. _Profiles_ are configured globally for an _extension instance_ and can be associated with individual services and endpoints.
* _configuration_ elements can be present under _extension instances_, _services_, _endpoints_ or _profiles_ and represent opaque, service specific configuration data that the consumers need in order to access and consume a service interface. _Configuration_ elements can be used to encode any information relevant for service clients: accounts and credentials, information describing the service or particular parameters that describe how the service should be used. For example, if endpoints are SSL secured, custom certificates (e.g. self-signed CA certificates) might be needed to access them and this should be included in the endpoint configuration. The information encoded in a _configuration_ element is only treated as sensitive information when present under a _profile_. Equivalent of a k8s configmap (or k8s secret, when under _profile_). 

Examples:

1. MLFlow instance deployed locally alongside FuseML and globally accessible:

  ```yaml
  name: mlflow
  product: mlflow
  description: MLFlow experiment tracking service
  instances:
    - name: default
      version: "1.19.0"
      location: local
      services:
        - name: mlflow-tracking
          service: mlflow-tracking
          description: MLFlow experiment tracking service API and UI
          endpoints:
            - name: cluster
              url: http://mlflow
              type: internal
            - name: ingress
              url: http://mlflow.10.110.120.130.nip.io
              type: external
              default: True
        - name: mlflow-store
          service: s3
          description: MLFlow minio S3 storage back-end
          profiles:
            - default-s3-account
          endpoints:
            - name: cluster
              url: http://mlflow-minio:9000
              type: internal
            - name: ingress
              url: http://mlflow.10.110.120.130.nip.io
              type: external
              default: True
      profiles:
        - name: default-s3-account
          scope: global
          configuration:
            - name: AWS_ACCESS_KEY_ID
              value: 24oT0SfbJPEu6kUbUKsH
            - name: AWS_SECRET_ACCESS_KEY
              value: cMGiZff8KqS5xWQ4eagRujh1tDcbQyRP0bEJSBOf
  ```

2. example showing that the minio sub-service from the previous MLFlow instance can also be registered as a generic minio/S3 service, although this is not recommended, because even though the s3 service can be consumed independently of the parent product instance, the way that data is organized and stored in the s3 back-end is specific to MLFlow and should be discoverable as such:

  ```yaml
  name: minio
  product: minio
  description: Minio S3 storage service
  instances:
    - name: mlflow-storage
      version: "4.1.3"
      location: local
      services:
        - name: s3
          service: s3
          description: MLFlow minio S3 storage back-end
          profiles:
            - default
          endpoints:
            - name: cluster
              url: http://mlflow-minio:9000
              type: internal
            - name: ingress
              url: http://mlflow.10.110.120.130.nip.io
              type: external
              default: True
      profiles:
        - name: default
          scope: global
          configuration:
            - name: AWS_ACCESS_KEY_ID
              value: 24oT0SfbJPEu6kUbUKsH
            - name: AWS_SECRET_ACCESS_KEY
              value: cMGiZff8KqS5xWQ4eagRujh1tDcbQyRP0bEJSBOf
  ```

3. example of an extension record for a third party gitea instance hosted in a location other than FuseML where each FuseML project is associated with a Gitea user

  ```yaml
  name: gitea
  product: gitea
  description: Gitea version control server
  instances:
    - name: gitea-devel-rd
      version: "1.14.3"
      location: remote
      services:
        - name: git+https
          service: git+https
          description: Gitea git/https API
          endpoints:
            - name: public
              url: https://mlflow-minio:9000
              type: external
              default: True
      profiles:
        - name: default
          scope: global
          configuration:
            - name: username
              value: fuseml
            - name: password
              value: 8KqS5xWQ4eagRu
  ```

4. example showing that the extension root element can be used to host different instances of different tools/services, even though this is not recommended:

  ```yaml
  name: s3-storage
  product: s3
  description: S3 storage services
  instances:
    - name: minio
      version: "4.1.3"
      location: local
      services:
        - name: s3
          service: s3
          description: Minio S3 storage deployed locally
          profiles:
            - local-minio
          endpoints:
            - name: cluster
              url: http://mlflow-minio:9000
              type: internal
            - name: ingress
              url: http://mlflow.10.110.120.130.nip.io
              type: external
              default: True
    - name: aws
      version: "4.1.3"
      location: local
      services:
        - name: aws
          service: s3
          description: AWS S3 storage
          profiles:
            - AWS-S3
          endpoints:
            - name: s3.amazon.com
              url: https://s3.amazonaws.com
              type: external
              default: True
      profiles:
        - name: local-minio
          scope: global
          configuration:
            - name: AWS_ACCESS_KEY_ID
              value: 24oT0SfbJPEu6kUbUKsH
            - name: AWS_SECRET_ACCESS_KEY
              value: cMGiZff8KqS5xWQ4eagRujh1tDcbQyRP0bEJSBOf
        - name: aws
          scope: global
          configuration:
            - name: AWS_ACCESS_KEY_ID
              value: sWRS24oT0SfbJPEu6kU3EWf
            - name: AWS_SECRET_ACCESS_KEY
              value: abl4SDcMGiZff8KqS5xWQ4eagRujh1tDcbQyRP0s
  ```
5. KFServing as an example of a k8s controller running in the same cluster as FuseML

  ```yaml
  name: kfserving
  product: kfserving
  description: KFServing prediction service
  instances:
    - name: local
      version: "0.6.0"
      location: local
      services:
        - name: API
          service: kfserving-api
          description: KFServing CRDs
          endpoints:
            - name: cluster
              type: cluster-api
              default: True
        - name: UI
          service: kfserving-ui
          description: KFServing UI
          endpoints:
            - name: ui
              url: http://kfserving.10.120.130.140.nip.io/
              type: external
    ```

6. Seldon core as an example of a k8s service running in another cluster as FuseML and more tightly regulated by the admin

  ```yaml
  name: seldon-core
  product: seldon
  description: Seldon core prediction platform
  instances:
    - name: production
      version: "1.9.1"
      location: remote
      services:
        - name: API
          service: seldon-core-api
          description: Seldon Core CRDs
          profiles:
            - project-alpha
            - project-beta
          endpoints:
            - name: cluster
              url: https://production-cluster-xasf.example.com:6443
              type: remote
              default: True
          configuration:
            - name: CERT_AUTH
              value: VsSCsdfLS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1anF1TT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=
            - name: INSECURE
              value: False
      profiles:
        - name: project-alpha
          scope: project
          project:
            - alpha
          configuration:
            - name: CLIENT_CERT
              value: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUM4akNDQNBVEUtLS0tLQo=
            - name: CLIENT_KEY
              value: cMGiZff8KqS5xWQ4eagRujh1tDcbQyRP0bEJSBOf
            - name: namespace
              value: prj_xbs228
        - name: project-beta
          scope: project
          project:
            - beta
          configuration:
            - name: CLIENT_CERT
              value: GHLS0t1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUM4akNDQWRxZ0F3BVEUtLS0tLQo=
            - name: CLIENT_KEY
              value: TyGiZff8KqS5xWQ4eagRujh1tDcbQyRP0bEJSBOf
            - name: namespace
              value: prj_hys568
    ```

With the extended extension model, referencing an extension from a workflow allows for more flexibility:


1. referencing a particular service instance by including the entire hierarchy in the selector

```yaml
[...]
steps:
[...]
  - name: trainer
    image: '{{ steps.builder.outputs.mlflow-env }}'
    inputs:
      - codeset:
          name: '{{ inputs.mlflow-codeset }}'
          path: '/project'
    extensions:
      - name: mlflow-tracker
        product: mlflow
        instance: mlflow-global
        service: mlflow-tracker
      - name: mlflow-storage
        product: mlflow
        instance: mlflow-global
        service: mlflow-storage
    outputs:
      - name: mlflow-model-url
```

2. referencing a service by service type and using a version specifier

```yaml
[...]
steps:
[...]

  - name: predictor
    image: ghcr.io/fuseml/kfserving-predictor:0.1
    inputs:
      - name: model
        value: '{{ steps.trainer.outputs.mlflow-model-url }}'
      - name: predictor
        value: '{{ inputs.predictor }}'
      - codeset:
          name: '{{ inputs.mlflow-codeset }}'
          path: '/project'
    extensions:
      - name: kfserving
        service: kfserving-api
        version: ">=0.4.0"
    outputs:
      - name: prediction-url
```

### Extension Registry Component

#### REST API

### Workflow Manager Updates

#### DSL Updates

### Installer

### CLI


