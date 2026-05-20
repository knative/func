//go:build e2e

package e2e

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestMatrix_Deploy_Envs ensures that environment variables passed via -e flag
// on deploy are correctly applied to the deployed function across all builders
// and runtimes.
//
// This test verifies that `func deploy -e KEY=VALUE` works correctly for:
// - All supported runtimes (go, python, node, typescript, rust, quarkus, springboot)
// - All supported builders (host, pack, s2i)
// - Both http and cloudevents templates
//
// The test creates a function that echoes back environment variables, deploys
// it with `func deploy -e TEST_VAR=test_value -e ANOTHER_VAR=another_value`,
// then invokes the function and verifies the env vars are present.
//
// Related: Issue #3514
func TestMatrix_Deploy_Envs(t *testing.T) {
	forEachPermutation(t, "deploy-envs", func(t *testing.T, name, runtime, builder, template string) {
		root := fromCleanEnv(t, name)

		// Register cleanup functions
		t.Cleanup(func() {
			cleanImages(t, name)
		})
		t.Cleanup(func() {
			clean(t, name, Namespace)
		})

		// Initialize function
		initArgs := []string{"init", "-l", runtime, "-t", template}
		initArgs, timeout := matrixExceptionsLocal(t, initArgs, runtime, builder, template)
		if err := newCmd(t, initArgs...).Run(); err != nil {
			t.Fatalf("Failed to create %s function with %s template: %v", runtime, template, err)
		}

		// Create function implementation that echoes back env vars
		impl := createEnvEchoFunction(t, root, runtime, template)
		targetFile := filepath.Join(root, impl.filename)

		// For some runtimes, we need to create parent directories
		if err := os.MkdirAll(filepath.Dir(targetFile), 0755); err != nil {
			t.Fatalf("Failed to create directory for %s: %v", targetFile, err)
		}

		if err := os.WriteFile(targetFile, []byte(impl.code), 0644); err != nil {
			t.Fatalf("Failed to write function implementation: %v", err)
		}

		// Deploy with -e flag
		deployArgs := []string{"deploy", "-e", "TEST_VAR=test_value", "-e", "ANOTHER_VAR=another_value", "--builder", builder}
		if err := newCmd(t, deployArgs...).Run(); err != nil {
			t.Fatal(err)
		}

		// Wait for function to be ready
		url := ksvcUrl(name)
		if !waitFor(t, url, withWaitTimeout(timeout), withTemplate(template)) {
			t.Fatal("function did not become ready")
		}

		// Invoke function and check response
		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Get(url)
		if err != nil {
			t.Fatalf("failed to invoke function: %v", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("failed to read response: %v", err)
		}

		// Check status code
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
		}

		// Verify env vars are present in response
		responseStr := string(body)
		if !strings.Contains(responseStr, "TEST_VAR=test_value") {
			t.Errorf("TEST_VAR not found or has wrong value in response. Got: %s", responseStr)
		}
		if !strings.Contains(responseStr, "ANOTHER_VAR=another_value") {
			t.Errorf("ANOTHER_VAR not found or has wrong value in response. Got: %s", responseStr)
		}
	})
}

// envEchoImpl holds the filename and code for a function implementation
// that echoes back environment variables
type envEchoImpl struct {
	filename string
	code     string
}

// createEnvEchoFunction creates a language-specific function implementation
// that echoes back the TEST_VAR and ANOTHER_VAR environment variables.
// Returns HTTP 500 if either variable is not set, HTTP 200 with the values otherwise.
func createEnvEchoFunction(t *testing.T, root, runtime, template string) envEchoImpl {
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

	case "node":
		return envEchoImpl{
			filename: "index.js",
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

	case "typescript":
		return envEchoImpl{
			filename: "index.ts",
			code: `import { Context } from 'faas-js-runtime';

const handle = async (context: Context): Promise<any> => {
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

export { handle };
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
import java.util.Optional;

public class Function {
    
    @ConfigProperty(name = "TEST_VAR")
    Optional<String> testVar;
    
    @ConfigProperty(name = "ANOTHER_VAR")
    Optional<String> anotherVar;
    
    @Funq
    public String function() {
        if (!testVar.isPresent() || testVar.get().isEmpty()) {
            throw new RuntimeException("TEST_VAR not set");
        }
        if (!anotherVar.isPresent() || anotherVar.get().isEmpty()) {
            throw new RuntimeException("ANOTHER_VAR not set");
        }
        return "TEST_VAR=" + testVar.get() + "\\nANOTHER_VAR=" + anotherVar.get();
    }
}
`,
		}

	case "springboot":
		return envEchoImpl{
			filename: "src/main/java/functions/CloudFunctionApplication.java",
			code: `package functions;

import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.context.annotation.Bean;
import org.springframework.core.env.Environment;

import java.util.function.Function;

@SpringBootApplication
public class CloudFunctionApplication {
    
    public static void main(String[] args) {
        SpringApplication.run(CloudFunctionApplication.class, args);
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
		t.Fatalf("unsupported runtime for env echo test: %s", runtime)
		return envEchoImpl{}
	}
}
