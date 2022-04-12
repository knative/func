package function

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	textTemplate "text/template"
)

// Static files
// .
//├── .gitignore
//├── .mvn
//│   └── wrapper
//│       ├── MavenWrapperDownloader.java
//│       └── maven-wrapper.properties
//├── mvnw
//├── mvnw.cmd
//├── pom.xml
//├── README.md
//└── src
//    └── main
//        └── resources
//            └── application.properties
//
//go:generate go run ./generate/templates/main.go quarkus_template_static_files zz_quarkus_template_static_files_generated.go quarkusTemplateStaticFilesZIP
var quarkusTemplateCommonStaticFilesFS = newZipFS(quarkusTemplateStaticFilesZIP)

var quarkusHttpTemplate Template = &quarkusTemplate{
	name:        "http",
	funcSrc:     httpFunction,
	funcTestSrc: httpFunctionTest,
}

var quarkusCloudEventTemplate Template = &quarkusTemplate{
	name:        "cloudevents",
	funcSrc:     cloudEventFunction,
	funcTestSrc: cloudEventFunctionTest,
}

type quarkusTemplate struct {
	name        string
	funcSrc     string
	funcTestSrc string
}

func (c *quarkusTemplate) Name() string {
	return c.name
}

func (c *quarkusTemplate) Runtime() string {
	return "quarkus"
}

func (c *quarkusTemplate) Repository() string {
	return "default"
}

func (c *quarkusTemplate) Fullname() string {
	return "default/" + c.name
}

func (c *quarkusTemplate) Write(ctx context.Context, f *Function) error {
	// write static files
	err := copyFromFS(".", f.Root, quarkusTemplateCommonStaticFilesFS)
	if err != nil {
		return err
	}

	// write Java sources

	pkg := "functions" // TODO read from input
	pkgParts := strings.Split(pkg, ".")
	pkgDir := filepath.Join(pkgParts...)

	type src struct {
		name    string
		typ     string
		content string
	}

	srcs := []src{
		{name: "Function.java", typ: "main", content: c.funcSrc},
		{name: "Input.java", typ: "main", content: inputBean},
		{name: "Output.java", typ: "main", content: outputBean},
		{name: "FunctionTest.java", typ: "test", content: c.funcTestSrc},
		{name: "NativeFunctionIT.java", typ: "test", content: nativeTest},
	}

	err = os.MkdirAll(filepath.Join(f.Root, "src", "main", "java", pkgDir), 0755)
	if err != nil {
		return err
	}
	err = os.MkdirAll(filepath.Join(f.Root, "src", "test", "java", pkgDir), 0755)
	if err != nil {
		return err
	}

	writeSrc := func(src src) error {
		var err error

		t := textTemplate.New("java-src")
		t, err = t.Parse(src.content)
		if err != nil {
			return err
		}

		f, err := os.OpenFile(filepath.Join(f.Root, "src", src.typ, "java", pkgDir, src.name), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			return err
		}
		defer f.Close()

		return t.Execute(f, struct {
			Package string
		}{Package: pkg})
	}

	for _, src := range srcs {
		err = writeSrc(src)
		if err != nil {
			return err
		}
	}

	// write fn.Function configuration values

	builders := map[string]string{
		"default": "quay.io/boson/faas-jvm-builder:v0.9.2",
		"jvm":     "quay.io/boson/faas-jvm-builder:v0.9.2",
		"native":  "quay.io/boson/faas-quarkus-native-builder:v0.9.2",
	}

	if f.Builder == "" { // as a special first case, this default comes from itself
		f.Builder = f.Builders["default"]
		if f.Builder == "" { // still nothing?  then use the template
			f.Builder = "default"
		}
	}

	if len(f.Builders) == 0 {
		f.Builders = builders
	}

	if f.HealthEndpoints.Liveness == "" {
		f.HealthEndpoints.Liveness = DefaultLivenessEndpoint
	}
	if f.HealthEndpoints.Readiness == "" {
		f.HealthEndpoints.Readiness = DefaultReadinessEndpoint
	}
	if f.Invocation.Format == "" {
		f.Invocation.Format = c.name
	}

	return nil
}

const inputBean = `package {{.Package}};

public class Input {
    private String message;

    public Input() {}

    public Input(String message) {
        this.message = message;
    }

    public String getMessage() {
        return message;
    }

    public void setMessage(String message) {
        this.message = message;
    }

    @Override
    public String toString() {
        return "Input{" +
                "message='" + message + '\'' +
                '}';
    }
}
`

const outputBean = `package {{.Package}};

public class Output {
    private String message;

    public Output() {}

    public Output(String message) {
        this.message = message;
    }

    public String getMessage() {
        return message;
    }

    public void setMessage(String message) {
        this.message = message;
    }

    @Override
    public String toString() {
        return "Output{" +
                "message='" + message + '\'' +
                '}';
    }
}
`

const nativeTest = `package {{.Package}};

import io.quarkus.test.junit.NativeImageTest;

@NativeImageTest
public class NativeFunctionIT extends FunctionTest {

    // Execute the same tests but in native mode.
}
`

const httpFunction = `package {{.Package}};

import io.quarkus.funqy.Funq;

/**
 * Your Function class
 */
public class Function {

    /**
     * Use the Quarkus Funqy extension for our function. This function simply echoes its input
     * @param input a Java bean
     * @return a Java bean
     */
    @Funq
    public Output function(Input input) {

        // Add business logic here

        return new Output(input.getMessage());
    }

}
`

const httpFunctionTest = `package {{.Package}};

import io.quarkus.test.junit.QuarkusTest;
import io.restassured.RestAssured;
import org.hamcrest.CoreMatchers;
import org.junit.jupiter.api.Assertions;
import org.junit.jupiter.api.Test;

import static org.hamcrest.Matchers.equalTo;
import static org.hamcrest.Matchers.notNullValue;

@QuarkusTest
public class FunctionTest {

    @Test
    void testFunction() {
        Output output = (new Function()).function(new Input("Hello!"));
        Assertions.assertEquals("Hello!", output.getMessage());
    }

    @Test
    public void testFunctionIntegration() {
        RestAssured.given().contentType("application/json")
                .body("{\"message\": \"Hello\"}")
                .header("ce-id", "42")
                .header("ce-specversion", "1.0")
                .post("/")
                .then().statusCode(200)
                .body("message", equalTo("Hello"));
    }

}
`

const cloudEventFunction = `package {{.Package}};

import io.quarkus.funqy.Funq;
import io.quarkus.funqy.knative.events.CloudEvent;
import io.quarkus.funqy.knative.events.CloudEventBuilder;

/**
 * Your Function class
 */
public class Function {

    /**
     * Use the Quarkus Funq extension for the function. This example
     * function simply echoes its input data.
     * @param input a CloudEvent
     * @return a CloudEvent
     */
    @Funq
    public CloudEvent<Output> function(CloudEvent<Input> input) {

        // Add your business logic here

        System.out.println(input);
        Output output = new Output(input.data().getMessage());
        return CloudEventBuilder.create().build(output);
    }

}
`

const cloudEventFunctionTest = `package {{.Package}};

import io.quarkus.funqy.knative.events.CloudEventBuilder;
import io.quarkus.test.junit.QuarkusTest;
import io.restassured.RestAssured;
import org.hamcrest.CoreMatchers;
import org.junit.jupiter.api.Assertions;
import org.junit.jupiter.api.Test;

import static org.hamcrest.Matchers.equalTo;
import static org.hamcrest.Matchers.notNullValue;

@QuarkusTest
public class FunctionTest {

    @Test
    void testFunction() {
        Output output = (new Function()).function(CloudEventBuilder.create().build(new Input("Hello!"))).data();
        Assertions.assertEquals("Hello!", output.getMessage());
    }

    @Test
    public void testFunctionIntegration() {
        RestAssured.given().contentType("application/json")
                .body("{\"message\": \"Hello!\"}")
                .header("ce-id", "42")
                .header("ce-specversion", "1.0")
                .post("/")
                .then().statusCode(200)
                .header("ce-id", notNullValue())
                .header("ce-specversion", equalTo("1.0"))
                .header("ce-source", equalTo("function"))
                .header("ce-type", equalTo("function.output"))
                .body("message", equalTo("Hello!"));
    }

}

`
