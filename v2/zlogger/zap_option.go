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

func WithIgnoreTrace() Option {
	return func(l *GormLogger) {
		l.ignoreTrace = true
	}
}

func WithSqlLog() Option {
	return func(l *GormLogger) {
		l.sqlLog = true
	}
}

func WithSlowThreshold(t time.Duration) Option {
	return func(l *GormLogger) {
		l.SlowThreshold = t
	}
}
