package orm

import "testing"

func TestContextWithWriteFlagAcceptsNilContext(t *testing.T) {
	ctx := ContextWithWriteFlag(nil)
	if !HasWriteFlag(ctx) {
		t.Fatal("expected write flag on returned context")
	}
}

func TestHasWriteFlagReturnsFalseForNilContext(t *testing.T) {
	if HasWriteFlag(nil) {
		t.Fatal("expected false for nil context")
	}
}
