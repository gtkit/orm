package orm

import (
	"context"
	"testing"
)

func TestContextWithWriteFlagAcceptsNilContext(t *testing.T) {
	var nilCtx context.Context

	ctx := ContextWithWriteFlag(nilCtx)
	if !HasWriteFlag(ctx) {
		t.Fatal("expected write flag on returned context")
	}
}

func TestHasWriteFlagReturnsFalseForNilContext(t *testing.T) {
	var nilCtx context.Context

	if HasWriteFlag(nilCtx) {
		t.Fatal("expected false for nil context")
	}
}
