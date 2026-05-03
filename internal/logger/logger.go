package logger

import (
	"context"
	"io"
	"log/slog"
	"strings"
)

// Logger is the application logging boundary.
type Logger interface {
	Info(ctx context.Context, msg string, keyValues ...interface{})
	Error(ctx context.Context, err error, msg string, keyValues ...interface{})
	WithName(name string) Logger
}

// NewSlogLogger wraps slog.Logger behind the application Logger interface.
func NewSlogLogger(log *slog.Logger) Logger {
	if log == nil {
		log = slog.Default()
	}
	return &slogLogger{log: log}
}

// NewJSONLogger creates a slog-backed Logger that writes JSON records.
func NewJSONLogger(w io.Writer, level slog.Leveler) Logger {
	return NewSlogLogger(slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: level,
	})))
}

type slogLogger struct {
	log  *slog.Logger
	name string
}

func (l *slogLogger) Info(ctx context.Context, msg string, keyValues ...interface{}) {
	l.log.LogAttrs(ctx, slog.LevelInfo, msg, l.attrs(keyValues)...)
}

func (l *slogLogger) Error(ctx context.Context, err error, msg string, keyValues ...interface{}) {
	attrs := l.attrs(keyValues)
	if err != nil {
		attrs = append(attrs, slog.Any("error", err))
	}
	l.log.LogAttrs(ctx, slog.LevelError, msg, attrs...)
}

func (l *slogLogger) WithName(name string) Logger {
	if l.name != "" {
		name = strings.Join([]string{l.name, name}, "/")
	}
	return &slogLogger{
		log:  l.log,
		name: name,
	}
}

func (l *slogLogger) attrs(keyValues []interface{}) []slog.Attr {
	attrs := make([]slog.Attr, 0, len(keyValues)/2+1)
	if l.name != "" {
		attrs = append(attrs, slog.String("logger", l.name))
	}
	for i := 0; i < len(keyValues); i += 2 {
		key, ok := keyValues[i].(string)
		if !ok || key == "" {
			key = "invalidKey"
		}
		if i+1 >= len(keyValues) {
			attrs = append(attrs, slog.Any(key, nil))
			continue
		}
		attrs = append(attrs, slog.Any(key, keyValues[i+1]))
	}
	return attrs
}
