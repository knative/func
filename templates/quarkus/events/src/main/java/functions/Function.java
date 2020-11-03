package {{.GroupId}}.{{.ArtifactId}};

import io.quarkus.funqy.Context;
import io.quarkus.funqy.Funq;
import io.quarkus.funqy.knative.events.CloudEvent;

public class Function {

    @Funq
    public Object echo(Object input, @Context CloudEvent cloudEvent) {
        if (cloudEvent != null) {
            System.out.println(
                    "CloudEvent{" +
                            "id='" + cloudEvent.id() + '\'' +
                            ", specVersion='" + cloudEvent.specVersion() + '\'' +
                            ", source='" + cloudEvent.source() + '\'' +
                            ", subject='" + cloudEvent.subject() + '\'' +
                            '}');
        }
        return input;
    }

}