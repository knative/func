# PR Slicing Strategy for GitHub Actions Workflow Feature

## Current Branch Analysis

**Branch:** `issue-744-generate-github-actions-workflow`
**Feature:** `func config ci --github` command to generate GitHub Actions workflows

### Changes Summary

The branch contains:

1. **Infrastructure Changes:**
   - `cmd/common/` - New package with extracted `FunctionLoaderSaver` interfaces
   - `cmd/testing/` - New test factory helpers
   - Modified `cmd/config.go` - Refactored to use common interfaces
   - Modified `cmd/root.go` - Uses common.DefaultLoaderSaver

2. **Feature Implementation:**
   - `cmd/ci/config.go` - CI configuration builder (~293 lines)
   - `cmd/ci/workflow.go` - Workflow YAML generation (~233 lines)
   - `cmd/ci/workflow_test.go` - Unit tests
   - `cmd/config_ci.go` - Main command implementation (~129 lines)
   - `cmd/config_ci_test.go` - Integration tests (~296 lines, 10 test cases)

3. **Feature Gate:**
   - Protected by `FUNC_ENABLE_CI_CONFIG=true` environment variable
   - Safe to merge without affecting existing users

### Dependencies

```text
cmd/common (foundation)
    ↓
cmd/config.go (refactored) + cmd/root.go
    ↓
cmd/testing (test helpers)
    ↓
cmd/ci/config.go (standalone)
    ↓
cmd/ci/workflow.go (depends on config.go)
    ↓
cmd/config_ci.go (integrates everything)
    ↓
cmd/config_ci_test.go (tests everything)
```

## Recommended Approach: Follow Test Order (6 PRs)

**Rationale:** The test progression naturally builds functionality incrementally. Each PR delivers working, testable end-to-end behavior.

### PR1: Fix Typos
**~10 lines**

**Files:**
- Various files with typo fixes (already in one commit)

**Value:** Clean up documentation and code comments.

**Merge Safety:** Very low risk - documentation only

**Note:** Can be opened in parallel with PR2

---

### PR2: Refactoring - Extract Common Interfaces
**~150 lines**

**Files:**
- NEW: `cmd/common/common.go` - FunctionLoaderSaver interfaces
- NEW: `cmd/common/common_test.go` - Interface tests
- MODIFY: `cmd/config.go` - Use common.FunctionLoaderSaver
- MODIFY: `cmd/root.go` - Use common.DefaultLoaderSaver

**Tests:**
- `TestStandardLoaderSaver_Load`
- `TestStandardLoaderSaver_Save`

**Value:** Clean refactoring that reduces duplication. No new features, pure improvement to existing code.

**Merge Safety:** Low risk - only refactoring existing functionality

---

### PR3: Command Skeleton with Feature Flag
**~200 lines**

**Files:**
- NEW: `cmd/config_ci.go` - Basic command with feature flag check
- NEW: `cmd/ci/config.go` - Minimal (just GithubOption constant and defaults)
- MODIFY: `cmd/config.go` - Wire in NewConfigCICmd()
- NEW: `cmd/config_ci_test.go` - First 3 tests

**Implementation:**
- Feature flag check (`FUNC_ENABLE_CI_CONFIG`)
- Command registration in config subcommand
- Function initialization check
- Return error: "Not implemented yet" (placeholder)

**Tests Passing:**
1. `TestNewConfigCICmd_RequiresFeatureFlag`
2. `TestNewConfigCICmd_CISubcommandAndGithubOptionExist`
3. `TestNewConfigCICmd_FailsWhenNotInitialized`

**Value:** Command exists and validates inputs. Feature flag protects users.

**Merge Safety:** Very safe - feature flagged, validates but doesn't generate anything yet

---

### PR4: Basic Workflow Generation
**~400 lines**

**Files:**
- NEW: `cmd/testing/factory.go` - Test helpers
- NEW: `cmd/ci/workflow.go` - Core workflow generation
- EXPAND: `cmd/ci/config.go` - Add Branch, WorkflowName configs
- MODIFY: `cmd/config_ci.go` - Implement actual workflow generation
- EXPAND: `cmd/config_ci_test.go` - Add tests 4-7

**Implementation:**
- Simple workflow: checkout → setup K8s context → install func → deploy
- Directory creation (`.github/workflows/`)
- File creation (`func-deploy.yaml`)
- Basic YAML structure
- Default values (branch: main, runner: ubuntu-latest)

**Tests Passing:**
4. `TestNewConfigCICmd_SuccessWhenInitialized`
5. `TestNewConfigCICmd_CreatesGithubWorkflowDirectory`
6. `TestNewConfigCICmd_GeneratesWorkflowFile`
7. `TestNewConfigCICmd_WorkflowYAMLHasCorrectStructure`

**Value:** End-to-end working feature! Generates a basic but functional GitHub Actions workflow.

**Merge Safety:** Safe - generates working workflow files, feature flagged

---

### PR5: Configuration Flags & Registry Auth
**~300 lines**

**Files:**
- EXPAND: `cmd/ci/config.go` - Add all configuration flags and builder methods
- EXPAND: `cmd/ci/workflow.go` - Add K8s context setup, registry login, custom flags
- EXPAND: `cmd/config_ci.go` - Wire all flags to command
- EXPAND: `cmd/config_ci_test.go` - Add tests 8-9

**Implementation:**
- K8s context setup step (kubeconfig secret)
- Docker registry login step (conditional)
- Custom configuration flags:
  - `--branch`, `--workflow-name`
  - `--kubeconfig-secret-name`
  - `--registry-login-url-variable-name`, `--registry-user-variable-name`, etc.
  - `--use-registry-login` (toggle)
  - `--self-hosted-runner`
- Registry URL behavior (with/without login)

**Tests Passing:**
8. `TestNewConfigCICmd_WorkflowYAMLHasCustomValues`
9. `TestNewConfigCICmd_WorkflowHasNoRegistryLogin`

**Value:** Production-ready with full configuration options and proper authentication.

**Merge Safety:** Safe - adds configuration flexibility, backward compatible defaults

---

### PR6: Advanced Features (Remote Build & Debug)
**~150 lines**

**Files:**
- EXPAND: `cmd/ci/config.go` - Add UseRemoteBuild, UseDebug flags
- EXPAND: `cmd/ci/workflow.go` - Remote build logic, debug mode logic
- EXPAND: `cmd/config_ci.go` - Wire remaining flags
- EXPAND: `cmd/config_ci_test.go` - Add tests 10-11

**Implementation:**
- `--remote` flag (changes deploy command to `func deploy --remote`)
- Workflow name changes based on mode
- `--debug` flag:
  - Adds `workflow_dispatch` trigger
  - Adds func CLI caching step
  - Conditional step execution

**Tests Passing:**
10. `TestNewConfigCICmd_RemoteBuildAndDeployWorkflow`
11. `TestNewConfigCICmd_HasWorkflowDispatchAndCacheInDebugMode`

**Value:** Complete feature set with remote builds and developer-friendly debug mode.

**Merge Safety:** Safe - advanced features are opt-in flags

---

## Summary

Each PR:
- ✅ Delivers working, testable functionality
- ✅ Easy to review (follows natural test progression)
- ✅ Can be merged independently
- ✅ Feature flagged (no risk to existing users)
- ✅ Includes comprehensive tests

Total: 6 PRs, ~1,210 lines of new code

**Note:** PR1 (typos) can be opened in parallel with PR2 (refactoring)

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

# Cherry-pick files
git checkout issue-744-generate-github-actions-workflow -- cmd/testing/factory.go
git checkout issue-744-generate-github-actions-workflow -- cmd/ci/workflow.go
git checkout issue-744-generate-github-actions-workflow -- cmd/ci/config.go
git checkout issue-744-generate-github-actions-workflow -- cmd/config_ci.go
git checkout issue-744-generate-github-actions-workflow -- cmd/config_ci_test.go

# Edit to keep only basic workflow functionality
# Keep tests 4-7 only in config_ci_test.go

# Run tests
FUNC_ENABLE_CI_CONFIG=true go test ./cmd/... -v

# Commit and push
git add .
git commit -m "feat: implement basic GitHub Actions workflow generation"
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

In `runConfigCIGithub()`, return early with:
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
