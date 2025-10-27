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
   - ✅ File named `deploy-local.yaml`

3. **Remote Build Mode**
   - ✅ `--remote` flag generates `deploy-remote.yaml`
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

- **Workflow naming:** `deploy-local.yaml` and `deploy-remote.yaml`
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

**Step 1.1: Command skeleton**

Test Cases:
- `TestNewConfigCICmd_CommandExists` - Command wired up correctly
- `TestNewConfigCICmd_FailsWhenNotInitialized` - Fail when not in function dir
- `TestConfigCI_RequiresGithubFlag` - --github flag required initially

Implementation:
- Basic command structure with --github flag
- Function initialization check using functionLoader
- Error handling and fail fast
- Wire into config.go

**Refactor:**
- Extract common patterns
- Consistent error messaging

---

### Phase 2: Workflow File Generation - Local Build

**Step 2.1: Directory and file creation**

Test Cases:
- `TestConfigCI_GitHub_CreatesWorkflowDirectory` - Creates .github/workflows/
- `TestConfigCI_GitHub_GeneratesLocalWorkflowFile` - Creates deploy-local.yaml
- `TestConfigCI_GitHub_LocalWorkflow_HasCorrectStructure` - Valid YAML structure

Implementation:
- Create workflow template (embedded or separate package)
- Directory creation logic
- File writing logic
- Basic YAML: checkout → setup func → deploy

**Refactor:**
- Extract template rendering
- Create workflow config struct

**Step 2.2: Go-specific workflow content**

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

**Step 3.1: Remote build flag**

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

**Step 4.1: Branch configuration**

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

**Step 5.1: Existing workflow detection**

Test Cases:
- `TestConfigCI_GitHub_Local_FailsWhenFileExists` - Fails if deploy-local.yaml exists
- `TestConfigCI_GitHub_Remote_FailsWhenFileExists` - Fails if deploy-remote.yaml exists
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

**Phase 1: Test Infrastructure & Basic Command Structure** ✅
- Created `cmd/common` package for reusable loader/saver interfaces
- Created `cmd/testing` factory with `CreateFuncInTempDir()` helper
- Created `cmd/config_ci.go` with basic command structure
- Created `cmd/config_ci_test.go` with `ciOpts` struct pattern
- Wired command into `cmd/config.go:74`
- Tests passing (3/3):
  - `TestNewConfigCICmd_CommandExists`
  - `TestNewConfigCICmd_FailsWhenNotInitialized`
  - `TestNewConfigCICmd_SuccessWhenInitialized`
- Commit: `bd22332f` - feat: add config ci command and refactor interfaces

**Phase 2, Step 2.1: Workflow directory/file creation** ✅
- Created YAML structure types: `GithubWorkflow`, `WorkflowTriggers`, `PushTrigger`, `Job`, `Step`
- Implemented workflow generation with hardcoded values:
  - Name: "Remote Build and Deploy"
  - Trigger: push to main branch
  - Runner: ubuntu-latest
  - Steps: checkout → func cli setup → deploy --remote
- Created `cmd/ci` package for CI logic separation
  - `cmd/ci/config.go` - CIConfig with path resolution methods
  - `cmd/ci/workflow.go` - Workflow structs and generation logic
- Added `NewGithubWorkflow(name)` factory function
- Added `PersistToDisk()` function for file writing
- Tests passing (5/5):
  - `TestNewConfigCICmd_CreatesGithubWorkflowDirectory`
  - `TestNewConfigCICmd_GeneratesLocalWorkflowFile`
  - `TestNewConfigCICmd_WorkflowYAMLHasCorrectStructure`
  - `TestNewConfigCICmd_WorkflowYAMLHasCustomName`
  - Plus 2 existing tests
- Commits:
  - `e4541136` - feat: generate github workflow for remote build
  - `d2c850fc` - refactor: extract ci logic into dedicated package

### 🔄 In Progress

**Phase 2, Step 2.2: Go-specific workflow content**
- Next: Detect runtime and conditionally add test step for Go functions

### ⏳ Next Steps

1. Complete Phase 2, Step 2.2 (4 tests for Go-specific content):
   - Runtime detection from Function struct
   - Conditional test step for Go
   - Validate default triggers
   - Cluster config placeholders
2. Begin Phase 3: Remote build support

---

## Resources

- Sample workflow: https://github.com/functions-dev/templates/blob/main/.github/workflows/invoke-all.yaml
- Func GitHub Action: https://github.com/gauron99/knative-func-action
