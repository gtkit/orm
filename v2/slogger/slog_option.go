package slogger

import (
	"log/slog"
	"time"
)

type Option func(l *GormSlogger)

func WithLogger(log *slog.Logger) Option {
	return func(l *GormSlogger) {
		l.slogger = log
	}
}

func WithSourceField(field string) Option {
	return func(l *GormSlogger) {
		l.sourceField = field
	}
}

func WithErrorField(field string) Option {
	return func(l *GormSlogger) {
		l.errorField = field
	}
}

func WithSlowThreshold(threshold time.Duration) Option {
	return func(l *GormSlogger) {
		l.slowThreshold = threshold
	}
}

func WithTraceAll() Option {
	return func(l *GormSlogger) {
		l.traceAll = true
	}
}

func SetLogLevel(key LogType, level slog.Level) Option {
	return func(l *GormSlogger) {
		l.logLevel[key] = level
	}
}

func WithRecordNotFoundError() Option {
	return func(l *GormSlogger) {
		l.ignoreRecordNotFoundError = false
	}
}

func WithIgnoreTrace() Option {
	return func(l *GormSlogger) {
		l.ignoreTrace = true
	}
}

func WithContextValue(slogAttrName, contextKey string) Option {
	return func(l *GormSlogger) {
		if l.contextKeys == nil {
			l.contextKeys = make(map[string]string)
		}
		l.contextKeys[slogAttrName] = contextKey
	}
}
