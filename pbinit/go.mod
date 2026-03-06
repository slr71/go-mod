module github.com/cyverse-de/go-mod/pbinit

go 1.25.0

replace github.com/cyverse-de/go-mod/gotelnats => ../gotelnats

require (
	github.com/cyverse-de/go-mod/gotelnats v0.0.15
	github.com/cyverse-de/p/go/analysis v0.0.20
	github.com/cyverse-de/p/go/header v0.0.6
	github.com/cyverse-de/p/go/monitoring v0.0.7
	github.com/cyverse-de/p/go/qms v0.2.3
	github.com/cyverse-de/p/go/requests v0.0.3
	go.opentelemetry.io/otel/trace v1.41.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cyverse-de/p/go/apps v0.0.3 // indirect
	github.com/cyverse-de/p/go/containers v0.0.4 // indirect
	github.com/cyverse-de/p/go/svcerror v0.0.10 // indirect
	github.com/cyverse-de/p/go/user v0.0.13 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/klauspost/compress v1.18.4 // indirect
	github.com/nats-io/nats.go v1.49.0 // indirect
	github.com/nats-io/nkeys v0.4.15 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel v1.41.0 // indirect
	go.opentelemetry.io/otel/metric v1.41.0 // indirect
	golang.org/x/crypto v0.48.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)
