# Change Log

<a name="unreleased"></a>
## [0.11.0](https://www.github.com/boson-project/func/compare/v0.10.0...v0.11.0) (2021-01-21)


### Features

* add --all-namespaces flag to `func list` ([#242](https://www.github.com/boson-project/func/issues/242)) ([8e72fd2](https://www.github.com/boson-project/func/commit/8e72fd2eba9f4e6e5d3a0bd366215025ba1d1004))


### Bug Fixes

* change --format flag to --output for list and describe commands ([#248](https://www.github.com/boson-project/func/issues/248)) ([6470d9e](https://www.github.com/boson-project/func/commit/6470d9e57462bc8d3a30583cf146d4f466e2d5f7))
* correct fn signatures in Go Events template ([#246](https://www.github.com/boson-project/func/issues/246)) ([5502492](https://www.github.com/boson-project/func/commit/55024921c26e044f83187cbd5510375d8702c6d9))
* correcting broken merge ([#252](https://www.github.com/boson-project/func/issues/252)) ([8d1f5b8](https://www.github.com/boson-project/func/commit/8d1f5b833d86fa959e3386db73f7e1b07bdd6dfd))
* fix the help text for the describe function ([#243](https://www.github.com/boson-project/func/issues/243)) ([5a3a0d6](https://www.github.com/boson-project/func/commit/5a3a0d6bdab4d01292c4c8f6011a3b67cadb8ef6))
* print "No functions found in [ns] namespace" for kn func list ([#240](https://www.github.com/boson-project/func/issues/240)) ([61ea8d4](https://www.github.com/boson-project/func/commit/61ea8d4fc6e841f0f10151244f10131862bf181c))
* set envVars when creating a function ([#250](https://www.github.com/boson-project/func/issues/250)) ([f0be048](https://www.github.com/boson-project/func/commit/f0be048c841be22fcd0d448fdecc0da33b8c77be))

## [0.10.0](https://www.github.com/boson-project/faas/compare/v0.9.0...v0.10.0) (2020-12-08)


### Features

* add spring cloud function runtime and templates ([#231](https://www.github.com/boson-project/faas/issues/231)) ([557361a](https://www.github.com/boson-project/faas/commit/557361a37446953dc613ae30f59913f1924dedd3))


### Bug Fixes

* Fix plugin version output ([#233](https://www.github.com/boson-project/faas/issues/233)) ([8a30ba1](https://www.github.com/boson-project/faas/commit/8a30ba193da6097a141332212cbd64e5a1a708e8))
* use image name for run command ([#238](https://www.github.com/boson-project/faas/issues/238)) ([985906b](https://www.github.com/boson-project/faas/commit/985906b0e1f692f94fc84e3e796893192d17bd4c))

## [0.9.0](https://www.github.com/boson-project/faas/compare/v0.8.0...v0.9.0) (2020-11-06)


### ⚠ BREAKING CHANGES

* rename faas to function (#210)
* remove create cli subcommand (#180)

### Features

* Better output of build/deploy/delete commands ([#206](https://www.github.com/boson-project/faas/issues/206)) ([ddbb95b](https://www.github.com/boson-project/faas/commit/ddbb95b075a383fb1847be2c75fd2c216870c7f8))
* change default runtime to Node.js HTTP ([#198](https://www.github.com/boson-project/faas/issues/198)) ([61cb56a](https://www.github.com/boson-project/faas/commit/61cb56aec3461e9f9b35282435dbc884999be2b3))
* list command - improved output ([#205](https://www.github.com/boson-project/faas/issues/205)) ([29ca077](https://www.github.com/boson-project/faas/commit/29ca07768ca455debb7992ebbf09b2db2058f56d))
* remove create cli subcommand ([#180](https://www.github.com/boson-project/faas/issues/180)) ([57e1236](https://www.github.com/boson-project/faas/commit/57e12362af18f48624a9c303c070846e1645e08d))
* rename faas to function ([#210](https://www.github.com/boson-project/faas/issues/210)) ([cd57692](https://www.github.com/boson-project/faas/commit/cd57692c9d04fecb918abf4f15cd37d45592cf82))


### Bug Fixes

* `delete` and `deploy sub-commands respects func.yaml conf ([d562498](https://www.github.com/boson-project/faas/commit/d5624980d5f31f98bc27e803ae94311491d4d078))
* return JSON in Node.js event template ([#211](https://www.github.com/boson-project/faas/issues/211)) ([beb838f](https://www.github.com/boson-project/faas/commit/beb838ff43d04c7ccec63a26fbe2af9fb167ae1a))

## [0.8.0](https://www.github.com/boson-project/faas/compare/v0.7.0...v0.8.0) (2020-10-20)


### ⚠ BREAKING CHANGES

* change all references of "repository" to "registry" for images (#156)
* combine deploy and update commands (#152)

### Features

* add health probes to node & go services ([#174](https://www.github.com/boson-project/faas/issues/174)) ([95c1eb5](https://www.github.com/boson-project/faas/commit/95c1eb5e59335cfee84ce536d086bd394268c81c))
* introduce CloudEvent data as first parameter for event functions ([#172](https://www.github.com/boson-project/faas/issues/172)) ([7451194](https://www.github.com/boson-project/faas/commit/74511948cefc368d898ad05b911fded74d44b759))
* user can set envvars ([5182487](https://www.github.com/boson-project/faas/commit/5182487df218685867fda10c3d1983b4c035c08a))
* **kn:** Enable faas to be integrated as plugin to kn ([#155](https://www.github.com/boson-project/faas/issues/155)) ([85a5f47](https://www.github.com/boson-project/faas/commit/85a5f475eb32269b9cced05fe36dc90f8befd000))
* ability for users to specify custom builders ([#147](https://www.github.com/boson-project/faas/issues/147)) ([c2b4a30](https://www.github.com/boson-project/faas/commit/c2b4a304bd3fa7d020c71db9f4d79c80c98d86d3))
* combine deploy and update commands ([#152](https://www.github.com/boson-project/faas/issues/152)) ([d5839ea](https://www.github.com/boson-project/faas/commit/d5839ea6c1e84e843ad643cc0611a82e2e6d2399))
* fish completion ([d822303](https://www.github.com/boson-project/faas/commit/d82230353d3d437e8f35e7f9ce3569988d765b42))


### Bug Fixes

* examples in readme ([5591e7f](https://www.github.com/boson-project/faas/commit/5591e7fa2ca9584f03bf8d065778cd120ea9054f))
* image parsing ([6a621a5](https://www.github.com/boson-project/faas/commit/6a621a5186ffffec79e6f34c34681cc37eeaa0bd))
* regenerate pkger files ([#183](https://www.github.com/boson-project/faas/issues/183)) ([1d14a8c](https://www.github.com/boson-project/faas/commit/1d14a8c10156098d66ef691f84ecce1bd25a6d88))
* root cmd init ([ec5327d](https://www.github.com/boson-project/faas/commit/ec5327d5201b57d6a33bcc7314332686582b676f))
* stop using manually edited completion ([bf9b048](https://www.github.com/boson-project/faas/commit/bf9b04881333fed6038251fa4de92368771840d9))
* update quarkus templates ([ffc6a12](https://www.github.com/boson-project/faas/commit/ffc6a123e469968865fef1ccb5f8d84a443baccb))
* update to Knative 0.17 ([#145](https://www.github.com/boson-project/faas/issues/145)) ([5fe7052](https://www.github.com/boson-project/faas/commit/5fe70526e531e283c6704d9526e3cdd7ef64f9e1))


### src

* change all references of "repository" to "registry" for images ([#156](https://www.github.com/boson-project/faas/issues/156)) ([e425c8f](https://www.github.com/boson-project/faas/commit/e425c8f08183b333e56d5d3cfe74fc9e85a6c903))

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
