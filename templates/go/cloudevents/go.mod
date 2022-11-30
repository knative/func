module function

go 1.18

require (
	github.com/cloudevents/sdk-go/v2 v2.12.0
	knative.dev/func v0.35.1
)

require (
	github.com/google/uuid v1.3.0 // indirect
	github.com/gorilla/mux v1.8.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	go.uber.org/atomic v1.10.0 // indirect
	go.uber.org/multierr v1.8.0 // indirect
	go.uber.org/zap v1.23.0 // indirect
)

// TODO: Fix this chicken and egg problem.
replace knative.dev/func => github.com/lance/func v1.1.3-0.20221130200745-483914d520ca
