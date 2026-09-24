module github.com/shing1211/hstongapi4go

go 1.26

// Build with a patched toolchain. The go directive above is the language floor
// for consumers; this pins the toolchain this module is built and scanned with.
// go1.26.1 through go1.26.5 carry standard-library vulnerabilities that
// `make govulncheck` reports against this module's own call graph (net/url,
// crypto/tls, net/http, encoding/asn1 and others, all fixed in go1.26.6).
// GOTOOLCHAIN=auto downloads it automatically, including in CI.
toolchain go1.26.6

require (
	github.com/shopspring/decimal v1.4.0
	go.opentelemetry.io/otel v1.36.0
	go.opentelemetry.io/otel/metric v1.36.0
	go.opentelemetry.io/otel/sdk/metric v1.36.0
	go.opentelemetry.io/otel/sdk v1.36.0
	go.opentelemetry.io/otel/trace v1.36.0
	go.uber.org/goleak v1.3.0
	google.golang.org/protobuf v1.36.12
)

require (
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/google/uuid v1.6.0 // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	golang.org/x/sys v0.33.0 // indirect
)
