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

### Extension Registry Component


#### REST API

### Workflow Manager Updates

#### DSL Updates

### Installer

### CLI


