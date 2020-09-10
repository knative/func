# Change Log

<a name="unreleased"></a>
### [0.6.3](https://www.github.com/boson-project/faas/compare/v0.6.2...v0.6.3) (2020-09-10)


### Bug Fixes

* delete command ([62a6901](https://www.github.com/boson-project/faas/commit/62a69017d5da8a16e9ae8624a85dc2e3f01e13e7))
* describe works without Eventing ([afec0c5](https://www.github.com/boson-project/faas/commit/afec0c5039fb75f481c0c629aa1644ef442e7411))

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
