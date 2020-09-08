package org.funqy.demo;

import io.quarkus.funqy.Funq;
import io.smallrye.mutiny.Uni;
import org.jboss.logging.Logger;

import javax.inject.Inject;
import java.util.concurrent.*;

public class GreetingFunctions {
    private static final Logger log = Logger.getLogger("funqy.greeting");

    private static final ScheduledExecutorService executor = Executors.newScheduledThreadPool(16);

    @Inject
    GreetingService service;

    @Funq
    public Uni<Greeting> greet(Identity name) {
        return Uni.createFrom().emitter(uniEmitter -> {
            executor.schedule(()-> {
                try {
                    log.info("*** In greeting service ***");
                    String message = service.hello(name.getName());
                    log.info("Sending back: " + message);
                    Greeting greeting = new Greeting();
                    greeting.setMessage(message);
                    greeting.setName(name.getName());
                    uniEmitter.complete(greeting);

                } catch (Throwable t) {
                    uniEmitter.fail(t);
                }
            }, 100, TimeUnit.MILLISECONDS);
        });

    }

}
