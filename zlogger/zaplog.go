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

// GormLogger preserves the v1 exported field surface for compatibility.
// Treat instances as immutable after construction; configure through options
// before sharing them across goroutines.
type GormLogger struct {
	// ZapLogger is exported for v1 compatibility. Do not mutate it after construction.
	ZapLogger *zap.Logger
	// SlowThreshold is exported for v1 compatibility. Do not mutate it after construction.
	SlowThreshold             time.Duration
	sqlLog                    bool
	ignoreTrace               bool
	logLevel                  gormlogger.LogLevel
	ignoreRecordNotFoundError bool
	parameterizedQueries      bool
}

func _() {
	var _ gormlogger.Interface = (*GormLogger)(nil)
}

func New(options ...Option) gormlogger.Interface {
	logger := GormLogger{
		ZapLogger:     zap.NewNop(),
		SlowThreshold: slowTime,
		logLevel:      gormlogger.Warn,
	}
	for _, option := range options {
		if option != nil {
			option(&logger)
		}
	}
	return logger
}

func (l GormLogger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	clone := l
	clone.logLevel = level
	return clone
}

func (l GormLogger) Info(_ context.Context, msg string, data ...any) {
	if l.logLevel < gormlogger.Info {
		return
	}
	l.sugar().Infof(msg, data...)
}

func (l GormLogger) Warn(_ context.Context, msg string, data ...any) {
	if l.logLevel < gormlogger.Warn {
		return
	}
	l.sugar().Warnf(msg, data...)
}

func (l GormLogger) Error(_ context.Context, msg string, data ...any) {
	if l.logLevel < gormlogger.Error {
		return
	}
	l.sugar().Errorf(msg, data...)
}

func (l GormLogger) Trace(_ context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	if l.logLevel <= gormlogger.Silent || l.ignoreTrace {
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
	case l.SlowThreshold != 0 && elapsed > l.SlowThreshold && l.logLevel >= gormlogger.Warn:
		l.base().Warn("gorm slow query", append(fields, zap.Duration("slow_threshold", l.SlowThreshold))...)
	case l.sqlLog || l.logLevel == gormlogger.Info:
		l.base().Info("gorm query", fields...)
	}
}

func (l GormLogger) ParamsFilter(_ context.Context, sql string, params ...interface{}) (string, []interface{}) {
	if l.parameterizedQueries {
		return sql, nil
	}
	return sql, params
}

func (l GormLogger) base() *zap.Logger {
	if l.ZapLogger == nil {
		return zap.NewNop()
	}
	return l.ZapLogger
}

func (l GormLogger) sugar() *zap.SugaredLogger {
	return l.base().Sugar()
}
