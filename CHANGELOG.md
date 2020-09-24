# Change Log

<a name="unreleased"></a>
## [0.7.0](https://www.github.com/boson-project/faas/compare/v0.6.2...v0.7.0) (2020-09-24)


### Features

* add local debugging to node.js templates ([#132](https://www.github.com/boson-project/faas/issues/132)) ([1b0bb15](https://www.github.com/boson-project/faas/commit/1b0bb15147889bb55ff33de1dc132cb0370d1da6))
* decouple function name from function domain ([#127](https://www.github.com/boson-project/faas/issues/127)) ([0258626](https://www.github.com/boson-project/faas/commit/025862689ec8dc460a1ef6f4402151c18a072ba3))
* default to no confirmation prompts for CLI commands ([566d8f9](https://www.github.com/boson-project/faas/commit/566d8f9255d532e88e72d5bce122bebaee88bc81))
* set builder images in templates and .faas.yaml ([#136](https://www.github.com/boson-project/faas/issues/136)) ([d6e131f](https://www.github.com/boson-project/faas/commit/d6e131f9153c20bd3edbf1441060610987fa5693))
* **ci/cd:** add release-please for automated release management ([8a60c5e](https://www.github.com/boson-project/faas/commit/8a60c5e0c44d28d2ff085e56299217e05e408df8))


### Bug Fixes

* correct value for config path and robustify ([#130](https://www.github.com/boson-project/faas/issues/130)) ([fae27da](https://www.github.com/boson-project/faas/commit/fae27dabc97c78cd98be400d296da6fc2fbeba65))
* delete command ([284b77f](https://www.github.com/boson-project/faas/commit/284b77f7ef6524195da958850131190399470375))
* describe works without Eventing ([6c16e65](https://www.github.com/boson-project/faas/commit/6c16e65d60543458f0b70c010d672cb4d45f6279))
* sync package-lock.json ([#137](https://www.github.com/boson-project/faas/issues/137)) ([02309a2](https://www.github.com/boson-project/faas/commit/02309a24a1d8779fb69e4f67fa4f7faea705b2ba))

## [Unreleased]


<a name="v0.6.2"></a>
## [v0.6.2] - 2020-09-09
### Build
- remove main branch from release

### Fix
- update pkger generated files
- signature of HTTP go function in template


<a name="v0.6.1"></a>
## [v0.6.1] - 2020-09-09
### Chore
- update quarkus version to 1.7.2.Final
- use organization-level secrets for image deployment
- **actions:** add binary uploads to develop branch CI ([#104](https://github.com/boson-project/faas/issues/104))

### Docs
- initial Go template READMEs

### Fix
- build releases from main branch only
- remove references to unused binaries appsody, kn, kubectl
- image override ([#88](https://github.com/boson-project/faas/issues/88))

### Release
- v0.6.1

### Templates
- **node:** make node templates use npx [@redhat](https://github.com/redhat)/faas-js-runtime ([#99](https://github.com/boson-project/faas/issues/99))


<a name="v0.6.0"></a>
## [v0.6.0] - 2020-08-31
### Chore
- build static binary

### Docs
- fix function typos
- setting up remote access to kind clusters
- wireguard configuraiton for OS X
- Kind cluster provisioning and TLS
- separate repository and system docs
- getting started with kubernetes, reorganization.

### Feat
- golangci-lint allow enum shorthand, use config file
- consolidate formatters - Replaces globally-scoped formatter function with methods - Defines enumerated Format types - Renames the 'output' flag 'format' due to confusion with command file descriptors - FunctionDescription now Function - Global verbose flag replaced with config struct based value throughout
- test suite
- consolidate knative client config construction
- cli usability enhancements and API simplification
- the `list` sub-command uses namespace
- version command respects verbose flag ([#61](https://github.com/boson-project/faas/issues/61))
- add init/build/deploy commands and customizable namespace ([#65](https://github.com/boson-project/faas/issues/65))
- JSON output for the `list` sub-command

### Fix
- return fs errors on config creation
- serialize trigger on faas.config
- default k8s namespace to 'faas' per documentation

### Fixup
- remove unnecessary WithVerbose option from progressListener

### Release
- v0.6.0

### Test
- add test targets for go and quarkus templates ([#72](https://github.com/boson-project/faas/issues/72))


<a name="v0.5.0"></a>
## [v0.5.0] - 2020-07-31
### Actions
- add CHANGELOG.md and a release target to Makefile ([#45](https://github.com/boson-project/faas/issues/45))

### Build
- reduce build verbosity for cross-platform compilations
- update container latest tag when releasing

### Chore
- add `-race` flag for tests
- add lint to GH actions CI

### Feat
- build and release cross-platform binaries
- version prints semver first
- http template for Quarkus stack

### Fix
- build using environmentally-defined settings for GOOS and GOARCH by default
- version flag


<a name="v0.4.0"></a>
## [v0.4.0] - 2020-07-17
### Actions
- add automated releases of faas binary


<a name="v0.3.0"></a>
## [v0.3.0] - 2020-07-12
### Docs
- improved description and initial setup

### Fixup
- remove node_modules from embedded node/http
- actuall embed the template

### Init
- add Node.js HTTP template


<a name="v0.2.2"></a>
## [v0.2.2] - 2020-07-08

<a name="v0.2.1"></a>
## [v0.2.1] - 2020-07-08
### Feat
- remove dependency on `kn` binary

### Fix
- remove dependency on `kubectl` binary
- remove dependency on `kn` binary
- creating new revision of ksvc

### Style
- formatting


<a name="v0.2.0"></a>
## [v0.2.0] - 2020-06-11
### Feat
- buildpacks

### Fix
- buildpack image reference


<a name="v0.1.0"></a>
## [v0.1.0] - 2020-06-01

<a name="v0.0.19"></a>
## [v0.0.19] - 2020-05-29

<a name="v0.0.18"></a>
## [v0.0.18] - 2020-05-25

<a name="v0.0.17"></a>
## [v0.0.17] - 2020-05-11
### Doc
- command description

### Feat
- 'describe' sub-command for faas cli


<a name="v0.0.16"></a>
## v0.0.16 - 2020-04-27
### Builder
- remove superfluous appsody deploy yaml after build

### Deployer
- move domain to labels

### Docs
- appsody with boson stacks config
- configuration additions
- configuration notes for repo namespace

### Feat
- list sub-command for faas cli

### Updater
- add kn-based implementation


[Unreleased]: https://github.com/boson-project/faas/compare/v0.6.2...HEAD
[v0.6.2]: https://github.com/boson-project/faas/compare/v0.6.1...v0.6.2
[v0.6.1]: https://github.com/boson-project/faas/compare/v0.6.0...v0.6.1
[v0.6.0]: https://github.com/boson-project/faas/compare/v0.5.0...v0.6.0
[v0.5.0]: https://github.com/boson-project/faas/compare/v0.4.0...v0.5.0
[v0.4.0]: https://github.com/boson-project/faas/compare/v0.3.0...v0.4.0
[v0.3.0]: https://github.com/boson-project/faas/compare/v0.2.2...v0.3.0
[v0.2.2]: https://github.com/boson-project/faas/compare/v0.2.1...v0.2.2
[v0.2.1]: https://github.com/boson-project/faas/compare/v0.2.0...v0.2.1
[v0.2.0]: https://github.com/boson-project/faas/compare/v0.1.0...v0.2.0
[v0.1.0]: https://github.com/boson-project/faas/compare/v0.0.19...v0.1.0
[v0.0.19]: https://github.com/boson-project/faas/compare/v0.0.18...v0.0.19
[v0.0.18]: https://github.com/boson-project/faas/compare/v0.0.17...v0.0.18
[v0.0.17]: https://github.com/boson-project/faas/compare/v0.0.16...v0.0.17
