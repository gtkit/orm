package orm

import "context"

type ctxKey int

const ctxKeyWriteFlag ctxKey = iota

// ContextWithWriteFlag marks the context to indicate that a recent write
// has occurred. When passed to [Cluster.ReaderClientCtx] or [Cluster.ReadDBCtx],
// reads will be routed to the primary instead of a replica, ensuring
// read-after-write consistency.
//
// Typical usage: call this right after a successful write, then use the
// returned context for subsequent reads within the same request.
//
//	ctx = orm.ContextWithWriteFlag(ctx)
//	// subsequent reads via ReaderClientCtx(ctx) will hit the primary
func ContextWithWriteFlag(ctx context.Context) context.Context {
	return context.WithValue(ctx, ctxKeyWriteFlag, true)
}

// HasWriteFlag reports whether the context carries a write flag
// set by [ContextWithWriteFlag].
func HasWriteFlag(ctx context.Context) bool {
	v, _ := ctx.Value(ctxKeyWriteFlag).(bool)
	return v
}
