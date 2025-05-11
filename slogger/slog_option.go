package slogger

import (
	"log/slog"
	"time"
)

type Option func(l *GormSlogger)

// WithLogger defines a custom logger to use
func WithLogger(log *slog.Logger) Option {
	return func(l *GormSlogger) {
		l.slogger = log
	}
}

// WithSourceField defines the field to set the file name and line number of the current file
func WithSourceField(field string) Option {
	return func(l *GormSlogger) {
		l.sourceField = field
	}
}

// WithErrorField defines the field to set the error
func WithErrorField(field string) Option {
	return func(l *GormSlogger) {
		l.errorField = field
	}
}

// WithSlowThreshold defines the threshold above which a sql query is considered slow
func WithSlowThreshold(threshold time.Duration) Option {
	return func(l *GormSlogger) {
		l.slowThreshold = threshold
	}
}

// WithTraceAll enables mode which logs all SQL messages.
func WithTraceAll() Option {
	return func(l *GormSlogger) {
		l.traceAll = true
	}
}

// SetLogLevel sets a new slog.Level for a LogType.
func SetLogLevel(key LogType, level slog.Level) Option {
	return func(l *GormSlogger) {
		l.logLevel[key] = level
	}
}

// WithRecordNotFoundError allows the slogger to log gorm.ErrRecordNotFound errors
func WithRecordNotFoundError() Option {
	return func(l *GormSlogger) {
		l.ignoreRecordNotFoundError = false
	}
}

// WithIgnoreTrace disables the tracing of SQL queries by the slogger
func WithIgnoreTrace() Option {
	return func(l *GormSlogger) {
		l.ignoreTrace = true
	}
}

// WithContextValue adds a context value to the log
func WithContextValue(slogAttrName, contextKey string) Option {
	return func(l *GormSlogger) {
		if l.contextKeys == nil {
			l.contextKeys = make(map[string]string, 0)
		}
		l.contextKeys[slogAttrName] = contextKey
	}
}
