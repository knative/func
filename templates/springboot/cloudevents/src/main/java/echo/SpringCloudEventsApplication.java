package echo;

import org.springframework.aot.hint.MemberCategory;
import org.springframework.aot.hint.RuntimeHints;
import org.springframework.aot.hint.RuntimeHintsRegistrar;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.cloud.function.context.message.MessageUtils;
import org.springframework.context.annotation.ImportRuntimeHints;

@SpringBootApplication
@ImportRuntimeHints(SpringCloudEventsApplication.MessageHeaderHints.class)
public class SpringCloudEventsApplication {

  public static void main(String[] args) {
    SpringApplication.run(SpringCloudEventsApplication.class, args);
  }

  static class MessageHeaderHints implements RuntimeHintsRegistrar {
    @Override
    public void registerHints(RuntimeHints hints, ClassLoader classLoader) {
      hints.reflection().registerType(MessageUtils.MessageStructureWithCaseInsensitiveHeaderKeys.class,
          MemberCategory.INVOKE_PUBLIC_METHODS);
    }
  }
}
