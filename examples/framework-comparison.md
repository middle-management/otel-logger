# Logging Framework Compatibility Guide

This guide shows how to configure `otel-logger` to work with different logging frameworks and their specific JSON field naming conventions.

## Overview

The `otel-logger` tool supports configurable field mappings, making it compatible with virtually any JSON logging format. This document provides configuration examples for popular logging frameworks.

## Default Field Mappings

If no custom field mappings are specified, `otel-logger` uses these defaults:

- **Timestamps**: `timestamp`, `ts`, `time`, `@timestamp`
- **Log Levels**: `level`, `lvl`, `severity`, `priority`
- **Messages**: `message`, `msg`, `text`, `content`

## Framework-Specific Configurations

### 1. Logstash / ELK Stack

**Typical Format:**
```json
{"@timestamp": "2024-01-15T10:30:45.123Z", "level": "INFO", "message": "User authenticated", "host": "web-01", "service": "auth-service"}
```

**Configuration:**
```bash
cat logstash.log | otel-logger --endpoint localhost:4317 \
  --timestamp-fields "@timestamp" \
  --level-fields "level" \
  --message-fields "message"
```

### 2. Winston (Node.js)

**Typical Format:**
```json
{"timestamp": "2024-01-15T10:30:45.123Z", "level": "info", "message": "HTTP request processed", "meta": {"method": "POST", "url": "/api/users"}}
```

**Configuration:**
```bash
cat winston.log | otel-logger --endpoint localhost:4317 \
  --timestamp-fields "timestamp" \
  --level-fields "level" \
  --message-fields "message"
```

### 3. Structured Logging (Go - logrus/zap)

**Logrus Format:**
```json
{"time": "2024-01-15T10:30:45.123Z", "level": "info", "msg": "Database connection established", "duration": "2.5ms"}
```

**Zap Format:**
```json
{"ts": 1705315845.123, "level": "info", "msg": "Request processed", "caller": "main.go:42"}
```

**Configuration:**
```bash
# For logrus
cat logrus.log | otel-logger --endpoint localhost:4317 \
  --timestamp-fields "time" \
  --level-fields "level" \
  --message-fields "msg"

# For zap
cat zap.log | otel-logger --endpoint localhost:4317 \
  --timestamp-fields "ts" \
  --level-fields "level" \
  --message-fields "msg"
```

### 4. Python Logging (JSON formatter)

**Typical Format:**
```json
{"asctime": "2024-01-15 10:30:45,123", "levelname": "INFO", "message": "Processing started", "name": "myapp", "funcName": "process_data"}
```

**Configuration:**
```bash
cat python.log | otel-logger --endpoint localhost:4317 \
  --timestamp-fields "asctime" \
  --level-fields "levelname" \
  --message-fields "message"
```

### 5. Java Logback (JSON encoder)

**Typical Format:**
```json
{"@timestamp": "2024-01-15T10:30:45.123+00:00", "@level": "INFO", "@message": "Application started", "logger": "com.example.App", "thread": "main"}
```

**Configuration:**
```bash
cat logback.log | otel-logger --endpoint localhost:4317 \
  --timestamp-fields "@timestamp" \
  --level-fields "@level" \
  --message-fields "@message"
```

### 6. .NET Serilog

**Typical Format:**
```json
{"@t": "2024-01-15T10:30:45.1234567Z", "@l": "Information", "@m": "User {UserId} logged in", "UserId": 12345, "SourceContext": "MyApp.Controllers.AuthController"}
```

**Configuration:**
```bash
cat serilog.log | otel-logger --endpoint localhost:4317 \
  --timestamp-fields "@t" \
  --level-fields "@l" \
  --message-fields "@m"
```

### 7. Fluentd

**Typical Format:**
```json
{"time": "2024-01-15T10:30:45.123Z", "level": "INFO", "message": "Data processed", "tag": "app.processing", "host": "worker-01"}
```

**Configuration:**
```bash
cat fluentd.log | otel-logger --endpoint localhost:4317 \
  --timestamp-fields "time" \
  --level-fields "level" \
  --message-fields "message"
```

### 8. Bunyan (Node.js)

**Typical Format:**
```json
{"name": "myapp", "hostname": "server-01", "pid": 1234, "level": 30, "msg": "Request received", "time": "2024-01-15T10:30:45.123Z", "v": 0}
```

**Configuration:**
```bash
cat bunyan.log | otel-logger --endpoint localhost:4317 \
  --timestamp-fields "time" \
  --level-fields "level" \
  --message-fields "msg"
```

### 9. Pino (Node.js)

**Typical Format:**
```json
{"level": 30, "time": 1705315845123, "pid": 1234, "hostname": "server-01", "msg": "Request completed", "reqId": "req-123"}
```

**Configuration:**
```bash
cat pino.log | otel-logger --endpoint localhost:4317 \
  --timestamp-fields "time" \
  --level-fields "level" \
  --message-fields "msg"
```

### 10. Custom Application Formats

#### E-commerce Application
```json
{"event_time": "2024-01-15T10:30:45Z", "severity": "high", "description": "Payment failed", "order_id": "ord-123", "amount": 99.99}
```

```bash
cat ecommerce.log | otel-logger --endpoint localhost:4317 \
  --timestamp-fields "event_time" \
  --level-fields "severity" \
  --message-fields "description"
```

#### Monitoring System
```json
{"occurred_at": "2024-01-15T10:30:45Z", "alert_level": "critical", "alert_message": "CPU usage exceeded 90%", "host": "web-01", "metric": "cpu.usage"}
```

```bash
cat monitoring.log | otel-logger --endpoint localhost:4317 \
  --timestamp-fields "occurred_at" \
  --level-fields "alert_level" \
  --message-fields "alert_message"
```

## Multiple Field Support

You can specify multiple field names for each category. The tool will use the first field it finds:

```bash
# Handle logs that might use different field names
cat mixed.log | otel-logger --endpoint localhost:4317 \
  --timestamp-fields "timestamp,@timestamp,time,ts,created_at" \
  --level-fields "level,severity,priority,@level,log_level" \
  --message-fields "message,msg,text,description,@message"
```

## Prefixed Logs with Custom Fields

Some applications prefix their JSON logs with timestamps or other information:

```bash
# Handle prefixed logs with custom fields
cat prefixed.log | otel-logger --endpoint localhost:4317 \
  --json-prefix "^\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}[.\\d]*Z?\\s*" \
  --timestamp-fields "@timestamp,time" \
  --level-fields "level,severity" \
  --message-fields "message,msg"
```

## Performance Considerations

For high-volume logs, consider adjusting batch settings:

```bash
cat high-volume.log | otel-logger --endpoint localhost:4317 \
  --timestamp-fields "timestamp" \
  --level-fields "level" \
  --message-fields "message" \
  --batch-size 500 \
  --flush-interval 1s
```

## Testing Your Configuration

To test your field mappings without a real OpenTelemetry collector:

```bash
# Test with a non-existent endpoint to see field mapping output
echo '{"your_timestamp": "2024-01-15T10:30:45Z", "your_level": "info", "your_message": "test"}' | \
  otel-logger --endpoint "test:4317" \
  --timestamp-fields "your_timestamp" \
  --level-fields "your_level" \
  --message-fields "your_message"
```

The tool will show you which field mappings it's using before attempting to connect.

## Framework Detection Script

Here's a simple script to help identify your log format:

```bash
#!/bin/bash
# analyze_logs.sh - Analyze JSON log format

echo "Analyzing first 5 log entries..."
head -5 "$1" | while read -r line; do
    echo "--- Log Entry ---"
    echo "$line" | jq -r 'keys[]' | sort
    echo
done
```

Usage:
```bash
./analyze_logs.sh myapp.log
```

This will show you all the JSON keys in your logs, helping you identify the correct field names for timestamps, levels, and messages.