package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/alexflint/go-arg"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	grpcinsecure "google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"

	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
)

var (
	version   = "dev"
	buildTime = "unknown"
	gitCommit = "unknown"
)

// Config holds all command-line arguments
type Config struct {
	Endpoint        string        `arg:"-e,--endpoint" default:"localhost:4317" help:"OpenTelemetry collector endpoint"`
	Protocol        string        `arg:"-p,--protocol" default:"grpc" help:"Protocol to use (grpc or http)"`
	ServiceName     string        `arg:"--service-name" default:"otel-logger" help:"Service name for telemetry"`
	ServiceVersion  string        `arg:"--service-version" default:"1.0.0" help:"Service version for telemetry"`
	Insecure        bool          `arg:"--insecure" help:"Use insecure connection"`
	Timeout         time.Duration `arg:"--timeout" default:"10s" help:"Request timeout"`
	Headers         []string      `arg:"--header,separate" help:"Additional headers (key=value)"`
	JSONPrefix      string        `arg:"--json-prefix" help:"Regex pattern to extract JSON from prefixed logs"`
	BatchSize       int           `arg:"--batch-size" default:"50" help:"Number of log entries to batch before sending"`
	FlushInterval   time.Duration `arg:"--flush-interval" default:"5s" help:"Interval to flush batched logs"`
	TimestampFields []string      `arg:"--timestamp-fields,separate" help:"JSON field names for timestamps (default: timestamp,ts,time,@timestamp)"`
	LevelFields     []string      `arg:"--level-fields,separate" help:"JSON field names for log levels (default: level,lvl,severity,priority)"`
	MessageFields   []string      `arg:"--message-fields,separate" help:"JSON field names for log messages (default: message,msg,text,content)"`
	ShowVersion     bool          `arg:"--version" help:"Show version information"`
}

func (Config) Version() string {
	return fmt.Sprintf("otel-logger %s (built: %s, commit: %s)", version, buildTime, gitCommit)
}

func (Config) Description() string {
	return `otel-logger reads logs from stdin and sends them to an OpenTelemetry collector.
It can handle JSON logs as well as partial JSON with prefixes like timestamps.
Field mappings are configurable to support different logging frameworks.

Examples:
  # Send JSON logs via gRPC (uses default field mappings)
  cat app.log | otel-logger --endpoint localhost:4317 --protocol grpc

  # Send logs via HTTP with custom service name
  tail -f app.log | otel-logger --endpoint http://localhost:4318 --protocol http --service-name myapp

  # Handle logs with timestamp prefix
  cat app.log | otel-logger --endpoint localhost:4317 --json-prefix "^\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}Z\\s*"

  # Logstash/ELK format with custom field mappings
  cat logstash.log | otel-logger --endpoint localhost:4317 \
    --timestamp-fields "@timestamp" --level-fields "level" --message-fields "message"

  # Custom application format with multiple field options
  cat custom.log | otel-logger --endpoint localhost:4317 \
    --timestamp-fields "created_at,event_time" \
    --level-fields "severity,priority" \
    --message-fields "description,content"

  # Winston.js format
  cat winston.log | otel-logger --endpoint localhost:4317 \
    --timestamp-fields "timestamp" --level-fields "level" --message-fields "message"

  # Batch logs for better performance
  cat app.log | otel-logger --endpoint localhost:4317 --batch-size 100 --flush-interval 5s

Field Mapping Defaults:
  Timestamps: timestamp, ts, time, @timestamp
  Levels:     level, lvl, severity, priority
  Messages:   message, msg, text, content`
}

// LogEntry represents a parsed log entry
type LogEntry struct {
	Timestamp time.Time
	Level     string
	Message   string
	Fields    map[string]interface{}
	Raw       string
}

// FieldMappings defines configurable field name mappings for JSON log parsing
type FieldMappings struct {
	TimestampFields []string
	LevelFields     []string
	MessageFields   []string
}

// JSONExtractor helps extract JSON from potentially prefixed log lines
type JSONExtractor struct {
	prefixRegex   *regexp.Regexp
	fieldMappings *FieldMappings
}

// LogBatcher batches log entries for efficient sending
type LogBatcher struct {
	entries       []*LogEntry
	maxSize       int
	flushInterval time.Duration
	flushFunc     func([]*LogEntry) error
	ticker        *time.Ticker
	done          chan struct{}
}

func NewJSONExtractor(prefix string, fieldMappings *FieldMappings) *JSONExtractor {
	var regex *regexp.Regexp
	if prefix != "" {
		regex = regexp.MustCompile(prefix)
	} else {
		// Default pattern to match common timestamp prefixes
		regex = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}[T\s]\d{2}:\d{2}:\d{2}[.\d]*[Z\-+\d:]*\s*)?(.*)$`)
	}
	return &JSONExtractor{
		prefixRegex:   regex,
		fieldMappings: fieldMappings,
	}
}

func (je *JSONExtractor) ExtractJSON(line string) string {
	matches := je.prefixRegex.FindStringSubmatch(line)
	if len(matches) == 0 {
		return line
	}

	// If we have groups, the last group should be the JSON part
	if len(matches) > 1 {
		jsonPart := matches[len(matches)-1]
		if jsonPart != "" {
			return jsonPart
		}
	}

	return line
}

func (je *JSONExtractor) ParseLogEntry(line string) (*LogEntry, error) {
	entry := &LogEntry{
		Fields: make(map[string]interface{}),
		Raw:    line,
	}

	// Extract JSON from the line
	jsonStr := je.ExtractJSON(line)

	// Try to parse as JSON
	var jsonData map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &jsonData); err != nil {
		// If JSON parsing fails, treat the entire line as a message
		entry.Message = strings.TrimSpace(line)
		entry.Timestamp = time.Now()
		entry.Level = "info"
		return entry, nil
	}

	// Extract timestamp using configurable field mappings
	timestampExtracted := false
	for _, field := range je.fieldMappings.TimestampFields {
		if timestampStr, ok := jsonData[field].(string); ok {
			if t, err := parseTimestamp(timestampStr); err == nil {
				entry.Timestamp = t
				timestampExtracted = true
			}
			delete(jsonData, field)
			break
		} else if timestampNum, ok := jsonData[field].(float64); ok {
			entry.Timestamp = time.Unix(int64(timestampNum), 0)
			timestampExtracted = true
			delete(jsonData, field)
			break
		}
	}

	if !timestampExtracted || entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	// Extract level using configurable field mappings
	levelExtracted := false
	for _, field := range je.fieldMappings.LevelFields {
		if level, ok := jsonData[field].(string); ok {
			entry.Level = level
			levelExtracted = true
			delete(jsonData, field)
			break
		}
	}
	if !levelExtracted {
		entry.Level = "info"
	}

	// Extract message using configurable field mappings
	messageExtracted := false
	for _, field := range je.fieldMappings.MessageFields {
		if message, ok := jsonData[field].(string); ok {
			entry.Message = message
			messageExtracted = true
			delete(jsonData, field)
			break
		}
	}
	if !messageExtracted {
		entry.Message = "Log entry"
	}

	// Store remaining fields
	entry.Fields = jsonData

	return entry, nil
}

func parseTimestamp(timeStr string) (time.Time, error) {
	// Try different timestamp formats
	formats := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02T15:04:05.000Z07:00",
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, timeStr); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse timestamp: %s", timeStr)
}

func NewLogBatcher(maxSize int, flushInterval time.Duration, flushFunc func([]*LogEntry) error) *LogBatcher {
	batcher := &LogBatcher{
		entries:       make([]*LogEntry, 0, maxSize),
		maxSize:       maxSize,
		flushInterval: flushInterval,
		flushFunc:     flushFunc,
		done:          make(chan struct{}),
	}

	if flushInterval > 0 {
		batcher.ticker = time.NewTicker(flushInterval)
		go batcher.flushLoop()
	}

	return batcher
}

func (b *LogBatcher) Add(entry *LogEntry) error {
	b.entries = append(b.entries, entry)
	if len(b.entries) >= b.maxSize {
		return b.flush()
	}
	return nil
}

func (b *LogBatcher) Flush() error {
	return b.flush()
}

func (b *LogBatcher) flush() error {
	if len(b.entries) == 0 {
		return nil
	}

	entries := make([]*LogEntry, len(b.entries))
	copy(entries, b.entries)
	b.entries = b.entries[:0]

	return b.flushFunc(entries)
}

func (b *LogBatcher) flushLoop() {
	for {
		select {
		case <-b.ticker.C:
			if err := b.flush(); err != nil {
				fmt.Fprintf(os.Stderr, "Error flushing logs: %v\n", err)
			}
		case <-b.done:
			return
		}
	}
}

func (b *LogBatcher) Close() error {
	if b.ticker != nil {
		b.ticker.Stop()
		close(b.done)
	}
	return b.flush()
}

func logLevelToSeverity(level string) logspb.SeverityNumber {
	switch strings.ToLower(level) {
	case "trace":
		return logspb.SeverityNumber_SEVERITY_NUMBER_TRACE
	case "debug":
		return logspb.SeverityNumber_SEVERITY_NUMBER_DEBUG
	case "info":
		return logspb.SeverityNumber_SEVERITY_NUMBER_INFO
	case "warn", "warning":
		return logspb.SeverityNumber_SEVERITY_NUMBER_WARN
	case "error":
		return logspb.SeverityNumber_SEVERITY_NUMBER_ERROR
	case "fatal":
		return logspb.SeverityNumber_SEVERITY_NUMBER_FATAL
	default:
		return logspb.SeverityNumber_SEVERITY_NUMBER_INFO
	}
}

func valueToAnyValue(value interface{}) *commonpb.AnyValue {
	switch v := value.(type) {
	case string:
		return &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: v}}
	case bool:
		return &commonpb.AnyValue{Value: &commonpb.AnyValue_BoolValue{BoolValue: v}}
	case int:
		return &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: int64(v)}}
	case int64:
		return &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: v}}
	case float64:
		return &commonpb.AnyValue{Value: &commonpb.AnyValue_DoubleValue{DoubleValue: v}}
	default:
		return &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: fmt.Sprintf("%v", v)}}
	}
}

func createExportRequest(entries []*LogEntry, config *Config) *collogspb.ExportLogsServiceRequest {
	// Create resource
	resource := &resourcepb.Resource{
		Attributes: []*commonpb.KeyValue{
			{
				Key:   "service.name",
				Value: valueToAnyValue(config.ServiceName),
			},
			{
				Key:   "service.version",
				Value: valueToAnyValue(config.ServiceVersion),
			},
		},
	}

	var logRecords []*logspb.LogRecord
	for _, entry := range entries {
		// Create attributes from fields
		var attributes []*commonpb.KeyValue
		for key, value := range entry.Fields {
			attributes = append(attributes, &commonpb.KeyValue{
				Key:   key,
				Value: valueToAnyValue(value),
			})
		}

		// Add standard attributes
		attributes = append(attributes, &commonpb.KeyValue{
			Key:   "log.level",
			Value: valueToAnyValue(entry.Level),
		})

		logRecord := &logspb.LogRecord{
			TimeUnixNano:   uint64(entry.Timestamp.UnixNano()),
			SeverityNumber: logLevelToSeverity(entry.Level),
			SeverityText:   entry.Level,
			Body:           valueToAnyValue(entry.Message),
			Attributes:     attributes,
		}

		logRecords = append(logRecords, logRecord)
	}

	scopeLogs := &logspb.ScopeLogs{
		Scope: &commonpb.InstrumentationScope{
			Name:    "otel-logger",
			Version: "1.0.0",
		},
		LogRecords: logRecords,
	}

	resourceLogs := &logspb.ResourceLogs{
		Resource:  resource,
		ScopeLogs: []*logspb.ScopeLogs{scopeLogs},
	}

	return &collogspb.ExportLogsServiceRequest{
		ResourceLogs: []*logspb.ResourceLogs{resourceLogs},
	}
}

func sendLogsHTTP(entries []*LogEntry, config *Config) error {
	req := createExportRequest(entries, config)
	data, err := proto.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	url := config.Endpoint
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		if config.Insecure {
			url = "http://" + url
		} else {
			url = "https://" + url
		}
	}
	if !strings.HasSuffix(url, "/v1/logs") {
		url = strings.TrimSuffix(url, "/") + "/v1/logs"
	}

	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/x-protobuf")
	for _, header := range config.Headers {
		parts := strings.SplitN(header, "=", 2)
		if len(parts) == 2 {
			httpReq.Header.Set(parts[0], parts[1])
		}
	}

	client := &http.Client{
		Timeout: config.Timeout,
	}
	if config.Insecure {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func sendLogsGRPC(entries []*LogEntry, config *Config) error {
	var creds credentials.TransportCredentials
	if config.Insecure {
		creds = grpcinsecure.NewCredentials()
	} else {
		creds = credentials.NewTLS(&tls.Config{})
	}

	conn, err := grpc.Dial(config.Endpoint, grpc.WithTransportCredentials(creds))
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer conn.Close()

	client := collogspb.NewLogsServiceClient(conn)
	req := createExportRequest(entries, config)

	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	_, err = client.Export(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to export logs: %w", err)
	}

	return nil
}

func getDefaultFieldMappings() *FieldMappings {
	return &FieldMappings{
		TimestampFields: []string{"timestamp", "ts", "time", "@timestamp"},
		LevelFields:     []string{"level", "lvl", "severity", "priority"},
		MessageFields:   []string{"message", "msg", "text", "content"},
	}
}

func processLogs(extractor *JSONExtractor, batcher *LogBatcher) error {
	scanner := bufio.NewScanner(os.Stdin)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		entry, err := extractor.ParseLogEntry(line)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing log entry: %v\n", err)
			continue
		}

		if err := batcher.Add(entry); err != nil {
			fmt.Fprintf(os.Stderr, "Error adding log entry to batch: %v\n", err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading from stdin: %w", err)
	}

	return nil
}

func runCommand(config *Config) error {
	// Create flush function based on protocol
	var flushFunc func([]*LogEntry) error
	switch strings.ToLower(config.Protocol) {
	case "grpc":
		flushFunc = func(entries []*LogEntry) error {
			return sendLogsGRPC(entries, config)
		}
	case "http":
		flushFunc = func(entries []*LogEntry) error {
			return sendLogsHTTP(entries, config)
		}
	default:
		return fmt.Errorf("unsupported protocol: %s (supported: grpc, http)", config.Protocol)
	}

	// Create batcher
	batcher := NewLogBatcher(config.BatchSize, config.FlushInterval, flushFunc)
	defer batcher.Close()

	// Create field mappings
	var fieldMappings *FieldMappings
	if len(config.TimestampFields) > 0 || len(config.LevelFields) > 0 || len(config.MessageFields) > 0 {
		fieldMappings = &FieldMappings{
			TimestampFields: config.TimestampFields,
			LevelFields:     config.LevelFields,
			MessageFields:   config.MessageFields,
		}
		// Use defaults for any empty fields
		if len(fieldMappings.TimestampFields) == 0 {
			fieldMappings.TimestampFields = getDefaultFieldMappings().TimestampFields
		}
		if len(fieldMappings.LevelFields) == 0 {
			fieldMappings.LevelFields = getDefaultFieldMappings().LevelFields
		}
		if len(fieldMappings.MessageFields) == 0 {
			fieldMappings.MessageFields = getDefaultFieldMappings().MessageFields
		}
	} else {
		fieldMappings = getDefaultFieldMappings()
	}

	// Create JSON extractor
	extractor := NewJSONExtractor(config.JSONPrefix, fieldMappings)

	// Process logs from stdin
	fmt.Fprintf(os.Stderr, "Reading logs from stdin and sending to %s://%s (batch_size=%d)\n", config.Protocol, config.Endpoint, config.BatchSize)
	fmt.Fprintf(os.Stderr, "Field mappings - Timestamp: %v, Level: %v, Message: %v\n",
		fieldMappings.TimestampFields, fieldMappings.LevelFields, fieldMappings.MessageFields)
	if err := processLogs(extractor, batcher); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "Finished reading logs, flushing remaining entries...\n")
	return nil
}

func main() {
	var config Config
	arg.MustParse(&config)

	if config.ShowVersion {
		fmt.Println(config.Version())
		return
	}

	if err := runCommand(&config); err != nil {
		log.Fatal(err)
	}
}
