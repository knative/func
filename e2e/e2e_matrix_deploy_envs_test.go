//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMatrix_Deploy_Envs checks that environment variables passed with
// `func deploy -e KEY=VALUE` reach the running container for every supported
// builder and runtime.
//
// Only the http template is covered. CloudEvents handlers are a different
// contract, and the previous run of this test died at `func deploy` for those
// permutations. An http handler that echoes the env is enough to prove the
// value is in the container (issue #3514).
//
// Each implementation is written over the template's real handler. The stock
// templates answer "OK"; these answer a body containing "ENV_ECHO", so a
// response of "OK" means the echo was never what got deployed. A body that
// has the marker but not TEST_VAR=test_value means deploy -e did not apply
// and #3514 is still open. That failure is the result. Do not loosen the
// assertion to hide it.
func TestMatrix_Deploy_Envs(t *testing.T) {
	forEachPermutation(t, "deploy-envs", func(t *testing.T, name, runtime, builder, template string) {
		if template != "http" {
			t.Skip("http proves deploy -e reaches the container; cloudevents handlers are a different contract")
		}

		root := fromCleanEnv(t, name)

		// Image cleanup runs after the cluster cleanup (LIFO).
		t.Cleanup(func() {
			cleanImages(t, name)
		})
		t.Cleanup(func() {
			clean(t, name, Namespace)
		})

		initArgs := []string{"init", "-l", runtime, "-t", template}
		initArgs, timeout := matrixExceptionsLocal(t, initArgs, runtime, builder, template)
		if err := newCmd(t, initArgs...).Run(); err != nil {
			t.Fatalf("Failed to create %s function with %s template: %v", runtime, template, err)
		}

		writeEnvEcho(t, root, runtime)

		deployArgs := []string{
			"deploy", "--builder", builder,
			"-e", "TEST_VAR=test_value",
			"-e", "ANOTHER_VAR=another_value",
		}
		if err := newCmd(t, deployArgs...).Run(); err != nil {
			t.Fatal(err)
		}

		url := functionURL(t, name)
		if !waitFor(t, url,
			withWaitTimeout(timeout),
			withContentMatch("ENV_ECHO")) {
			t.Fatal("echo function did not become ready")
		}

		body := getBody(t, url)
		if !strings.Contains(body, "TEST_VAR=test_value") ||
			!strings.Contains(body, "ANOTHER_VAR=another_value") {
			t.Fatalf("deploy -e did not reach the container (#3514). Got: %s", body)
		}
	})
}

// writeEnvEcho replaces the template handler (and the template tests that
// assert the stock "OK" body) so the built image serves the env echo.
func writeEnvEcho(t *testing.T, root, runtime string) {
	t.Helper()
	files, ok := envEchoFiles[runtime]
	if !ok {
		t.Fatalf("unsupported runtime for env echo test: %s", runtime)
	}
	for rel, content := range files {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("creating %s: %v", path, err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("writing %s: %v", path, err)
		}
	}
}

// envEchoFiles is the http-template path of each file the echo replaces.
// Paths match templates/<runtime>/http. A wrong path leaves the stock
// handler in the image, which is what the first revision of this test did.
var envEchoFiles = map[string]map[string]string{
	"go": {
		"function.go": goEnvEcho,
	},
	"node": {
		"index.js":            nodeEnvEcho,
		"test/unit.js":        nodeEnvEchoUnit,
		"test/integration.js": nodeEnvEchoIntegration,
	},
	"typescript": {
		"src/index.ts":        tsEnvEcho,
		"test/unit.ts":        tsEnvEchoUnit,
		"test/integration.ts": tsEnvEchoIntegration,
	},
	"python": {
		"function/func.py": pythonEnvEcho,
	},
	"rust": {
		"src/handler.rs": rustEnvEcho,
	},
	"quarkus": {
		"src/main/java/functions/Function.java":     quarkusEnvEcho,
		"src/test/java/functions/FunctionTest.java": quarkusEnvEchoTest,
	},
	"springboot": {
		"src/main/java/functions/CloudFunctionApplication.java": springEnvEcho,
		"src/test/java/functions/EchoCaseFunctionTest.java":     springEnvEchoTest,
	},
}

const goEnvEcho = `package function

import (
	"fmt"
	"net/http"
	"os"
)

// MyFunction is the function provided by this library.
type MyFunction struct{}

// New constructs an instance of the function.
func New() *MyFunction {
	return &MyFunction{}
}

// Handle reports the deploy -e values. The ENV_ECHO marker distinguishes
// this handler from the stock template, which answers "OK".
func (f *MyFunction) Handle(res http.ResponseWriter, _ *http.Request) {
	fmt.Fprintf(res, "ENV_ECHO TEST_VAR=%s ANOTHER_VAR=%s\n",
		os.Getenv("TEST_VAR"), os.Getenv("ANOTHER_VAR"))
}
`

const nodeEnvEcho = `'use strict';

const handle = async (context, body) => {
  const testVar = process.env.TEST_VAR || '';
  const anotherVar = process.env.ANOTHER_VAR || '';
  context.log.info('env echo', body);
  return {
    body: 'ENV_ECHO TEST_VAR=' + testVar + ' ANOTHER_VAR=' + anotherVar,
    headers: {
      'content-type': 'text/plain'
    }
  };
};

module.exports = { handle };
`

const nodeEnvEchoUnit = `'use strict';

const func = require('..').handle;
const test = require('tape');

const fixture = { log: { info: console.log } };

function expectedBody() {
  const testVar = process.env.TEST_VAR || '';
  const anotherVar = process.env.ANOTHER_VAR || '';
  return 'ENV_ECHO TEST_VAR=' + testVar + ' ANOTHER_VAR=' + anotherVar;
}

test('Unit: handles an HTTP GET', async t => {
  t.plan(1);
  const result = await func({ ...fixture, method: 'GET', query: { name: 'tiger' } });
  t.deepEqual(result, { body: expectedBody(), headers: { 'content-type': 'text/plain' } });
  t.end();
});

test('Unit: handles an HTTP POST', async t => {
  t.plan(1);
  const body = { name: 'tiger' };
  const result = await func({ ...fixture, method: 'POST', body }, body);
  t.deepEqual(result, { body: expectedBody(), headers: { 'content-type': 'text/plain' } });
  t.end();
});
`

const nodeEnvEchoIntegration = `'use strict';

const { start } = require('faas-js-runtime');
const request = require('supertest');

const func = require('..').handle;
const test = require('tape');

function expectedBody() {
  const testVar = process.env.TEST_VAR || '';
  const anotherVar = process.env.ANOTHER_VAR || '';
  return 'ENV_ECHO TEST_VAR=' + testVar + ' ANOTHER_VAR=' + anotherVar;
}

const errHandler = t => err => {
  t.error(err);
  t.end();
};

test('Integration: handles an HTTP GET', t => {
  start(func).then(server => {
    t.plan(2);
    request(server)
      .get('/?name=tiger')
      .expect(200)
      .end((err, res) => {
        t.error(err, 'No error');
        t.equal(res.text, expectedBody());
        t.end();
        server.close();
      });
  }, errHandler(t));
});

test('Integration: handles an HTTP POST', t => {
  start(func).then(server => {
    t.plan(2);
    request(server)
      .post('/')
      .send({ name: 'tiger' })
      .expect(200)
      .end((err, res) => {
        t.error(err, 'No error');
        t.equal(res.text, expectedBody());
        t.end();
        server.close();
      });
  }, errHandler(t));
});
`

const tsEnvEcho = `import { Context, StructuredReturn } from 'faas-js-runtime';

const handle = async (context: Context, body: string): Promise<StructuredReturn> => {
  const testVar = process.env.TEST_VAR ?? '';
  const anotherVar = process.env.ANOTHER_VAR ?? '';
  context.log.info('env echo ' + (body ?? ''));
  return {
    body: 'ENV_ECHO TEST_VAR=' + testVar + ' ANOTHER_VAR=' + anotherVar,
    headers: {
      'content-type': 'text/plain'
    }
  };
};

export { handle };
`

const tsEnvEchoUnit = `'use strict';

import test from 'tape';
import { expectType } from 'tsd';
import { Context, HTTPFunction } from 'faas-js-runtime';
import { handle } from '../build/index.js';

function expectedBody(): string {
  const testVar = process.env.TEST_VAR ?? '';
  const anotherVar = process.env.ANOTHER_VAR ?? '';
  return 'ENV_ECHO TEST_VAR=' + testVar + ' ANOTHER_VAR=' + anotherVar;
}

test('Unit: handles a valid request', async (t) => {
  t.plan(2);
  const body: Record<string, string> = {
    name: 'tiger',
    customerId: '01234'
  };

  const result = await handle({ method: 'POST', log: { info: (_) => _ } } as Context, body);
  t.ok(result);
  t.equal(result.body, expectedBody());
  t.end();
});

expectType<HTTPFunction>(handle);
`

const tsEnvEchoIntegration = `'use strict';
import { start, InvokerOptions } from 'faas-js-runtime';
import request from 'supertest';

import * as func from '../build';
import test, { Test } from 'tape';

const data = {
  name: 'tiger',
  customerId: '01234'
};

function expectedBody(): string {
  const testVar = process.env.TEST_VAR ?? '';
  const anotherVar = process.env.ANOTHER_VAR ?? '';
  return 'ENV_ECHO TEST_VAR=' + testVar + ' ANOTHER_VAR=' + anotherVar;
}

const errHandler = (t: Test) => (err: Error) => {
  t.error(err);
  t.end();
};

test('Integration: handles a valid request', (t) => {
  start(func.handle, {} as InvokerOptions).then((server) => {
    t.plan(3);
    request(server)
      .post('/')
      .send(data)
      .expect(200)
      .end((err, result) => {
        t.error(err, 'No error');
        t.ok(result);
        t.equal(result.text, expectedBody());
        t.end();
        server.close();
      });
  }, errHandler(t));
});
`

const pythonEnvEcho = `# Function
import logging
import os


def new():
    """ New is the only method that must be implemented by a Function.
    The instance returned can be of any name.
    """
    return Function()


class Function:
    def __init__(self):
        """ The init method is an optional method where initialization can be
        performed. See the start method for a startup hook which includes
        configuration.
        """

    async def handle(self, scope, receive, send):
        """ Handle all HTTP requests to this Function other than readiness
        and liveness probes."""

        test_var = os.environ.get("TEST_VAR", "")
        another_var = os.environ.get("ANOTHER_VAR", "")
        payload = f"ENV_ECHO TEST_VAR={test_var} ANOTHER_VAR={another_var}".encode()

        logging.info("ENV_ECHO: Request Received")

        await send({
            'type': 'http.response.start',
            'status': 200,
            'headers': [
                [b'content-type', b'text/plain'],
            ],
        })
        await send({
            'type': 'http.response.body',
            'body': payload,
        })

    def start(self, cfg):
        """ start is an optional method which is called when a new Function
        instance is started, such as when scaling up or during an update.
        Provided is a dictionary containing all environmental configuration.
        Args:
            cfg (Dict[str, str]): A dictionary containing environmental config.
                In most cases this will be a copy of os.environ, but it is
                best practice to use this cfg dict instead of os.environ.
        """
        logging.info("Function starting")

    def stop(self):
        """ stop is an optional method which is called when a function is
        stopped, such as when scaled down, updated, or manually canceled.  Stop
        can block while performing function shutdown/cleanup operations.  The
        process will eventually be killed if this method blocks beyond the
        platform's configured maximum shutdown timeout.
        """
        logging.info("Function stopping")

    def alive(self):
        """ alive is an optional method for performing a deep check on your
        Function's liveness.  If removed, the system will assume the function
        is ready if the process is running. This is exposed by default at the
        path /health/liveness.  The optional string return is a message.
        """
        return True, "Alive"

    def ready(self):
        """ ready is an optional method for performing a deep check on your
        Function's readiness.  If removed, the system will assume the function
        is ready if the process is running.  This is exposed by default at the
        path /health/readiness.
        """
        return True, "Ready"
`

const rustEnvEcho = `use actix_web::{HttpRequest, HttpResponse};
use log::info;
use std::env;

fn env_echo() -> String {
    let test_var = env::var("TEST_VAR").unwrap_or_default();
    let another_var = env::var("ANOTHER_VAR").unwrap_or_default();
    format!("ENV_ECHO TEST_VAR={test_var} ANOTHER_VAR={another_var}")
}

// Implement your function's logic here
pub async fn index(req: HttpRequest) -> HttpResponse {
    info!("{:#?}", req);
    HttpResponse::Ok().body(env_echo())
}

#[cfg(test)]
mod tests {
    use super::*;
    use actix_web::{body::to_bytes, http, test::TestRequest};

    #[actix_rt::test]
    async fn get() {
        let req = TestRequest::get().to_http_request();
        let resp = index(req).await;
        assert_eq!(resp.status(), http::StatusCode::OK);
        assert_eq!(
            to_bytes(resp.into_body()).await.unwrap().as_ref(),
            env_echo().as_bytes()
        );
    }

    #[actix_rt::test]
    async fn post() {
        let req = TestRequest::post().to_http_request();
        let resp = index(req).await;
        assert!(resp.status().is_success());
        assert_eq!(
            to_bytes(resp.into_body()).await.unwrap().as_ref(),
            env_echo().as_bytes()
        );
    }
}
`

const quarkusEnvEcho = `package functions;

import io.quarkus.funqy.Funq;

/**
 * Your Function class
 */
public class Function {

    static String envEcho() {
        String testVar = System.getenv("TEST_VAR");
        String anotherVar = System.getenv("ANOTHER_VAR");
        if (testVar == null) {
            testVar = "";
        }
        if (anotherVar == null) {
            anotherVar = "";
        }
        return "ENV_ECHO TEST_VAR=" + testVar + " ANOTHER_VAR=" + anotherVar;
    }

    /**
     * Use the Quarkus Funqy extension for our function.
     * @param input a Java bean
     * @return a Java bean
     */
    @Funq
    public Output function(Input input) {
        return new Output(envEcho());
    }

}
`

const quarkusEnvEchoTest = `package functions;

import io.quarkus.test.junit.QuarkusTest;
import io.restassured.RestAssured;
import org.junit.jupiter.api.Assertions;
import org.junit.jupiter.api.Test;

import static org.hamcrest.Matchers.equalTo;

@QuarkusTest
public class FunctionTest {

    @Test
    void testFunction() {
        Output output = (new Function()).function(new Input("Hello!"));
        Assertions.assertEquals(Function.envEcho(), output.getMessage());
    }

    @Test
    public void testFunctionIntegration() {
        RestAssured.given().contentType("application/json")
                .body("{\"message\": \"Hello\"}")
                .header("ce-id", "42")
                .header("ce-specversion", "1.0")
                .post("/")
                .then().statusCode(200)
                .body("message", equalTo(Function.envEcho()));
    }

}
`

const springEnvEcho = `package functions;

import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.context.annotation.Bean;
import org.springframework.messaging.Message;

import java.util.function.Function;

@SpringBootApplication
public class CloudFunctionApplication {

  static String envEcho() {
    String testVar = System.getenv("TEST_VAR");
    String anotherVar = System.getenv("ANOTHER_VAR");
    if (testVar == null) {
      testVar = "";
    }
    if (anotherVar == null) {
      anotherVar = "";
    }
    return "ENV_ECHO TEST_VAR=" + testVar + " ANOTHER_VAR=" + anotherVar;
  }

  public static void main(String[] args) {
    SpringApplication.run(CloudFunctionApplication.class, args);
  }

  @Bean
  public Function<Message<String>, String> echo() {
    return inputMessage -> envEcho();
  }
}
`

const springEnvEchoTest = `package functions;

import java.net.URI;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.boot.test.context.SpringBootTest.WebEnvironment;
import org.springframework.boot.test.web.client.TestRestTemplate;
import org.springframework.http.RequestEntity;
import org.springframework.http.ResponseEntity;

import static org.hamcrest.MatcherAssert.assertThat;
import static org.hamcrest.Matchers.*;

@SpringBootTest(classes = CloudFunctionApplication.class,
  webEnvironment = WebEnvironment.RANDOM_PORT)
public class EchoCaseFunctionTest {

  @Autowired
  private TestRestTemplate rest;

  @Test
  public void testReqWithBody() throws Exception {
    ResponseEntity<String> response = this.rest.exchange(
      RequestEntity.post(new URI("/echo"))
                   .body("hello"), String.class);
    assertThat(response.getStatusCode()
                       .value(), equalTo(200));
    assertThat(response.getBody(), equalTo(CloudFunctionApplication.envEcho()));
  }

  @Test
  public void testReqWithoutBody() throws Exception {
    ResponseEntity<String> response = this.rest.exchange(
      RequestEntity.get(new URI("/echo/"))
          .header("custom-header", "custom-value")
            .build(), String.class);
    assertThat(response.getStatusCode()
      .value(), equalTo(200));
    assertThat(response.getBody(), equalTo(CloudFunctionApplication.envEcho()));
  }
}
`
