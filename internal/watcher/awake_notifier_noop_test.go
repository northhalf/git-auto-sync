package watcher

import (
	"context"
	"testing"
)

// @description    Verifies the Android awake notifier is an immediate no-op.
//
// @param           t  "test handle used for assertions"
func TestAwakeNotifierAndroidStartIsNoop(t *testing.T) {
	n := &AwakeNotifierAndroid{}
	out := make(chan bool, 1)

	if err := n.Start(context.Background(), out); err != nil {
		t.Fatalf("Start returned error %v, want nil", err)
	}
	select {
	case value := <-out:
		t.Fatalf("Start emitted awake value %v, want no event", value)
	default:
	}
}
