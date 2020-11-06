package functions;

import io.quarkus.funqy.Funq;

public class Function {

    @Funq
    public Output function(Input input) {
        return new Output(input.getMessage());
    }

}
