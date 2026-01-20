package functions;

import io.quarkus.funqy.Funq;

/**
 * Your Function class
 */
public class Function {

    /**
     * Use the Quarkus Funqy extension for our function. This function returns "OK".
     * @param input a Java bean
     * @return a Java bean
     */
    @Funq
    public Output function(Input input) {

        // Add business logic here

        return new Output("OK");
    }

}
