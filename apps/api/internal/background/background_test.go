package background

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestWaitWaitsForRunningTasks(t *testing.T) {
	var tasks Tasks
	var finished atomic.Int32
	for range 3 {
		tasks.Go(func() {
			time.Sleep(50 * time.Millisecond)
			finished.Add(1)
		})
	}
	if left := tasks.Wait(context.Background()); left != 0 {
		t.Fatalf("left = %d, want 0", left)
	}
	if got := finished.Load(); got != 3 {
		t.Fatalf("finished = %d, want 3", got)
	}
}

func TestWaitReturnsLeftoverAtDeadline(t *testing.T) {
	var tasks Tasks
	release := make(chan struct{})
	defer close(release)
	tasks.Go(func() { <-release })
	tasks.Go(func() {})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if left := tasks.Wait(ctx); left != 1 {
		t.Fatalf("left = %d, want 1", left)
	}
}

func TestGoAfterWaitAcceptsFollowUpOnly(t *testing.T) {
	var tasks Tasks
	var followUp atomic.Bool
	tasks.Go(func() {
		time.Sleep(50 * time.Millisecond) // Wait が始まってから続きの仕事を始める
		tasks.Go(func() {
			time.Sleep(50 * time.Millisecond)
			followUp.Store(true)
		})
	})
	if left := tasks.Wait(context.Background()); left != 0 {
		t.Fatalf("left = %d, want 0", left)
	}
	if !followUp.Load() {
		t.Fatal("動いている仕事が始めた続きの仕事を待たなかった")
	}

	var late atomic.Bool
	tasks.Go(func() { late.Store(true) })
	time.Sleep(20 * time.Millisecond)
	if late.Load() {
		t.Fatal("全部終わったあとに来た仕事を動かした")
	}
}
