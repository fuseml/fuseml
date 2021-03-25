Our processes
=============

This repository is used through the entired development process. All the code is publicly available, and the code reviews are public. **Anyone** is welcome to contribute to those.

In terms of design, **anyone** is welcomed to propose his requirements. Do not hesitate to share any design requirements or improvements. We will discuss them publicly in this repository. This means that any user, developer, or contributor of an eventual fork is welcomed to propose code or design changes.

We aim to maintain a healthy community were decisions are made using lazy consensus. All our processes should be described in this document.

We aim to have simple, lightweight processes. Any suggestion is welcomed.

How to contribute
=================

Process
-------

Our process to contribute is the usual github workflow.

If you didn't set up git yet, have a look at https://docs.github.com/en/github/getting-started-with-github/set-up-git

The next step is to fork our project: https://docs.github.com/en/github/getting-started-with-github/fork-a-repo

Then create your own branch for your bug/feature, edit the code/documentation, commit, and push your changes in your repository

    $ git checkout -b <branchname>
    $ ...
    $ git commit
    $ git push origin HEAD

Finally, create a pull request (using the web interface or using your favorite github client tool).

Expectations for commit and pull requests are expressed below.

If your contribution doesn't land immediately, you might need to rebase your change based on the branch you are targetting with your PR (e.g. `main`):

    ## Go back to the target branch (e.g. main) branch
    $ git checkout <origin target branch name>
    ## Update your local repo with the latest origin data
    $ git pull origin <local target branch name >
    ## Go back into your development branch
    $ git checkout <branchname>
    ## Update your development branch
    $ git rebase <targetbranch>
    ## Resolve any conflicts here
    $ git commit --amend
    ## Now force push to replace your PR's content
    $ git push origin HEAD -f

Commits requirements
--------------------

- Don't mix whitespace changes with functional code changes
- Don't mix two unrelated functional changes
- Don't send large new features in a giant commit.
- Do not assume the reviewer understands what the original problem was, provide all the necessary tools for reviewers to do their job
- Describe why a change is made, not how a change is made. It is encouraged to mention any limitations or problems of the current code, and how the commit is fixing those pains.
- We are following conventionalcommits (https://www.conventionalcommits.org/en/v1.0.0/). Please write your commit messages appropriately.

See also: https://wiki.openstack.org/wiki/GitCommitMessages

Next to this, it is recommended to sign your commits.

Committing large changes
------------------------

A large change must always come with the following:
- An issue clearly explaining the problem
- An principle approval by the maintainers in the issue or a merged (=approved) design document.
- A PR that contain multiple single unit of changes, each separately reviewable

Pull requests requirements
--------------------------

If you are sending a PR, the team expects you to monitor it at least until it merges. You should not expect the community to finish the work for you.
Upon merging, you are considered an owner of the code. Please make yourself available for any questions regarding your code. By being available, you will help others to understand your code, and will strengthen the community.

PRs can contain one or more commits. Ideally PRs should contain only one commit (one logical change). Please squash your commits into units of logical change.

PRs must include documentation.

Documentation and language
--------------------------

Sadly, there isn't one single language. We want to be welcoming of all languages in the future. Currently, the language for contributions and conversations is English.

Review expectations
-------------------

Anyone is welcomed to review code.
Reviewers must ensure the rules for commits and PR are applied (including the presence of documentation).

If you are regularly reviewing code, you will be invited to the maintainer group.

Maintainer duties
-----------------

Maintainers are people with more access to the repository.
We expect the maintainers to be the heralds of the good practices of this document.

A few extra rules apply to them:

- Maintainers must NOT push to existing branches without going through a PR.
- Maintainers must NOT force merge patches. An exception can be made to unbreak tests.
- A maintainer must regularily triage issues and review PRs
- A maintainer is responsible to enforce the code of conduct.


Community member behaviour
==========================

Everyone must follow our code of conduct
