package functions;

import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.context.annotation.Bean;
import org.springframework.messaging.Message;

import java.util.function.Function;

@SpringBootApplication
public class CloudFunctionApplication {

  public static void main(String[] args) {
    SpringApplication.run(CloudFunctionApplication.class, args);
  }

  @Bean
  public Function<Message<String>, String> echo() {
    return (inputMessage) -> {

      var stringBuilder = new StringBuilder();
      inputMessage.getHeaders()
        .forEach((key, value) -> {
          stringBuilder.append(key).append(": ").append(value).append(" ");
        });

      var payload = inputMessage.getPayload();

      if (!payload.isBlank()) {
        stringBuilder.append("echo: ").append(payload);
      }

      return stringBuilder.toString();
    };
  }
}
