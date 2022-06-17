package functions;

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
  public void testEchoWithBody() throws Exception {
    ResponseEntity<String> response = this.rest.exchange(
      RequestEntity.post(new URI("/echo"))
                   .body("hello"), String.class);
    assertThat(response.getStatusCode()
                       .value(), equalTo(200));
    assertThat(response.getBody(), containsString("echo: hello"));
  }

  @Test
  public void testEchoWithoutBody() throws Exception {
    ResponseEntity<String> response = this.rest.exchange(
      RequestEntity.get(new URI("/echo/"))
          .header("custom-header", "custom-value")
            .build(), String.class);
    assertThat(response.getStatusCode()
      .value(), equalTo(200));
    assertThat(response.getBody(), containsString("custom-header: custom-value"));
  }
}
