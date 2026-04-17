package orm

import (
	"context"
	"database/sql"
	"errors"
	"math/rand/v2"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

const (
	defaultMaxRetries    = 3
	defaultRetryBaseWait = 5 * time.Millisecond
	defaultRetryMaxWait  = 50 * time.Millisecond
	mysqlErrDeadlock     = 1213
	mysqlErrLockWait     = 1205
)

var errNilTxFunc = errors.New("orm/v2: nil transaction function")

type TxRetryEvent struct {
	ClientName string
	Attempt    int
	MaxRetries int
	Wait       time.Duration
	Err        error
}

type TxRetryObserver func(ctx context.Context, event TxRetryEvent)

// TxOption 配置事务重试行为。
type TxOption func(*txOptions)

type txOptions struct {
	maxRetries    int
	retryBaseWait time.Duration
	retryMaxWait  time.Duration
}

func defaultTxOptions() txOptions {
	return txOptions{
		maxRetries:    defaultMaxRetries,
		retryBaseWait: defaultRetryBaseWait,
		retryMaxWait:  defaultRetryMaxWait,
	}
}

// WithMaxRetries 设置死锁后的最大重试次数。
// 设为 0 表示禁用重试。默认值：3。
func WithMaxRetries(n int) TxOption {
	return func(o *txOptions) {
		if n >= 0 {
			o.maxRetries = n
		}
	}
}

// WithRetryBaseWait 设置指数退避的基础等待时间。
// 默认值：5ms。
func WithRetryBaseWait(d time.Duration) TxOption {
	return func(o *txOptions) {
		if d > 0 {
			o.retryBaseWait = d
		}
	}
}

// WithRetryMaxWait 设置单次重试退避的最大等待时间。
// 默认值：50ms。
func WithRetryMaxWait(d time.Duration) TxOption {
	return func(o *txOptions) {
		if d > 0 {
			o.retryMaxWait = d
		}
	}
}

func (c *Client) WithTx(
	ctx context.Context, opts *sql.TxOptions, fn func(tx *gorm.DB) error, txOpts ...TxOption,
) error {
	if fn == nil {
		return errNilTxFunc
	}

	// 快路径：未传 TxOption 时直接使用编译期默认值，只在死锁时重试，避免额外分配。
	if len(txOpts) == 0 {
		return c.withTxRetry(ctx, opts, fn, defaultMaxRetries, defaultRetryBaseWait, defaultRetryMaxWait)
	}

	retryOpts := defaultTxOptions()
	for _, opt := range txOpts {
		if opt != nil {
			opt(&retryOpts)
		}
	}
	return c.withTxRetry(ctx, opts, fn, retryOpts.maxRetries, retryOpts.retryBaseWait, retryOpts.retryMaxWait)
}

func (c *Client) withTxRetry(
	ctx context.Context, opts *sql.TxOptions, fn func(tx *gorm.DB) error,
	maxRetries int, baseWait, maxWait time.Duration,
) error {
	var lastErr error
	for attempt := range maxRetries + 1 {
		lastErr = c.execTx(ctx, opts, fn)
		if lastErr == nil {
			return nil
		}
		if !isDeadlock(lastErr) {
			return lastErr
		}
		// 检测到死锁后进行带抖动的退避重试，最后一次不再等待。
		if attempt < maxRetries {
			sleep := retryBackoff(attempt, baseWait, maxWait)
			if observer := c.config.TxRetryObserver; observer != nil {
				observer(normalizeContext(ctx), TxRetryEvent{
					ClientName: c.effectiveName("default"),
					Attempt:    attempt + 1,
					MaxRetries: maxRetries,
					Wait:       sleep,
					Err:        lastErr,
				})
			}
			select {
			case <-ctx.Done():
				return errors.Join(lastErr, ctx.Err())
			case <-time.After(sleep):
			}
		}
	}
	return lastErr
}

func (c *Client) WithReadTx(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return c.WithTx(ctx, &sql.TxOptions{ReadOnly: true}, fn)
}

// execTx 执行单次事务尝试。
func (c *Client) execTx(ctx context.Context, opts *sql.TxOptions, fn func(tx *gorm.DB) error) (err error) {
	txDB := c.db.WithContext(normalizeContext(ctx))
	var tx *gorm.DB
	if opts != nil {
		tx = txDB.Begin(opts)
	} else {
		tx = txDB.Begin()
	}
	if tx.Error != nil {
		return tx.Error
	}

	defer func() {
		if r := recover(); r != nil {
			_ = tx.Rollback().Error
			panic(r)
		}
	}()

	if err = fn(tx); err != nil {
		return errors.Join(err, rollbackError(tx))
	}

	return tx.Commit().Error
}

func rollbackError(tx *gorm.DB) error {
	if tx == nil {
		return nil
	}
	err := tx.Rollback().Error
	if errors.Is(err, gorm.ErrInvalidTransaction) {
		return nil
	}
	return err
}

// isDeadlock 判断错误是否属于 MySQL 死锁（1213）或锁等待超时（1205）。
func isDeadlock(err error) bool {
	var mysqlErr *mysqldriver.MySQLError
	if !errors.As(err, &mysqlErr) {
		return false
	}
	return mysqlErr.Number == mysqlErrDeadlock || mysqlErr.Number == mysqlErrLockWait
}

// retryBackoff 返回带抖动的指数退避时长。
// 公式：min(baseWait * 2^attempt + jitter, maxWait)。
func retryBackoff(attempt int, baseWait, maxWait time.Duration) time.Duration {
	wait := baseWait << attempt // baseWait * 2^attempt
	// 增加最多 50% 的随机抖动，避免大量请求同时重试。
	const jitterDivisor = 2
	jitter := time.Duration(rand.Int64N(int64(wait/jitterDivisor) + 1)) //nolint:gosec // jitter for backoff, not security
	wait += jitter
	if wait > maxWait {
		return maxWait
	}
	return wait
}
