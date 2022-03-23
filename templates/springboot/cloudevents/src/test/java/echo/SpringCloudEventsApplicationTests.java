package echo;

import static org.hamcrest.MatcherAssert.assertThat;
import static org.hamcrest.Matchers.equalTo;
import static org.hamcrest.Matchers.notNullValue;
import static org.springframework.cloud.function.cloudevent.CloudEventMessageUtils.ID;
import static org.springframework.cloud.function.cloudevent.CloudEventMessageUtils.SOURCE;
import static org.springframework.cloud.function.cloudevent.CloudEventMessageUtils.SPECVERSION;
import static org.springframework.cloud.function.cloudevent.CloudEventMessageUtils.SUBJECT;
import static org.springframework.cloud.function.cloudevent.CloudEventMessageUtils.TYPE;

import java.net.URI;
import java.util.UUID;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.boot.test.context.SpringBootTest.WebEnvironment;
import org.springframework.boot.test.web.client.TestRestTemplate;
import org.springframework.http.HttpHeaders;
import org.springframework.http.MediaType;
import org.springframework.http.RequestEntity;
import org.springframework.http.ResponseEntity;

@SpringBootTest(classes = SpringCloudEventsApplication.class,
    webEnvironment = WebEnvironment.RANDOM_PORT)
public class SpringCloudEventsApplicationTests {
  
  @Autowired
  private TestRestTemplate rest;

  @Test
  public void testEchoInput() throws Exception {

    String input ="hello";

    HttpHeaders ceHeaders = new HttpHeaders();
    ceHeaders.add(SPECVERSION, "1.0");
    ceHeaders.add(ID, UUID.randomUUID()
        .toString());
    ceHeaders.add(TYPE, "echo");
    ceHeaders.add(SOURCE, "http://localhost:8080/echo");
    ceHeaders.add(SUBJECT, "Echo content");

    ResponseEntity<String> response = this.rest.exchange(
        RequestEntity.post(new URI("/echo"))
            .contentType(MediaType.APPLICATION_JSON)
            .headers(ceHeaders)
            .body(input),
        String.class);

    assertThat(response.getStatusCode()
        .value(), equalTo(200));
    String body = response.getBody();
    assertThat(body, notNullValue());
    assertThat(body, equalTo(input));
  }

  @Test
  public void testEchoRoutingBasedOnType() throws Exception {

    String input ="hello";

    HttpHeaders ceHeaders = new HttpHeaders();
    ceHeaders.add(SPECVERSION, "1.0");
    ceHeaders.add(ID, UUID.randomUUID()
      .toString());
    ceHeaders.add(TYPE, "echo");
    ceHeaders.add(SOURCE, "http://localhost:8080/echo");
    ceHeaders.add(SUBJECT, "Echo content");

    ResponseEntity<String> response = this.rest.exchange(
      RequestEntity.post(new URI("/"))
        .contentType(MediaType.APPLICATION_JSON)
        .headers(ceHeaders)
        .body(input),
      String.class);

    assertThat(response.getStatusCode()
      .value(), equalTo(200));
    String body = response.getBody();
    assertThat(body, notNullValue());
    assertThat(body, equalTo(input));
  }
}
