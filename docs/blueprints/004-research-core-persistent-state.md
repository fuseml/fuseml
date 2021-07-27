## Research: Persistent storage for fuseml-core

## Overview

Story: [Research: Persistent state support options for fuseml-core components](https://github.com/fuseml/fuseml/issues/68)

As a stateful application, fuseml-core needs to save some of its data in a form of persistent storage so it can survive service restarts without losing its current state.
The intent of this research is to evaluate the options for persistent storage for fuseml-core to support on taking a decision when choosing a solution that best fit its requirements.

## Current state

In its current version, fuseml-core does not provide a persistent storage for any of it components, except for `Codesets` and `Projects` which is able to save/load its state from a Git solution (Gitea). The rest of the components (`Application`, `Workflow`, `Runnable` and `Extension`) relies on a basic implementation of a in-memory storage, meaning that in case of a service restart fuseml-core would lose any information related to those components.

So, at this point of fuseml-core development this research has the goal of storing data for the following fuseml-core components:
- `Application`
- `Workflow`
- `Runnable`
- `Extension`

We can also assume that for any future component the same solution(s) will be used.

The following diagram shows the main fuseml-core objects and its relations, also where they are being stored in the current implementation:

![fuseml-core](fuseml-core-storage-memory.png?raw=true)

## Options

### Relational database

Relational database is probably the most common solution when it comes to storing state from applications. It stores data into classifications (tables), with each table consisting of one or more records (rows) identified by a primary key. Tables may be related through their keys, allowing queries to join data from multiple tables together to access any/all required data. Relational databases require fixed schemas on a per-table basis that are enforced for each row in a table.
Recommended for storing predictable, structured data with a finite number of individuals or applications accessing it.

#### Advantages:
- Support transactions and all ACID properties
- Most of the solutions available in the market are mature and have been tested over a period of time to solve various kinds of problems
- Flexibility: SQL is a standard language used by all the relational database vendors with relatively minor changes in implementation
- Considers storing as well as searching as the same problem
- Precision: The usage of relational algebra and relational calculus in the manipulation of he relations between the tables ensures that there is no ambiguity, which may otherwise arise in establishing the linkages in a complicated network type database.

#### Disadvantages:
- The data model needs to be kept in sync with the business logic
- Not distributed by nature, which can impact availability and scalability
- Cost: the underlying cost involved in a relational database is quite expensive
- Can quickly become complex as the amount of data grows, and the relations between pieces of data become more complicated

### Key/Value store

Key/Value stores offer great simplicity in data storage, allowing for massive scalability of both reads and writes. Values are stored with a unique key ("foo") and a value ("1234-987654321") and may be manipulated using the following operations: Add, Reassign (Update), Remove, and Read. Some storage engines offer additional data structure management within these simple operations. The logic for searching is pretty complex in key-value but they are usually way faster than database.
Recommended whenever quick access times are needed for large quantities of data.

#### Advantages:
- Predictable performance: Simple queries like get, put and delete in most cases allows the system’s performance to be a lot more predictable
- Not based on schema and so you are bound by the model, also changes to the database can be made while in operation
- Does not require expensive hardware
- Better performance when compared to relational databases
- Easy to scale by adding an extra nodes

#### Disadvantages:
- Simple queries can be complex due to the lack of indexes and scanning capabilities
- Migrating from one product to another product is difficult in as the API used to access data is store specific
- Updates requires updating the whole value

#### Example:

##### [BadgerDB](https://dgraph.io/docs/badger/)
BadgerDB is an embeddable persistent key-value store written in Go. It uses a log-structured merge (LSM) tree based implementation, which has an advantage in throughput when compared to B+ tree since background (disk) writes in LSM maintain a sequential access pattern. Badger supports ACID transactions and Multi-Version Concurrency Control with Snapshot Isolation, it acquires locks on directories when accessing data, so multiple processes cannot open the same database at the same time. If the transaction is read-write, the transaction checks if there is a conflict and return an error if there is one. Regarding queries, it supports Get, Set, Delete, and Iterate functions. Multiple updates can be batched-up into a single transaction, allowing the user to do a lot of writes at a time.

A simple implementation for fuseml-core workflow store using BadgerDB is available at: https://github.com/fuseml/fuseml-core/pull/88, Note that this implementation does not uses BadgerDB directly, instead it uses [BadgerHold](https://github.com/timshannon/badgerhold) which provides a simplified interface for for storing/querying Go types stored on BadgerDB.

Takeaways:
- Very simple to use as it does not rely on another service. This also has the obvious disadvantage of not working in case of running multiple fuseml-core in parallel, for example in an auto-scale scenario.
- Fit almost seamlessly the data structure that is already in use by the in-memory store currently implemented in fuseml-core
- In the case of the workflow store, where most of the objects stored are referred by pointers, the queries does not work as it tries to compare the pointers. But this is mostly a limitation of BadgerHold instead of BadgerDB

In conclusion, I think that BadgerDB is a viable short-term solution to achieve persistent storage with minimal effort, however it is clear that does not meet the requirements of a production grade storage.

### Kubernetes Native Application

A Kubernetes native application is an application that has been specifically designed to run on Kubernetes platforms, generating software designed to maximize the functionalities of Kubernetes API and components and facilitate infrastructure management.
A Kubernetes Native application is entirely managed by Kubernetes APIs and kubectl tooling and cohesively deployed on Kubernetes as a single object.
This kind of application makes use of Kubernete's [Custom Resources](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/) to extend the kubernetes API through [Custom Controllers](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/#custom-controllers) which is responsible for keeping the current state of Kubernetes objects in sync with the desired state.

When it comes to storing the application state, a declarative API allows you to declare or specify the desired state of your resource and tries to keep the current state of Kubernetes objects in sync with the desired state. The controller interprets the structured data as a record of the user's desired state, and continually maintains this state.
This structured data is handled by the Kubernetes API and it is stored in etcd.

This option would require a complete redesign of fuseml-core and although it offers many advantages, not only regarding persistent state, it needs a better evaluation which
is out of the scope of this research. A nice table that helps on taking such decision can be found here: [Should I add a custom resource to my Kubernetes Cluster?](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/#should-i-add-a-custom-resource-to-my-kubernetes-cluster).

#### Advantages:
- Kubernetes native application leverages many of the best practices/features offered by Kubernetes, as listed in: [Advanced features and flexibility](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/#advanced-features-and-flexibility) and [Common Features](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/#common-features)
- Created resources (CRDs) are stored in the etcd cluster with proper replication and lifecycle management

#### Disadvantages
- Dependency on Kubernetes
- As the application API is the Kubernetes API, interacting with the application requires access to the Kubernetes API

## Conclusion
After discussion with the team, we decided to use BadgerDB as the storage solution for fuseml-core. This is because it is a very simple and easy to use storage solution, which runs embedded into the code dispensing the requirement of managing a separate service. Besides, it also fits almost seamlessly the data structure that is already in use by the in-memory store currently implemented in fuseml-core.

It is clear that BadgerDB does not meet the requirements of a production grade storage, however it is still a viable short-term solution to achieve persistent storage with minimal effort, and it might be the perfect solution considering the current state of fuseml-core as it allows for a simple and quick deployment for testing purposes.

### Current State
At this point, persistent storage for fuseml-core is already implemented using BadgerDB for the following components:
- `Application`
- `Workflow`
- `Extension`

The following diagram shows the current state of the storage solutions in use by fuseml-core:

![fuseml-core](fuseml-core-storage-badger.png?raw=true)
