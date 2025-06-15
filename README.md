# OpenTelemetry Log Forwarder

A lightweight CLI tool that reads logs from stdin and forwards them to an OpenTelemetry collector. It intelligently parses JSON logs, handles various timestamp formats, and can extract JSON from logs with prefixes (like timestamps or log levels).

## Features

- **Multiple protocols**: Supports both gRPC and HTTP/HTTPS protocols
- **JSON parsing**: Automatically detects and parses JSON log entries
- **Prefix handling**: Extracts JSON from logs with timestamps or other prefixes
- **Flexible timestamp parsing**: Supports multiple timestamp formats (RFC3339, Unix timestamps, etc.)
- **Batching**: Efficiently batches log entries for better performance
- **Custom headers**: Supports additional headers for authentication
- **Insecure connections**: Optional support for insecure connections (useful for development)
- **Service metadata**: Configurable service name and version for telemetry

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

### Recognized JSON Fields

The tool automatically extracts these common fields:

- **Timestamps**: `timestamp`, `ts`, `time` (supports various formats including Unix timestamps)
- **Log levels**: `level`, `lvl`, `severity`
- **Messages**: `message`, `msg`
- **All other fields**: Preserved as log attributes

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

### Example 2: Docker application logs

```bash
# Forward Docker container logs
docker logs -f myapp 2>&1 | ./otel-logger --endpoint localhost:4317 --service-name myapp
```

### Example 3: Application with prefixed logs

```bash
# Handle logs with timestamp prefixes
tail -f /var/log/myapp.log | ./otel-logger \
  --endpoint localhost:4317 \
  --service-name myapp \
  --json-prefix "^\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}[.\\d]*Z?\\s*"
```

### Example 4: High-throughput scenario

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

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

[Add your license information here]

## See Also

- [OpenTelemetry Specification](https://opentelemetry.io/docs/reference/specification/)
- [OpenTelemetry Collector](https://opentelemetry.io/docs/collector/)
- [OpenTelemetry Go Documentation](https://opentelemetry.io/docs/instrumentation/go/)