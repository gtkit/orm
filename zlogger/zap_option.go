package zlogger

import (
	"time"

	"go.uber.org/zap"
	gormlogger "gorm.io/gorm/logger"
)

type Option func(l *GormLogger)

func WithLogger(log *zap.Logger) Option {
	return func(l *GormLogger) {
		if log == nil {
			l.ZapLogger = zap.NewNop()
			return
		}
		l.ZapLogger = log
	}
}

func WithSlowThreshold(t time.Duration) Option {
	return func(l *GormLogger) {
		l.SlowThreshold = t
	}
}

func WithLogLevel(level gormlogger.LogLevel) Option {
	return func(l *GormLogger) {
		l.logLevel = level
	}
}

func WithIgnoreRecordNotFoundError(enabled bool) Option {
	return func(l *GormLogger) {
		l.ignoreRecordNotFoundError = enabled
	}
}

func WithParameterizedQueries(enabled bool) Option {
	return func(l *GormLogger) {
		l.parameterizedQueries = enabled
	}
}

func WithIgnoreTrace() Option {
	return func(l *GormLogger) {
		l.ignoreTrace = true
	}
}

//nolint:revive // preserve v1 public API spelling for compatibility
func WithSqlLog() Option {
	return func(l *GormLogger) {
		l.sqlLog = true
	}
}
