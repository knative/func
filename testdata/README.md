# testdata

Contains test templates and directory targets for domain and subdomain-level tests.

## repositories

An example of an on-disk group of template repositories.  Each
subdirectory is a single named repository.  in practice these
would likely be Git repositories, but only the file structure
is expected:  [repo name]/[language runtime]/[template name]

## repository.git

A bare git repository used to test specifying a repo directly.
Tests use a local file URI, but in practice this will likely
be specified as an HTTP URL.

### Initial Setup

- creating as a bare clone
- remove origin from `config`
- remove sample hooks
- touch a .gitinclude in refs/heads and refs/tags

## repository-a.git

Used by tests in repositories_tests.go

