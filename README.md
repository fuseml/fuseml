![GitHub Workflow Status](https://img.shields.io/github/workflow/status/fuseml/fuseml/CI?style=for-the-badge&logo=suse) ![Release](https://img.shields.io/badge/release-pre--alpha-brightgreen?style=for-the-badge&logo=suse) ![Roadmap](https://img.shields.io/badge/roadmap-on--track-blue?style=for-the-badge&logo=suse) ![GitHub last commit](https://img.shields.io/github/last-commit/fuseml/fuseml?style=for-the-badge&logo=suse)

<h1 align=center>FuseML</h1>
<h2 align=center>Fuse your favourite AI/ML tools together for MLOps orchestration</h2>
<p align="center">
<img src="./docs/fuseml-logo.png" width="30%" height="30%"></center>
</p>

Build your own custom MLOps orchestration workflows from composable automation recipes adapted to your favorite AI/ML tools, to get you from ML code to inference serving in production as fast as lighting a fuse.

## Overview

Use FuseML to build a coherent stack of community shared AI/ML tools to run your ML operations. FuseML is powered by a flexible framework designed for consistent operations and a rich collection of integration formulas reflecting real world use cases that help you reduce technical debt and avoid vendor lock-in.

* Curious to find out more ? Read the [FuseML Documentation](https://fuseml.github.io/docs)
* Follow our [quickstart guide](https://fuseml.github.io/docs/quickstart.html) and have your first MLOps workflow up and running in no time 
* Join our [community Slack channel](https://join.slack.com/t/fuseml/shared_invite/zt-rcs6kepe-rGrMzlj0hrRlalcahWzoWg) to ask questions and receive announcements about upcoming features and releases
* Contemplating becoming a contributor ? Find out [how](https://fuseml.github.io/docs/CONTRIBUTING.html) 
* Watch some of the [FuseML Tutorial and Talk Videos or recorded Communitiy Meetings](https://www.youtube.com/channel/UCQLoLTikJDDMXvywWd27FBg) 

## Inception and Roadmap

FuseML originated as a fork of our sister open source project [Epinio](https://github.com/epinio/epinio), a lightweight open source PaaS built on top of Kubernetes, then has been gradually transformed and infused with the MLOps concepts that make it the AI/ML orchestration tool that it is today.

The project is under heavy development following the main directions:
1. adding features and enhancements to improve flexibility and extensibility
2. adding support for more community shared AI/ML tools
3. creating more composable automation blocks adapted to the existing as well as new AI/ML tools

Take a look at [our Project Board](https://github.com/orgs/fuseml/projects/1) to see what we're working on and what's in store for the next release.

## Basic Workflow

The basic FuseML workflow can be described as an MLOps type of workflow that starts with your ML code and automatically runs all the steps necessary to build and serve your machine learning model. FuseML's job begins when your machine learning code is ready for execution.

1. install FuseML in a kubernetes cluster of your choice (see [Installation Instructions](https://fuseml.github.io/docs/quickstart.html))
2. write your code using the AI/ML library of your choice (e.g. TensorFlow, PyTorch, SKLearn, XGBoost)
3. organize your code using one of the [conventions and experiment tracking tools](#experiment-tracking-and-versioning) supported by FuseML
4. use the FuseML CLI to push your code to the FuseML Orchestrator instance and, optionally, supply parameters to customize the end-to-end MLOps workflow
5. from this point onward, the process is completely automated: FuseML takes care of all aspects that involve building and packaging code, creating container images, running training jobs, storing and converting ML models in the right format and finally serving those models

## Supported 3rd Party Tools

### Experiment Tracking and Versioning

* MLFlow
* DVC (TBD)

### Model Training

* MLFlow
* DeterminedAI (TBD)

### Model Serving and Monitoring

* KFServing
* KNative Serving (coming soon)
* Seldon Core (coming soon)