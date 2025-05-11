package zlogger

import (
	"time"

	"go.uber.org/zap"
)

type Option func(l *GormLogger)

func WithLogger(log *zap.Logger) Option {
	return func(l *GormLogger) {
		l.ZapLogger = log
	}
}

// WithIgnoreTrace disables the tracing of SQL queries by the slogger.
func WithIgnoreTrace() Option {
	return func(l *GormLogger) {
		l.ignoreTrace = true
	}
}

// WithSqlLog enables the logging of SQL queries by the slogger.
func WithSqlLog() Option {
	return func(l *GormLogger) {
		l.sqlLog = true
	}
}

// WithSlowThreshold sets the threshold for logging slow SQL queries by the slogger.
func WithSlowThreshold(t time.Duration) Option {
	return func(l *GormLogger) {
		l.SlowThreshold = t
	}
}
