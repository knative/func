package functions;

import io.quarkus.funqy.Funq;
import io.quarkus.funqy.knative.events.CloudEvent;
import io.quarkus.funqy.knative.events.CloudEventBuilder;

/**
 * Your Function class
 */
public class Function {

    /**
     * Use the Quarkus Funq extension for the function. This example
     * function returns "OK" as the response data.
     * @param input a CloudEvent
     * @return a CloudEvent
     */
    @Funq
    public CloudEvent<Output> function(CloudEvent<Input> input) {

        // Add your business logic here

        System.out.println(input);
        Output output = new Output("OK");
        return CloudEventBuilder.create().build(output);
    }

}
