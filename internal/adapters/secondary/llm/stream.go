package llm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// A model is given as long as it needs to write, provided it keeps writing
// (docs/02_features/19_draft_checks.md D4). A reasoning model run locally,
// measured with deepseek-r1, does not finish a letter in 30 seconds, which
// was once the limit on the whole request.
const (
	// idleTimeout is how long a server may go quiet once it has started
	// answering before the request is abandoned.
	idleTimeout = 60 * time.Second
	// firstByteTimeout is how long it may take to start: a large model
	// loading from disk sends nothing until it is ready.
	firstByteTimeout = 5 * time.Minute
	// checkTimeout bounds a connectivity check, which asks for a list and
	// writes nothing.
	checkTimeout = 10 * time.Second
)

// errStalled is the cause a request is cancelled with when its server goes
// quiet, so it can be told apart from the caller cancelling it.
var errStalled = errors.New("the model stopped sending anything")

// newHTTPClient is a client with no limit on a whole request: generation
// is bounded by watchStall instead, and a connectivity check by its own
// context.
func newHTTPClient() *http.Client {
	return &http.Client{}
}

// patience is how long a server may stay quiet, before and after it starts
// answering.
type patience struct {
	firstByte, idle time.Duration
}

// defaultPatience is the patience every provider is built with.
var defaultPatience = patience{firstByte: firstByteTimeout, idle: idleTimeout}

// stallWatch cancels a request whose server has gone quiet: after
// firstByte before the response starts, and after idle between reads once
// it has.
type stallWatch struct {
	patience patience
	mu       sync.Mutex
	timer    *time.Timer
	started  bool
	ctx      context.Context
}

// watchStall returns a watch whose context a request is sent with. The
// context is cancelled with errStalled when the server goes quiet; stop
// ends the watch.
func watchStall(ctx context.Context, p patience) (watch *stallWatch, stop func()) {
	ctx, cancel := context.WithCancelCause(ctx)
	watch = &stallWatch{patience: p, ctx: ctx}
	watch.timer = time.AfterFunc(p.firstByte, func() { cancel(errStalled) })
	return watch, func() {
		watch.timer.Stop()
		cancel(nil)
	}
}

// body wraps a response body so that every read that returns anything
// restarts the idle clock.
func (w *stallWatch) body(r io.Reader) io.Reader {
	w.heard()
	return readerFunc(func(p []byte) (int, error) {
		n, err := r.Read(p)
		if n > 0 {
			w.heard()
		}
		return n, err
	})
}

func (w *stallWatch) heard() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.started = true
	w.timer.Reset(w.patience.idle)
}

type readerFunc func([]byte) (int, error)

func (f readerFunc) Read(p []byte) (int, error) { return f(p) }

// explain says why a request failed when its server went quiet, and
// returns err unchanged otherwise.
func (w *stallWatch) explain(name string, err error) error {
	if !errors.Is(context.Cause(w.ctx), errStalled) {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.started {
		return fmt.Errorf("%s stopped sending anything for %s and was given up on: %w", name, w.patience.idle, errStalled)
	}
	return fmt.Errorf("%s sent nothing for %s and was given up on: %w", name, w.patience.firstByte, errStalled)
}
