package zlogger

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/gtkit/logger"
	"go.uber.org/zap"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

const slowTime = 200 * time.Millisecond

type GormLogger struct {
	ZapLogger     *zap.Logger
	SlowThreshold time.Duration
	sqlLog        bool
	ignoreTrace   bool
}

func _() {
	var _ gormlogger.Interface = (*GormLogger)(nil)
}

func New(options ...Option) gormlogger.Interface {
	l := GormLogger{
		SlowThreshold: slowTime,
	}
	for _, option := range options {
		option(&l)
	}
	if l.ZapLogger == nil {
		logger.NewZap(logger.WithFile(true), logger.WithConsole(true))
		logger.ZInfo("**** gorm new zap logger ****")
		l.ZapLogger = logger.Zlog()
	}

	return l
}

func (l GormLogger) LogMode(_ gormlogger.LogLevel) gormlogger.Interface {
	return l
}

func (l GormLogger) Info(_ context.Context, s string, i ...any) {
	l.sugar().Debugf(s, i...)
}

func (l GormLogger) Warn(_ context.Context, s string, i ...any) {
	l.sugar().Warnf(s, i...)
}

func (l GormLogger) Error(_ context.Context, s string, i ...any) {
	l.sugar().Errorf(s, i...)
}

func (l GormLogger) Trace(_ context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	if l.ignoreTrace {
		return
	}

	elapsed := time.Since(begin)
	sql, rows := fc()

	logFields := []zap.Field{
		zap.String("sql", sql),
		zap.String("time", fmt.Sprintf("%.3fms", float64(elapsed)/float64(time.Millisecond))),
		zap.Int64("rows", rows),
	}
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			l.logger().Warn("Database ErrRecordNotFound", logFields...)
		} else {
			logFields = append(logFields, zap.Error(err))
			l.logger().Error("Database Error", logFields...)
		}
	}

	if l.SlowThreshold != 0 && elapsed > l.SlowThreshold {
		l.logger().Warn("Database Slow Log", logFields...)
	}

	if l.sqlLog {
		l.logger().Debug("Database Query", logFields...)
	}
}

func (l GormLogger) logger() *zap.Logger {
	var (
		gormPackage    = filepath.Join("gorm.io", "gorm")
		zapgormPackage = filepath.Join("moul.io", "zapgorm2")
	)

	clone := l.ZapLogger.WithOptions(zap.AddCallerSkip(-2))

	for i := 2; i < 15; i++ {
		_, file, _, ok := runtime.Caller(i)
		switch {
		case !ok:
		case strings.HasSuffix(file, "_test.go"):
		case strings.Contains(file, gormPackage):
		case strings.Contains(file, zapgormPackage):
		default:
			return clone.WithOptions(zap.AddCallerSkip(i))
		}
	}
	return l.ZapLogger
}

func (l GormLogger) sugar() *zap.SugaredLogger {
	return l.logger().Sugar()
}
