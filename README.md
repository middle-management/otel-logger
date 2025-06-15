# OpenTelemetry Log Forwarder

A lightweight CLI tool that reads logs from stdin and forwards them to an OpenTelemetry collector using the **official OpenTelemetry Go SDK**. It intelligently parses JSON logs, handles various timestamp formats, and can extract JSON from logs with prefixes (like timestamps or log levels).

## Features

- **Official OpenTelemetry SDK**: Built with the official OpenTelemetry Go SDK (beta logging support)
- **Lightweight CLI**: Built with `go-arg` for clean, fast argument parsing
- **Multiple protocols**: Supports both gRPC and HTTP/HTTPS protocols via official OTLP exporters
- **JSON parsing**: Automatically detects and parses JSON log entries
- **Configurable field mappings**: Support for different logging frameworks (Logstash, Winston, etc.)
- **Prefix handling**: Extracts JSON from logs with timestamps or other prefixes
- **Flexible timestamp parsing**: Supports multiple timestamp formats (RFC3339, Unix timestamps, etc.)
- **Official batching**: Uses OpenTelemetry SDK's built-in batching for optimal performance
- **Custom headers**: Supports additional headers for authentication
- **Insecure connections**: Optional support for insecure connections (useful for development)
- **Service metadata**: Configurable service name and version for telemetry
- **Standards compliance**: Full OTLP specification compliance via official SDK

## Installation

### Build from source

```bash
git clone <repository-url>
cd otel-logger
go build -o otel-logger .
```

### Using go install

```bash
go install github.com/middle-management/otel-logger@latest
```

## Usage

### Basic Usage

```bash
# Send JSON logs via gRPC (default)
cat app.log | otel-logger --endpoint localhost:4317

# Send logs via HTTP
tail -f app.log | otel-logger --endpoint http://localhost:4318 --protocol http

# With custom service information
cat app.log | otel-logger --endpoint localhost:4317 --service-name myapp --service-version 1.2.3
```

### Advanced Usage

```bash
# Handle logs with timestamp prefixes using regex
cat app.log | otel-logger --endpoint localhost:4317 \
  --json-prefix "^\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}[.\\d]*Z?\\s*"

# Batch logs for better performance
cat app.log | otel-logger --endpoint localhost:4317 \
  --batch-size 100 --flush-interval 5s

# Use with authentication headers
cat app.log | otel-logger --endpoint https://api.example.com/v1/logs \
  --protocol http \
  --header "Authorization=Bearer your-token" \
  --header "X-API-Key=your-api-key"

# Handle Logstash/ELK format logs
cat logstash.log | otel-logger --endpoint localhost:4317 \
  --timestamp-fields "@timestamp" \
  --level-fields "level" \
  --message-fields "message"

# Handle Winston (Node.js) format logs
cat winston.log | otel-logger --endpoint localhost:4317 \
  --timestamp-fields "timestamp" \
  --level-fields "level" \
  --message-fields "message,msg"

# Handle custom application logs
cat custom.log | otel-logger --endpoint localhost:4317 \
  --timestamp-fields "created_at,time" \
  --level-fields "severity,priority" \
  --message-fields "description,content,text"

# Insecure connection (for development)
cat app.log | otel-logger --endpoint localhost:4317 --insecure
```

## Command Line Options

| Flag | Default | Description |
|------|---------|-------------|
| `--endpoint`, `-e` | `localhost:4317` | OpenTelemetry collector endpoint |
| `--protocol`, `-p` | `grpc` | Protocol to use (`grpc` or `http`) |
| `--service-name` | `otel-logger` | Service name for telemetry |
| `--service-version` | `1.0.0` | Service version for telemetry |
| `--insecure` | `false` | Use insecure connection |
| `--timeout` | `10s` | Request timeout |
| `--header` | `[]` | Additional headers (key=value format) |
| `--json-prefix` | `""` | Regex pattern to extract JSON from prefixed logs |
| `--batch-size` | `50` | Number of log entries to batch before sending |
| `--flush-interval` | `5s` | Interval to flush batched logs |
| `--timestamp-fields` | `[]` | JSON field names for timestamps (see defaults below) |
| `--level-fields` | `[]` | JSON field names for log levels (see defaults below) |
| `--message-fields` | `[]` | JSON field names for log messages (see defaults below) |

## Supported Log Formats

### Pure JSON Logs

```json
{"timestamp": "2024-01-15T10:30:45Z", "level": "info", "message": "Application started", "service": "web-server"}
{"ts": "2024-01-15T10:30:46.123Z", "lvl": "debug", "msg": "Database connected", "db_host": "localhost"}
```

### Prefixed JSON Logs

```
2024-01-15T10:30:45.123Z {"level": "info", "message": "Service initialized", "service": "api-gateway"}
[2024-01-15T10:30:47.456Z] {"level": "warn", "message": "High memory usage", "memory": "85%"}
2024-01-15 10:30:46 {"lvl": "debug", "msg": "Configuration loaded", "config_file": "/app/config.yaml"}
```

### Mixed Logs

The tool gracefully handles mixed log formats, treating non-JSON lines as plain text messages:

```
2024-01-15T10:30:45Z INFO Starting application server on port 8080
{"timestamp": "2024-01-15T10:30:46Z", "level": "info", "message": "Database connected"}
[ERROR] 2024-01-15 10:30:47 - Failed to load configuration file
```

### Configurable JSON Field Mappings

The tool can be configured to recognize different JSON field names for timestamps, log levels, and messages. This makes it compatible with various logging frameworks and formats.

#### Default Field Mappings

- **Timestamps**: `timestamp`, `ts`, `time`, `@timestamp` (supports various formats including Unix timestamps)
- **Log levels**: `level`, `lvl`, `severity`, `priority`
- **Messages**: `message`, `msg`, `text`, `content`
- **All other fields**: Preserved as log attributes

#### Custom Field Mappings

You can override the default field mappings using command-line flags:

```bash
# Custom timestamp fields (e.g., for Logstash format)
--timestamp-fields "@timestamp,created_at"

# Custom level fields (e.g., for Winston format)
--level-fields "level,severity,priority"

# Custom message fields (e.g., for custom application format)
--message-fields "message,description,log_message"
```

The tool will check each field in the order specified and use the first match found.

## Log Level Mapping

| Input Level | OpenTelemetry Severity |
|-------------|----------------------|
| `trace` | `SEVERITY_NUMBER_TRACE` |
| `debug` | `SEVERITY_NUMBER_DEBUG` |
| `info` | `SEVERITY_NUMBER_INFO` |
| `warn`, `warning` | `SEVERITY_NUMBER_WARN` |
| `error` | `SEVERITY_NUMBER_ERROR` |
| `fatal` | `SEVERITY_NUMBER_FATAL` |

## OpenTelemetry Collector Configuration

### Example collector configuration for receiving logs:

```yaml
# otel-collector-config.yaml
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
      http:
        endpoint: 0.0.0.0:4318

processors:
  batch:

exporters:
  logging:
    loglevel: debug
  # Add your preferred exporters here (e.g., jaeger, prometheus, etc.)

service:
  pipelines:
    logs:
      receivers: [otlp]
      processors: [batch]
      exporters: [logging]
```

### Running the collector:

```bash
otelcol --config-file otel-collector-config.yaml
```

## Examples

### Example 1: Basic JSON logs

```bash
# Create some sample logs
echo '{"timestamp": "2024-01-15T10:30:45Z", "level": "info", "message": "Hello World"}' | \
  ./otel-logger --endpoint localhost:4317
```

### Example 2: Custom field mappings

```bash
# Logstash/Elasticsearch format
echo '{"@timestamp": "2024-01-15T10:30:45Z", "level": "ERROR", "message": "Database connection failed"}' | \
  ./otel-logger --endpoint localhost:4317 \
  --timestamp-fields "@timestamp" \
  --level-fields "level" \
  --message-fields "message"

# Custom application format
echo '{"created_at": "2024-01-15T10:30:45Z", "severity": "high", "description": "Payment processing error", "details": "Card declined"}' | \
  ./otel-logger --endpoint localhost:4317 \
  --timestamp-fields "created_at,timestamp" \
  --level-fields "severity,level" \
  --message-fields "description,message"
```

### Example 3: Docker application logs

```bash
# Forward Docker container logs
docker logs -f myapp 2>&1 | ./otel-logger --endpoint localhost:4317 --service-name myapp
```

### Example 4: Application with prefixed logs

```bash
# Handle logs with timestamp prefixes
tail -f /var/log/myapp.log | ./otel-logger \
  --endpoint localhost:4317 \
  --service-name myapp \
  --json-prefix "^\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}[.\\d]*Z?\\s*"
```

### Example 5: High-throughput scenario

```bash
# Optimize for high throughput
cat large-log-file.log | ./otel-logger \
  --endpoint localhost:4317 \
  --batch-size 500 \
  --flush-interval 1s \
  --service-name batch-processor
```

## Performance Considerations

- **Batching**: Use larger batch sizes for high-throughput scenarios
- **Flush interval**: Shorter intervals provide faster log delivery but may increase overhead
- **Protocol choice**: gRPC typically offers better performance than HTTP for high-volume scenarios
- **Network**: Consider network latency when setting timeouts

## Testing

### Running Tests

```bash
# Run all tests
go test -v

# Run only unit tests
make test

# Run integration tests
make test-integration

# Run configuration tests
make test-config

# Run benchmarks
make bench
```

### Test Coverage

The project includes comprehensive tests:
- **Unit tests**: Core functionality, JSON parsing, field mappings
- **Integration tests**: Complete log processing pipeline
- **Configuration tests**: CLI argument parsing and validation
- **Benchmark tests**: Performance measurement and optimization

## Troubleshooting

### Common Issues

1. **Connection refused**: Ensure the OpenTelemetry collector is running and accessible
2. **Timeout errors**: Increase the `--timeout` value for slow networks
3. **JSON parsing errors**: Check log format and consider using `--json-prefix` for prefixed logs
4. **Authentication failures**: Verify headers are correctly formatted (`key=value`)

### Debug Mode

Add verbose output by redirecting stderr:

```bash
cat app.log | ./otel-logger --endpoint localhost:4317 2> debug.log
```

### Performance Testing

Check performance with built-in benchmarks:

```bash
make bench
```

## Technical Details

- **OpenTelemetry SDK**: Uses the official OpenTelemetry Go SDK (v1.36.0+ with beta logging support)
- **OTLP Exporters**: Official OTLP gRPC and HTTP exporters for standards compliance
- **CLI Framework**: Uses `go-arg` for clean, lightweight argument parsing
- **Protocol Support**: Native gRPC and HTTP/HTTPS with protobuf serialization via official exporters
- **Performance**: Official SDK batching with configurable batch sizes and flush intervals
- **Memory**: Low memory footprint with SDK-optimized resource management
- **Testing**: Comprehensive Go test suite with unit, integration, and benchmark tests
- **Error Handling**: Graceful handling of malformed JSON and connection issues
- **Standards Compliance**: Full OTLP specification compliance via official SDK components

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

### Development Workflow

1. **Build the project**: `make build`
2. **Run tests**: `go test -v` or `make test`
3. **Run integration tests**: `make test-integration`
4. **Check performance**: `make bench`
5. **Lint code**: `make lint` (requires golangci-lint)

### Adding Tests

- Add unit tests to `main_test.go`
- Add configuration tests to `config_test.go`
- Add integration tests to `integration_test.go`
- Include benchmarks for performance-critical code

## License

[Add your license information here]

## See Also

- [OpenTelemetry Specification](https://opentelemetry.io/docs/reference/specification/)
- [OpenTelemetry Collector](https://opentelemetry.io/docs/collector/)
- [OpenTelemetry Go Documentation](https://opentelemetry.io/docs/instrumentation/go/)