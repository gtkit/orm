package slogger

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
	"gorm.io/gorm/utils"
)

type LogType string

const (
	ErrorLogType     LogType = "sql_error"
	SlowQueryLogType LogType = "slow_query"
	DefaultLogType   LogType = "default"

	SourceField    = "file"
	ErrorField     = "error"
	QueryField     = "sql"
	DurationField  = "time"
	SlowQueryField = "slow_query"
	RowsField      = "rows"
)

func _() {
	var _ gormlogger.Interface = (*GormSlogger)(nil)
}

// New creates a new logger for gorm.io/gorm.
func New(options ...Option) *GormSlogger {
	l := GormSlogger{
		ignoreRecordNotFoundError: true,
		errorField:                ErrorField,
		sourceField:               SourceField,
		logLevel: map[LogType]slog.Level{
			ErrorLogType:     slog.LevelError,
			SlowQueryLogType: slog.LevelWarn,
			DefaultLogType:   slog.LevelInfo,
		},
	}

	for _, option := range options {
		option(&l)
	}

	if l.slogger == nil {
		l.slogger = slog.Default()
	}

	return &l
}

type GormSlogger struct {
	slogger                   *slog.Logger
	ignoreTrace               bool
	ignoreRecordNotFoundError bool
	traceAll                  bool
	slowThreshold             time.Duration
	logLevel                  map[LogType]slog.Level
	contextKeys               map[string]string

	sourceField string
	errorField  string
}

func (l GormSlogger) LogMode(_ gormlogger.LogLevel) gormlogger.Interface {
	return l
}

func (l GormSlogger) Info(ctx context.Context, msg string, args ...any) {
	l.log(l.slogger.InfoContext, ctx, msg, args...)
}

func (l GormSlogger) Warn(ctx context.Context, msg string, args ...any) {
	l.log(l.slogger.WarnContext, ctx, msg, args...)
}

func (l GormSlogger) Error(ctx context.Context, msg string, args ...any) {
	l.log(l.slogger.ErrorContext, ctx, msg, args...)
}

func (l GormSlogger) log(f func(ctx context.Context, msg string, args ...any), ctx context.Context, msg string, args ...any) {
	args = l.appendContextAttributes(ctx, args)
	f(ctx, msg, args...)
}

func (l GormSlogger) Trace(ctx context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	if l.ignoreTrace {
		return
	}

	elapsed := time.Since(begin)
	switch {
	case err != nil && (!errors.Is(err, gorm.ErrRecordNotFound) || !l.ignoreRecordNotFoundError):
		sql, rows := fc()
		attributes := l.appendContextAttributes(ctx, []any{
			slog.Any(l.errorField, err),
			slog.String(QueryField, sql),
			slog.Duration(DurationField, elapsed),
			slog.Int64(RowsField, rows),
			slog.String(l.sourceField, utils.FileWithLineNum()),
		})
		l.slogger.Log(ctx, l.logLevel[ErrorLogType], err.Error(), attributes...)

	case l.slowThreshold != 0 && elapsed > l.slowThreshold:
		sql, rows := fc()
		attributes := l.appendContextAttributes(ctx, []any{
			slog.Bool(SlowQueryField, true),
			slog.String(QueryField, sql),
			slog.Duration(DurationField, elapsed),
			slog.Int64(RowsField, rows),
			slog.String(l.sourceField, utils.FileWithLineNum()),
		})
		l.slogger.Log(ctx, l.logLevel[SlowQueryLogType], fmt.Sprintf("slow sql query [%s >= %v]", elapsed, l.slowThreshold), attributes...)

	case l.traceAll:
		sql, rows := fc()
		attributes := l.appendContextAttributes(ctx, []any{
			slog.String(QueryField, sql),
			slog.Duration(DurationField, elapsed),
			slog.Int64(RowsField, rows),
			slog.String(l.sourceField, utils.FileWithLineNum()),
		})
		l.slogger.Log(ctx, l.logLevel[DefaultLogType], fmt.Sprintf("SQL query executed [%s]", elapsed), attributes...)
	}
}

func (l GormSlogger) appendContextAttributes(ctx context.Context, args []any) []any {
	if args == nil {
		args = []any{}
	}
	for k, v := range l.contextKeys {
		if value := ctx.Value(v); value != nil {
			args = append(args, slog.Any(k, value))
		}
	}
	return args
}
