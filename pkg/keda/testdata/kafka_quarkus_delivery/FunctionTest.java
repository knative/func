package functions;

import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.assertTrue;

/**
 * Placeholder unit test for the TestInt_KafkaQuarkusDelivery handler fixture.
 *
 * The handler's real behaviour is exercised end-to-end over Kafka + HTTP by the
 * Go integration test. This class only exists so the scaffold's src/test sources
 * still compile after Function.java is replaced: the stock FunctionTest invoked
 * the old function(Input) signature that the fixture removes, and
 * NativeFunctionIT extends this class.
 */
public class FunctionTest {

    @Test
    void placeholder() {
        assertTrue(true);
    }
}
