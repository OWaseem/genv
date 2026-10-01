package hooks

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/ks1686/genv/internal/schema"
)

// A hook that backgrounds a process leaves the grandchild holding the stdout
// pipe. Without WaitDelay, Wait blocks until the grandchild exits even though
// the hook itself already finished.
func TestExecRunner_ReturnsWhenGrandchildHoldsPipe(t *testing.T) {
	// `sleep 30 &` exits immediately, leaving a backgrounded child holding
	// the inherited stdout pipe for 30s.
	script := `sleep 30 & echo done`

	done := make(chan error, 1)
	go func() {
		done <- (execRunner{}).Run(context.Background(),
			[]string{"sh", "-c", script}, nil, nil, io.Discard, io.Discard)
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("Run() blocked waiting for a backgrounded grandchild to close the pipe")
	}
}

// The same shape under a context deadline: the direct child is killed, and the
// runner must still return promptly rather than waiting on the pipe.
func TestExecRunner_TimedOutCommandReturnsPromptly(t *testing.T) {
	script := `sleep 30 & echo started; sleep 30`

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- (execRunner{}).Run(ctx,
			[]string{"sh", "-c", script}, nil, nil, io.Discard, io.Discard)
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Run() error = nil, want a timeout")
		}
		var ee *exec.ExitError
		if !errors.As(err, &ee) && !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Run() error = %v, want exit error or deadline", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("Run() blocked past its deadline on an inherited pipe")
	}
}

func TestExecRunner_NormalCommandUnaffected(t *testing.T) {
	err := (execRunner{}).Run(context.Background(),
		[]string{"sh", "-c", "echo hello"}, nil, nil, io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestHookTimeoutStillReturnsWithinHookBudget(t *testing.T) {
	e := NewExecutor(io.Discard, io.Discard)
	e.goos = "linux"
	start := time.Now()
	err := e.runPhase(context.Background(), "post-apply",
		[]schema.Hook{{Name: "slow", Command: `sleep 30 & echo hi`}},
		RunOptions{Timeout: 500 * time.Millisecond})
	if err == nil {
		t.Fatal("runPhase() error = nil, want a timeout")
	}
	if elapsed := time.Since(start); elapsed > 20*time.Second {
		t.Fatalf("runPhase() took %s, want it bounded by the hook timeout", elapsed)
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("runPhase() error = %v, want a timeout message", err)
	}
}
