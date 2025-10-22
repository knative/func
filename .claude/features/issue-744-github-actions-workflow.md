# Issue 744: GitHub Actions Workflow Generation

## Feature Summary

Add `func config ci --github` command to generate GitHub Actions workflow files for function deployment.

**Workflow contains:**

- Checkout code
- Setup func CLI (using knative-func-action)
- Optional test step (language-specific)
- Deploy using `func deploy` (local) or `func deploy --remote` (remote)

**Build modes:**

- Local build (default): Builds in GitHub runner, deploys to cluster
- Remote build (--remote): Build and deploy on-cluster

## Acceptance Criteria

1. **Basic Command Functionality**
   - ✅ Creates `.github/workflows/` directory with valid workflow YAML
   - ✅ Fails with error if workflow already exists (no overwrite)
   - ✅ Fails fast if not in initialized function directory

2. **Local Build Mode (Default)**
   - ✅ Generates workflow with: checkout → setup func → test → deploy
   - ✅ Triggers on push to `main` branch (default)
   - ✅ Runs on `ubuntu-latest`
   - ✅ File named `build-local-deploy-remote.yaml`

3. **Remote Build Mode**
   - ✅ `--remote` flag generates `build-and-deploy-remote.yaml`
   - ✅ Uses `func deploy --remote`
   - ✅ Includes cluster auth configuration (defaults initially)

4. **Configuration Options**
   - ✅ `--branch <name>` sets trigger branch
   - ✅ Triggers on `push` events only (not pull_request yet)
   - ✅ Sensible defaults when flags not specified

5. **Runtime Support**
   - ✅ Go functions: includes test step
   - ⏳ Python functions: future iteration
   - ❌ Other runtimes: not in scope initially

## Key Decisions

- **Workflow naming:** `build-local-deploy-remote.yaml` and `build-and-deploy-remote.yaml`
- **Default branch:** `main`
- **Event triggers:** `push` first, `pull_request` later, workflow_dispatch later
- **Runtime priority:** Go → Python → others
- **Test step:** Included for supported runtimes
- **Cluster configuration approach:**
  1. Use defaults first
  2. Add option flags
  3. Read from existing function config
  4. Interactive prompts (last)
- **Multiple workflows:** Support both local and remote (different clusters possible)

## Implementation Phases

### Phase 1: Test Infrastructure & Basic Command Structure

#### Step 1.1: Command skeleton

Test Cases:

- `TestNewConfigCICmd_CommandExists` - Command wired up correctly
- `TestNewConfigCICmd_FailsWhenNotInitialized` - Fail when not in function dir
- `TestConfigCI_RequiresGitHubFlag` - --github flag required initially

Implementation:

- Basic command structure with --github flag
- Function initialization check using functionLoader
- Error handling and fail fast
- Wire into config.go

Refactor:

- Extract common patterns
- Consistent error messaging

---

### Phase 2: Workflow File Generation - Local Build

#### Step 2.1: Directory and file creation

Test Cases:

- `TestConfigCI_GitHub_CreatesWorkflowDirectory` - Creates .github/workflows/
- `TestConfigCI_GitHub_GeneratesLocalWorkflowFile` - Creates build-local-deploy-remote.yaml
- `TestConfigCI_GitHub_LocalWorkflow_HasCorrectStructure` - Valid YAML structure

Implementation:

- Create workflow template (embedded or separate package)
- Directory creation logic
- File writing logic
- Basic YAML: checkout → setup func → deploy

Refactor:

- Extract template rendering
- Create workflow config struct

#### Step 2.2: Go-specific workflow content

Test Cases:

- `TestConfigCI_GitHub_GoFunction_IncludesTestStep` - Test step for Go
- `TestConfigCI_GitHub_DefaultTrigger_PushToMain` - Triggers on push to main
- `TestConfigCI_GitHub_UsesUbuntuRunner` - Runner is ubuntu-latest
- `TestConfigCI_GitHub_IncludesClusterConfig` - Cluster config placeholders

Implementation:

- Detect function runtime from Function struct
- Add conditional test step for Go
- Set default trigger: push on main
- Add cluster config (env vars/secrets placeholders)

**Refactor:**

- Runtime-specific customization logic
- Maintainable/extensible template

---

### Phase 3: Remote Build Support

#### Step 3.1: Remote build flag

Test Cases:

- `TestConfigCI_GitHub_Remote_GeneratesRemoteWorkflowFile` - Creates deploy-remote.yaml
- `TestConfigCI_GitHub_Remote_UsesRemoteDeployCommand` - Uses func deploy --remote
- `TestConfigCI_GitHub_Remote_IncludesAuthConfig` - Cluster auth config present

Implementation:

- Add --remote flag
- Template variation for remote builds
- Generate deploy-remote.yaml when --remote set
- Remote-specific cluster auth setup

**Refactor:**

- Consolidate local/remote template logic
- Use conditionals or separate templates

---

### Phase 4: Configuration Options

#### Step 4.1: Branch configuration

Test Cases:

- `TestConfigCI_GitHub_CustomBranch_SetsTrigger` - --branch flag sets trigger branch
- `TestConfigCI_GitHub_DefaultBranch_IsMain` - Default is main

Implementation:

- Add --branch flag
- Use flag value in template
- Default to "main"

**Refactor:**

- Create configuration struct for all workflow options

---

### Phase 5: Collision Detection & Error Handling

#### Step 5.1: Existing workflow detection

Test Cases:

- `TestConfigCI_GitHub_Local_FailsWhenFileExists` - Fails if deploy-local.yaml exists
- `TestConfigCI_GitHub_Remote_FailsWhenFileExists` - Fails if build-and-deploy-remote.yaml exists
- `TestConfigCI_GitHub_ExistingWorkflow_ShowsHelpfulError` - Helpful error message

Implementation:

- Check file existence before generation
- Return descriptive error on collision
- Suggest alternatives to user

**Refactor:**

- Extract file existence checking
- Improve error messaging

---

## Current Status

### ✅ Completed

#### Phase 1: Test Infrastructure & Basic Command Structure

- Created `cmd/common` package for reusable loader/saver interfaces
- Created `cmd/testing` factory with `CreateFuncInTempDir()` helper
- Created [cmd/config_ci.go](cmd/config_ci.go) with comprehensive flag support
- Created [cmd/config_ci_test.go](cmd/config_ci_test.go) with 10 passing tests
- Wired command into `cmd/config.go`

#### Phase 2: Workflow File Generation

- Created YAML structure types: `GitHubWorkflow`, `WorkflowTriggers`, `PushTrigger`, `Job`, `Step`
- Created `cmd/ci` package for CI logic separation
  - [cmd/ci/config.go](cmd/ci/config.go) - CIConfig with builder pattern, flag reading via Cobra
  - [cmd/ci/workflow.go](cmd/ci/workflow.go) - Workflow generation with conditional steps
- Workflow generation features:
  - Checkout → K8s context setup → Registry login (conditional) → func CLI install → Deploy
  - Kubernetes context setup using kubeconfig secret
  - Conditional registry authentication step
  - Conditional debug features (workflow_dispatch + CLI caching)
  - Deploy command varies based on build mode
  - Registry URL format changes based on login mode

#### Phase 3: Remote Build Support

- Implemented `--remote` flag to switch between local/remote builds
- Remote build: `func deploy --remote` (builds on cluster with Tekton)
- Local build (default): `func deploy` (builds in GitHub runner with Docker)
- Workflow name auto-adjusts: "Func Deploy" vs "Remote Func Deploy"

#### Phase 4: Configuration Options

**Implemented flags:**

- `--github` - Enable GitHub Actions workflow generation
- `--workflow-name <name>` - Custom workflow name (default: "Func Deploy")
- `--branch <name>` - Customize trigger branch (default: "main")
- `--remote` - Use remote build on cluster (default: false)
- `--self-hosted-runner` - Use self-hosted runner instead of ubuntu-latest
- `--use-registry-login` - Include docker/login-action step (default: true)
- `--debug` - Add workflow_dispatch trigger + func CLI caching for fast iterations
- `--kubeconfig-secret-name <name>` - Custom kubeconfig secret name (default: "KUBECONFIG")
- `--registry-login-url-variable-name <name>` - Custom registry login URL variable (default: "REGISTRY_LOGIN_URL")
- `--registry-user-variable-name <name>` - Custom registry user variable (default: "REGISTRY_USERNAME")
- `--registry-pass-secret-name <name>` - Custom registry password secret (default: "REGISTRY_PASSWORD")
- `--registry-url-variable-name <name>` - Custom registry URL variable for no-login mode (default: "REGISTRY_URL")

**Registry URL behavior:**

- With `--use-registry-login=true`: `--registry=${{ vars.REGISTRY_LOGIN_URL }}/${{ vars.REGISTRY_USERNAME }}`
- With `--use-registry-login=false`: `--registry=${{ vars.REGISTRY_URL }}`

#### Phase 4 (continued): Feature Flag Support

**Implementation:**

- Added `ConfigCIFeatureFlag` constant in [cmd/config_ci.go:86](cmd/config_ci.go#L86)
- Feature flag check in `runConfigCIGitHub()` (line 92-94)
- Environment variable: `FUNC_ENABLE_CI_CONFIG=true`
- Error message: "Set FUNC_ENABLE_CI_CONFIG to 'true' to use this feature"
- Test helper `defaultOpts()` for common test setup
- Refactored test assertions into helper functions

#### Test Coverage

All 11 tests passing:

- `TestNewConfigCICmd_RequiresFeatureFlag` - **NEW** - Tests feature flag enforcement
- `TestNewConfigCICmd_CISubcommandAndGitHubOptionExist`
- `TestNewConfigCICmd_FailsWhenNotInitialized`
- `TestNewConfigCICmd_SuccessWhenInitialized`
- `TestNewConfigCICmd_CreatesGitHubWorkflowDirectory`
- `TestNewConfigCICmd_GeneratesWorkflowFile`
- `TestNewConfigCICmd_WorkflowYAMLHasCorrectStructure`
- `TestNewConfigCICmd_WorkflowYAMLHasCustomValues` - Tests custom branch, secret/var names, self-hosted
- `TestNewConfigCICmd_WorkflowHasNoRegistryLogin` - Tests `--use-registry-login=false`
- `TestNewConfigCICmd_RemoteBuildAndDeployWorkflow` - Tests `--remote` flag
- `TestNewConfigCICmd_HasWorkflowDispatchAndCacheInDebugMode` - Tests `--debug` flag

#### Recent Commits

- `a231cc9d` - feat: add flags for all configurable parameters
- `4d6cd5d1` - refactor: distinguish secrets from vars in CI
- `fd6bc843` - refactor: move ci config to command flags
- `1465082a` - feat: add local build workflow and debug options

### 🔜 Next Steps

#### Phase 4 (continued): Additional Configuration Options

**Verbose Logging (Do Now):**

- Add progress messages with `-v` flag
- Log: "Generating workflow...", "Writing to file...", "Complete"
- Use existing `func` CLI logging infrastructure
- Test: `TestNewConfigCICmd_VerboseOutput`

**Path Flag Support (After Phase 5):**

- Support `--path` flag to specify function directory
- Use provided path instead of CWD
- Validate path points to initialized function
- Test: `TestNewConfigCICmd_CustomPath`

#### Phase 5: Collision Detection & Error Handling

**Objective:** Prevent accidental overwrites and provide helpful error messages

**Test Cases:**

- `TestNewConfigCICmd_FailsWhenFileExists` - Fails if workflow file already exists
- `TestNewConfigCICmd_ExistingWorkflow_ShowsHelpfulError` - Returns descriptive error with suggestions

**Implementation:**

1. Check if workflow file exists before generation in [cmd/ci/workflow.go](cmd/ci/workflow.go)
2. Return error with suggestions:
   - Delete existing file manually
   - Use different workflow name with `--workflow-name`
   - Generate to different directory
3. Update `Persist()` method to check file existence

**Acceptance:** Command fails gracefully when workflow already exists, preventing data loss

#### Phase 6: Runtime Detection & Test Steps (Later)

**Objective:** Add language-specific test steps to workflow

**Implementation:**

1. Read `Function.Runtime` from function config
2. Add conditional test step based on runtime:
   - Go: `go test ./...`
   - Python: `pytest`
   - Node: `npm test`
3. Skip test step if runtime not supported

**Test Cases:**

- `TestNewConfigCICmd_GoFunction_IncludesTestStep`
- `TestNewConfigCICmd_PythonFunction_IncludesTestStep`
- `TestNewConfigCICmd_UnsupportedRuntime_NoTestStep`

**Acceptance:** Generated workflows include appropriate test commands for supported runtimes

### ⏳ Optional Future Enhancements (Not Required)

- Support for `pull_request` event triggers
- Support for multiple concurrent workflows (local + remote)
- Interactive mode with prompts for missing configuration
- `--force` flag to allow overwriting existing workflows

---

## Resources

- Sample workflow: <https://github.com/functions-dev/templates/blob/main/.github/workflows/invoke-all.yaml>
- Func GitHub Action: <https://github.com/gauron99/knative-func-action>
