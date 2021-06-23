# Change Log

<a name="unreleased"></a>
## [0.16.0](https://www.github.com/boson-project/func/compare/v0.15.1...v0.16.0) (2021-06-23)


### ⚠ BREAKING CHANGES

* change --trigger and --templates flags
* function signatures implied from trigger

### Features

* `func config envs` - interactive prompt ([#396](https://www.github.com/boson-project/func/issues/396)) ([83a9ca6](https://www.github.com/boson-project/func/commit/83a9ca684f1b74458b4804fe0e0efe5e95507077))
* `func config volumes` - interactive prompt ([#391](https://www.github.com/boson-project/func/issues/391)) ([4ba95b6](https://www.github.com/boson-project/func/commit/4ba95b69a8926ef56773166951ab8fa577111d37))
* add a URL output type for `func describe` ([#389](https://www.github.com/boson-project/func/issues/389)) ([947fcaa](https://www.github.com/boson-project/func/commit/947fcaa968a90efed4b6037cafa19e8fadda1fc7)), closes [#387](https://www.github.com/boson-project/func/issues/387)
* allow setting autoscaling options to deployed KService ([#374](https://www.github.com/boson-project/func/issues/374)) ([a937c49](https://www.github.com/boson-project/func/commit/a937c490b7e1ad31c3596f91c310c3f4560329fd))
* allow setting resource requests/limits ([#386](https://www.github.com/boson-project/func/issues/386)) ([12c5cda](https://www.github.com/boson-project/func/commit/12c5cda8e2157a775e9fc0bb14fc051c5119f86a))
* reference ConfigMaps in `envs` and `volumes` sections in config ([#371](https://www.github.com/boson-project/func/issues/371)) ([1dbb5ae](https://www.github.com/boson-project/func/commit/1dbb5aecbf73cd77a648eaff5e52c1c3ce282a67))
* reference Secrets in `envs` and `volumes` sections in config ([#369](https://www.github.com/boson-project/func/issues/369)) ([9d7fd34](https://www.github.com/boson-project/func/commit/9d7fd346495b119e895747d747c1c0a5bacb988e))
* Rust templates ([#376](https://www.github.com/boson-project/func/issues/376)) ([4711638](https://www.github.com/boson-project/func/commit/4711638495692e5b8fc1ccca34000c44afa3832c))
* typed errors for templates use cases ([40f1027](https://www.github.com/boson-project/func/commit/40f10277a4efc3239bbec7a35586c3eabf3337ee))


### Bug Fixes

* disable selinux labeling ([6e8517c](https://www.github.com/boson-project/func/commit/6e8517c023fa815c616606640657344785dbe4ff))
* password read on windows ([84f896b](https://www.github.com/boson-project/func/commit/84f896b3298fffe9c8aeec2706c83b6a0fb48141))
* use credsStore ([88ea081](https://www.github.com/boson-project/func/commit/88ea081cc0addb644ca4a575735a6dd3393197a2))


### Code Refactoring

* change --trigger and --templates flags ([ce29ff6](https://www.github.com/boson-project/func/commit/ce29ff6285d68bc008fbf0cfbd956982044104bc))
* function signatures implied from trigger ([b30e883](https://www.github.com/boson-project/func/commit/b30e883e671477ebfa217df03e6825778e84a3df))

### [0.15.1](https://www.github.com/boson-project/func/compare/v0.15.0...v0.15.1) (2021-05-27)


### Bug Fixes

* Revert "chore: bump Knative deps to 0.22.0 ([#358](https://www.github.com/boson-project/func/issues/358))" ([#366](https://www.github.com/boson-project/func/issues/366)) ([72584ce](https://www.github.com/boson-project/func/commit/72584ced0dc3af86852f56ce36171ba567481b41))

## [0.15.0](https://www.github.com/boson-project/func/compare/v0.14.0...v0.15.0) (2021-05-26)


### ⚠ BREAKING CHANGES

* **templates:** modify the nodejs event template to accept a cloudevent (#356)

### Features

* add 'kn func emit' command ([#332](https://www.github.com/boson-project/func/issues/332)) ([49594d9](https://www.github.com/boson-project/func/commit/49594d976627c593ff18e42086199225ddcf5130))
* add typescript templates ([#355](https://www.github.com/boson-project/func/issues/355)) ([d3eafe2](https://www.github.com/boson-project/func/commit/d3eafe2a8451ebc28124b913f03c12e9359d5e30))


### Bug Fixes

* minor typos in node template docs ([#351](https://www.github.com/boson-project/func/issues/351)) ([ea0a75a](https://www.github.com/boson-project/func/commit/ea0a75a7ccb6d00b8c859ff4cd311ad33fb8dbc3))


### src

* **templates:** modify the nodejs event template to accept a cloudevent ([#356](https://www.github.com/boson-project/func/issues/356)) ([caf0659](https://www.github.com/boson-project/func/commit/caf0659900a79650bb11877ffaeadbc30be9f922))

## [0.14.0](https://www.github.com/boson-project/func/compare/v0.13.0...v0.14.0) (2021-05-12)


### ⚠ BREAKING CHANGES

* revert bump to go 1.16 and template changes (#340)

### src

* revert bump to go 1.16 and template changes ([#340](https://www.github.com/boson-project/func/issues/340)) ([2b025df](https://www.github.com/boson-project/func/commit/2b025df19942e990050ef344784662fe77fd7309))

## [0.13.0](https://www.github.com/boson-project/func/compare/v0.12.1...v0.13.0) (2021-05-12)


### ⚠ BREAKING CHANGES

* change envVars to env in func.yaml (#316)

### Features

* add support for annotations in func.yaml ([#314](https://www.github.com/boson-project/func/issues/314)) ([5feb0e2](https://www.github.com/boson-project/func/commit/5feb0e20f366f8dc46f339257d87419bc852753c))
* add/improve spinner for build and deploy ([#322](https://www.github.com/boson-project/func/issues/322)) ([857b0fd](https://www.github.com/boson-project/func/commit/857b0fd19d2a716c804426196e907a3ad31d559e))
* create templates archive on go generate ([63b7f11](https://www.github.com/boson-project/func/commit/63b7f1147176ce5cfd21c3b74094fcc8154298df))
* function name matches KService name ([#317](https://www.github.com/boson-project/func/issues/317)) ([541e858](https://www.github.com/boson-project/func/commit/541e8586f7348fa92ee83f246ef34730b1801b9f))
* positive error when runtimme or template unrecognized ([acc56b0](https://www.github.com/boson-project/func/commit/acc56b0900113ca68270bd3ac68310864e42b5a7))
* preserve file modes using in-memory tar FS ([7dc772e](https://www.github.com/boson-project/func/commit/7dc772ec62536fc77b84b16550bf7d2a1f0b6a09))
* support windows paths in embedded templates FS ([c2b2168](https://www.github.com/boson-project/func/commit/c2b216857bcc1e18555a2e41fa3ad675e75cf1c3))
* usage of local evnvvar in func cfg file ([7f8e595](https://www.github.com/boson-project/func/commit/7f8e5954a939563486661a98198b22f41eebc195))


### Bug Fixes

* added checks on delete command test for lint ([94e387c](https://www.github.com/boson-project/func/commit/94e387c9326aed79ede95f36b97da4de97c42dec))
* default for `--builder` flag ([06455f4](https://www.github.com/boson-project/func/commit/06455f4bac02e8581ae4471e72909ba9fe7dbd4d))
* func delete with explicity name as argument ([#323](https://www.github.com/boson-project/func/issues/323)) with strict validation ([8ab0ba2](https://www.github.com/boson-project/func/commit/8ab0ba243ae4c40867a2426b2ca965559a03cd53))
* lint issues ([895872a](https://www.github.com/boson-project/func/commit/895872aee76b44be739bd0eafb9f2cdcdc137494))


### Code Refactoring

* change envVars to env in func.yaml ([#316](https://www.github.com/boson-project/func/issues/316)) ([89ff286](https://www.github.com/boson-project/func/commit/89ff286a1f3afae655a2c724a05cb3bc3c281786))

### [0.12.1](https://www.github.com/boson-project/func/compare/v0.12.0...v0.12.1) (2021-04-14)


### Bug Fixes

* build needs to use legacy jar ([129dc5a](https://www.github.com/boson-project/func/commit/129dc5a8348dc8e4e14f5891871cf6b50ae35ccc))

## [0.12.0](https://www.github.com/boson-project/func/compare/v0.11.0...v0.12.0) (2021-03-30)


### Features

* add --build (default: true) flag to func deploy ([8a91cac](https://www.github.com/boson-project/func/commit/8a91cac6cc78b5cf56d5158f3eb03a4076a34ffe))
* basic lifecycle integraiton tests ([8edd0df](https://www.github.com/boson-project/func/commit/8edd0df836055b33473f9a7774e8ae755f46ac2e))
* integration tests target ([ddf4ab8](https://www.github.com/boson-project/func/commit/ddf4ab86c46912f78e56a52a14efcf89fd187103))
* local cluster allocation, configuration and teardown ([9c499b6](https://www.github.com/boson-project/func/commit/9c499b69c4991b86e51127081cee7fb0fc34d554))
* using custom docker daemon (e.g podman) ([6d2d8c6](https://www.github.com/boson-project/func/commit/6d2d8c63b01e12f6cf277c2cd18c3f7298ce86ab))


### Bug Fixes

* `func deploy` uses Docker API, not binary ([dc2fbee](https://www.github.com/boson-project/func/commit/dc2fbee67f7f2304bece83a9b4d4f051ed19cd61))
* `func run` now uses Docker API, not binary ([db0945e](https://www.github.com/boson-project/func/commit/db0945ed3ecb9e6e4283a0cb478d39657c6803dc))
* compare service names in integraiton tests ([1551d69](https://www.github.com/boson-project/func/commit/1551d69b5d287becaafdf3d5b99a6ba8da926fa6))
* exposed port ([7ed2e86](https://www.github.com/boson-project/func/commit/7ed2e86d9672f285c1def490a3d325ceb9e8471f))
* increase remove timeout to 120s ([80e366b](https://www.github.com/boson-project/func/commit/80e366b14234c184932d91db4188bdabb0742e7a))
* sprint-boot template ([38fd673](https://www.github.com/boson-project/func/commit/38fd673fdbef1094558b32910a42fcdff2d8bb0c))
* update pack dependency ([c3c2165](https://www.github.com/boson-project/func/commit/c3c21657b2bc3cba9e4ba87864d8fe0c5d4e43af))

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
