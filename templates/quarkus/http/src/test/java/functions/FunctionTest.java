package functions;

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
