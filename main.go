package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/alexflint/go-arg"

	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	semconv "go.opentelemetry.io/otel/semconv/v1.32.0"
)

var (
	version   = "dev"
	gitCommit = "unknown"
)

// Config holds all command-line arguments
type Config struct {
	Timeout             time.Duration `arg:"--timeout" default:"10s" help:"Request timeout"`
	JSONPrefix          string        `arg:"--json-prefix" help:"Regex pattern to extract JSON from prefixed logs"`
	BatchSize           int           `arg:"--batch-size" default:"50" help:"Number of log entries to batch before sending"`
	FlushInterval       time.Duration `arg:"--flush-interval" default:"5s" help:"Interval to flush batched logs"`
	TimestampFields     []string      `arg:"--timestamp-fields,separate" help:"JSON field names for timestamps (default: timestamp,ts,time,@timestamp)"`
	LevelFields         []string      `arg:"--level-fields,separate" help:"JSON field names for log levels (default: level,lvl,severity,priority)"`
	MessageFields       []string      `arg:"--message-fields,separate" help:"JSON field names for log messages (default: message,msg,text,content)"`
	PassthroughStdout   bool          `arg:"--passthrough-stdout" help:"Pass command stdout to our stdout in addition to logging"`
	PassthroughStderr   bool          `arg:"--passthrough-stderr" help:"Pass command stderr to our stderr in addition to logging"`
	Verbose             bool          `arg:"--verbose,-v" help:"Enable verbose logging output"`
	ContinuationPattern string        `arg:"--continuation-pattern" default:"^[ \\t]" help:"Regex pattern for continuation lines (default: lines starting with whitespace)"`
	Command             []string      `arg:"positional" help:"Command to execute and capture logs from (if not provided, reads from stdin)"`
}

func (Config) Version() string {
	return fmt.Sprintf("otel-logger %s (commit: %s)", version, gitCommit)
}

func (Config) Description() string {
	return `otel-logger reads logs from stdin or wraps a command and sends logs to an OpenTelemetry collector.
It can handle JSON logs as well as partial JSON with prefixes like timestamps.
Field mappings are configurable to support different logging frameworks.

Configuration uses standard OpenTelemetry environment variables.

Examples:
  # Read from stdin and send JSON logs via gRPC (uses default field mappings)
  OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317 cat app.log | otel-logger

  # Wrap a command and capture both stdout and stderr
  OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317 OTEL_SERVICE_NAME=myapp \
    otel-logger -- ./myapp --config config.yaml

  # Docker entrypoint usage - capture all output from application
  OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector:4317 OTEL_SERVICE_NAME=webapp \
    otel-logger -- npm start

  # Wrap a shell command with custom batching
  OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317 \
    otel-logger --batch-size 100 -- sh -c "while true; do echo 'test'; sleep 1; done"

  # Send logs via HTTP with custom service name
  OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318 OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf \
    OTEL_SERVICE_NAME=myapp tail -f app.log | otel-logger

  # Handle logs with timestamp prefix
  OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317 cat app.log | otel-logger \
    --json-prefix "^\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}Z\\s*"

  # Logstash/ELK format with custom field mappings
  OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317 cat logstash.log | otel-logger \
    --timestamp-fields "@timestamp" --level-fields "level" --message-fields "message"

  # Custom application format with multiple field options
  OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317 cat custom.log | otel-logger \
    --timestamp-fields "created_at,event_time" \
    --level-fields "severity,priority" \
    --message-fields "description,content"

  # Winston.js format
  OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317 cat winston.log | otel-logger \
    --timestamp-fields "timestamp" --level-fields "level" --message-fields "message"

  # Batch logs for better performance
  OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317 cat app.log | otel-logger \
    --batch-size 100 --flush-interval 5s

Field Mapping Defaults:
  Timestamps: timestamp, ts, time, @timestamp
  Levels:     level, lvl, severity, priority
  Messages:   message, msg, text, content

When wrapping commands:
  - stdout logs are tagged with stream=stdout
  - stderr logs are tagged with stream=stderr
  - Command exit code is logged as a final entry
  - Signals are properly forwarded to the wrapped process`
}

// LogEntry represents a parsed log entry
type LogEntry struct {
	Timestamp time.Time
	Level     string
	Message   string
	Fields    map[string]any
	Raw       string
	Stream    string // stdout, stderr, or empty for stdin
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

// LogProcessor wraps the OpenTelemetry logger for stdin processing
type LogProcessor struct {
	logger log.Logger
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
		Fields: make(map[string]any),
		Raw:    line,
	}

	// Extract JSON from the line
	jsonStr := je.ExtractJSON(line)

	// Try to parse as JSON
	var jsonData map[string]any
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

func NewLogProcessor(logger log.Logger) *LogProcessor {
	return &LogProcessor{logger: logger}
}

func (p *LogProcessor) ProcessLogEntry(ctx context.Context, entry *LogEntry) {
	// Create log record using OTEL API
	var record log.Record
	record.SetTimestamp(entry.Timestamp)
	record.SetBody(log.StringValue(entry.Message))
	record.SetSeverityText(entry.Level)
	record.SetSeverity(logLevelToSeverity(entry.Level))

	// Add attributes from parsed fields
	attrs := make([]log.KeyValue, 0, len(entry.Fields)+3)
	for key, value := range entry.Fields {
		attrs = append(attrs, log.String(key, fmt.Sprintf("%v", value)))
	}

	// Add standard attributes
	attrs = append(attrs, log.KeyValueFromAttribute(semconv.LogRecordOriginal(entry.Raw)))

	// Add stream information if available
	if entry.Stream != "" {
		attrs = append(attrs, log.KeyValueFromAttribute(semconv.LogIostreamKey.String(entry.Stream)))
	}

	record.AddAttributes(attrs...)

	// Emit the record through OTEL SDK
	p.logger.Emit(ctx, record)
}

func logLevelToSeverity(level string) log.Severity {
	switch strings.ToLower(level) {
	case "trace":
		return log.SeverityTrace1
	case "debug":
		return log.SeverityDebug1
	case "info":
		return log.SeverityInfo1
	case "warn", "warning":
		return log.SeverityWarn1
	case "error":
		return log.SeverityError1
	case "fatal":
		return log.SeverityFatal1
	default:
		return log.SeverityInfo1
	}
}

func createExporter(ctx context.Context) (sdklog.Exporter, error) {
	protocol := "http/protobuf"
	if proto, ok := os.LookupEnv("OTEL_EXPORTER_OTLP_LOGS_PROTOCOL"); ok {
		protocol = proto
	} else if proto, ok := os.LookupEnv("OTEL_EXPORTER_OTLP_PROTOCOL"); ok {
		protocol = proto
	}
	switch strings.ToLower(protocol) {
	case "grpc":
		return otlploggrpc.New(ctx)
	case "http", "http/protobuf", "http/json":
		return otlploghttp.New(ctx)
	default:
		return nil, fmt.Errorf("unsupported protocol (supported: grpc, http/protobuf, http/json): %s", protocol)
	}
}

func createLoggerProvider(ctx context.Context, config *Config) (*sdklog.LoggerProvider, error) {
	exporter, err := createExporter(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create exporter: %w", err)
	}

	// Create processor with batching configuration
	processor := sdklog.NewBatchProcessor(exporter,
		sdklog.WithExportMaxBatchSize(config.BatchSize),
		sdklog.WithExportInterval(config.FlushInterval),
		sdklog.WithExportTimeout(config.Timeout),
	)

	// Create logger provider
	provider := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(processor),
	)

	return provider, nil
}

func getDefaultFieldMappings() *FieldMappings {
	return &FieldMappings{
		TimestampFields: []string{"timestamp", "ts", "time", "@timestamp"},
		LevelFields:     []string{"level", "lvl", "severity", "priority"},
		MessageFields:   []string{"message", "msg", "text", "content"},
	}
}

// Logging helper functions
func logInfo(verbose bool, format string, args ...any) {
	if verbose {
		fmt.Fprintf(os.Stderr, format, args...)
	}
}

func logError(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format, args...)
}

func logDebug(verbose bool, format string, args ...any) {
	if verbose {
		fmt.Fprintf(os.Stderr, format, args...)
	}
}

// multilineLogIterator creates an iterator that combines multiline log entries
// based on improved heuristics for detecting log entry starts
func multilineLogIterator(reader io.Reader, continuationPattern *regexp.Regexp) iter.Seq[string] {

	isLogEntryStart := func(line string) bool {
		// Empty lines are not log starts
		if len(line) == 0 {
			return false
		}

		// Lines starting with whitespace are usually continuations
		if continuationPattern.MatchString(line) {
			return false
		}

		return true
	}

	return func(yield func(string) bool) {
		scanner := bufio.NewScanner(reader)
		var currentEntry strings.Builder

		for scanner.Scan() {
			line := scanner.Text()

			// Skip completely empty lines
			if len(line) == 0 {
				continue
			}

			// Check if this line starts a new log entry
			if isLogEntryStart(line) {
				// If we have a current entry, yield it first
				if currentEntry.Len() > 0 {
					if !yield(currentEntry.String()) {
						return
					}
					currentEntry.Reset()
				}
				// Start new entry
				currentEntry.WriteString(line)
			} else if currentEntry.Len() > 0 {
				// This is a continuation line and we have an active entry, append to it
				currentEntry.WriteString("\n")
				currentEntry.WriteString(line)
			}
			// If currentEntry.Len() == 0 and line is not a log start,
			// we ignore it as it's likely orphaned continuation
		}

		// Yield the final entry if we have one
		if currentEntry.Len() > 0 {
			yield(currentEntry.String())
		}
	}
}

func processLogs(ctx context.Context, config *Config, extractor *JSONExtractor, processor *LogProcessor) error {
	continuationPattern, err := regexp.Compile(config.ContinuationPattern)
	if err != nil {
		return fmt.Errorf("failed to compile continuation pattern: %w", err)
	}

	for logEntry := range multilineLogIterator(os.Stdin, continuationPattern) {
		entry, err := extractor.ParseLogEntry(logEntry)
		if err != nil {
			logError("Error parsing log entry: %v\n", err)
			continue
		}

		processor.ProcessLogEntry(ctx, entry)
	}

	return nil
}

// processStream processes logs from a single stream (stdout or stderr)
func processStream(ctx context.Context, reader io.Reader, stream string, extractor *JSONExtractor, processor *LogProcessor, wg *sync.WaitGroup, passthrough bool, output io.Writer, continuationPattern *regexp.Regexp) {
	defer wg.Done()

	for logEntry := range multilineLogIterator(reader, continuationPattern) {
		// If passthrough is enabled, write to output
		if passthrough && output != nil {
			fmt.Fprintln(output, logEntry)
		}

		entry, err := extractor.ParseLogEntry(logEntry)
		if err != nil {
			logError("Error parsing log entry from %s: %v\n", stream, err)
			continue
		}

		// Tag with stream information
		entry.Stream = stream

		processor.ProcessLogEntry(ctx, entry)
	}
}

// executeCommand executes the given command and processes its output
func executeCommand(ctx context.Context, config *Config, extractor *JSONExtractor, processor *LogProcessor) error {
	if len(config.Command) == 0 {
		return fmt.Errorf("no command specified")
	}

	continuationPattern, err := regexp.Compile(config.ContinuationPattern)
	if err != nil {
		return fmt.Errorf("failed to compile continuation pattern: %w", err)
	}

	// Create command
	var cmd *exec.Cmd
	if len(config.Command) == 1 {
		cmd = exec.CommandContext(ctx, config.Command[0])
	} else {
		cmd = exec.CommandContext(ctx, config.Command[0], config.Command[1:]...)
	}

	// Create pipes for stdout and stderr
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	cmd.Stdin = os.Stdin

	// Start the command
	logInfo(config.Verbose, "Starting command: %s\n", strings.Join(config.Command, " "))
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start command: %w", err)
	}

	// Process streams concurrently
	var wg sync.WaitGroup
	wg.Add(2)

	go processStream(ctx, stdoutPipe, "stdout", extractor, processor, &wg, config.PassthroughStdout, os.Stdout, continuationPattern)
	go processStream(ctx, stderrPipe, "stderr", extractor, processor, &wg, config.PassthroughStderr, os.Stderr, continuationPattern)

	// Set up signal forwarding
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Wait for command completion or signal
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	var cmdErr error
	select {
	case sig := <-sigChan:
		logInfo(config.Verbose, "Received signal %v, forwarding to process...\n", sig)
		if cmd.Process != nil {
			cmd.Process.Signal(sig)
		}
		cmdErr = <-done
	case cmdErr = <-done:
		// Command completed normally
	}

	// Wait for stream processing to complete
	wg.Wait()

	// Log the command exit
	exitCode := 0
	if cmdErr != nil {
		if exitError, ok := cmdErr.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
		}
	}

	// Create a log entry for the command completion
	exitEntry := &LogEntry{
		Timestamp: time.Now(),
		Level:     "info",
		Message:   fmt.Sprintf("Command completed with exit code %d", exitCode),
		Fields: map[string]any{
			"command":     strings.Join(config.Command, " "),
			"exit_code":   exitCode,
			"exit_status": cmdErr != nil,
		},
		Raw:    fmt.Sprintf("Command exit: %d", exitCode),
		Stream: "system",
	}

	processor.ProcessLogEntry(ctx, exitEntry)

	logInfo(config.Verbose, "Command completed with exit code: %d\n", exitCode)

	if cmdErr != nil && exitCode != 0 {
		return fmt.Errorf("command failed with exit code %d", exitCode)
	}

	return nil
}

func runCommand(config *Config) error {
	ctx := context.Background()

	// Create logger provider using OTEL SDK
	provider, err := createLoggerProvider(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to create logger provider: %w", err)
	}
	defer func() {
		if err := provider.Shutdown(ctx); err != nil {
			logError("Error shutting down logger provider: %v\n", err)
		}
	}()

	// Create logger and processor
	logger := provider.Logger("otel-logger")
	processor := NewLogProcessor(logger)

	// Create field mappings
	fieldMappings := getDefaultFieldMappings()
	if len(config.TimestampFields) > 0 {
		fieldMappings.TimestampFields = config.TimestampFields
	}
	if len(config.MessageFields) > 0 {
		fieldMappings.MessageFields = config.MessageFields
	}
	if len(config.LevelFields) > 0 {
		fieldMappings.LevelFields = config.LevelFields
	}

	// Create JSON extractor
	extractor := NewJSONExtractor(config.JSONPrefix, fieldMappings)

	logInfo(config.Verbose, "Field mappings - Timestamp: %v, Level: %v, Message: %v\n",
		fieldMappings.TimestampFields, fieldMappings.LevelFields, fieldMappings.MessageFields)

	var processingErr error

	// Check if we should execute a command or read from stdin
	if len(config.Command) > 0 {
		// Execute command and process its output
		logInfo(config.Verbose, "Executing command and sending logs (batch_size=%d)\n", config.BatchSize)
		processingErr = executeCommand(ctx, config, extractor, processor)
	} else {
		// Process logs from stdin
		logInfo(config.Verbose, "Reading logs from stdin and sending (batch_size=%d)\n", config.BatchSize)
		processingErr = processLogs(ctx, config, extractor, processor)
	}

	// Force flush before exit
	if err := provider.ForceFlush(ctx); err != nil {
		return fmt.Errorf("failed to flush logs: %w", err)
	}

	logInfo(config.Verbose, "Finished processing logs and flushed to collector\n")

	if processingErr != nil {
		return processingErr
	}

	return nil
}

func main() {
	var config Config
	arg.MustParse(&config)

	if err := runCommand(&config); err != nil {
		logError("%s\n", err.Error())
		os.Exit(1)
	}
}
