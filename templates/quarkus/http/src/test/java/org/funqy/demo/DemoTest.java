package org.funqy.demo;

import io.quarkus.test.junit.QuarkusTest;
import io.restassured.RestAssured;
import io.restassured.parsing.Parser;
import org.hamcrest.Matcher;
import org.hamcrest.Matchers;
import org.junit.jupiter.api.Test;

import static io.restassured.RestAssured.given;
import static org.hamcrest.CoreMatchers.is;
import static org.hamcrest.Matchers.*;
import static org.hamcrest.Matchers.equalTo;

@QuarkusTest
public class DemoTest {

    @Test
    public void testGreet() {
        RestAssured.given().contentType("application/json")
                .body("{\"name\": \"Bill\"}")
                .post("/greet")
                .then().statusCode(200)
                .body("name", equalTo("Bill"))
                .body("message", equalTo("Hello Bill!"));
    }

    @Test
    public void testDoubleIt() {
        RestAssured.given().contentType("application/json")
                .body("5")
                .post("/doubleIt")
                .then().statusCode(200)
                .body(equalTo("10"));
    }

    @Test
    public void testToLowerCase() {
        RestAssured.given().contentType("application/json")
                .body("\"AsTrInG\"")
                .post("/toLowerCase")
                .then().statusCode(200)
                .body(equalTo("\"astring\""));
    }
}