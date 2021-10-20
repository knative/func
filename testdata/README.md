# testdata

Contains test templates and directory targets for domain and subdomain-level tests.

## Updating Embedded Repositories

The repositories included herein are lightly modified bare repositories used
by tests which involve "adding" them (cloning).  As such they can not be
directly modifed because they have no working branch.  To modify, first check
out the repository, make changes, and then remove the clone.  For example:
```
$ git clone repository.git
$ cd repository
[make changes, committing the result]
$ git push
$ cd .. && rm -rf repository
[commit changes which will now appear in ./repository.git
```
## Creating Embedded Repos

To create a new embedded repo such as repository.git:

- create as a --bare clone
- remove `origin` from `config`
- remove sample hooks
- touch a `.gitinclude` in `refs/heads` and `refs/tags`

## ./repositories

An example of an on-disk group of template repositories.  Each
subdirectory is a single named repository.  in practice these
would likely be Git repositories, but only the file structure
is expected:  [repo name]/[language runtime]/[template name]

## ./repository.git

A bare git repository used to test specifying a repo directly.
Tests use a local file URI, but in practice this will likely
be specified as an HTTP URL.

This repository exemplifies the base case of a remote repository with all
defaults, no metadata, comprised of only templates (grouped by runtime)

## ./repository-a.git

This repository exemplifies the complete case of a repository with a fully
populated manifest which includes an alternate location for templates, a
default name, etc.
