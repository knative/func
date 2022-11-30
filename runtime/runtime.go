package runtime

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"

	"github.com/gorilla/mux"
)

var (
	usage   = "run\n\nRuns a CloudFunction CloudEvent handler."
	verbose = flag.Bool("V", false, "Verbose logging [$VERBOSE]")
	port    = flag.Int("port", 8080, "Listen on all interfaces at the given port [$PORT]")
)

func Start(handler interface{}) {
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, usage)
		flag.PrintDefaults()
	}
	parseEnv()   // override static defaults with environment.
	flag.Parse() // override env vars with flags.
	if err := run(handler); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

}

// parseEnv parses environment variables, populating the destination flags
// prior to the builtin flag parsing.  Invalid values exit 1.
func parseEnv() {
	parseBool := func(key string, dest *bool) {
		if val, ok := os.LookupEnv(key); ok {
			b, err := strconv.ParseBool(val)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%v is not a valid boolean\n", key)
				os.Exit(1)
			}
			*dest = b
		}
	}
	parseInt := func(key string, dest *int) {
		if val, ok := os.LookupEnv(key); ok {
			n, err := strconv.Atoi(val)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%v is not a valid integer\n", key)
				os.Exit(1)
			}
			*dest = n
		}
	}

	parseBool("VERBOSE", verbose)
	parseInt("PORT", port)
}

// run a cloudevents client in receive mode which invokes
// the user-defined function.Handler on receipt of an event.
func run(handler interface{}) error {
	ctx, cancel := context.WithCancel(context.Background())

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		cancel()
	}()

	// Use a gorilla mux for handling all HTTP requests
	router := mux.NewRouter()

	// Add handlers for readiness and liveness endpoints
	router.HandleFunc("/health/{endpoint:readiness|liveness}", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "OK")
	})

	httpHandler := toHttpHandler(handler, ctx)

	if httpHandler == nil {
		if *verbose {
			fmt.Printf("Initializing CloudEvent function\n")
		}
		protocol, err := cloudevents.NewHTTP(
			cloudevents.WithPort(*port),
			cloudevents.WithPath("/"),
		)
		if err != nil {
			return err
		}
		eventHandler, err := cloudevents.NewHTTPReceiveHandler(ctx, protocol, handler)
		if err != nil {
			return err
		}
		router.Handle("/", eventHandler)
	} else {
		if *verbose {
			fmt.Printf("Initializing HTTP function\n")
		}
		router.Handle("/", httpHandler)
	}

	httpServer := &http.Server{
		Addr:           fmt.Sprintf(":%d", *port),
		Handler:        router,
		ReadTimeout:    1 * time.Minute,
		WriteTimeout:   1 * time.Minute,
		MaxHeaderBytes: 1 << 20,
	}

	listenAndServeErr := make(chan error, 1)
	go func() {
		if *verbose {
			fmt.Printf("listening on http port %v\n", *port)
		}
		err := httpServer.ListenAndServe()
		cancel()
		listenAndServeErr <- err
	}()

	<-ctx.Done()
	shutdownCtx, shutdownCancelFn := context.WithTimeout(context.Background(), time.Second*5)
	defer shutdownCancelFn()
	err := httpServer.Shutdown(shutdownCtx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error on server shutdown: %v\n", err)
	}

	err = <-listenAndServeErr
	if http.ErrServerClosed == err {
		return nil
	}
	return err
}

// if the handler signature is compatible with http handler the function returns an instance of `http.Handler`,
// otherwise nil is returned
func toHttpHandler(handler interface{}, ctx context.Context) http.Handler {

	if f, ok := handler.(func(rw http.ResponseWriter, req *http.Request)); ok {
		return recoverMiddleware(http.HandlerFunc(f))
	}

	if f, ok := handler.(func(ctx context.Context, rw http.ResponseWriter, req *http.Request)); ok {
		ff := func(rw http.ResponseWriter, req *http.Request) {
			f(ctx, rw, req)
		}
		return recoverMiddleware(http.HandlerFunc(ff))
	}

	return nil
}
