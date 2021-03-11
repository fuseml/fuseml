# FuseML - Flexible Universal Service Orchestration for Machine Learning

![CI](https://github.com/SUSE/carrier/workflows/CI/badge.svg)

Build your own MLOps orchestration workspace from composable automation recipes adapted to your favorite AI/ML tools.

<img src="./docs/fuseml-logo.png" width="50%" height="50%">

## The status of MLOps

The machine learning software domain provides an impressive collection of specialized AI/ML software libraries, frameworks, and platforms that **Data Scientists**, **Data Engineers**, and **DevOps Engineers** can use to coordinate and automate their activities. They have to support a wide range of services, from data extraction and exploration to model training to inference serving and monitoring.
Choosing the right set of tools to suit the needs of your machine learning project isn't an easy task. To make matters worse, this set of tools the **Engineer** eventually decide to use might not be compatible and interoperable by default, so often there's additional work that needs to be done to have a working and comprehensive MLOps stack running on your target infrastructure. What starts as a simple machine learning project eventually ends up being an inflexible **DYI MLOps** platform accruing a lot of technical debt and locking you into a fixed set of tools.

## Introducing FuseML

*Wouldn't it be great if there was a software solution that could solve the complexity of this and simply **Fuse** together your favorite **AI/ML** tools, while at the same time being flexible enough to allow you to make changes later on without incurring massive operational costs?*

**FuseML** aims to achieve solve that and more providing:

* An MLOps framework that provides the glue required to dynamically integrate together with the AI/ML tools of your choice.
* An extensible tool built through collaboration, where Data Engineers and DevOps Engineers can come together and contribute with reusable integration code and use-cases addressing their specific needs and tools, that everyone else can benefit from.
* A set of extensible integration abstractions and conventions defined around common AI/ML concepts and implemented through tool-specific plugins and automation recipes. The abstractions are specific enough to support a complex orchestration layer to be implemented on top but at the same time flexible enough not to hide nor infringe upon the nuances of the various AI/ML tools they are wrapped around.
* An ML orchestrator combining aspects of MLOps and GitOps together into a set of services that Data Scientists, Data Engineers and DevOps Engineers can use to collaboratively manage the end-to-end lifecycle of their AI/ML projects, from code to production, across infrastructure domains, while seamlessly coupling together different AI/ML libraries and tools playing localized roles in a complete MLOps pipeline.

What **FuseML** is **NOT**:

* An opinionated open source MLOps platform. Flexibility and extensibility are FuseML's core principles. Instead of being a set of tightly integrated components, it relies on extension mechanisms and custom automation recipes to dynamically and loosely integrate the 3rd party AI/ML tools of your choice.
* A complete lifecycle manager. FuseML's flexibility does come with a cost, which is vital to reduce complexity: managing the complete lifecycle (installation and upgrade processes) of supported 3rd party AI/ML tools is out of scope. However, FuseML will provide registration and auto-detection mechanisms for existing 3rd party tool installations, and may even go so far as to add lifecycle management to its list of supported extension mechanisms.

## FuseML Principles

* *Flexibility* - create and manage dynamic MLOps workflows connecting different AI/ML tools across multiple infrastructure domains

* *Extensibility* - leverage FuseML's set of abstractions and extension mechanisms to add support for your favorite AI/ML tools

* *Composability* - build complex MLOps workflows for your projects out of composable building blocks implementing a wide range of machine learning functions     

* *Collaboration* - use MLOps automation and tool integration recipes created in collaboration by all AI/ML team roles - Data Scientists, Data Engineers, and DevOps Engineers

## Supported 3rd Party Tools

### Experiment Tracking and Versioning

* MLFlow
* DVC (TBD)

### Model Training

* DeterminedAI (TBD)

### Model Serving and Monitoring

* MLFLow
* KNative Serving
* Seldon Core (coming soon)
* KFServing (coming soon)

## Usage

### Install

```bash

$ carrier install

```
### Uninstall

```bash

$ carrier uninstall

```

### Push an application

Run the following command for any supported application directory (e.g. inside [sample-app directory](sample-app)).

```bash

$ carrier push NAME PATH_TO_APPLICATION_SOURCES

```

Note that the path argument is __optional__.
If not specified the __current working directory__ will be used.
Always ensure that the chosen directory contains a supported application.

### Delete an application

```bash

$ carrier delete NAME

```

### Create a separate org

```bash

$ carrier create-org NAME

```

### Target an org

```bash

$ carrier target NAME

```

### List all commands

```bash

$ carrier help

```

### Detailed help for each command

```bash

$ carrier COMMAND --help

```

## Configuration

Carrier places its configuration at `$HOME/.config/carrier/config.yaml` by default.

For exceptional situations, this can be overridden by either specifying

* The global command-line option `--config-file`, or

* The environment variable `CARRIER_CONFIG`.
