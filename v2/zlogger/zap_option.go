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
			l.zapLogger = zap.NewNop()
			return
		}
		l.zapLogger = log
	}
}

func WithSlowThreshold(t time.Duration) Option {
	return func(l *GormLogger) {
		l.slowThreshold = t
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
