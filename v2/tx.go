package orm

import (
	"context"
	"database/sql"
	"errors"

	"gorm.io/gorm"
)

var errNilTxFunc = errors.New("orm/v2: nil transaction function")

func (c *Client) WithTx(ctx context.Context, opts *sql.TxOptions, fn func(tx *gorm.DB) error) (err error) {
	if fn == nil {
		return errNilTxFunc
	}

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

func (c *Client) WithReadTx(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return c.WithTx(ctx, &sql.TxOptions{ReadOnly: true}, fn)
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
