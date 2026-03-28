package zlogger

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
	"gorm.io/gorm/utils"
)

const slowTime = 200 * time.Millisecond

type GormLogger struct {
	zapLogger                 *zap.Logger
	slowThreshold             time.Duration
	logLevel                  gormlogger.LogLevel
	ignoreRecordNotFoundError bool
	parameterizedQueries      bool
}

func _() {
	var _ gormlogger.Interface = (*GormLogger)(nil)
}

func New(options ...Option) gormlogger.Interface {
	logger := &GormLogger{
		zapLogger:     zap.NewNop(),
		slowThreshold: slowTime,
		logLevel:      gormlogger.Warn,
	}
	for _, option := range options {
		if option != nil {
			option(logger)
		}
	}
	return logger
}

func (l *GormLogger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	clone := *l
	clone.logLevel = level
	return &clone
}

func (l *GormLogger) Info(_ context.Context, msg string, data ...any) {
	if l.logLevel < gormlogger.Info {
		return
	}
	l.sugar().Infof(msg, data...)
}

func (l *GormLogger) Warn(_ context.Context, msg string, data ...any) {
	if l.logLevel < gormlogger.Warn {
		return
	}
	l.sugar().Warnf(msg, data...)
}

func (l *GormLogger) Error(_ context.Context, msg string, data ...any) {
	if l.logLevel < gormlogger.Error {
		return
	}
	l.sugar().Errorf(msg, data...)
}

func (l *GormLogger) Trace(_ context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	if l.logLevel <= gormlogger.Silent {
		return
	}

	elapsed := time.Since(begin)
	sql, rows := fc()

	fields := []zap.Field{
		zap.String("source", utils.FileWithLineNum()),
		zap.Duration("elapsed", elapsed),
		zap.String("sql", sql),
	}
	if rows >= 0 {
		fields = append(fields, zap.Int64("rows", rows))
	}

	recordNotFoundIgnored := errors.Is(err, gorm.ErrRecordNotFound) && l.ignoreRecordNotFoundError

	switch {
	case err != nil && l.logLevel >= gormlogger.Error && !recordNotFoundIgnored:
		l.base().Error("gorm query error", append(fields, zap.Error(err))...)
	case l.slowThreshold != 0 && elapsed > l.slowThreshold && l.logLevel >= gormlogger.Warn:
		l.base().Warn("gorm slow query", append(fields, zap.Duration("slow_threshold", l.slowThreshold))...)
	case l.logLevel == gormlogger.Info:
		l.base().Info("gorm query", fields...)
	}
}

func (l *GormLogger) ParamsFilter(_ context.Context, sql string, params ...interface{}) (string, []interface{}) {
	if l.parameterizedQueries {
		return sql, nil
	}
	return sql, params
}

func (l *GormLogger) base() *zap.Logger {
	if l == nil || l.zapLogger == nil {
		return zap.NewNop()
	}
	return l.zapLogger
}

func (l *GormLogger) sugar() *zap.SugaredLogger {
	return l.base().Sugar()
}
