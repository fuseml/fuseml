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

* the root _extension_ model element represents a single instance or installation of a framework/platform/service/product developed and released or hosted under a unique name and operated as a single cohesive unit. Different installations of the same product can be grouped together based on the _product_ they were installed from. They can also be grouped together based on the the infrastructure domain, location, region, zone, area or kubernetes cluster where the extension is running, based on a _zone_ identifier. We'll keep the list of possible values for the _product_ field open and unregulated for the time being, but in the future, as we add extensions, we should also consider maintaining a pre-populated list of products as a common reference that is valid across FuseML installations.
* several individual _services_, which can be consumed separately, can be provided by the same _extension_. For example, an MLFlow instance is composed of an experiment tracking service/API, a model store service and a UI. A _service_ is represented by a single API or UI. For extensions implemented as cloud-native applications, a _service_ is the equivalent of a k8s service that is used to expose a public API or UI. _Services_ are also classified into known resource types (e.g. s3, git, ui) and service categories (e.g. model store, feature store, prediction platform), to make it easier to support portable workflows (see optional requirements), where a workflow step lists a service type and/or a category as a requirement, and FuseML automatically resolves that to whatever particular service instance is available at runtime. Together with the extension _product_, the resource type and service category can be used to uniquely identify a service.
* a _service_ is exposed through several individual _endpoints_. Having a list of _endpoints_ associated with a single _service_ is particularly important for representing k8s services, which can be exposed both internally (cluster IP) and externally (e.g. ingress). Depending on the consumer location, FuseML can choose the endpoint that is accessible to and closer to the consumer. All _endpoints_ grouped under the same _service_ must be equivalent in the sense that they are backed by the same API and/or protocol.
* a _service_ can be accessed using one of several sets of _credentials_. A set of _credentials_ can be generally used to embed information pertaining to the authentication and authorization features supported by a service. This element allows administrators and operators of 3rd party tools integrated with FuseML to configure different accounts and credentials (tokens, certificates, passwords) to be associated with different FuseML organization entities (users, projects, groups etc.). All information embedded in a _credentials_ entry is treated as sensitive information. In the future, we could further specialize this element to model a predefined list of supported standard authentication and authorization schemes. Each _credentials_ entry has an associated scope that controls who has access to this information (e.g. global, project, user). This is the equivalent of a k8s secret.
* _configuration_ elements can be present under _extension, _service_, _endpoint_ or _credentials_ and represent opaque, service specific configuration data that the consumers need in order to access and consume a service interface. _Configuration_ elements can be used to encode any information relevant for service clients: accounts and credentials, information describing the service or particular parameters that describe how the service should be used. For example, if endpoints are SSL secured, custom certificates (e.g. self-signed CA certificates) might be needed to access them and this should be included in the endpoint configuration. The information encoded in a _configuration_ element is only treated as sensitive information when present under _credentials_. Equivalent of a k8s configmap (or k8s secret, when under _credentials_). 

Examples:

1. MLFlow instance deployed locally alongside FuseML and globally accessible:

  ```yaml
  name: mlflow-0001
  product: mlflow
  version: "1.19.0"
  description: MLFlow experiment tracking service
  zone: cluster-alpha
  services:
    - name: mlflow-tracking
      resource: mlflow-tracking
      category: experiment-tracking
      description: MLFlow experiment tracking service API and UI
      endpoints:
        - name: cluster
          url: http://mlflow
          type: internal
        - name: ingress
          url: http://mlflow.10.110.120.130.nip.io
          type: external
    - name: mlflow-store
      resource: s3
      category: model-store
      description: MLFlow minio S3 storage back-end
      credentials:
        - name: default-s3-account
          scope: global
          configuration:
            - name: AWS_ACCESS_KEY_ID
              value: 24oT0SfbJPEu6kUbUKsH
            - name: AWS_SECRET_ACCESS_KEY
              value: cMGiZff8KqS5xWQ4eagRujh1tDcbQyRP0bEJSBOf
      endpoints:
        - name: cluster
          url: http://mlflow-minio:9000
          type: internal
        - name: ingress
          url: http://mlflow.10.110.120.130.nip.io
          type: external
  ```

2. example showing that the minio sub-service from the previous MLFlow instance can also be registered as a generic minio/S3 service, although this is not recommended, because even though the s3 service can be consumed independently of the parent product instance, the way that data is organized and stored in the s3 back-end is specific to MLFlow and should be discoverable as such:

  ```yaml
  name: mlflow-model-store-0001
  product: minio
  version: "4.1.3"
  description: Minio S3 storage service
  zone: cluster-alpha
  services:
    - name: s3
      resource: s3
      category: object-storage
      description: MLFlow minio S3 storage back-end
      credentials:
        - name: default
          scope: global
          configuration:
            - name: AWS_ACCESS_KEY_ID
              value: 24oT0SfbJPEu6kUbUKsH
            - name: AWS_SECRET_ACCESS_KEY
              value: cMGiZff8KqS5xWQ4eagRujh1tDcbQyRP0bEJSBOf
      endpoints:
        - name: cluster
          url: http://mlflow-minio:9000
          type: internal
        - name: ingress
          url: http://mlflow.10.110.120.130.nip.io
          type: external
  ```

3. example of an extension record for a third party gitea instance hosted in a location other than FuseML where each FuseML project is associated with a Gitea user

  ```yaml
  name: gitea-devel-rd
  product: gitea
  version: "1.14.3"
  description: Gitea version control server running in the R&D cloud
  zone: rd-cloud
  services:
    - name: git+http
      resource: git+http
      category: VCS
      description: Gitea git/http API
      endpoints:
        - name: public
          url: http://gitea.10.110.120.130.nip.io
          type: external
      credentials:
        - name: admin
          scope: global
          configuration:
            - name: username
              value: admin
            - name: password
              value: 8KqS5xWQ4eagRu
  ```

4. example showing two different extensions, instances of different products, that provide the same type of resource:

  ```yaml
  name: minio-s3-storage
  product: minio
  version: "4.1.3"
  description: Minio S3 storage services
  zone: development-cluster
  services:
    - name: s3
      service: s3
      category: object-storage
      description: Minio S3 storage deployed in the development cluster
      credentials:
        - name: local-minio
          scope: global
          configuration:
            - name: AWS_ACCESS_KEY_ID
              value: 24oT0SfbJPEu6kUbUKsH
            - name: AWS_SECRET_ACCESS_KEY
              value: cMGiZff8KqS5xWQ4eagRujh1tDcbQyRP0bEJSBOf
      endpoints:
        - name: cluster
          url: http://mlflow-minio:9000
          type: internal
        - name: ingress
          url: http://mlflow.10.110.120.130.nip.io
          type: external
  ```

  ```yaml
  name: aws-object-storage
  product: aws-s3
  description: AWS cloud object storage
  zone: eu-central-1
  services:
    - name: aws
      resource: s3
      category: object-storage
      description: AWS S3 object storage
      credentials:
        - name: aws
          scope: global
          configuration:
            - name: AWS_ACCESS_KEY_ID
              value: sWRS24oT0SfbJPEu6kU3EWf
            - name: AWS_SECRET_ACCESS_KEY
              value: abl4SDcMGiZff8KqS5xWQ4eagRujh1tDcbQyRP0s
      endpoints:
        - name: s3.eu-central-1
          url: https://s3.eu-central-1.amazonaws.com
          type: external
  ```

5. KFServing as an example of a k8s controller running in the same cluster as FuseML

  ```yaml
  name: kfserving-local
  product: kfserving
  version: "0.6.0"
  description: KFServing prediction service
  zone: cluster.local
  services:
    - name: API
      resource: kfserving-api
      category: prediction-serving
      description: KFServing CRDs
      endpoints:
        - name: cluster-internal
          url: kubernetes.default.svc
          type: internal
      endpoints:
        - name: cluster-external
          url: https://10.120.130.140/
          type: external
    - name: UI
      resource: kfserving-ui
      description: KFServing UI
      endpoints:
        - name: ui
          url: https://kfserving.10.120.130.140.nip.io/
          type: external
  ```

6. Seldon core as an example of a k8s service running in another cluster as FuseML and more tightly regulated by the admin

  ```yaml
  name: seldon-core-production
  product: seldon
  version: "1.9.1"
  zone: production-cluster-001
  description: Seldon core inference serving in production cluster
  services:
    - name: API
      resource: seldon-core-api
      category: prediction-serving
      description: Seldon Core CRDs
      credentials:
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
          scope: user
          project:
            - alpha
            - beta
          user:
            - alainturing2000
          configuration:
            - name: CLIENT_CERT
              value: GHLS0t1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUM4akNDQWRxZ0F3BVEUtLS0tLQo=
            - name: CLIENT_KEY
              value: TyGiZff8KqS5xWQ4eagRujh1tDcbQyRP0bEJSBOf
            - name: namespace
              value: prj_hys568
      endpoints:
        - name: cluster
          url: https://production-cluster-xasf.example.com:6443
          type: external
      configuration:
        - name: CERT_AUTH
          value: VsSCsdfLS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1anF1TT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=
        - name: INSECURE
          value: False
  ```

With the extended extension model, referencing an extension from a workflow allows for more flexibility. We're still limited by the fact that the FuseML core service must be able to _unambiguously_ resolve the information given in the workflow to a particular extension endpoint and a set of credentials (if present). The updated workflow model should allow users to explicitly point to an extension endpoint and set of credentials by explicitly specifying their identifiers (unique names). It should also allow users to reference an extension service by using the group identifiers that are universally valid (i.e. valid across FuseML installations), such as the product, service type and service category, to facilitate support for reusable workflows.

1. explicitly referencing a particular service endpoint by name

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
          extension: mlflow-0001
          service: mlflow-tracker
          endpoint: ingress
        - name: mlflow-storage
          extension: mlflow-0001
          service: mlflow-storage
          endpoint: ingress
      outputs:
        - name: mlflow-model-url
  ```

2. referencing a service independently of running instance and even independent of extension, by using the product and
resource type identifiers and using a version specifier

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
          product: kfserving
          resource: kfserving-api
          version: ">=0.4.0"
      outputs:
        - name: prediction-url
  ```

3. example of a minimal reference, specifying only the resource type

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
          resource: mlflow-tracker
        - name: mlflow-storage
          resource: mlflow-storage
      outputs:
        - name: mlflow-model-url
  ```



The other workflow model modification is with respect to how information from the extension record is provided to the workflow step containers:

* as a default, the extension endpoint record in YAML format could be saved as a secret and/or configmap and mounted in the container at a predefined root path (similarly to how it's done for inputs)
* also as a default, the configuration entries collected from the instance/service/endpoint/credentials could be merged and mapped to environment variables
* the model should also support explicitly mapping new env variable names to the configuration entry names, e.g.:

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
          resource: mlflow-tracker
          endpoint: cluster
        - name: mlflow-storage
          product: mlflow
          resource: s3
          endpoint: cluster
          environment:
            - name: S3_ACCESS_KEY_ID
              configuration: AWS_ACCESS_KEY_ID
            - name: S3_SECRET_ACCESS_KEY
              configuration: AWS_SECRET_ACCESS_KEY
      outputs:
        - name: mlflow-model-url
  ```

* a more advanced feature is to reuse the expression support to expand fields in the extension record

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
        - name: s3_access_key
          value: '{{ extension.mlflow-storage.AWS_ACCESS_KEY_ID }}'
        - name: s3_access_key
          value: '{{ extension.mlflow-storage.AWS_SECRET_ACCESS_KEY }}'
      extensions:
        - name: mlflow-tracker
          product: mlflow
          resource: mlflow-tracker
        - name: mlflow-storage
          product: mlflow
          resource: s3
      outputs:
        - name: mlflow-model-url
  ```

#### Complexity Concerns

The extension domain model could be quite complex to work with from an user experience perspective. There are several ways to hide this complexity from the end user while keeping the extended domain model untouched:

* for 3rd party tools installable through the FuseML installer, registration should be done automatically, e.g. by means of YAML templates or scripts or a combination of both
* implementing shortcuts in the FuseML installer to address simpler use-cases and allowing end users to quickly register extensions through the command line with the option to provide a full-blown YAML file for more complex use-cases. For example, a shortcut could be to register one service endpoint with a single command and extrapolate as much information as possible (names, descriptions) from that
* using external extension record templates (e.g. maintained by the FuseML community for the more popular tools and services that can be used with FuseML). The end user would provide a template to the FuseML installer as a base and fill in only missing information (e.g. actual URLs and credentials). These could be the same templates packaged with the installer extensions and used to register extensions automatically
* ultimately, we could make the REST API a simplified version of the domain model (e.g. collapse some of the hierarchy levels) and implement conversion logic in the REST service

### Extension Registry Component

#### REST API

### Workflow Manager Updates

#### Workflow DSL Updates

### Installer

### CLI

