package functions;

import io.quarkus.funqy.Funq;
import io.quarkus.funqy.knative.events.CloudEvent;
import io.quarkus.funqy.knative.events.CloudEventMapping;

/**
 * Test handler fixture for TestInt_KafkaQuarkusDelivery.
 *
 * A plain CloudEvents-over-HTTP Quarkus (Funqy) function -- it contains no Kafka
 * code -- that echoes the received payload to stdout so the integration test can
 * confirm the exact record produced to Kafka was delivered to this function via
 * the injected kafka-runtime sidecar. Both delivery shapes the sidecar emits are
 * covered here, routed by CloudEvent type:
 *
 *  - A plain Kafka record is wrapped by the sidecar as a CloudEvent of type
 *    dev.knative.kafka.event with application/octet-stream data; raw() binds the
 *    raw bytes and echoes them.
 *  - A record already carrying ce_* headers and a JSON body is passed through by
 *    the sidecar with its own type; typed() binds it to the Input POJO (Funqy's
 *    typed model) and echoes the message.
 *
 * Returning void yields a 2xx, so the sidecar commits the offset
 * (at-least-once) and the consumer group drains.
 */
public class Function {

    @Funq
    @CloudEventMapping(trigger = "dev.knative.kafka.event")
    public void raw(CloudEvent<byte[]> event) {
        System.out.println("FUNC-E2E-RAW data=" + new String(event.data()));
    }

    @Funq
    @CloudEventMapping(trigger = "e2e.kafka.typed")
    public void typed(CloudEvent<Input> event) {
        System.out.println("FUNC-E2E-TYPED message=" + event.data().getMessage());
    }
}
