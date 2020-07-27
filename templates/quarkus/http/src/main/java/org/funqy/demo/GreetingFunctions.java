package org.funqy.demo;

import io.quarkus.funqy.Funq;
import org.jboss.logging.Logger;

import javax.inject.Inject;
import java.util.concurrent.*;

public class GreetingFunctions {
    private static final Logger log = Logger.getLogger("funqy.greeting");

    private static final ScheduledExecutorService executor = Executors.newScheduledThreadPool(16);

    @Inject
    GreetingService service;

    @Funq
    public CompletionStage<Greeting> greet(Identity name) {
        CompletableFuture<Greeting> result = new CompletableFuture<>();
        executor.schedule(()-> {
            try {
                log.info("*** In greeting service ***");
                String message = service.hello(name.getName());
                log.info("Sending back: " + message);
                Greeting greeting = new Greeting();
                greeting.setMessage(message);
                greeting.setName(name.getName());
                result.complete(greeting);

            } catch (Throwable t) {
                result.completeExceptionally(t);
            }
        }, 100, TimeUnit.MILLISECONDS);
        return result;
    }

}
