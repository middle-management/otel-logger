# otel-logger: Effortless Log Forwarding to OpenTelemetry

**otel-logger** is a lightweight CLI tool for developers and operators who want to get their application logs into OpenTelemetry—fast. Pipe your logs or wrap your app, and otel-logger will take care of parsing, enriching, and forwarding them to your OTEL Collector using the official Go SDK.

---

## Why Use otel-logger?

- **Drop-in for existing apps**: No code changes required. Pipe logs or wrap any command.
- **Great for containers**: Use as a Docker ENTRYPOINT to instantly ship logs.
- **Handles real-world log messiness**: Works with JSON, timestamps, mixed plain/text, and custom fields.
- **Production-ready**: Supports batching, custom headers, gRPC/HTTP, robust signal handling, and flexible field mappings.

---

## Quick Start

**Install:**

```bash
go install github.com/middle-management/otel-logger@latest
```

**Pipe logs:**

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317 \
cat app.log | otel-logger
```

**Wrap your app (captures stdout/stderr):**

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317 \
otel-logger -- ./myapp --config config.yaml
```

---

## Features

- **OpenTelemetry Go SDK** (beta logging support)
- **CLI**: Simple flags via `go-arg`
- **Two modes**: stdin log ingestion _or_ wrap any process
- **Multiple protocols**: gRPC and HTTP/HTTPS OTLP
- **JSON parsing/detection** with prefix support
- **Custom field mappings** (Logstash, Winston, etc.)
- **Batching/performance**: Official OTEL batching
- **Custom headers** for authentication
- **Signal and stream tagging**
- **Docker-friendly** and supports insecure/dev environments

---

## Usage Examples

### Pipe logs from a file

```bash
cat app.log | otel-logger
```

### Wrap any command

```bash
otel-logger -- node server.js
```

### With custom service info

```bash
OTEL_SERVICE_NAME=myapp \
OTEL_SERVICE_VERSION=1.2.3 \
cat app.log | otel-logger
```

### Docker ENTRYPOINT

```Dockerfile
ENTRYPOINT ["otel-logger", "--"]
CMD ["./your-app"]
```

### Advanced: Handle log prefixes & custom fields

```bash
cat logstash.log | otel-logger \
  --json-prefix "^\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}" \
  --timestamp-fields "@timestamp" \
  --level-fields "level" \
  --message-fields "message"
```

---

## Configuration

Supports standard OpenTelemetry environment variables plus CLI flags for extra control.

**Key environment variables:**

| Variable                       | Default                 | Purpose                               |
|------------------------------- |------------------------ |---------------------------------------|
| OTEL_EXPORTER_OTLP_ENDPOINT    | http://localhost:4318   | OTEL Collector endpoint               |
| OTEL_EXPORTER_OTLP_PROTOCOL    | http/protobuf           | Protocol (`grpc`/`http/protobuf`)     |
| OTEL_EXPORTER_OTLP_INSECURE    | false                   | Use insecure connection (dev/test)    |
| OTEL_EXPORTER_OTLP_HEADERS     | ""                      | Extra headers (comma-separated)       |
| OTEL_SERVICE_NAME              | otel-logger             | Service name for telemetry            |

**Useful CLI flags:**
See `otel-logger --help` for a full list.

- `--timeout` (default: 10s)
- `--json-prefix` (extract JSON from prefixed logs)
- `--batch-size` (default: 50)
- `--flush-interval` (default: 5s)
- `--timestamp-fields`, `--level-fields`, `--message-fields` (custom field mappings)
- `--version` (show version info)

---

## Supported Log Formats

- **JSON**: Any shape, with customizable field mappings
- **Prefixed JSON**: Handles timestamps or other prefixes (see `--json-prefix`)
- **Mixed text/JSON**: Treats non-JSON lines as messages
- **Stream tagging**: When wrapping commands, logs are tagged with `stream=stdout|stderr|system`

---

## Troubleshooting

- **Connection refused?** Double-check your OTEL Collector URL and port.
- **Timeouts?** Try increasing `--timeout` for slow networks.
- **Weird log formats?** Use `--json-prefix` or custom field mappings.
- **Auth errors?** Check your `OTEL_EXPORTER_OTLP_HEADERS` formatting.
- **Debugging?** Pipe stderr to a file:
  `otel-logger ... 2> debug.log`

---

## Developing & Contributing

Contributions welcome!
- Build: `make build`
- Test: `go test -v` or `make test`
- Benchmarks: `make bench`
- Lint: `make lint` (needs golangci-lint)

---

## License

[Add your license information here]

---

## See Also

- [OpenTelemetry Spec](https://opentelemetry.io/docs/reference/specification/)
- [OpenTelemetry Collector](https://opentelemetry.io/docs/collector/)
- [OpenTelemetry Go Docs](https://opentelemetry.io/docs/instrumentation/go/)
