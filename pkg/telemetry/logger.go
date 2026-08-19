package telemetry

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/kardianos/service"
	"gopkg.in/natefinch/lumberjack.v2"
)

type contextKey string

const requestIDKey contextKey = "request_id"

// WithRequestID attaches a request ID to the given context.
func WithRequestID(ctx context.Context, reqID string) context.Context {
	return context.WithValue(ctx, requestIDKey, reqID)
}

// RequestIDFromContext retrieves the request ID from context if present.
func RequestIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey).(string); ok {
		return v
	}
	return ""
}

// FromContext enriches the provided logger with context-specific attributes (such as request_id).
func FromContext(ctx context.Context, logger *slog.Logger) *slog.Logger {
	if logger == nil {
		logger = slog.Default()
	}
	reqID := RequestIDFromContext(ctx)
	if reqID != "" {
		return logger.With(slog.String("request_id", reqID))
	}
	return logger
}

// nopCloser is a dummy closer for loggers that don't need cleanup.
type nopCloser struct{}

func (nopCloser) Close() error { return nil }

// NewInteractiveLogger configures structured logging to both stdout and a rotating log file.
// It returns the configured logger and an io.Closer to release file locks on shutdown.
func NewInteractiveLogger(stdout io.Writer, logFilePath string, level slog.Level) (*slog.Logger, io.Closer) {
	dir := filepath.Dir(logFilePath)
	if dir != "" && dir != "." {
		_ = os.MkdirAll(dir, 0750)
	}

	fileRotator := &lumberjack.Logger{
		Filename:   logFilePath,
		MaxSize:    10, // Megabytes
		MaxBackups: 5,
		Compress:   false,
	}

	var writer io.Writer
	if stdout != nil {
		writer = io.MultiWriter(stdout, fileRotator)
	} else {
		writer = fileRotator
	}

	handler := slog.NewTextHandler(writer, &slog.HandlerOptions{
		Level: level,
	})

	return slog.New(handler), fileRotator
}

// serviceLoggerHandler adapts slog.Handler to kardianos/service.Logger.
type serviceLoggerHandler struct {
	svcLogger service.Logger
	level     slog.Level
	attrs     []slog.Attr
	group     string
}

func (h *serviceLoggerHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *serviceLoggerHandler) Handle(ctx context.Context, r slog.Record) error {
	var sb strings.Builder
	sb.WriteString(r.Message)

	// Append handler-level attributes
	for _, a := range h.attrs {
		sb.WriteString(fmt.Sprintf(" %s=%v", a.Key, a.Value))
	}

	// Append record-level attributes
	r.Attrs(func(a slog.Attr) bool {
		sb.WriteString(fmt.Sprintf(" %s=%v", a.Key, a.Value))
		return true
	})

	msg := sb.String()

	if h.svcLogger == nil {
		return nil
	}

	switch {
	case r.Level >= slog.LevelError:
		return h.svcLogger.Error(msg)
	case r.Level >= slog.LevelWarn:
		return h.svcLogger.Warning(msg)
	default:
		return h.svcLogger.Info(msg)
	}
}

func (h *serviceLoggerHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	copy(newAttrs[len(h.attrs):], attrs)
	return &serviceLoggerHandler{
		svcLogger: h.svcLogger,
		level:     h.level,
		attrs:     newAttrs,
		group:     h.group,
	}
}

func (h *serviceLoggerHandler) WithGroup(name string) slog.Handler {
	return &serviceLoggerHandler{
		svcLogger: h.svcLogger,
		level:     h.level,
		attrs:     h.attrs,
		group:     name,
	}
}

// NewServiceLogger creates an slog.Logger that delegates to a platform service.Logger.
func NewServiceLogger(svcLogger service.Logger, level slog.Level) *slog.Logger {
	handler := &serviceLoggerHandler{
		svcLogger: svcLogger,
		level:     level,
	}
	return slog.New(handler)
}

// InitLogger initializes the logger based on whether the application is running interactively.
func InitLogger(isInteractive bool, logDir string, level slog.Level, svcLogger service.Logger) (*slog.Logger, io.Closer) {
	if isInteractive {
		if logDir == "" {
			logDir = "logs"
		}
		logFilePath := filepath.Join(logDir, "router.log")
		return NewInteractiveLogger(os.Stdout, logFilePath, level)
	}

	if svcLogger != nil {
		return NewServiceLogger(svcLogger, level), nopCloser{}
	}

	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})), nopCloser{}
}
