# How to contribute

`oria-operator` is Apache 2.0 licensed and accepts contributions via GitHub pull requests. This document outlines some of the conventions on commit message formatting, contact points for developers, and other resources to help get contributions into `oria-operator`.

# Email and Chat

- Email: [operator-framework][operator_framework]  

## Getting started

- Fork the repository on GitHub
- Install Go >= `1.18`. See the [Go installation instructions][go-install] for more information.
- Install Docker. See the [Docker installation instructions][docker-install] for more information.
- Install KinD (Kubernetes-in-Docker). See the [KinD installation instructions][kind-install] for more information.

## Reporting bugs and creating issues

Reporting bugs is one of the best ways to contribute. However, a good bug report has some very specific qualities. Please review the following information on reporting bugs and creating issues:

If any part of the `oria-operator` project has bugs or documentation mistakes, please let us know by opening an issue. We treat bugs and mistakes very seriously and believe no issue is too small. Before creating a bug report, please check that an issue reporting the same problem does not already exist.

To make the bug report accurate and easy to understand, please try to create bug reports that are:

- Specific. Include as much details as possible: which version, what environment, what configuration, etc.

- Reproducible. Include the steps to reproduce the problem. We understand some issues might be hard to reproduce, please include the steps that might lead to the problem.

- Isolated. Please try to isolate and reproduce the bug with minimum dependencies. It would significantly slow down the speed to fix a bug if too many dependencies are involved in a bug report. Debugging external systems that rely on `oria-operator` is out of scope, but we are happy to provide guidance in the right direction or help with using `oria-operator` itself.

- Unique. Do not duplicate existing bug report.

- Scoped. One bug per report. Do not follow up with another bug inside one report.

It may be worthwhile to read [Elika Etemad’s article on filing good bug reports](https://fantasai.inkedblade.net/style/talks/filing-good-bugs/) before creating a bug report.

We might ask for further information to locate a bug. A duplicated bug report will be closed.

## Contribution flow

This is a rough outline of what a contributor's workflow looks like:

- Create a new branch based from the `main` branch.
- Make commits of logical units.
- Make sure commit messages are in the proper format (see below).
- Push branch with changes to a personal fork of the repository.
- Submit a pull request to operator-framework/oria-operator.
- The PR must be approved by an approver and receive a LGTM from a reviewer. Approvers and reviewers can be found in the OWNERS file.

Thanks for contributing!

### Code style

The coding style suggested by the Go community is used in `oria-operator`. See the [style doc][golang-style-doc] for details.

Please follow this style to make `oria-operator` easy to review, maintain and develop.

### Format of the commit message

We follow a rough convention for commit messages that are designed to answer two
questions: what changed and why. The subject line should feature the what and
the body of the commit should describe the why.

```
scripts: add the test-cluster command

this uses tmux to setup a test cluster that can easily be killed and started for debugging.

Fixes #38
```

The format can be described more formally as follows:

```
<subsystem>: <what changed>
<BLANK LINE>
<why this change was made>
<BLANK LINE>
<footer>
```

The first line is the subject and should be no longer than 70 characters, the second line is always blank, and other lines should be wrapped at 80 characters. This allows the message to be easier to read on GitHub as well as in various git tools.

## Documentation

Most contributions involve some sort of documentation. Currently, all documentation resides in the `docs/` folder or a succinct section in the README. When making contributions, it is important to ensure that all documentation is updated as necessary.

If there is a Pull Request with a change that may require a documentation update, a reviewer/maintainer may request that the documentation is updated as part of the Pull Request.

[operator_framework]: https://groups.google.com/forum/#!forum/operator-framework
[golang-style-doc]: https://github.com/golang/go/wiki/CodeReviewComments
[go-install]:https://go.dev/doc/install
[docker-install]:https://docs.docker.com/engine/install/
[kind-install]:https://kind.sigs.k8s.io/docs/user/quick-start/#installation
