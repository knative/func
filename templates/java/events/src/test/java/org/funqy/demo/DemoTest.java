package org.funqy.demo;

import io.quarkus.test.junit.QuarkusTest;
import io.restassured.RestAssured;
import io.restassured.parsing.Parser;
import org.junit.jupiter.api.Test;

import static io.restassured.RestAssured.given;
import static org.hamcrest.CoreMatchers.is;
import static org.hamcrest.Matchers.*;
import static org.hamcrest.Matchers.equalTo;

@QuarkusTest
public class DemoTest {

    @Test
    public void testVanilla() {
        RestAssured.given().contentType("application/json")
                .body("{\"name\": \"Bill\"}")
                .post("/")
                .then().statusCode(200)
                .header("ce-id", nullValue())
                .body("name", equalTo("Bill"))
                .body("message", equalTo("Hello Bill!"));
    }

    @Test
    public void testBinary() {
        RestAssured.given().contentType("application/json")
                .body("{\"name\": \"Bill\"}")
                .header("ce-id", "1234")
                .header("ce-specversion", "1.0")
                .post("/")
                .then().statusCode(200)
                .header("ce-id", notNullValue())
                .header("ce-specversion", equalTo("1.0"))
                .header("ce-source", equalTo("greet"))
                .header("ce-type", equalTo("greet.output"))
                .body("name", equalTo("Bill"))
                .body("message", equalTo("Hello Bill!"));
    }

    static final String event = "{ \"id\" : \"1234\", " +
            "  \"specversion\": \"1.0\", " +
            "  \"source\": \"/foo\", " +
            "  \"type\": \"sometype\", " +
            "  \"datacontenttype\": \"application/json\", " +
            "  \"data\": { \"name\": \"Bill\" } " +
            "}";

    @Test
    public void testStructured() {
        RestAssured.given().contentType("application/cloudevents+json")
                .body(event)
                .post("/")
                .then().statusCode(200)
                .defaultParser(Parser.JSON)
                .body("id", notNullValue())
                .body("specversion", equalTo("1.0"))
                .body("type", equalTo("greet.output"))
                .body("source", equalTo("greet"))
                .body("datacontenttype", equalTo("application/json"))
                .body("data.name", equalTo("Bill"))
                .body("data.message", equalTo("Hello Bill!"));
    }

}