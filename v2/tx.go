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

// TxOption configures transaction retry behavior.
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

// WithMaxRetries sets the maximum number of retries on deadlock.
// Set to 0 to disable retry. Default: 3.
func WithMaxRetries(n int) TxOption {
	return func(o *txOptions) {
		if n >= 0 {
			o.maxRetries = n
		}
	}
}

// WithRetryBaseWait sets the base wait time for exponential backoff.
// Default: 5ms.
func WithRetryBaseWait(d time.Duration) TxOption {
	return func(o *txOptions) {
		if d > 0 {
			o.retryBaseWait = d
		}
	}
}

func (c *Client) WithTx(
	ctx context.Context, opts *sql.TxOptions, fn func(tx *gorm.DB) error, txOpts ...TxOption,
) error {
	if fn == nil {
		return errNilTxFunc
	}

	// Fast path: no TxOptions passed — execute once, only retry on deadlock
	// using compiled-in defaults. Zero extra allocations.
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
		// Deadlock detected — retry with jittered backoff unless last attempt.
		if attempt < maxRetries {
			sleep := retryBackoff(attempt, baseWait, maxWait)
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

// execTx runs a single transaction attempt.
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

// isDeadlock checks if the error is a MySQL deadlock (1213) or lock wait timeout (1205).
func isDeadlock(err error) bool {
	var mysqlErr *mysqldriver.MySQLError
	if !errors.As(err, &mysqlErr) {
		return false
	}
	return mysqlErr.Number == mysqlErrDeadlock || mysqlErr.Number == mysqlErrLockWait
}

// retryBackoff returns a jittered exponential backoff duration.
// Formula: min(baseWait * 2^attempt + jitter, maxWait).
func retryBackoff(attempt int, baseWait, maxWait time.Duration) time.Duration {
	wait := baseWait << attempt // baseWait * 2^attempt
	// Add random jitter up to 50% of wait to prevent thundering herd.
	const jitterDivisor = 2
	jitter := time.Duration(rand.Int64N(int64(wait/jitterDivisor) + 1)) //nolint:gosec // jitter for backoff, not security
	wait += jitter
	if wait > maxWait {
		return maxWait
	}
	return wait
}
