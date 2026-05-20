# Issue #3514 - Implementation Plan

## Problem Confirmed

After deep analysis, I've confirmed the issue:

**The existing E2E test `TestMetadata_Envs_Add` tests `func config envs add`, NOT `func deploy -e`!**

This means there's NO test coverage for the `-e` flag on deploy, which is exactly what lkingland said needs to be added.

## Root Cause

The `-e` flag on deploy is supposed to work, but there's no test to verify it actually does. The code flow looks correct, but without tests, we can't be sure it works across all builders and languages.

## Solution: Add E2E Matrix Test

### Test Specification (from lkingland)

> "This can be closed by adding an E2E matrix test that:
> 1. Creates a new Function which returns (in its response) whether a given env variable was set in its environment.
> 2. Runs `func deploy -e KEY=VALUE` against it, then invokes the deployed function and asserts the env reached the container.
> 
> Because it's a matrix test, the assertion runs for every builder × OS × language combination we support."

### Implementation Steps

#### Step 1: Create the Matrix Test

**File:** `e2e/e2e_matrix_test.go`

Add a new test function: `TestMatrix_Deploy_Envs`

```go
// TestMatrix_Deploy_Envs ensures that environment variables passed via -e flag
// on deploy are correctly applied to the deployed function across all builders
// and runtimes.
//
// This test verifies that `func deploy -e KEY=VALUE` works correctly.
func TestMatrix_Deploy_Envs(t *testing.T) {
    forEachPermutation(t, "deploy-envs", func(t *testing.T, name, runtime, builder, template string) {
        root := fromCleanEnv(t, name)
        
        // Initialize function
        initArgs, timeout := matrixExceptionsLocal(t, []string{}, runtime, builder, template)
        initArgs = append(initArgs, "-l", runtime, "-t", template)
        if err := newCmd(t, append([]string{"init"}, initArgs...)...).Run(); err != nil {
            t.Fatal(err)
        }
        
        // Create function implementation that echoes back env vars
        impl := createEnvEchoFunction(t, root, runtime)
        if err := os.WriteFile(filepath.Join(root, impl.filename), []byte(impl.code), 0644); err != nil {
            t.Fatal(err)
        }
        
        // Deploy with -e flag
        deployArgs := []string{"deploy", "-e", "TEST_VAR=test_value", "-e", "ANOTHER_VAR=another_value"}
        if builder != "" {
            deployArgs = append(deployArgs, "--builder", builder)
        }
        
        cmd := newCmd(t, deployArgs...)
        cmd.Env = append(os.Environ(), fmt.Sprintf("FUNC_BUILDER=%s", builder))
        if err := cmd.Run(); err != nil {
            t.Fatal(err)
        }
        
        defer func() {
            clean(t, name, Namespace)
        }()
        
        // Wait for function to be ready and invoke it
        url := ksvcUrl(name)
        if !waitFor(t, url, timeout) {
            t.Fatal("function did not become ready")
        }
        
        // Invoke function and check response
        resp, err := http.Get(url)
        if err != nil {
            t.Fatalf("failed to invoke function: %v", err)
        }
        defer resp.Body.Close()
        
        body, err := io.ReadAll(resp.Body)
        if err != nil {
            t.Fatalf("failed to read response: %v", err)
        }
        
        // Verify env vars are present
        responseStr := string(body)
        if !strings.Contains(responseStr, "TEST_VAR=test_value") {
            t.Errorf("TEST_VAR not found in response. Got: %s", responseStr)
        }
        if !strings.Contains(responseStr, "ANOTHER_VAR=another_value") {
            t.Errorf("ANOTHER_VAR not found in response. Got: %s", responseStr)
        }
    })
}
```

#### Step 2: Create Helper Function for Language-Specific Implementations

```go
type envEchoImpl struct {
    filename string
    code     string
}

func createEnvEchoFunction(t *testing.T, root, runtime string) envEchoImpl {
    t.Helper()
    
    switch runtime {
    case "go":
        return envEchoImpl{
            filename: "function.go",
            code: `package function

import (
    "fmt"
    "net/http"
    "os"
)

type Function struct{}

func New() *Function { return &Function{} }

func (f *Function) Handle(w http.ResponseWriter, _ *http.Request) {
    testVar := os.Getenv("TEST_VAR")
    anotherVar := os.Getenv("ANOTHER_VAR")
    
    if testVar == "" {
        http.Error(w, "TEST_VAR not set", http.StatusInternalServerError)
        return
    }
    if anotherVar == "" {
        http.Error(w, "ANOTHER_VAR not set", http.StatusInternalServerError)
        return
    }
    
    fmt.Fprintf(w, "TEST_VAR=%s\nANOTHER_VAR=%s\n", testVar, anotherVar)
}
`,
        }
        
    case "node", "typescript":
        filename := "index.js"
        if runtime == "typescript" {
            filename = "index.ts"
        }
        return envEchoImpl{
            filename: filename,
            code: `const handle = async (context) => {
  const testVar = process.env.TEST_VAR;
  const anotherVar = process.env.ANOTHER_VAR;
  
  if (!testVar) {
    return {
      statusCode: 500,
      body: 'TEST_VAR not set'
    };
  }
  if (!anotherVar) {
    return {
      statusCode: 500,
      body: 'ANOTHER_VAR not set'
    };
  }
  
  return {
    statusCode: 200,
    body: ` + "`TEST_VAR=${testVar}\\nANOTHER_VAR=${anotherVar}`" + `
  };
};

module.exports = { handle };
`,
        }
        
    case "python":
        return envEchoImpl{
            filename: "func.py",
            code: `import os
from parliament import Context

def main(context: Context):
    test_var = os.getenv('TEST_VAR')
    another_var = os.getenv('ANOTHER_VAR')
    
    if not test_var:
        return {'statusCode': 500, 'body': 'TEST_VAR not set'}
    if not another_var:
        return {'statusCode': 500, 'body': 'ANOTHER_VAR not set'}
    
    return {
        'statusCode': 200,
        'body': f'TEST_VAR={test_var}\\nANOTHER_VAR={another_var}'
    }
`,
        }
        
    case "rust":
        return envEchoImpl{
            filename: "src/lib.rs",
            code: `use std::env;

pub fn handle(_req: http::Request<Vec<u8>>) -> http::Response<Vec<u8>> {
    let test_var = env::var("TEST_VAR").unwrap_or_default();
    let another_var = env::var("ANOTHER_VAR").unwrap_or_default();
    
    if test_var.is_empty() {
        return http::Response::builder()
            .status(500)
            .body("TEST_VAR not set".as_bytes().to_vec())
            .unwrap();
    }
    if another_var.is_empty() {
        return http::Response::builder()
            .status(500)
            .body("ANOTHER_VAR not set".as_bytes().to_vec())
            .unwrap();
    }
    
    let body = format!("TEST_VAR={}\\nANOTHER_VAR={}", test_var, another_var);
    http::Response::builder()
        .status(200)
        .body(body.as_bytes().to_vec())
        .unwrap()
}
`,
        }
        
    case "quarkus":
        return envEchoImpl{
            filename: "src/main/java/functions/Function.java",
            code: `package functions;

import io.quarkus.funqy.Funq;
import org.eclipse.microprofile.config.inject.ConfigProperty;

import javax.inject.Inject;

public class Function {
    
    @ConfigProperty(name = "TEST_VAR")
    String testVar;
    
    @ConfigProperty(name = "ANOTHER_VAR")
    String anotherVar;
    
    @Funq
    public String function() {
        if (testVar == null || testVar.isEmpty()) {
            throw new RuntimeException("TEST_VAR not set");
        }
        if (anotherVar == null || anotherVar.isEmpty()) {
            throw new RuntimeException("ANOTHER_VAR not set");
        }
        return "TEST_VAR=" + testVar + "\\nANOTHER_VAR=" + anotherVar;
    }
}
`,
        }
        
    case "springboot":
        return envEchoImpl{
            filename: "src/main/java/functions/Function.java",
            code: `package functions;

import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.context.annotation.Bean;
import org.springframework.core.env.Environment;

import java.util.function.Function;

@SpringBootApplication
public class FunctionApplication {
    
    public static void main(String[] args) {
        SpringApplication.run(FunctionApplication.class, args);
    }
    
    @Bean
    public Function<String, String> function(Environment env) {
        return input -> {
            String testVar = env.getProperty("TEST_VAR");
            String anotherVar = env.getProperty("ANOTHER_VAR");
            
            if (testVar == null || testVar.isEmpty()) {
                throw new RuntimeException("TEST_VAR not set");
            }
            if (anotherVar == null || anotherVar.isEmpty()) {
                throw new RuntimeException("ANOTHER_VAR not set");
            }
            
            return "TEST_VAR=" + testVar + "\\nANOTHER_VAR=" + anotherVar;
        };
    }
}
`,
        }
        
    default:
        t.Fatalf("unsupported runtime: %s", runtime)
        return envEchoImpl{}
    }
}
```

#### Step 3: Run the Test

```bash
# Run the matrix test
FUNC_E2E_MATRIX=true go test -v ./e2e -run TestMatrix_Deploy_Envs

# Or run for specific runtime/builder
FUNC_E2E_MATRIX=true FUNC_E2E_MATRIX_RUNTIMES=go FUNC_E2E_MATRIX_BUILDERS=pack go test -v ./e2e -run TestMatrix_Deploy_Envs
```

#### Step 4: If Test Fails, Debug and Fix

If the test fails, it will reveal exactly where the bug is:
- Which builder doesn't pass envs correctly?
- Which runtime doesn't receive envs?
- Is it a deployer issue?

Then we can fix the specific issue.

## Expected Outcome

### If Test Passes

Great! The feature works, and we've added test coverage to prevent regressions.

### If Test Fails

The test will reveal the bug, and we can fix it. Possible failure scenarios:

1. **Envs not reaching container** → Fix deployer logic
2. **Builder-specific issue** → Fix builder implementation
3. **Runtime-specific issue** → Fix runtime template

## Files to Create/Modify

### 1. e2e/e2e_matrix_test.go

Add `TestMatrix_Deploy_Envs` function and `createEnvEchoFunction` helper.

### 2. Potential Fixes (if test fails)

- `pkg/knative/deployer.go` - Knative deployer
- `pkg/k8s/deployer.go` - Kubernetes deployer
- `pkg/keda/deployer.go` - Keda deployer
- `pkg/builders/pack/builder.go` - Pack builder
- `pkg/builders/s2i/builder.go` - S2I builder

## Testing Strategy

### Phase 1: Add Test (This PR)

1. Add `TestMatrix_Deploy_Envs` to `e2e/e2e_matrix_test.go`
2. Add `createEnvEchoFunction` helper
3. Run test locally for one runtime/builder combo
4. Commit and push

### Phase 2: Run Full Matrix (CI)

1. CI will run the full matrix test
2. If it passes → Issue is closed
3. If it fails → We know exactly where the bug is

### Phase 3: Fix (If Needed)

1. Debug the specific failing combination
2. Fix the bug
3. Verify test passes

## Timeline

- **Step 1-2:** Add test code (2-3 hours)
- **Step 3:** Run local test (30 minutes)
- **Step 4:** Debug and fix if needed (1-2 hours)
- **Total:** 4-6 hours

## Success Criteria

✅ Test added for all runtimes and builders  
✅ Test passes for all combinations  
✅ Issue #3514 closed  
✅ Future regressions prevented  

## Notes

- This is exactly what lkingland asked for
- The test will run in CI for every PR
- It covers all builder × runtime combinations
- It's a proper E2E test that actually deploys and invokes

