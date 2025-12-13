# PR Slicing Strategy for GitHub Actions Workflow Feature

## Current Branch Analysis

**Branch:** `issue-744-generate-github-actions-workflow`
**Feature:** `func config ci` command to generate GitHub Actions workflows

### Merge Status

✅ **PR1: Fix Typos (#3257)** - Merged & Rebased
✅ **PR2: Extract Common Interfaces (#3262)** - Merged & Rebased
✅ **PR3: Command Skeleton (#3266)** - Merged (NOT yet rebased into current branch)
⏭️ **Next: PR4 - Basic Workflow Generation**

### Changes Summary

The branch contains:

1. **Infrastructure Changes:**
   - `cmd/common/` - New package with `FunctionLoaderSaver` interfaces and `MockLoaderSaver`
   - `cmd/testing/` - New test factory helpers
   - Modified `cmd/config.go` - Refactored to use common interfaces
   - Modified `cmd/root.go` - Uses common.DefaultLoaderSaver

2. **Feature Implementation:**
   - `cmd/ci/config.go` - Viper-based CI configuration (~142 lines)
   - `cmd/ci/workflow.go` - Workflow YAML generation (~193 lines)
   - `cmd/ci/workflow_test.go` - Unit tests (~22 lines)
   - `cmd/ci/writer.go` - WorkflowWriter interface for I/O decoupling (~45 lines)
   - `cmd/config_ci.go` - Main command implementation (~127 lines)
   - `cmd/config_ci_test.go` - Unit & integration tests (~373 lines, 14 test cases)

3. **Feature Gate:**
   - Protected by `FUNC_ENABLE_CI_CONFIG=true` environment variable
   - Feature flag checked at command registration (not runtime)
   - Safe to merge without affecting existing users

### Dependencies

```text
cmd/common (foundation + MockLoaderSaver)
    ↓
cmd/config.go (refactored) + cmd/root.go
    ↓
cmd/testing (test helpers)
    ↓
cmd/ci/config.go (viper-based configuration)
    ↓
cmd/ci/workflow.go (depends on config.go)
    ↓
cmd/ci/writer.go (WorkflowWriter interface)
    ↓
cmd/config_ci.go (integrates everything)
    ↓
cmd/config_ci_test.go (unit + integration tests)
```

## Recommended Approach: Follow Test Order (6 PRs)

**Rationale:** The test progression naturally builds functionality incrementally. Each PR delivers working, testable end-to-end behavior.

### ✅ PR1: Fix Typos (#3257)

**Status:** Merged & Rebased

**~10 lines**

**Files:** Various files with typo fixes

**Value:** Clean up documentation and code comments.

---

### ✅ PR2: Extract Common Interfaces (#3262)

**Status:** Merged & Rebased

**~150 lines**

**Files:**

- NEW: `cmd/common/common.go` - FunctionLoaderSaver interfaces + MockLoaderSaver
- NEW: `cmd/common/common_test.go` - Interface tests
- MODIFY: `cmd/config.go` - Use common.FunctionLoaderSaver
- MODIFY: `cmd/root.go` - Use common.DefaultLoaderSaver

**Value:** Clean refactoring that reduces duplication and provides test mocks.

---

### ✅ PR3: Command Skeleton (#3266)

**Status:** Merged (NOT yet rebased into current branch)

**~256 lines**

**Files:**

- NEW: `cmd/config_ci.go` - Basic command with feature flag
- NEW: `cmd/ci/config.go` - Minimal configuration constants
- MODIFY: `cmd/config.go` - Wire in NewConfigCICmd()
- NEW: `cmd/config_ci_test.go` - First 3 tests
- NEW: `cmd/testing/factory.go` - Test factory helpers
- NEW: `docs/reference/func_config_ci.md` - Documentation

**Implementation:**

- Feature flag check at command registration (`FUNC_ENABLE_CI_CONFIG`)
- Function initialization check
- Returns "not implemented" placeholder

**Tests:**

1. `RequiresFeatureFlag`
2. `CISubcommandExist`
3. `FailsWhenNotInitialized`

**Value:** Command exists and validates inputs. Feature flag protects users.

---

### ⏭️ PR4: Basic Workflow Generation

**Status:** Next to implement

**~350 lines**

**Files:**

- NEW: `cmd/ci/writer.go` - WorkflowWriter interface for I/O decoupling (~45 lines)
- NEW: `cmd/ci/workflow.go` - Core workflow generation with private types (~120 lines)
- EXPAND: `cmd/ci/config.go` - Add viper-based configuration (~80 lines)
- MODIFY: `cmd/config_ci.go` - Implement workflow generation (~70 lines)
- EXPAND: `cmd/config_ci_test.go` - Add unit tests with mocked I/O (~100 lines)

**Architecture:**

- **WorkflowWriter Interface:** Decouples workflow generation from filesystem
  - `fileWriter` for production (writes to disk)
  - `bufferWriter` for unit tests (writes to memory)
- **Private Types:** `githubWorkflow`, `job`, `step` (encapsulation)
- **Viper Configuration:** Read flags directly via viper (no builder pattern)

**Implementation:**

- Simple workflow: checkout → setup K8s → install func → deploy
- K8s context setup using `kubeconfig` secret
- Default values: branch=main, runner=ubuntu-latest
- YAML generation with proper indentation
- Feature flag enforced at command registration

**Tests (Unit tests with mocked I/O):**

1. `WritesWorkflowFile` - Verifies YAML is written via bufferWriter
2. `WorkflowYAMLHasCorrectStructure` - Validates YAML content structure

**Tests (Integration tests with real filesystem):**

1. `SuccessWhenInitialized` - End-to-end with real function directory
2. `CreatesGitHubWorkflowDirectory` - Verifies `.github/workflows/` created
3. `WritesWorkflowFileToFSWithCorrectYAMLStructure` - Full filesystem integration

**Value:** Clean architecture with testable I/O. Generates functional GitHub Actions workflow.

**Merge Safety:** Safe - feature flagged, well-tested with both unit and integration tests

---

### PR5: Configuration Flags & Registry Auth

**~200 lines**

**Files:**

- EXPAND: `cmd/ci/config.go` - Add all configuration flag constants and getters (~60 lines)
- EXPAND: `cmd/ci/workflow.go` - Add registry login step logic (~40 lines)
- EXPAND: `cmd/config_ci.go` - Wire all flags using viper (~50 lines)
- EXPAND: `cmd/config_ci_test.go` - Add customization tests (~50 lines)

**Implementation:**

- **Secrets vs Variables distinction:**
  - Secrets: `kubeconfig`, `registry-pass` (via `${{ secrets.X }}`)
  - Variables: `registry-login-url`, `registry-user`, `registry-url` (via `${{ vars.X }}`)
- **Configuration flags (all customizable):**
  - `--branch` (default: main)
  - `--workflow-name` (default: "Func Deploy")
  - `--kubeconfig-secret-name` (default: KUBECONFIG)
  - `--registry-login-url-variable-name` (default: REGISTRY_LOGIN_URL)
  - `--registry-user-variable-name` (default: REGISTRY_USERNAME)
  - `--registry-pass-secret-name` (default: REGISTRY_PASSWORD)
  - `--registry-url-variable-name` (default: REGISTRY_URL)
  - `--use-registry-login` (default: true)
  - `--self-hosted-runner` (default: false)
- **Conditional registry login:** Only adds Docker login step if `--use-registry-login=true`
- **Registry URL logic:**
  - With login: `${{ vars.REGISTRY_LOGIN_URL }}/${{ vars.REGISTRY_USERNAME }}`
  - Without login: `${{ vars.REGISTRY_URL }}`

**Tests:**

1. `WorkflowYAMLHasCustomValues` - Validates custom flag values in YAML
2. `WorkflowHasNoRegistryLogin` - Verifies login step removed when disabled

**Value:** Production-ready configuration with proper GitHub Actions secrets/vars.

**Merge Safety:** Safe - backward compatible defaults, all features configurable

---

### PR6: Advanced Features (Remote Build & Debug)

**~100 lines**

**Files:**

- EXPAND: `cmd/ci/config.go` - Add UseRemoteBuild, UseDebug flags (~20 lines)
- EXPAND: `cmd/ci/workflow.go` - Remote build & debug mode logic (~50 lines)
- EXPAND: `cmd/config_ci.go` - Wire `--remote` and `--debug` flags (~15 lines)
- EXPAND: `cmd/config_ci_test.go` - Add advanced feature tests (~15 lines)

**Implementation:**

- **`--remote` flag (default: false):**
  - Changes deploy command: `func deploy` → `func deploy --remote`
  - Changes workflow name: "Func Deploy" → "Remote Func Deploy"
- **`--debug` flag (default: false, hidden from docs):**
  - Adds `workflow_dispatch` trigger (allows manual workflow runs)
  - Adds func CLI caching step (speeds up iterations):
    - ID: `func-cli-cache`
    - Cache key: `func-cli-knative-v1.19.1`
    - Cached path: `func` binary
  - Conditional step execution using `if: ${{ steps.func-cli-cache.outputs.cache-hit != 'true' }}`
  - Adds func to GITHUB_PATH for cached binary

**Tests:**

1. `RemoteBuildAndDeployWorkflow` - Verifies `--remote` flag behavior
2. `HasWorkflowDispatchAndCacheInDebugMode` - Validates debug mode features

**Value:** Complete feature with remote builds and fast debug iterations.

**Merge Safety:** Safe - opt-in flags, debug flag hidden from users

---

## Summary

Each PR:

- ✅ Delivers working, testable functionality
- ✅ Easy to review (follows natural test progression)
- ✅ Can be merged independently
- ✅ Feature flagged (no risk to existing users)
- ✅ Includes comprehensive tests

**PR Breakdown:**

| PR | Lines | Status |
|----|-------|--------|
| PR1: Fix Typos | ~10 | ✅ Merged & Rebased |
| PR2: Common Interfaces | ~150 | ✅ Merged & Rebased |
| PR3: Command Skeleton | ~256 | ✅ Merged (needs rebase) |
| PR4: Basic Workflow | ~350 | ⏭️ Next |
| PR5: Config Flags | ~200 | Pending |
| PR6: Remote/Debug | ~100 | Pending |
| **Total** | **~1,066** | **3/6 merged** |

**Current Implementation:** ~929 lines across all files

---

## Execution Strategy with Git Worktrees

### Initial Setup

Create worktrees for each PR in a dedicated subdirectory:

```bash
# From your main repository (stay on issue-744-generate-github-actions-workflow branch)
cd /home/sjakusch/Dev/active/knative/forks/knative-func

# Create worktrees directory structure
mkdir -p ../knative-func-worktrees/issue-744

# Create worktrees for each PR (all branching from main)
git worktree add ../knative-func-worktrees/issue-744/pr-1 -b issue-744-fix-typos main
git worktree add ../knative-func-worktrees/issue-744/pr-2 -b issue-744-extract-common-interfaces main
git worktree add ../knative-func-worktrees/issue-744/pr-3 -b issue-744-command-skeleton main
git worktree add ../knative-func-worktrees/issue-744/pr-4 -b issue-744-basic-workflow main
git worktree add ../knative-func-worktrees/issue-744/pr-5 -b issue-744-config-flags main
git worktree add ../knative-func-worktrees/issue-744/pr-6 -b issue-744-advanced-features main
```

### Directory Structure

```
knative-func/                              # Original repo (on issue-744-generate-github-actions-workflow)
knative-func-worktrees/
  └── issue-744/
      ├── pr-1/                            # issue-744-fix-typos
      ├── pr-2/                            # issue-744-extract-common-interfaces
      ├── pr-3/                            # issue-744-command-skeleton
      ├── pr-4/                            # issue-744-basic-workflow
      ├── pr-5/                            # issue-744-config-flags
      └── pr-6/                            # issue-744-advanced-features
```

### Workflow for Each PR

Open original repo in one IDE window (as reference) and PR worktree in another:

```bash
# Window 1: Source (read-only reference)
code /home/sjakusch/Dev/active/knative/forks/knative-func

# Window 2: Target (active editing)
code ../knative-func-worktrees/issue-744/pr-1
```

#### PR1: Fix Typos

```bash
cd ../knative-func-worktrees/issue-744/pr-1

# Cherry-pick the typo fix commit
git cherry-pick <commit-hash-for-typos>

# Or if you know the commit hash from the feature branch
git log issue-744-generate-github-actions-workflow --oneline | grep typo

# Run tests
go test ./...

# Push
git push -u origin issue-744-fix-typos
```

#### PR2: Extract Common Interfaces

```bash
cd ../knative-func-worktrees/issue-744/pr-2

# Cherry-pick specific files from feature branch
git checkout issue-744-generate-github-actions-workflow -- cmd/common/
git checkout issue-744-generate-github-actions-workflow -- cmd/config.go
git checkout issue-744-generate-github-actions-workflow -- cmd/root.go

# Edit cmd/config.go to remove CI command wiring (not needed yet)
# Open in VSCode and remove the line: cmd.AddCommand(NewConfigCICmd(loadSaver))

# Run tests
go test ./cmd/common/... ./cmd/config_test.go -v

# Commit
git add .
git commit -m "refactor: extract common function loader interfaces"

# Push
git push -u origin issue-744-extract-common-interfaces
```

#### PR3: Command Skeleton

```bash
cd ../knative-func-worktrees/issue-744/pr-3

# First, merge or rebase on top of PR2 (after PR2 is merged to main)
# Or manually cherry-pick PR2 changes if working in parallel

# Cherry-pick files
git checkout issue-744-generate-github-actions-workflow -- cmd/config_ci.go
git checkout issue-744-generate-github-actions-workflow -- cmd/ci/config.go
git checkout issue-744-generate-github-actions-workflow -- cmd/config.go

# Edit config_ci.go to return "not implemented" placeholder
# Create minimal cmd/ci/config.go with just constants
# Add first 3 tests to cmd/config_ci_test.go (create file)

# Run tests with feature flag
FUNC_ENABLE_CI_CONFIG=true go test ./cmd/config_ci_test.go -v

# Commit and push
git add .
git commit -m "feat: add config ci command skeleton with feature flag"
git push -u origin issue-744-command-skeleton
```

#### PR4: Basic Workflow Generation

```bash
cd ../knative-func-worktrees/issue-744/pr-4

# Rebase on latest main (PR3 should be merged by now)
git fetch origin
git rebase origin/main

# Cherry-pick files from feature branch
git checkout issue-744-generate-github-actions-workflow -- cmd/ci/writer.go
git checkout issue-744-generate-github-actions-workflow -- cmd/ci/workflow.go
git checkout issue-744-generate-github-actions-workflow -- cmd/ci/config.go
git checkout issue-744-generate-github-actions-workflow -- cmd/config_ci.go
git checkout issue-744-generate-github-actions-workflow -- cmd/config_ci_test.go

# Edit config_ci_test.go to keep only:
# - WritesWorkflowFile
# - WorkflowYAMLHasCorrectStructure
# - SuccessWhenInitialized
# - CreatesGitHubWorkflowDirectory
# - WritesWorkflowFileToFSWithCorrectYAMLStructure
# Remove all tests related to custom flags, registry login, remote build, debug mode

# Edit ci/workflow.go to remove:
# - Registry login logic
# - Remote build logic
# - Debug mode logic
# Keep only basic workflow with checkout, K8s setup, func install, deploy

# Edit ci/config.go to keep only:
# - Basic flags: PathFlag, BranchFlag, WorkflowNameFlag, KubeconfigSecretNameFlag
# - Remove all registry-related flags, remote, debug, self-hosted-runner

# Run tests
FUNC_ENABLE_CI_CONFIG=true go test ./cmd/... -v

# Commit and push
git add .
git commit -m "feat: implement basic GitHub Actions workflow generation with WorkflowWriter interface"
git push -u origin issue-744-basic-workflow
```

#### PR5: Configuration Flags

```bash
cd ../knative-func-worktrees/issue-744/pr-5

# Expand existing files with registry auth and configuration
git checkout issue-744-generate-github-actions-workflow -- cmd/ci/config.go
git checkout issue-744-generate-github-actions-workflow -- cmd/ci/workflow.go
git checkout issue-744-generate-github-actions-workflow -- cmd/config_ci.go
git checkout issue-744-generate-github-actions-workflow -- cmd/config_ci_test.go

# Keep tests 1-9 in config_ci_test.go

# Run tests
FUNC_ENABLE_CI_CONFIG=true go test ./cmd/... -v

# Commit and push
git add .
git commit -m "feat: add configuration flags and registry authentication"
git push -u origin issue-744-config-flags
```

#### PR6: Advanced Features

```bash
cd ../knative-func-worktrees/issue-744/pr-6

# Cherry-pick final changes
git checkout issue-744-generate-github-actions-workflow -- cmd/ci/
git checkout issue-744-generate-github-actions-workflow -- cmd/config_ci.go
git checkout issue-744-generate-github-actions-workflow -- cmd/config_ci_test.go

# All tests should pass
FUNC_ENABLE_CI_CONFIG=true go test ./cmd/... -v

# Commit and push
git add .
git commit -m "feat: add remote build and debug mode support"
git push -u origin issue-744-advanced-features
```

### Managing Worktrees

**List all worktrees:**

```bash
git worktree list
```

**Remove worktree when done:**

```bash
git worktree remove ../knative-func-worktrees/issue-744/pr-1
```

**Cleanup after all PRs are merged:**

```bash
# Remove all worktrees
rm -rf ../knative-func-worktrees/issue-744/
git worktree prune
```

---

## Implementation Notes

### PR3 Placeholder Implementation

In `runConfigCIGitHub()`, return early with:

```go
return fmt.Errorf("workflow generation not implemented yet")
```

This allows tests 1-3 to pass while deferring actual generation to PR4.

### Testing Strategy

Each PR must:

1. Pass all new tests added in that PR
2. Not break any existing tests
3. Pass with feature flag enabled: `FUNC_ENABLE_CI_CONFIG=true go test ./cmd/...`

### PR Dependencies

- PR1 (typos) - Independent, can be opened in parallel
- PR2 (refactoring) - Independent, can be opened in parallel with PR1
- PR3 depends on PR2 (needs `cmd/common`)
- PR4 depends on PR3 (needs command structure)
- PR5 depends on PR4 (expands existing workflow)
- PR6 depends on PR5 (expands existing workflow)

**Recommendation:** Open PR1 and PR2 in parallel, then merge sequentially before opening subsequent PRs.
