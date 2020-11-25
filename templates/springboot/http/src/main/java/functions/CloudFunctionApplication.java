package functions;

import java.util.function.Function;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.context.annotation.Bean;

@SpringBootApplication
public class CloudFunctionApplication {

  public static void main(String[] args) {
    SpringApplication.run(CloudFunctionApplication.class, args);
  }

  @Bean
  public Function<String, String> uppercase() {
    return String::toUpperCase;
  }

  /**
   * Health checks
   * 
   * @return
   */
  @Bean
  public Function<String, String> health() {
    return probe -> {
      if ("readiness".equals(probe)) {
        return "ready";
      } else if ("liveness".equals(probe)) {
        return "live";
      } else {
        return "OK";
      }
    };
  }

}
