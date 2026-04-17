package jetorm

import (
	"context"
	"database/sql"

	jetmysql "github.com/go-jet/jet/v2/mysql"
)

type Tx struct {
	tx     *sql.Tx
	config Config
}

func (c *Client) WithTx(ctx context.Context, opts *sql.TxOptions, fn func(*Tx) error) (err error) {
	if fn == nil {
		return ErrNilTxFunc
	}

	beginCtx, cancel := normalizeContext(ctx, c.config.QueryTimeout)
	defer cancel()

	sqlTx, err := c.db.BeginTx(beginCtx, opts)
	if err != nil {
		return err
	}

	tx := &Tx{tx: sqlTx, config: c.config.Clone()}

	defer func() {
		if recovered := recover(); recovered != nil {
			_ = sqlTx.Rollback()
			panic(recovered)
		}
		if err != nil {
			_ = sqlTx.Rollback()
			return
		}
		err = sqlTx.Commit()
	}()

	err = fn(tx)
	return err
}

func (t *Tx) ExecContext(ctx context.Context, stmt jetmysql.Statement) (sql.Result, error) {
	queryCtx, cancel := normalizeContext(ctx, t.config.QueryTimeout)
	defer cancel()

	return stmt.ExecContext(queryCtx, t.tx)
}

func (t *Tx) QueryContext(ctx context.Context, stmt jetmysql.Statement, dest any) error {
	queryCtx, cancel := normalizeContext(ctx, t.config.QueryTimeout)
	defer cancel()

	return stmt.QueryContext(queryCtx, t.tx, dest)
}

func (t *Tx) Rows(ctx context.Context, stmt jetmysql.Statement) (*jetmysql.Rows, error) {
	queryCtx, cancel := normalizeContext(ctx, t.config.QueryTimeout)
	defer cancel()

	return stmt.Rows(queryCtx, t.tx)
}
