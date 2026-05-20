# Issue #3514 - Deep Analysis

## Issue Summary

**Title:** Environment variables passed via -e flag not applied on deploy  
**Issue:** https://github.com/knative/func/issues/3514  
**Status:** Open, assigned to you (@Itx-Psycho0)  
**Labels:** kind/good-first-issue, status/ready  

## Problem Statement

Environment variables passed with `func deploy -e KEY=VALUE` are NOT included in the deployed service spec.

**Works:**
```bash
func run -e MYVAR=value  # ✅ Env var is set in local container
```

**Doesn't Work:**
```bash
func deploy -e MYVAR=value  # ❌ Env var is NOT set in deployed service
```

**Expected:** `-e MYVAR=value` on deploy should set the env var in the Knative service container spec, the same way it does for local runs.

## Code Flow Analysis

### 1. Flag Definition (cmd/deploy.go:195-198)

```go
cmd.Flags().StringArrayP("env", "e", []string{},
    "Environment variable to set in the form NAME=VALUE. "+
        "You may provide this flag multiple times for setting multiple environment variables. "+
        "To unset, specify the environment variable name followed by a \"-\" (e.g., NAME-).")
```

✅ **Flag is correctly defined**

### 2. Config Reading (cmd/deploy.go:562-590)

```go
func newDeployConfig(cmd *cobra.Command) deployConfig {
    cfg := deployConfig{
        // ... other fields ...
        Env: viper.GetStringSlice("env"),
        // ... other fields ...
    }
    
    // NOTE: .Env should be viper.GetStringSlice, but this returns unparsed
    // results and appears to be an open issue since 2017
    var err error
    if cfg.Env, err = cmd.Flags().GetStringArray("env"); err != nil {
        fmt.Fprintf(cmd.OutOrStdout(), "error reading envs: %v", err)
    }
    
    return cfg
}
```

✅ **Config correctly reads the -e flag values**

### 3. Applying Envs to Function (cmd/deploy.go:594-647)

```go
func (c deployConfig) Configure(f fn.Function) (fn.Function, error) {
    // ... other configuration ...
    
    // Envs
    // Preprocesses any Envs provided (which may include removals) into a final set
    f.Run.Envs, err = applyEnvs(f.Run.Envs, c.Env)
    if err != nil {
        return f, err
    }
    
    return f, nil
}
```

✅ **Envs are correctly applied to f.Run.Envs**

### 4. Deployer Using Envs (pkg/knative/deployer.go:312-314)

```go
newEnv, newEnvFrom, err := k8s.ProcessEnvs(f.Run.Envs, &referencedSecrets, &referencedConfigMaps)
if err != nil {
    return fn.DeploymentResult{}, err
}
```

✅ **Deployer correctly reads f.Run.Envs**

### 5. Updating Service (pkg/knative/deployer.go:328)

```go
_, err = client.UpdateServiceWithRetry(ctx, f.Name, updateService(f, previousService, newEnv, newEnvFrom, newVolumes, newVolumeMounts, d.decorator, daprInstalled), 3)
```

✅ **Service is updated with the processed envs**

## The Mystery

**All the code looks correct!** So why doesn't it work?

## Hypothesis

After analyzing the code and lkingland's comment, I believe the issue is:

### The Problem: Envs are NOT persisted to func.yaml before deploy

Look at the flow:

1. User runs: `func deploy -e MYVAR=value`
2. Config reads the flag ✅
3. Envs are applied to `f.Run.Envs` ✅
4. **BUT**: `f.Write()` is called AFTER the deploy in `runDeploy()` (line 395)
5. For **remote deploys** (`--remote`), `f.Write()` is called BEFORE the pipeline (line 363)
6. For **local deploys**, the function is deployed with the in-memory `f.Run.Envs`

### The Real Issue

The issue is likely in **remote deploys** or when the function is **already deployed** and being updated.

Let me check the `runDeploy` function more carefully:

```go
func runDeploy(cmd *cobra.Command, newClient ClientFactory) (err error) {
    // ... initialization ...
    
    if f, err = cfg.Configure(f); err != nil {  // Line 295 - Envs applied here
        return
    }
    
    // ... more setup ...
    
    if cfg.Remote {
        // Write func.yaml before the pipeline uploads sources
        if err = f.Write(); err != nil {  // Line 363 - Write BEFORE remote deploy
            return
        }
        // ... remote deploy ...
    } else {
        // ... local build/push ...
        
        if f, err = client.Deploy(cmd.Context(), f, fn.WithDeploySkipBuildCheck(cfg.Build == "false")); err != nil {
            return wrapDeploymentError(err)
        }
    }
    
    // Write
    if err = f.Write(); err != nil {  // Line 395 - Write AFTER local deploy
        return
    }
    
    return f.Stamp()
}
```

### The Bug

For **local deploys**, the envs ARE applied to `f.Run.Envs` in memory and passed to the deployer. So local deploys SHOULD work.

But there might be an issue with:
1. **Existing deployments** - when updating an existing service
2. **The deployer not correctly merging envs**
3. **The envs being overwritten somewhere**

## lkingland's Solution

lkingland says:

> "This can be closed by adding an E2E matrix test that:
> 1. Creates a new Function which returns (in its response) whether a given env variable was set in its environment.
> 2. Runs `func deploy -e KEY=VALUE` against it, then invokes the deployed function and asserts the env reached the container.
> 
> Because it's a matrix test, the assertion runs for every builder × OS × language combination we support — so we're verifying that each builder's distinct env-handling path is wired up correctly, and any future regression in any one builder's env-applying logic has not regressed."

### What This Means

The issue is that **there's no test coverage** for the `-e` flag on deploy. The existing test `TestDeploy_Envs` only tests that envs are correctly parsed and stored in `f.Run.Envs`, but it doesn't test that they actually reach the deployed container.

## Root Cause Investigation

Let me check if there's a difference between how `func run` and `func deploy` handle envs:

### func run (cmd/run.go:358)

```go
f.Run.Envs, err = applyEnvs(f.Run.Envs, c.Env)
```

### func deploy (cmd/deploy.go:629)

```go
f.Run.Envs, err = applyEnvs(f.Run.Envs, c.Env)
```

**They're identical!** So the issue must be in the deployer or in how the service is created/updated.

## Potential Root Causes

### 1. Deployer Not Applying Envs Correctly

The deployer might be reading `f.Run.Envs` but not actually applying them to the service spec.

### 2. Service Update Logic Overwrites Envs

When updating an existing service, the update logic might be overwriting the envs instead of merging them.

### 3. Builder-Specific Issue

Different builders (pack, s2i) might handle envs differently, and some might not pass them through correctly.

### 4. Timing Issue

The envs might be applied correctly initially, but then overwritten by a subsequent operation.

## The Fix Strategy

Based on lkingland's comment and the code analysis, here's what needs to be done:

### Phase 1: Add E2E Matrix Test (REQUIRED)

Create a test that:
1. Creates a function that echoes back environment variables
2. Deploys it with `func deploy -e TEST_VAR=test_value`
3. Invokes the function
4. Asserts that `TEST_VAR=test_value` is present in the response
5. Runs this for all builder × OS × language combinations

### Phase 2: Fix the Bug (IF FOUND)

Once the test is in place and failing, we can identify exactly where the bug is and fix it.

### Phase 3: Verify Fix

Run the E2E test to confirm the fix works across all combinations.

## Next Steps

1. **Create the E2E matrix test** following lkingland's specification
2. **Run the test** to see if it fails (confirming the bug)
3. **Debug** to find where envs are being lost
4. **Fix** the issue
5. **Verify** the fix with the test

## Files to Modify

### 1. Test File (NEW)

Create: `cmd/deploy_test.go` or `pkg/knative/deployer_int_test.go`

Add E2E matrix test for env vars on deploy.

### 2. Potential Fix Locations

Depending on where the bug is found:
- `pkg/knative/deployer.go` - Knative deployer
- `pkg/k8s/deployer.go` - Kubernetes deployer  
- `pkg/keda/deployer.go` - Keda deployer
- `cmd/deploy.go` - Deploy command logic

## Conclusion

The issue is **NOT a simple code bug** - it's a **lack of test coverage** that has allowed a subtle bug to exist. The fix requires:

1. ✅ Adding comprehensive E2E tests
2. ❓ Finding and fixing the actual bug (once tests reveal it)
3. ✅ Ensuring all builders handle envs correctly

This is a **good first issue** because:
- The code flow is clear
- The fix location is well-defined (add tests)
- lkingland has provided exact specifications
- It teaches E2E testing patterns

But it's also **non-trivial** because:
- Requires understanding E2E test infrastructure
- Requires testing across multiple builders/languages
- Requires actual deployment to a cluster

