package logging

import (
	"context"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// ZapLogger implements Logger interface using uber-go/zap
type ZapLogger struct {
	logger *zap.Logger
	sugar  *zap.SugaredLogger
	fields []Field
}

// NewZapLogger creates a new zap-based logger
func NewZapLogger(config Config) (*ZapLogger, error) {
	var zapConfig zap.Config

	if config.Format == "json" {
		zapConfig = zap.NewProductionConfig()
	} else {
		zapConfig = zap.NewDevelopmentConfig()
	}

	// Set log level
	zapConfig.Level = zap.NewAtomicLevelAt(toZapLevel(config.Level))

	// Configure encoder
	zapConfig.EncoderConfig.TimeKey = "timestamp"
	zapConfig.EncoderConfig.EncodeTime = zapcore.RFC3339TimeEncoder
	zapConfig.EncoderConfig.StacktraceKey = "stacktrace"

	logger, err := zapConfig.Build(
		zap.AddCallerSkip(1), // Skip wrapper functions
	)
	if err != nil {
		return nil, err
	}

	return &ZapLogger{
		logger: logger,
		sugar:  logger.Sugar(),
		fields: nil,
	}, nil
}

// NewZapLoggerWithWriter creates a logger that writes to a specific writer
func NewZapLoggerWithWriter(config Config) (*ZapLogger, error) {
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "timestamp",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "message",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.RFC3339TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	var encoder zapcore.Encoder
	if config.Format == "json" {
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	} else {
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	}

	output := config.Output
	if output == nil {
		output = os.Stdout
	}

	core := zapcore.NewCore(
		encoder,
		zapcore.AddSync(output),
		toZapLevel(config.Level),
	)

	logger := zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))

	return &ZapLogger{
		logger: logger,
		sugar:  logger.Sugar(),
		fields: nil,
	}, nil
}

// Debug logs a debug message
func (l *ZapLogger) Debug(msg string, fields ...Field) {
	l.logger.Debug(msg, toZapFields(append(l.fields, fields...))...)
}

// Info logs an info message
func (l *ZapLogger) Info(msg string, fields ...Field) {
	l.logger.Info(msg, toZapFields(append(l.fields, fields...))...)
}

// Warn logs a warning message
func (l *ZapLogger) Warn(msg string, fields ...Field) {
	l.logger.Warn(msg, toZapFields(append(l.fields, fields...))...)
}

// Error logs an error message
func (l *ZapLogger) Error(msg string, fields ...Field) {
	l.logger.Error(msg, toZapFields(append(l.fields, fields...))...)
}

// Fatal logs a fatal message and exits
func (l *ZapLogger) Fatal(msg string, fields ...Field) {
	l.logger.Fatal(msg, toZapFields(append(l.fields, fields...))...)
}

// With returns a logger with additional fields
func (l *ZapLogger) With(fields ...Field) Logger {
	newFields := make([]Field, len(l.fields)+len(fields))
	copy(newFields, l.fields)
	copy(newFields[len(l.fields):], fields)

	return &ZapLogger{
		logger: l.logger,
		sugar:  l.sugar,
		fields: newFields,
	}
}

// WithContext returns a logger that extracts correlation IDs from context
func (l *ZapLogger) WithContext(ctx context.Context) Logger {
	if ctx == nil {
		return l
	}

	var contextFields []Field

	if correlationID := GetCorrelationID(ctx); correlationID != "" {
		contextFields = append(contextFields, String("correlation_id", correlationID))
	}

	if traceID := GetTraceID(ctx); traceID != "" {
		contextFields = append(contextFields, String("trace_id", traceID))
	}

	if agentID := GetAgentID(ctx); agentID != "" {
		contextFields = append(contextFields, String("agent_id", agentID))
	}

	if len(contextFields) == 0 {
		return l
	}

	return l.With(contextFields...)
}

// Sync flushes any buffered log entries
func (l *ZapLogger) Sync() error {
	return l.logger.Sync()
}

// toZapLevel converts LogLevel to zapcore.Level
func toZapLevel(level LogLevel) zapcore.Level {
	switch level {
	case DebugLevel:
		return zapcore.DebugLevel
	case InfoLevel:
		return zapcore.InfoLevel
	case WarnLevel:
		return zapcore.WarnLevel
	case ErrorLevel:
		return zapcore.ErrorLevel
	case FatalLevel:
		return zapcore.FatalLevel
	default:
		return zapcore.InfoLevel
	}
}

// toZapFields converts Field slice to zap.Field slice
func toZapFields(fields []Field) []zap.Field {
	zapFields := make([]zap.Field, len(fields))
	for i, f := range fields {
		zapFields[i] = zap.Any(f.Key, f.Value)
	}
	return zapFields
}

// Global logger instance
var globalLogger Logger

// SetGlobalLogger sets the global logger instance
func SetGlobalLogger(logger Logger) {
	globalLogger = logger
}

// GetGlobalLogger returns the global logger instance
func GetGlobalLogger() Logger {
	if globalLogger == nil {
		// Create default logger
		logger, _ := NewZapLogger(DefaultConfig())
		globalLogger = logger
	}
	return globalLogger
}

// Package-level logging functions using global logger

// Debug logs a debug message using the global logger
func Debug(msg string, fields ...Field) {
	GetGlobalLogger().Debug(msg, fields...)
}

// Info logs an info message using the global logger
func Info(msg string, fields ...Field) {
	GetGlobalLogger().Info(msg, fields...)
}

// Warn logs a warning message using the global logger
func Warn(msg string, fields ...Field) {
	GetGlobalLogger().Warn(msg, fields...)
}

// Error logs an error message using the global logger
func Error(msg string, fields ...Field) {
	GetGlobalLogger().Error(msg, fields...)
}

// Fatal logs a fatal message using the global logger
func Fatal(msg string, fields ...Field) {
	GetGlobalLogger().Fatal(msg, fields...)
}
