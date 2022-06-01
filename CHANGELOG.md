# Changelog

All notable changes to this project will be documented in this file. See [standard-version](https://github.com/conventional-changelog/standard-version) for commit guidelines.

## [0.24.0](https://github.com/knative-sandbox/kn-plugin-func/compare/v0.23.0...v0.24.0) (2022-06-01)


### Features

* configurable s2i builder images ([#1024](https://github.com/knative-sandbox/kn-plugin-func/issues/1024)) ([096085d](https://github.com/knative-sandbox/kn-plugin-func/commit/096085d751f105b5592a09c849fef364b84145b3))
* enable Paketo builders for Python functions ([#979](https://github.com/knative-sandbox/kn-plugin-func/issues/979)) ([5af934b](https://github.com/knative-sandbox/kn-plugin-func/commit/5af934be8d97b0de676fb7d81e07f7b07a33a9e3))
* expose default builder image logic for in-cluster builds ([#1021](https://github.com/knative-sandbox/kn-plugin-func/issues/1021)) ([dc8abf1](https://github.com/knative-sandbox/kn-plugin-func/commit/dc8abf179c63582d197e5a9add2006ad0c026ff6))
* improved invoke verbosity ([#1007](https://github.com/knative-sandbox/kn-plugin-func/issues/1007)) ([867d4c2](https://github.com/knative-sandbox/kn-plugin-func/commit/867d4c26d6dd0b4d06cf6428dc932004e20ca981))
* invoke verbose metadata ([#944](https://github.com/knative-sandbox/kn-plugin-func/issues/944)) ([c3c1456](https://github.com/knative-sandbox/kn-plugin-func/commit/c3c1456ede7b471a271d0680107657ac0e4f0568))
* make templates consistent across runtimes ([#948](https://github.com/knative-sandbox/kn-plugin-func/issues/948)) ([13d4222](https://github.com/knative-sandbox/kn-plugin-func/commit/13d4222461c553d5da1c8448713cba9e4a3e828a))
* s2i builder env var interpolation ([#991](https://github.com/knative-sandbox/kn-plugin-func/issues/991)) ([1424831](https://github.com/knative-sandbox/kn-plugin-func/commit/14248311b1254fe8601b394f064e2fdd92dd4ced))
* s2i builder quarkus support ([#993](https://github.com/knative-sandbox/kn-plugin-func/issues/993)) ([397ce65](https://github.com/knative-sandbox/kn-plugin-func/commit/397ce65598c1f61d2312c62016ed78453e1299a3))
* s2i builder typescript support ([#957](https://github.com/knative-sandbox/kn-plugin-func/issues/957)) ([3be1a77](https://github.com/knative-sandbox/kn-plugin-func/commit/3be1a77388647055dc67e3901da323c240dc77f4))


### Bug Fixes

* ignore `is forbidden` errors when deleting function and resources ([#988](https://github.com/knative-sandbox/kn-plugin-func/issues/988)) ([ce26a23](https://github.com/knative-sandbox/kn-plugin-func/commit/ce26a23352513747cad25b7668def43edff6f0fe))
* ignore node_modules for s2i builds ([#1019](https://github.com/knative-sandbox/kn-plugin-func/issues/1019)) ([1d367c6](https://github.com/knative-sandbox/kn-plugin-func/commit/1d367c6be50d36e045b475c7ff173001b1eaa3d0))
* read pwd from non-tty input ([#996](https://github.com/knative-sandbox/kn-plugin-func/issues/996)) ([e9932cd](https://github.com/knative-sandbox/kn-plugin-func/commit/e9932cdf43eb560ff496a98f83766d1c3e1fdc96))
* update various doc links ([#980](https://github.com/knative-sandbox/kn-plugin-func/issues/980)) ([bc6383e](https://github.com/knative-sandbox/kn-plugin-func/commit/bc6383e55c89349e8e25a79e0f90be1760818e50))


### Documentation

* add a language pack "contract" document ([#918](https://github.com/knative-sandbox/kn-plugin-func/issues/918)) ([76c647a](https://github.com/knative-sandbox/kn-plugin-func/commit/76c647a1c72f57efa8fad89c6802796d02254bd0))


### Miscellaneous

* add release process and func_darwin_arm64 to the release artifacts ([#945](https://github.com/knative-sandbox/kn-plugin-func/issues/945)) ([4e369a0](https://github.com/knative-sandbox/kn-plugin-func/commit/4e369a013ac3109bfea06dc730082b22f6d1cf36))
* add version.txt to .gitattributes for style ([#966](https://github.com/knative-sandbox/kn-plugin-func/issues/966)) ([23d1188](https://github.com/knative-sandbox/kn-plugin-func/commit/23d118831923bf798c5d839c276382f48c048a32))
* clean Repository and Runtimes structs ([#973](https://github.com/knative-sandbox/kn-plugin-func/issues/973)) ([e502d55](https://github.com/knative-sandbox/kn-plugin-func/commit/e502d554c8aafacc0f4d75bb7f6e1aef57e218fa))
* **deps:** bump faas-js-runtime version ([#1000](https://github.com/knative-sandbox/kn-plugin-func/issues/1000)) ([83c081d](https://github.com/knative-sandbox/kn-plugin-func/commit/83c081d34a08b4775787c61eb00f9ca39cfcd33d))
* templates ([#961](https://github.com/knative-sandbox/kn-plugin-func/issues/961)) ([34cb893](https://github.com/knative-sandbox/kn-plugin-func/commit/34cb893545a5f74a120783f66bb3a37d2b283d64))
* update allocate script with latest knative ([#965](https://github.com/knative-sandbox/kn-plugin-func/issues/965)) ([4ffb1f9](https://github.com/knative-sandbox/kn-plugin-func/commit/4ffb1f9cba3ee1bac248a033ece6e8473965f7a7))
* use paketo builders for all runtimes ([#1001](https://github.com/knative-sandbox/kn-plugin-func/issues/1001)) ([31c1d66](https://github.com/knative-sandbox/kn-plugin-func/commit/31c1d66eb3f0089541f8219f07722c75c8a82692))

<a name="unreleased"></a>


---
### [0.23.0](https://github.com/knative-sandbox/kn-plugin-func/compare/v0.23.0...v0.22.0) (2022-04-06)


### Features

* command help text template preprocessing ([#875](https://github.com/knative-sandbox/kn-plugin-func/issues/875)) ([2bd5254](https://github.com/knative-sandbox/kn-plugin-func/commit/2bd5254f19a14d6aae9bd4a4b59971ef36e96fad))
* invoke verbose metadata ([#944](https://github.com/knative-sandbox/kn-plugin-func/issues/944)) ([c3c1456](https://github.com/knative-sandbox/kn-plugin-func/commit/c3c1456ede7b471a271d0680107657ac0e4f0568))
* on cluster build doens't require privileged cluster permissions ([#934](https://github.com/knative-sandbox/kn-plugin-func/issues/934)) ([e9251f5](https://github.com/knative-sandbox/kn-plugin-func/commit/e9251f518cc806768f0221f11e39f04fa4619537))
* s2i builder with preliminary node support ([#923](https://github.com/knative-sandbox/kn-plugin-func/issues/923)) ([a91bcc5](https://github.com/knative-sandbox/kn-plugin-func/commit/a91bcc5fcfe66948c86ce3e33cf0d28230536f1c)), closes [#921](https://github.com/knative-sandbox/kn-plugin-func/issues/921)


### Bug Fixes

* apply updated spring-boot-function dependency ([#936](https://github.com/knative-sandbox/kn-plugin-func/issues/936)) ([4a4cebb](https://github.com/knative-sandbox/kn-plugin-func/commit/4a4cebb1ea7226e7d7c1dbfb9e3fa8e5ec22c31d))
* bind verbose flag to root ([#884](https://github.com/knative-sandbox/kn-plugin-func/issues/884)) ([25524a1](https://github.com/knative-sandbox/kn-plugin-func/commit/25524a1f8435cd310b45f283e987eee7a8736ceb))
* full clone of template repos on add ([#904](https://github.com/knative-sandbox/kn-plugin-func/issues/904)) ([564a34b](https://github.com/knative-sandbox/kn-plugin-func/commit/564a34b3f53381bdd59262dcb78d2953f973c8bb))
* minor typos in docs ([#862](https://github.com/knative-sandbox/kn-plugin-func/issues/862)) ([efc3b20](https://github.com/knative-sandbox/kn-plugin-func/commit/efc3b208cb5ab76f1eb73801501bcbfc23f16928))
* use full root name for cmd help prefixes ([#873](https://github.com/knative-sandbox/kn-plugin-func/issues/873)) ([3f30c91](https://github.com/knative-sandbox/kn-plugin-func/commit/3f30c91116344b592bf392e92b63cb845b25428a))


### Miscellaneous

* add Apple M1 build in cross-platform target ([#932](https://github.com/knative-sandbox/kn-plugin-func/issues/932)) ([00d5a82](https://github.com/knative-sandbox/kn-plugin-func/commit/00d5a8272284ea40ebeefa4f22f12c2d375aadae))
---
## [0.22.0](https://www.github.com/knative-sandbox/kn-plugin-func/compare/v0.21.2...v0.22.0) (2022-02-22)

### Chore

- Bump Node.js builder image to the latest paketo builder, removing a non-fatal warning that was issued at function startup. (#[8](https://github.com/knative-sandbox/kn-plugin-func/runs/5297690460?check_suite_focus=true#step:6:8)26, @lance)
- Update boson builder images to most recent versions (#8[10](https://github.com/knative-sandbox/kn-plugin-func/runs/5297690460?check_suite_focus=true#step:6:10), @matejvasek)

### Enhancement

- Adds the --build flag for 'func deploy' to the shell completions (#802, @matejvasek)

### Documentation

- Clarify podman requirements on Linux vs. MacOS and Linux in podman.md guide (#836, @matejvasek)

### Bug or Regression

- Fix a bug where interactive prompt defaults were not being used (#821, @lkingland)
- Fixes a bug during func create when confirm option is used with the go language runtime (#8[15](https://github.com/knative-sandbox/kn-plugin-func/runs/5297690460?check_suite_focus=true#step:6:15), @senthilnathan)
- Fixes a bug where the invoke and describe commands could fail if Knative Eventing is not installed on the cluster. (#8[23](https://github.com/knative-sandbox/kn-plugin-func/runs/5297690460?check_suite_focus=true#step:6:23), @lance)
- Fixes missing `cloudevent` invocation format for Node.js, SpringBoot and Rust CloudEvent templates (#846, @lance)

### Uncategorized

- Detects when deploying to OpenShift and use internal registry (#8[25](https://github.com/knative-sandbox/kn-plugin-func/runs/5297690460?check_suite_focus=true#step:6:25), @matejvasek)

---
## [0.21.2](https://www.github.com/knative-sandbox/kn-plugin-func/compare/v0.21.1...v0.21.2) (2022-01-28)

## What's Changed
* backport: bug fixes for 0.21 by @lance in https://github.com/knative-sandbox/kn-plugin-func/pull/793


**Full Changelog**: https://github.com/knative-sandbox/kn-plugin-func/compare/v0.21.1...v0.21.2

---
## [0.21.1](https://www.github.com/knative-sandbox/kn-plugin-func/compare/v0.21.0...v0.21.1) (2022-01-27)

### Enhancement

- Adds a label `function.knative.dev/name: functionName` to every resouce created for a Function ([#757](https://github.com/knative-sandbox/kn-plugin-func/pull/757), [@zroubalik](https://github.com/zroubalik))
- Adds the ability to build a Function on the cluster using Tekton Pipelines. The build on the cluster is enabled by fetching Function source code from a remote Git repository. ([#743](https://github.com/knative-sandbox/kn-plugin-func/pull/743), [@zroubalik](https://github.com/zroubalik))

### Bug or Regression

- Changes the springboot function templates to use the base builder instead of the tiny builder. ([#792](https://github.com/knative-sandbox/kn-plugin-func/pull/792), [@lance](https://github.com/lance))


**Full Changelog**: https://github.com/knative-sandbox/kn-plugin-func/compare/v0.21.0...v0.21.1

---
## [0.21.0](https://www.github.com/knative-sandbox/kn-plugin-func/compare/v0.20.0...v0.21.0) (2022-01-12)


### Features

* add possibility to disable pushing of image in `deploy` command ([#736](https://www.github.com/knative-sandbox/kn-plugin-func/issues/736)) ([4e5a5e8](https://www.github.com/knative-sandbox/kn-plugin-func/commit/4e5a5e830799b73f65aba8ee248a52bf1c643acb))
* add possibility to disable pushing of image in `deploy` command ([#739](https://www.github.com/knative-sandbox/kn-plugin-func/issues/739)) ([64ba17b](https://www.github.com/knative-sandbox/kn-plugin-func/commit/64ba17b4fbe9033e8279fa34aeebdb12edcee25d))

## [0.20.0](https://www.github.com/knative-sandbox/kn-plugin-func/compare/v0.19.0...v0.20.0) (2021-12-20)


### ⚠ BREAKING CHANGES

* use `function.knative.dev` for Functions related labels (#717)

### Features

* add flag to push image at the end of a successful build ([#681](https://www.github.com/knative-sandbox/kn-plugin-func/issues/681)) ([2f24182](https://www.github.com/knative-sandbox/kn-plugin-func/commit/2f241824ff3a2664a987fe742aed2f0b56aeb9ab))
* add POD_NAME as an environment variable ([#660](https://www.github.com/knative-sandbox/kn-plugin-func/issues/660)) ([64473b7](https://www.github.com/knative-sandbox/kn-plugin-func/commit/64473b7197bb5a821b6724a8b914784891b1a828))
* add telemetry to Node.js and TypeScript function templates ([#719](https://www.github.com/knative-sandbox/kn-plugin-func/issues/719)) ([d7cfe6e](https://www.github.com/knative-sandbox/kn-plugin-func/commit/d7cfe6ead76f15c4bcd34a132d0c17c02a149548))
* allow build to be triggered from run when fn.Image is missing ([#644](https://www.github.com/knative-sandbox/kn-plugin-func/issues/644)) ([b190b52](https://www.github.com/knative-sandbox/kn-plugin-func/commit/b190b527542bf659f06bf931d94d09542d012c36))
* allow push to cluster internal registries ([#718](https://www.github.com/knative-sandbox/kn-plugin-func/issues/718)) ([8d51393](https://www.github.com/knative-sandbox/kn-plugin-func/commit/8d51393181adca0c74a4b08cfb2dc2da390f983b))
* automatically start podman service ([#648](https://www.github.com/knative-sandbox/kn-plugin-func/issues/648)) ([bfdfb76](https://www.github.com/knative-sandbox/kn-plugin-func/commit/bfdfb760cff575146764f9b841a0cafcb31bcd58))
* custom default HTTP transport ([#711](https://www.github.com/knative-sandbox/kn-plugin-func/issues/711)) ([a13f897](https://www.github.com/knative-sandbox/kn-plugin-func/commit/a13f897fbb996dbfcb2120965745477321087a9c))
* ensure config and repos path exists ([#683](https://www.github.com/knative-sandbox/kn-plugin-func/issues/683)) ([db9ad07](https://www.github.com/knative-sandbox/kn-plugin-func/commit/db9ad07c7048361946a8c7d45c549323eee44a58))
* function creation timestamp ([#651](https://www.github.com/knative-sandbox/kn-plugin-func/issues/651)) ([1bf17ec](https://www.github.com/knative-sandbox/kn-plugin-func/commit/1bf17ec976130551da366e75b38f5169b3daed4e))
* function version migrations ([#664](https://www.github.com/knative-sandbox/kn-plugin-func/issues/664)) ([ccf0015](https://www.github.com/knative-sandbox/kn-plugin-func/commit/ccf00152be0ceba1794267f8e03a09cb32fee514))
* In cluster dialer to proxy TCP connections to unexposed services ([#688](https://www.github.com/knative-sandbox/kn-plugin-func/issues/688)) ([98ef5a0](https://www.github.com/knative-sandbox/kn-plugin-func/commit/98ef5a00356a5b93ef1a6c581ae8d5ba86ee09e4))
* make SpringBoot template SpringNative by default ([#649](https://www.github.com/knative-sandbox/kn-plugin-func/issues/649)) ([c70a21e](https://www.github.com/knative-sandbox/kn-plugin-func/commit/c70a21e9a459d726a4118e177835082323698f83))
* use `function.knative.dev` for Functions related labels ([#717](https://www.github.com/knative-sandbox/kn-plugin-func/issues/717)) ([feaf8f9](https://www.github.com/knative-sandbox/kn-plugin-func/commit/feaf8f91091afc21bcc3e99ed3098c9ff0679883))


### Bug Fixes

* make registry validation work again ([#690](https://www.github.com/knative-sandbox/kn-plugin-func/issues/690)) ([10f2cf4](https://www.github.com/knative-sandbox/kn-plugin-func/commit/10f2cf44c74884b4585114affd3c05cfba4f7613))
* move integration port to unregistered range ([#701](https://www.github.com/knative-sandbox/kn-plugin-func/issues/701)) ([f63af0d](https://www.github.com/knative-sandbox/kn-plugin-func/commit/f63af0d34e97a549df51c47f12e22a7558504278))
* remove stray manifest.yaml files ([#628](https://www.github.com/knative-sandbox/kn-plugin-func/issues/628)) ([c810efc](https://www.github.com/knative-sandbox/kn-plugin-func/commit/c810efc7a4eb0d87f37f3acfafc46c1e4639fdc4))
* remove template manifest from final Function ([#703](https://www.github.com/knative-sandbox/kn-plugin-func/issues/703)) ([79ad65d](https://www.github.com/knative-sandbox/kn-plugin-func/commit/79ad65ddf42b1bfffacd3f6fe4d606885b1a3766))
* revert hostname alias ([#712](https://www.github.com/knative-sandbox/kn-plugin-func/issues/712)) ([07062c1](https://www.github.com/knative-sandbox/kn-plugin-func/commit/07062c144aa19e4eb4c4ef27d5c5cb1ebb8eb185))
* schema-generate should point to `function.go` ([#677](https://www.github.com/knative-sandbox/kn-plugin-func/issues/677)) ([c7d18c8](https://www.github.com/knative-sandbox/kn-plugin-func/commit/c7d18c89edaf0a4fcb5c7b684e5e151c217430c2))
* use default socket path for TCP connections ([#669](https://www.github.com/knative-sandbox/kn-plugin-func/issues/669)) ([ee96bef](https://www.github.com/knative-sandbox/kn-plugin-func/commit/ee96bef9aea34c9370cf1f871bc4c558ed449a13))
* use specific version of paketo builder ([#670](https://www.github.com/knative-sandbox/kn-plugin-func/issues/670)) ([834e8ae](https://www.github.com/knative-sandbox/kn-plugin-func/commit/834e8ae46c833d2052171dc8dde23648a1da5112))

## [0.19.0](https://www.github.com/knative-sandbox/kn-plugin-func/compare/v0.18.0...v0.19.0) (2021-11-03)


### ⚠ BREAKING CHANGES

* rename event templates to 'cloudevents' (#584)

### Features

* add support for manifest.yaml at repo/language/template levels ([#558](https://www.github.com/knative-sandbox/kn-plugin-func/issues/558)) ([e319ea3](https://www.github.com/knative-sandbox/kn-plugin-func/commit/e319ea3b62150ea2939876cb078ce150b7580bdf))
* allow developers to provide Build Envs to buildpacks in `func.yaml` ([#571](https://www.github.com/knative-sandbox/kn-plugin-func/issues/571)) ([114a5fa](https://www.github.com/knative-sandbox/kn-plugin-func/commit/114a5faee3ab9ae1ec37ae5c4375c6218405c3b7))
* create cli ([#547](https://www.github.com/knative-sandbox/kn-plugin-func/issues/547)) ([4fe9fdc](https://www.github.com/knative-sandbox/kn-plugin-func/commit/4fe9fdcab08552814c86d85194c552b591f52cd7))
* Improve build performance ([#569](https://www.github.com/knative-sandbox/kn-plugin-func/issues/569)) ([ef7b986](https://www.github.com/knative-sandbox/kn-plugin-func/commit/ef7b986900e1eb6afd4203067aeac3dc99a4ede5))
* periodically update progress during build ([#537](https://www.github.com/knative-sandbox/kn-plugin-func/issues/537)) ([01689e7](https://www.github.com/knative-sandbox/kn-plugin-func/commit/01689e7c131dd79db1e469c3ce54bd011464a6ef))
* rename event templates to 'cloudevents' ([#584](https://www.github.com/knative-sandbox/kn-plugin-func/issues/584)) ([68b0904](https://www.github.com/knative-sandbox/kn-plugin-func/commit/68b0904b17dee5bc6a5ec71132d35c45d52f4b71))
* Save password after user input ([#560](https://www.github.com/knative-sandbox/kn-plugin-func/issues/560)) ([e85a4aa](https://www.github.com/knative-sandbox/kn-plugin-func/commit/e85a4aa7f38681eb7554cac818eab7b1ca3bded1))
* ssh connection to remote docker daemon ([#594](https://www.github.com/knative-sandbox/kn-plugin-func/issues/594)) ([e1f164d](https://www.github.com/knative-sandbox/kn-plugin-func/commit/e1f164d2ca6b0e720ebcf881b8d5fd123dfd0d82))
* validation for registry/namespace to not contain image name ([#601](https://www.github.com/knative-sandbox/kn-plugin-func/issues/601)) ([cf9596c](https://www.github.com/knative-sandbox/kn-plugin-func/commit/cf9596c83e6eae8229ca61a18fafb5ba4df31d6b))


### Bug Fixes

* ConfigMap/Secret key validation ([#623](https://www.github.com/knative-sandbox/kn-plugin-func/issues/623)) ([0ed1e81](https://www.github.com/knative-sandbox/kn-plugin-func/commit/0ed1e816920b3e750c0aa0ed25fd1e0308e80a2e))
* Environment values -> Environment variables ([#622](https://www.github.com/knative-sandbox/kn-plugin-func/issues/622)) ([ac9de9d](https://www.github.com/knative-sandbox/kn-plugin-func/commit/ac9de9dfc68969ecb825c9ee2a92aa9918ae5ea7))
* hide a fmt.Println behind verbose flag ([#538](https://www.github.com/knative-sandbox/kn-plugin-func/issues/538)) ([ad4607b](https://www.github.com/knative-sandbox/kn-plugin-func/commit/ad4607bd50ae0c41ba0792d46318757089239de4))
* improve error message when invalid function name is used ([#567](https://www.github.com/knative-sandbox/kn-plugin-func/issues/567)) ([0e3c676](https://www.github.com/knative-sandbox/kn-plugin-func/commit/0e3c6764ef716cf24a3f60676e139d0c61161693))
* registry URL comparison ([#549](https://www.github.com/knative-sandbox/kn-plugin-func/issues/549)) ([b10c484](https://www.github.com/knative-sandbox/kn-plugin-func/commit/b10c48453cc5817c4c28077be13fc03baee5d818))
* stop the progress ticker after build completes ([#544](https://www.github.com/knative-sandbox/kn-plugin-func/issues/544)) ([4f3e5fd](https://www.github.com/knative-sandbox/kn-plugin-func/commit/4f3e5fdb7a40a3419d8d731d5a0c916b81af069b))
* update-pkger.sh sed error on osX ([#541](https://www.github.com/knative-sandbox/kn-plugin-func/issues/541)) ([25f8b4d](https://www.github.com/knative-sandbox/kn-plugin-func/commit/25f8b4d6ead2f47c3ab6541e2bdb5016b4a423aa))

## [0.18.0](https://www.github.com/knative-sandbox/kn-plugin-func/compare/v0.17.1...v0.18.0) (2021-09-16)


### ⚠ BREAKING CHANGES

* change `describe` command to `info` (#474)
* use key&value for Labels (#472)

### Features

* allow language packs to set function metadata ([#465](https://www.github.com/knative-sandbox/kn-plugin-func/issues/465)) ([48f40c3](https://www.github.com/knative-sandbox/kn-plugin-func/commit/48f40c35e3a239d09d6a87fc4603ad21db46bc37))
* builders/buildpacks configured in client ([#495](https://www.github.com/knative-sandbox/kn-plugin-func/issues/495)) ([668804e](https://www.github.com/knative-sandbox/kn-plugin-func/commit/668804e53e76ce153a887289efb2b05f88203a1f))
* change `describe` command to `info` ([#474](https://www.github.com/knative-sandbox/kn-plugin-func/issues/474)) ([10a0757](https://www.github.com/knative-sandbox/kn-plugin-func/commit/10a07578e9f6ab6bbbb8028633b37e3400fd22bb))
* client effective runtimes list ([#490](https://www.github.com/knative-sandbox/kn-plugin-func/issues/490)) ([e0aad6f](https://www.github.com/knative-sandbox/kn-plugin-func/commit/e0aad6f936067892e04a463f85ca46689714716c))
* generate json schema for func.yaml ([#460](https://www.github.com/knative-sandbox/kn-plugin-func/issues/460)) ([8939f89](https://www.github.com/knative-sandbox/kn-plugin-func/commit/8939f89beae7d5b2f66bc18b921ca3059f89e629))
* make func schema if config updated ([#468](https://www.github.com/knative-sandbox/kn-plugin-func/issues/468)) ([6ae2157](https://www.github.com/knative-sandbox/kn-plugin-func/commit/6ae215754930c8a1e1dc4b5cd0b8ef3d99bb2893))
* move go, typescript and nodejs to paketo builders ([#485](https://www.github.com/knative-sandbox/kn-plugin-func/issues/485)) ([a4b15ad](https://www.github.com/knative-sandbox/kn-plugin-func/commit/a4b15ad9926112910251a8d74747e2db368c86e9))
* repository and templates client api ([#475](https://www.github.com/knative-sandbox/kn-plugin-func/issues/475)) ([3f56a8f](https://www.github.com/knative-sandbox/kn-plugin-func/commit/3f56a8fd7a66b923294043bcaa68ad59b1228831))
* repository management cli ([#514](https://www.github.com/knative-sandbox/kn-plugin-func/issues/514)) ([ae638c3](https://www.github.com/knative-sandbox/kn-plugin-func/commit/ae638c349c46c035bad74645bfc612380c871a85))
* repository management client api ([#467](https://www.github.com/knative-sandbox/kn-plugin-func/issues/467)) ([9fd2475](https://www.github.com/knative-sandbox/kn-plugin-func/commit/9fd247557ae8ee30cc7c5f0107d80fa72fbe8086))
* use key&value for Labels ([#472](https://www.github.com/knative-sandbox/kn-plugin-func/issues/472)) ([5569681](https://www.github.com/knative-sandbox/kn-plugin-func/commit/55696811e317a51767e09acab3d4d4e2abc6e982))


### Bug Fixes

* `build` should honor registry specified in `-r` ([#510](https://www.github.com/knative-sandbox/kn-plugin-func/issues/510)) ([8aba038](https://www.github.com/knative-sandbox/kn-plugin-func/commit/8aba038073f5584133eb3d08ba85289800e2e770))
* `config labels` panic ([#493](https://www.github.com/knative-sandbox/kn-plugin-func/issues/493)) ([f2efbe5](https://www.github.com/knative-sandbox/kn-plugin-func/commit/f2efbe5b42a6e0af36ecc6be429a630312e0c6e5))
* better cleanup before pkger run ([#479](https://www.github.com/knative-sandbox/kn-plugin-func/issues/479)) ([25b1d63](https://www.github.com/knative-sandbox/kn-plugin-func/commit/25b1d63b9c1b332e1d59e494af83bdc3a1f576e9))
* control chars on progress listener for Windows OS ([#498](https://www.github.com/knative-sandbox/kn-plugin-func/issues/498)) ([1172a85](https://www.github.com/knative-sandbox/kn-plugin-func/commit/1172a85c80f834ff3958073bc36ff4a5173c9de6))
* enable healt checks for Quarkus ([#477](https://www.github.com/knative-sandbox/kn-plugin-func/issues/477)) ([72a1cf8](https://www.github.com/knative-sandbox/kn-plugin-func/commit/72a1cf885e092340295cc6ace3580e7420640cda))
* fast-fail on create if Function already exists ([#496](https://www.github.com/knative-sandbox/kn-plugin-func/issues/496)) ([25f7007](https://www.github.com/knative-sandbox/kn-plugin-func/commit/25f7007300c020b5a1d336740a2bbc2f546bf3da))
* regenerate pkged.go ([#478](https://www.github.com/knative-sandbox/kn-plugin-func/issues/478)) ([c7b3af4](https://www.github.com/knative-sandbox/kn-plugin-func/commit/c7b3af41b8cac0b9edfb96d3a01230d2606e320a))
* removal of repositories ([#524](https://www.github.com/knative-sandbox/kn-plugin-func/issues/524)) ([90c60b6](https://www.github.com/knative-sandbox/kn-plugin-func/commit/90c60b693d6b2dbb2c8edee27a7cf7b6e8d1c399))
* support nested subdirs in remote templates ([#482](https://www.github.com/knative-sandbox/kn-plugin-func/issues/482)) ([fcf9e77](https://www.github.com/knative-sandbox/kn-plugin-func/commit/fcf9e77cb93808d28d0c60f3a0959fac605771fb))
* use full image names ([#535](https://www.github.com/knative-sandbox/kn-plugin-func/issues/535)) ([16ee28c](https://www.github.com/knative-sandbox/kn-plugin-func/commit/16ee28c83debcc19092abb250ef20354eca09710))

### [0.17.1](https://www.github.com/knative-sandbox/kn-plugin-func/compare/v0.17.0...v0.17.1) (2021-08-05)


### Bug Fixes

* hide progress indicator if asking for creds ([#458](https://www.github.com/knative-sandbox/kn-plugin-func/issues/458)) ([79e2234](https://www.github.com/knative-sandbox/kn-plugin-func/commit/79e2234cbc62319f35b18a9b2a39ca4dffe89d4d))
* use ascii chars in progress indicator on win ([#459](https://www.github.com/knative-sandbox/kn-plugin-func/issues/459)) ([6fd42a4](https://www.github.com/knative-sandbox/kn-plugin-func/commit/6fd42a421ea58a4e9e1b6b6bff3f97d1da99d349))

## [0.17.0](https://www.github.com/knative-sandbox/kn-plugin-func/compare/v0.16.0...v0.17.0) (2021-08-03)


### Features

* Add proper example of configuring Rust functions. ([#436](https://www.github.com/knative-sandbox/kn-plugin-func/issues/436)) ([7656c40](https://www.github.com/knative-sandbox/kn-plugin-func/commit/7656c4097283ed54b9e5f0472947cff931973365))
* add support for labels in func.yaml ([#373](https://www.github.com/knative-sandbox/kn-plugin-func/issues/373)) ([0dba677](https://www.github.com/knative-sandbox/kn-plugin-func/commit/0dba67751e5a4c594701d674b44b101a043e9a2c))
* Configure Rust functions ([#430](https://www.github.com/knative-sandbox/kn-plugin-func/issues/430)) ([a08b843](https://www.github.com/knative-sandbox/kn-plugin-func/commit/a08b843a9c2639d6b237f4248341b35f3bd8b954))
* print emit response output if it's a cloudevent ([#444](https://www.github.com/knative-sandbox/kn-plugin-func/issues/444)) ([a25b723](https://www.github.com/knative-sandbox/kn-plugin-func/commit/a25b723dbcd50d544566a385441cbdd883017947))
* remote template repositories ([#437](https://www.github.com/knative-sandbox/kn-plugin-func/issues/437)) ([9db1a3d](https://www.github.com/knative-sandbox/kn-plugin-func/commit/9db1a3d902016d59e60b732de43bdf4be198334f))


### Bug Fixes

* closing stdout ([6f40b29](https://www.github.com/knative-sandbox/kn-plugin-func/commit/6f40b29d3e02193c51317a29737c20dc11730c5a))
* do not trust builder when using podman ([#420](https://www.github.com/knative-sandbox/kn-plugin-func/issues/420)) ([894f4fe](https://www.github.com/knative-sandbox/kn-plugin-func/commit/894f4febda1d7da5d3f47e1003b29b339b1f8cd4))
* fix unit tests for Node.js event templates ([#438](https://www.github.com/knative-sandbox/kn-plugin-func/issues/438)) ([d71532a](https://www.github.com/knative-sandbox/kn-plugin-func/commit/d71532a070b24ec70dd5b77221e11b53bd300e8d))
* unnecessary template repackaging ([#449](https://www.github.com/knative-sandbox/kn-plugin-func/issues/449)) ([435d1ac](https://www.github.com/knative-sandbox/kn-plugin-func/commit/435d1ac2a39c4e3abf1a6518b05be3151d132a57))
* update builders version ([#421](https://www.github.com/knative-sandbox/kn-plugin-func/issues/421)) ([771a230](https://www.github.com/knative-sandbox/kn-plugin-func/commit/771a2307a13d105a188a0fd2c2fa843f3a535277))

## [0.16.0](https://github.com/knative-sandbox/kn-plugin-func/compare/v0.15.1...v0.16.0) (2021-06-23)


### ⚠ BREAKING CHANGES

* change --trigger and --templates flags
* function signatures implied from trigger

### Features

* `func config envs` - interactive prompt ([#396](https://github.com/knative-sandbox/kn-plugin-func/issues/396)) ([83a9ca6](https://github.com/knative-sandbox/kn-plugin-func/commit/83a9ca684f1b74458b4804fe0e0efe5e95507077))
* `func config volumes` - interactive prompt ([#391](https://github.com/knative-sandbox/kn-plugin-func/issues/391)) ([4ba95b6](https://github.com/knative-sandbox/kn-plugin-func/commit/4ba95b69a8926ef56773166951ab8fa577111d37))
* add a URL output type for `func describe` ([#389](https://github.com/knative-sandbox/kn-plugin-func/issues/389)) ([947fcaa](https://github.com/knative-sandbox/kn-plugin-func/commit/947fcaa968a90efed4b6037cafa19e8fadda1fc7)), closes [#387](https://github.com/knative-sandbox/kn-plugin-func/issues/387)
* allow setting autoscaling options to deployed KService ([#374](https://github.com/knative-sandbox/kn-plugin-func/issues/374)) ([a937c49](https://github.com/knative-sandbox/kn-plugin-func/commit/a937c490b7e1ad31c3596f91c310c3f4560329fd))
* allow setting resource requests/limits ([#386](https://github.com/knative-sandbox/kn-plugin-func/issues/386)) ([12c5cda](https://github.com/knative-sandbox/kn-plugin-func/commit/12c5cda8e2157a775e9fc0bb14fc051c5119f86a))
* reference ConfigMaps in `envs` and `volumes` sections in config ([#371](https://github.com/knative-sandbox/kn-plugin-func/issues/371)) ([1dbb5ae](https://github.com/knative-sandbox/kn-plugin-func/commit/1dbb5aecbf73cd77a648eaff5e52c1c3ce282a67))
* reference Secrets in `envs` and `volumes` sections in config ([#369](https://github.com/knative-sandbox/kn-plugin-func/issues/369)) ([9d7fd34](https://github.com/knative-sandbox/kn-plugin-func/commit/9d7fd346495b119e895747d747c1c0a5bacb988e))
* Rust templates ([#376](https://github.com/knative-sandbox/kn-plugin-func/issues/376)) ([4711638](https://github.com/knative-sandbox/kn-plugin-func/commit/4711638495692e5b8fc1ccca34000c44afa3832c))
* typed errors for templates use cases ([40f1027](https://github.com/knative-sandbox/kn-plugin-func/commit/40f10277a4efc3239bbec7a35586c3eabf3337ee))


### Bug Fixes

* disable selinux labeling ([6e8517c](https://github.com/knative-sandbox/kn-plugin-func/commit/6e8517c023fa815c616606640657344785dbe4ff))
* password read on windows ([84f896b](https://github.com/knative-sandbox/kn-plugin-func/commit/84f896b3298fffe9c8aeec2706c83b6a0fb48141))
* use credsStore ([88ea081](https://github.com/knative-sandbox/kn-plugin-func/commit/88ea081cc0addb644ca4a575735a6dd3393197a2))


### Code Refactoring

* change --trigger and --templates flags ([ce29ff6](https://github.com/knative-sandbox/kn-plugin-func/commit/ce29ff6285d68bc008fbf0cfbd956982044104bc))
* function signatures implied from trigger ([b30e883](https://github.com/knative-sandbox/kn-plugin-func/commit/b30e883e671477ebfa217df03e6825778e84a3df))

### [0.15.1](https://github.com/knative-sandbox/kn-plugin-func/compare/v0.15.0...v0.15.1) (2021-05-27)


### Bug Fixes

* Revert "chore: bump Knative deps to 0.22.0 ([#358](https://github.com/knative-sandbox/kn-plugin-func/issues/358))" ([#366](https://github.com/knative-sandbox/kn-plugin-func/issues/366)) ([72584ce](https://github.com/knative-sandbox/kn-plugin-func/commit/72584ced0dc3af86852f56ce36171ba567481b41))

## [0.15.0](https://github.com/knative-sandbox/kn-plugin-func/compare/v0.14.0...v0.15.0) (2021-05-26)


### ⚠ BREAKING CHANGES

* **templates:** modify the nodejs event template to accept a cloudevent (#356)

### Features

* add 'kn func emit' command ([#332](https://github.com/knative-sandbox/kn-plugin-func/issues/332)) ([49594d9](https://github.com/knative-sandbox/kn-plugin-func/commit/49594d976627c593ff18e42086199225ddcf5130))
* add typescript templates ([#355](https://github.com/knative-sandbox/kn-plugin-func/issues/355)) ([d3eafe2](https://github.com/knative-sandbox/kn-plugin-func/commit/d3eafe2a8451ebc28124b913f03c12e9359d5e30))


### Bug Fixes

* minor typos in node template docs ([#351](https://github.com/knative-sandbox/kn-plugin-func/issues/351)) ([ea0a75a](https://github.com/knative-sandbox/kn-plugin-func/commit/ea0a75a7ccb6d00b8c859ff4cd311ad33fb8dbc3))


### src

* **templates:** modify the nodejs event template to accept a cloudevent ([#356](https://github.com/knative-sandbox/kn-plugin-func/issues/356)) ([caf0659](https://github.com/knative-sandbox/kn-plugin-func/commit/caf0659900a79650bb11877ffaeadbc30be9f922))

## [0.14.0](https://github.com/knative-sandbox/kn-plugin-func/compare/v0.13.0...v0.14.0) (2021-05-12)


### ⚠ BREAKING CHANGES

* revert bump to go 1.16 and template changes (#340)

### src

* revert bump to go 1.16 and template changes ([#340](https://github.com/knative-sandbox/kn-plugin-func/issues/340)) ([2b025df](https://github.com/knative-sandbox/kn-plugin-func/commit/2b025df19942e990050ef344784662fe77fd7309))

## [0.13.0](https://github.com/knative-sandbox/kn-plugin-func/compare/v0.12.1...v0.13.0) (2021-05-12)


### ⚠ BREAKING CHANGES

* change envVars to env in func.yaml (#316)

### Features

* add support for annotations in func.yaml ([#314](https://github.com/knative-sandbox/kn-plugin-func/issues/314)) ([5feb0e2](https://github.com/knative-sandbox/kn-plugin-func/commit/5feb0e20f366f8dc46f339257d87419bc852753c))
* add/improve spinner for build and deploy ([#322](https://github.com/knative-sandbox/kn-plugin-func/issues/322)) ([857b0fd](https://github.com/knative-sandbox/kn-plugin-func/commit/857b0fd19d2a716c804426196e907a3ad31d559e))
* create templates archive on go generate ([63b7f11](https://github.com/knative-sandbox/kn-plugin-func/commit/63b7f1147176ce5cfd21c3b74094fcc8154298df))
* function name matches KService name ([#317](https://github.com/knative-sandbox/kn-plugin-func/issues/317)) ([541e858](https://github.com/knative-sandbox/kn-plugin-func/commit/541e8586f7348fa92ee83f246ef34730b1801b9f))
* positive error when runtimme or template unrecognized ([acc56b0](https://github.com/knative-sandbox/kn-plugin-func/commit/acc56b0900113ca68270bd3ac68310864e42b5a7))
* preserve file modes using in-memory tar FS ([7dc772e](https://github.com/knative-sandbox/kn-plugin-func/commit/7dc772ec62536fc77b84b16550bf7d2a1f0b6a09))
* support windows paths in embedded templates FS ([c2b2168](https://github.com/knative-sandbox/kn-plugin-func/commit/c2b216857bcc1e18555a2e41fa3ad675e75cf1c3))
* usage of local evnvvar in func cfg file ([7f8e595](https://github.com/knative-sandbox/kn-plugin-func/commit/7f8e5954a939563486661a98198b22f41eebc195))


### Bug Fixes

* added checks on delete command test for lint ([94e387c](https://github.com/knative-sandbox/kn-plugin-func/commit/94e387c9326aed79ede95f36b97da4de97c42dec))
* default for `--builder` flag ([06455f4](https://github.com/knative-sandbox/kn-plugin-func/commit/06455f4bac02e8581ae4471e72909ba9fe7dbd4d))
* func delete with explicity name as argument ([#323](https://github.com/knative-sandbox/kn-plugin-func/issues/323)) with strict validation ([8ab0ba2](https://github.com/knative-sandbox/kn-plugin-func/commit/8ab0ba243ae4c40867a2426b2ca965559a03cd53))
* lint issues ([895872a](https://github.com/knative-sandbox/kn-plugin-func/commit/895872aee76b44be739bd0eafb9f2cdcdc137494))


### Code Refactoring

* change envVars to env in func.yaml ([#316](https://github.com/knative-sandbox/kn-plugin-func/issues/316)) ([89ff286](https://github.com/knative-sandbox/kn-plugin-func/commit/89ff286a1f3afae655a2c724a05cb3bc3c281786))

### [0.12.1](https://github.com/knative-sandbox/kn-plugin-func/compare/v0.12.0...v0.12.1) (2021-04-14)


### Bug Fixes

* build needs to use legacy jar ([129dc5a](https://github.com/knative-sandbox/kn-plugin-func/commit/129dc5a8348dc8e4e14f5891871cf6b50ae35ccc))

## [0.12.0](https://github.com/knative-sandbox/kn-plugin-func/compare/v0.11.0...v0.12.0) (2021-03-30)


### Features

* add --build (default: true) flag to func deploy ([8a91cac](https://github.com/knative-sandbox/kn-plugin-func/commit/8a91cac6cc78b5cf56d5158f3eb03a4076a34ffe))
* basic lifecycle integraiton tests ([8edd0df](https://github.com/knative-sandbox/kn-plugin-func/commit/8edd0df836055b33473f9a7774e8ae755f46ac2e))
* integration tests target ([ddf4ab8](https://github.com/knative-sandbox/kn-plugin-func/commit/ddf4ab86c46912f78e56a52a14efcf89fd187103))
* local cluster allocation, configuration and teardown ([9c499b6](https://github.com/knative-sandbox/kn-plugin-func/commit/9c499b69c4991b86e51127081cee7fb0fc34d554))
* using custom docker daemon (e.g podman) ([6d2d8c6](https://github.com/knative-sandbox/kn-plugin-func/commit/6d2d8c63b01e12f6cf277c2cd18c3f7298ce86ab))


### Bug Fixes

* `func deploy` uses Docker API, not binary ([dc2fbee](https://github.com/knative-sandbox/kn-plugin-func/commit/dc2fbee67f7f2304bece83a9b4d4f051ed19cd61))
* `func run` now uses Docker API, not binary ([db0945e](https://github.com/knative-sandbox/kn-plugin-func/commit/db0945ed3ecb9e6e4283a0cb478d39657c6803dc))
* compare service names in integraiton tests ([1551d69](https://github.com/knative-sandbox/kn-plugin-func/commit/1551d69b5d287becaafdf3d5b99a6ba8da926fa6))
* exposed port ([7ed2e86](https://github.com/knative-sandbox/kn-plugin-func/commit/7ed2e86d9672f285c1def490a3d325ceb9e8471f))
* increase remove timeout to 120s ([80e366b](https://github.com/knative-sandbox/kn-plugin-func/commit/80e366b14234c184932d91db4188bdabb0742e7a))
* sprint-boot template ([38fd673](https://github.com/knative-sandbox/kn-plugin-func/commit/38fd673fdbef1094558b32910a42fcdff2d8bb0c))
* update pack dependency ([c3c2165](https://github.com/knative-sandbox/kn-plugin-func/commit/c3c21657b2bc3cba9e4ba87864d8fe0c5d4e43af))

## [0.11.0](https://github.com/knative-sandbox/kn-plugin-func/compare/v0.10.0...v0.11.0) (2021-01-21)


### Features

* add --all-namespaces flag to `func list` ([#242](https://github.com/knative-sandbox/kn-plugin-func/issues/242)) ([8e72fd2](https://github.com/knative-sandbox/kn-plugin-func/commit/8e72fd2eba9f4e6e5d3a0bd366215025ba1d1004))


### Bug Fixes

* change --format flag to --output for list and describe commands ([#248](https://github.com/knative-sandbox/kn-plugin-func/issues/248)) ([6470d9e](https://github.com/knative-sandbox/kn-plugin-func/commit/6470d9e57462bc8d3a30583cf146d4f466e2d5f7))
* correct fn signatures in Go Events template ([#246](https://github.com/knative-sandbox/kn-plugin-func/issues/246)) ([5502492](https://github.com/knative-sandbox/kn-plugin-func/commit/55024921c26e044f83187cbd5510375d8702c6d9))
* correcting broken merge ([#252](https://github.com/knative-sandbox/kn-plugin-func/issues/252)) ([8d1f5b8](https://github.com/knative-sandbox/kn-plugin-func/commit/8d1f5b833d86fa959e3386db73f7e1b07bdd6dfd))
* fix the help text for the describe function ([#243](https://github.com/knative-sandbox/kn-plugin-func/issues/243)) ([5a3a0d6](https://github.com/knative-sandbox/kn-plugin-func/commit/5a3a0d6bdab4d01292c4c8f6011a3b67cadb8ef6))
* print "No functions found in [ns] namespace" for kn func list ([#240](https://github.com/knative-sandbox/kn-plugin-func/issues/240)) ([61ea8d4](https://github.com/knative-sandbox/kn-plugin-func/commit/61ea8d4fc6e841f0f10151244f10131862bf181c))
* set envVars when creating a function ([#250](https://github.com/knative-sandbox/kn-plugin-func/issues/250)) ([f0be048](https://github.com/knative-sandbox/kn-plugin-func/commit/f0be048c841be22fcd0d448fdecc0da33b8c77be))

## [0.10.0](https://github.com/knative-sandbox/kn-plugin-func/compare/v0.9.0...v0.10.0) (2020-12-08)


### Features

* add spring cloud function runtime and templates ([#231](https://github.com/knative-sandbox/kn-plugin-func/issues/231)) ([557361a](https://github.com/knative-sandbox/kn-plugin-func/commit/557361a37446953dc613ae30f59913f1924dedd3))


### Bug Fixes

* Fix plugin version output ([#233](https://github.com/knative-sandbox/kn-plugin-func/issues/233)) ([8a30ba1](https://github.com/knative-sandbox/kn-plugin-func/commit/8a30ba193da6097a141332212cbd64e5a1a708e8))
* use image name for run command ([#238](https://github.com/knative-sandbox/kn-plugin-func/issues/238)) ([985906b](https://github.com/knative-sandbox/kn-plugin-func/commit/985906b0e1f692f94fc84e3e796893192d17bd4c))

## [0.9.0](https://github.com/knative-sandbox/kn-plugin-func/compare/v0.8.0...v0.9.0) (2020-11-06)


### ⚠ BREAKING CHANGES

* rename faas to function (#210)
* remove create cli subcommand (#180)

### Features

* Better output of build/deploy/delete commands ([#206](https://github.com/knative-sandbox/kn-plugin-func/issues/206)) ([ddbb95b](https://github.com/knative-sandbox/kn-plugin-func/commit/ddbb95b075a383fb1847be2c75fd2c216870c7f8))
* change default runtime to Node.js HTTP ([#198](https://github.com/knative-sandbox/kn-plugin-func/issues/198)) ([61cb56a](https://github.com/knative-sandbox/kn-plugin-func/commit/61cb56aec3461e9f9b35282435dbc884999be2b3))
* list command - improved output ([#205](https://github.com/knative-sandbox/kn-plugin-func/issues/205)) ([29ca077](https://github.com/knative-sandbox/kn-plugin-func/commit/29ca07768ca455debb7992ebbf09b2db2058f56d))
* remove create cli subcommand ([#180](https://github.com/knative-sandbox/kn-plugin-func/issues/180)) ([57e1236](https://github.com/knative-sandbox/kn-plugin-func/commit/57e12362af18f48624a9c303c070846e1645e08d))
* rename faas to function ([#210](https://github.com/knative-sandbox/kn-plugin-func/issues/210)) ([cd57692](https://github.com/knative-sandbox/kn-plugin-func/commit/cd57692c9d04fecb918abf4f15cd37d45592cf82))


### Bug Fixes

* `delete` and `deploy sub-commands respects func.yaml conf ([d562498](https://github.com/knative-sandbox/kn-plugin-func/commit/d5624980d5f31f98bc27e803ae94311491d4d078))
* return JSON in Node.js event template ([#211](https://github.com/knative-sandbox/kn-plugin-func/issues/211)) ([beb838f](https://github.com/knative-sandbox/kn-plugin-func/commit/beb838ff43d04c7ccec63a26fbe2af9fb167ae1a))

## [0.8.0](https://github.com/knative-sandbox/kn-plugin-func/compare/v0.7.0...v0.8.0) (2020-10-20)


### ⚠ BREAKING CHANGES

* change all references of "repository" to "registry" for images (#156)
* combine deploy and update commands (#152)

### Features

* add health probes to node & go services ([#174](https://github.com/knative-sandbox/kn-plugin-func/issues/174)) ([95c1eb5](https://github.com/knative-sandbox/kn-plugin-func/commit/95c1eb5e59335cfee84ce536d086bd394268c81c))
* introduce CloudEvent data as first parameter for event functions ([#172](https://github.com/knative-sandbox/kn-plugin-func/issues/172)) ([7451194](https://github.com/knative-sandbox/kn-plugin-func/commit/74511948cefc368d898ad05b911fded74d44b759))
* user can set envvars ([5182487](https://github.com/knative-sandbox/kn-plugin-func/commit/5182487df218685867fda10c3d1983b4c035c08a))
* **kn:** Enable faas to be integrated as plugin to kn ([#155](https://github.com/knative-sandbox/kn-plugin-func/issues/155)) ([85a5f47](https://github.com/knative-sandbox/kn-plugin-func/commit/85a5f475eb32269b9cced05fe36dc90f8befd000))
* ability for users to specify custom builders ([#147](https://github.com/knative-sandbox/kn-plugin-func/issues/147)) ([c2b4a30](https://github.com/knative-sandbox/kn-plugin-func/commit/c2b4a304bd3fa7d020c71db9f4d79c80c98d86d3))
* combine deploy and update commands ([#152](https://github.com/knative-sandbox/kn-plugin-func/issues/152)) ([d5839ea](https://github.com/knative-sandbox/kn-plugin-func/commit/d5839ea6c1e84e843ad643cc0611a82e2e6d2399))
* fish completion ([d822303](https://github.com/knative-sandbox/kn-plugin-func/commit/d82230353d3d437e8f35e7f9ce3569988d765b42))


### Bug Fixes

* examples in readme ([5591e7f](https://github.com/knative-sandbox/kn-plugin-func/commit/5591e7fa2ca9584f03bf8d065778cd120ea9054f))
* image parsing ([6a621a5](https://github.com/knative-sandbox/kn-plugin-func/commit/6a621a5186ffffec79e6f34c34681cc37eeaa0bd))
* regenerate pkger files ([#183](https://github.com/knative-sandbox/kn-plugin-func/issues/183)) ([1d14a8c](https://github.com/knative-sandbox/kn-plugin-func/commit/1d14a8c10156098d66ef691f84ecce1bd25a6d88))
* root cmd init ([ec5327d](https://github.com/knative-sandbox/kn-plugin-func/commit/ec5327d5201b57d6a33bcc7314332686582b676f))
* stop using manually edited completion ([bf9b048](https://github.com/knative-sandbox/kn-plugin-func/commit/bf9b04881333fed6038251fa4de92368771840d9))
* update quarkus templates ([ffc6a12](https://github.com/knative-sandbox/kn-plugin-func/commit/ffc6a123e469968865fef1ccb5f8d84a443baccb))
* update to Knative 0.17 ([#145](https://github.com/knative-sandbox/kn-plugin-func/issues/145)) ([5fe7052](https://github.com/knative-sandbox/kn-plugin-func/commit/5fe70526e531e283c6704d9526e3cdd7ef64f9e1))


### src

* change all references of "repository" to "registry" for images ([#156](https://github.com/knative-sandbox/kn-plugin-func/issues/156)) ([e425c8f](https://github.com/knative-sandbox/kn-plugin-func/commit/e425c8f08183b333e56d5d3cfe74fc9e85a6c903))

## [0.7.0](https://github.com/knative-sandbox/kn-plugin-func/compare/v0.6.2...v0.7.0) (2020-09-24)


### Features

* add local debugging to node.js templates ([#132](https://github.com/knative-sandbox/kn-plugin-func/issues/132)) ([1b0bb15](https://github.com/knative-sandbox/kn-plugin-func/commit/1b0bb15147889bb55ff33de1dc132cb0370d1da6))
* decouple function name from function domain ([#127](https://github.com/knative-sandbox/kn-plugin-func/issues/127)) ([0258626](https://github.com/knative-sandbox/kn-plugin-func/commit/025862689ec8dc460a1ef6f4402151c18a072ba3))
* default to no confirmation prompts for CLI commands ([566d8f9](https://github.com/knative-sandbox/kn-plugin-func/commit/566d8f9255d532e88e72d5bce122bebaee88bc81))
* set builder images in templates and .faas.yaml ([#136](https://github.com/knative-sandbox/kn-plugin-func/issues/136)) ([d6e131f](https://github.com/knative-sandbox/kn-plugin-func/commit/d6e131f9153c20bd3edbf1441060610987fa5693))
* **ci/cd:** add release-please for automated release management ([8a60c5e](https://github.com/knative-sandbox/kn-plugin-func/commit/8a60c5e0c44d28d2ff085e56299217e05e408df8))


### Bug Fixes

* correct value for config path and robustify ([#130](https://github.com/knative-sandbox/kn-plugin-func/issues/130)) ([fae27da](https://github.com/knative-sandbox/kn-plugin-func/commit/fae27dabc97c78cd98be400d296da6fc2fbeba65))
* delete command ([284b77f](https://github.com/knative-sandbox/kn-plugin-func/commit/284b77f7ef6524195da958850131190399470375))
* describe works without Eventing ([6c16e65](https://github.com/knative-sandbox/kn-plugin-func/commit/6c16e65d60543458f0b70c010d672cb4d45f6279))
* sync package-lock.json ([#137](https://github.com/knative-sandbox/kn-plugin-func/issues/137)) ([02309a2](https://github.com/knative-sandbox/kn-plugin-func/commit/02309a24a1d8779fb69e4f67fa4f7faea705b2ba))

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
- **actions:** add binary uploads to develop branch CI ([#104](https://github.com/knative-sandbox/kn-plugin-func/issues/104))

### Docs
- initial Go template READMEs

### Fix
- build releases from main branch only
- remove references to unused binaries appsody, kn, kubectl
- image override ([#88](https://github.com/knative-sandbox/kn-plugin-func/issues/88))

### Release
- v0.6.1

### Templates
- **node:** make node templates use npx [@redhat](https://github.com/redhat)/faas-js-runtime ([#99](https://github.com/knative-sandbox/kn-plugin-func/issues/99))


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
- version command respects verbose flag ([#61](https://github.com/knative-sandbox/kn-plugin-func/issues/61))
- add init/build/deploy commands and customizable namespace ([#65](https://github.com/knative-sandbox/kn-plugin-func/issues/65))
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
- add test targets for go and quarkus templates ([#72](https://github.com/knative-sandbox/kn-plugin-func/issues/72))


<a name="v0.5.0"></a>
## [v0.5.0] - 2020-07-31
### Actions
- add CHANGELOG.md and a release target to Makefile ([#45](https://github.com/knative-sandbox/kn-plugin-func/issues/45))

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


[Unreleased]: https://github.com/knative-sandbox/kn-plugin-func/compare/v0.6.2...HEAD
[v0.6.2]: https://github.com/knative-sandbox/kn-plugin-func/compare/v0.6.1...v0.6.2
[v0.6.1]: https://github.com/knative-sandbox/kn-plugin-func/compare/v0.6.0...v0.6.1
[v0.6.0]: https://github.com/knative-sandbox/kn-plugin-func/compare/v0.5.0...v0.6.0
[v0.5.0]: https://github.com/knative-sandbox/kn-plugin-func/compare/v0.4.0...v0.5.0
[v0.4.0]: https://github.com/knative-sandbox/kn-plugin-func/compare/v0.3.0...v0.4.0
[v0.3.0]: https://github.com/knative-sandbox/kn-plugin-func/compare/v0.2.2...v0.3.0
[v0.2.2]: https://github.com/knative-sandbox/kn-plugin-func/compare/v0.2.1...v0.2.2
[v0.2.1]: https://github.com/knative-sandbox/kn-plugin-func/compare/v0.2.0...v0.2.1
[v0.2.0]: https://github.com/knative-sandbox/kn-plugin-func/compare/v0.1.0...v0.2.0
[v0.1.0]: https://github.com/knative-sandbox/kn-plugin-func/compare/v0.0.19...v0.1.0
[v0.0.19]: https://github.com/knative-sandbox/kn-plugin-func/compare/v0.0.18...v0.0.19
[v0.0.18]: https://github.com/knative-sandbox/kn-plugin-func/compare/v0.0.17...v0.0.18
[v0.0.17]: https://github.com/knative-sandbox/kn-plugin-func/compare/v0.0.16...v0.0.17
