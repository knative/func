package {{.GroupId}}.{{.ArtifactId}};

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
    void testEcho() {
        Object obj = (new Function()).echo(42);
        Assertions.assertEquals(42, obj);
    }

    @Test
    public void testEchoIntegration() {
        RestAssured.given().contentType("application/json")
                .body("{\"message\": \"Hello\"}")
                .header("ce-id", "42")
                .header("ce-specversion", "1.0")
                .post("/echo")
                .then().statusCode(200)
                .body("message", equalTo("Hello"));
    }

}