package {{.GroupId}}.{{.ArtifactId}};

import io.quarkus.funqy.Funq;

public class Function {

    @Funq
    public Object echo(Object input) {
        return input;
    }

}
