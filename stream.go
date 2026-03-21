package llmprovider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"runtime/debug"
	"sync"
)

type Stream struct {
	nextFn  func() bool
	eventFn func() StreamEvent
	errFn   func() error
	closeFn func() error
}

func (s *Stream) Next() bool         { return s.nextFn() }
func (s *Stream) Event() StreamEvent { return s.eventFn() }
func (s *Stream) Err() error         { return s.errFn() }
func (s *Stream) Close() error       { return s.closeFn() }

var _ io.Closer = (*Stream)(nil)

func NewStream(
	next func() bool,
	event func() StreamEvent,
	errFn func() error,
	closeFn func() error,
) *Stream {
	if next == nil || event == nil || errFn == nil || closeFn == nil {
		panic("llmprovider: NewStream requires non-nil callbacks")
	}
	return &Stream{
		nextFn:  next,
		eventFn: event,
		errFn:   errFn,
		closeFn: closeFn,
	}
}

type streamTerminal uint8

const (
	streamOpen streamTerminal = iota
	streamDone
	streamFailed
	streamClosed
)

type StreamPanicError struct {
	Recovered any
	Stack     []byte
}

func (e *StreamPanicError) Error() string {
	return fmt.Sprintf("llmprovider: stream panic: %v", e.Recovered)
}

func NewStreamPanicError(recovered any) error {
	return &StreamPanicError{
		Recovered: recovered,
		Stack:     debug.Stack(),
	}
}

type chanStreamState struct {
	ctx    context.Context
	cancel context.CancelFunc
	ch     <-chan StreamEvent

	mu       sync.Mutex
	current  StreamEvent
	err      error
	terminal streamTerminal

	terminalOnce     sync.Once
	closeOnce        sync.Once
	producerDoneOnce sync.Once
	drainOnce        sync.Once
	done             chan struct{}
}

func (s *chanStreamState) setCurrent(ev StreamEvent) {
	s.mu.Lock()
	s.current = ev
	s.mu.Unlock()
}

func (s *chanStreamState) currentEvent() StreamEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current
}

func (s *chanStreamState) setTerminal(kind streamTerminal, err error) {
	s.terminalOnce.Do(func() {
		s.mu.Lock()
		s.terminal = kind
		s.err = err
		s.mu.Unlock()
	})
}

func (s *chanStreamState) terminalState() streamTerminal {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.terminal
}

func (s *chanStreamState) terminalErr() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

func (s *chanStreamState) markProducerDone() {
	s.producerDoneOnce.Do(func() {
		close(s.done)
	})
}

func (s *chanStreamState) startDrain() {
	s.drainOnce.Do(func() {
		go func() {
			defer s.markProducerDone()
			defer func() {
				if r := recover(); r != nil {
					s.setTerminal(streamFailed, NewStreamPanicError(r))
					s.cancel()
				}
			}()
			for range s.ch {
			}
		}()
	})
}

// NewStreamFromChan wraps a provider-owned event channel.
//
// Required provider contract:
//   - ctx must be the provider-owned child context used by the transport
//   - cancel must cancel that child context
//   - the producer goroutine must close ch on exit
//   - the producer goroutine must recover panics and emit StreamEvent{Error: ...}
//
// Why the explicit producer-side recover? Because Go cannot recover a panic
// from a different goroutine. NewStreamFromChan recovers its own receive/drain
// paths, but the provider goroutine is still the crash boundary for transport code.
func NewStreamFromChan(ctx context.Context, ch <-chan StreamEvent, cancel context.CancelFunc) *Stream {
	if ctx == nil || ch == nil || cancel == nil {
		panic("llmprovider: NewStreamFromChan requires non-nil ctx, ch, and cancel")
	}

	state := &chanStreamState{
		ctx:      ctx,
		cancel:   cancel,
		ch:       ch,
		terminal: streamOpen,
		done:     make(chan struct{}),
	}

	next := func() (ok bool) {
		defer func() {
			if r := recover(); r != nil {
				state.setTerminal(streamFailed, NewStreamPanicError(r))
				state.cancel()
				ok = false
			}
		}()

		if state.terminalState() != streamOpen {
			return false
		}

		// The two-level select gives channel reads priority. If an event
		// is already buffered, it is delivered even when ctx is simultaneously
		// cancelled. Only when the channel would block does ctx.Done() take effect.
		select {
		case ev, open := <-state.ch:
			if !open {
				state.markProducerDone()
				state.setTerminal(streamDone, nil)
				return false
			}

			if ev.Error != nil {
				state.setTerminal(streamFailed, ev.Error)
				state.cancel()
				return false
			}

			state.setCurrent(ev)
			return true
		default:
			select {
			case ev, open := <-state.ch:
				if !open {
					state.markProducerDone()
					state.setTerminal(streamDone, nil)
					return false
				}

				if ev.Error != nil {
					state.setTerminal(streamFailed, ev.Error)
					state.cancel()
					return false
				}

				state.setCurrent(ev)
				return true
			case <-state.ctx.Done():
				state.setTerminal(streamFailed, state.ctx.Err())
				return false
			}
		}
	}

	closeFn := func() error {
		state.closeOnce.Do(func() {
			state.setTerminal(streamClosed, nil)
			state.cancel()
			state.startDrain()
			<-state.done
		})
		return nil
	}

	return NewStream(
		next,
		state.currentEvent,
		state.terminalErr,
		closeFn,
	)
}

func StreamFromSlice(events []StreamEvent, terminalErr error) *Stream {
	var (
		mu        sync.Mutex
		current   StreamEvent
		index     int
		closed    bool
		closeOnce sync.Once
	)

	next := func() bool {
		mu.Lock()
		defer mu.Unlock()

		if closed || index >= len(events) {
			return false
		}

		current = events[index]
		index++
		return true
	}

	event := func() StreamEvent {
		mu.Lock()
		defer mu.Unlock()
		return current
	}

	errFn := func() error {
		mu.Lock()
		defer mu.Unlock()
		// Terminal error is preserved even after Close().
		// This matches NewStreamFromChan behavior where Close()
		// after clean exhaustion preserves the terminal state.
		if index >= len(events) {
			return terminalErr
		}
		return nil
	}

	closeFn := func() error {
		closeOnce.Do(func() {
			mu.Lock()
			closed = true
			mu.Unlock()
		})
		return nil
	}

	return NewStream(next, event, errFn, closeFn)
}

type StreamInterceptor struct {
	// OnEvent runs for each upstream event before the caller sees it.
	// If OnEvent returns a non-nil error, the returned StreamEvent is ignored,
	// upstream is closed, and the error becomes stream.Err().
	OnEvent func(StreamEvent) (StreamEvent, error)

	// OnDone runs exactly once after clean upstream exhaustion.
	// If it returns a non-nil error, that error becomes stream.Err()
	// even though upstream completed cleanly.
	OnDone func() error

	// OnErr runs exactly once after upstream failure.
	// The return value, if non-nil, replaces the upstream error.
	// Return nil to pass through unchanged.
	OnErr func(error) error

	// OnClose runs exactly once when Close() wins before clean exhaustion.
	// It never runs after OnDone.
	OnClose func() error
}

type transformState struct {
	mu       sync.Mutex
	current  StreamEvent
	err      error
	closeErr error
	terminal streamTerminal

	terminalOnce sync.Once
	callbackOnce sync.Once
	closeOnce    sync.Once
}

func (s *transformState) setCurrent(ev StreamEvent) {
	s.mu.Lock()
	s.current = ev
	s.mu.Unlock()
}

func (s *transformState) currentEvent() StreamEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current
}

func (s *transformState) setTerminal(kind streamTerminal, err error) {
	s.terminalOnce.Do(func() {
		s.mu.Lock()
		s.terminal = kind
		s.err = err
		s.mu.Unlock()
	})
}

func (s *transformState) terminalState() streamTerminal {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.terminal
}

func (s *transformState) terminalErr() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

func (s *transformState) appendCloseErr(err error) {
	if err == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closeErr == nil {
		s.closeErr = err
		return
	}
	s.closeErr = errors.Join(s.closeErr, err)
}

func (s *transformState) getCloseErr() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closeErr
}

func TransformStream(upstream *Stream, interceptor StreamInterceptor) *Stream {
	state := &transformState{terminal: streamOpen}

	next := func() (ok bool) {
		defer func() {
			if r := recover(); r != nil {
				func() {
					defer func() {
						if recover() != nil {
							// Best-effort cleanup only. The stream is already failing.
						}
					}()
					_ = upstream.Close()
				}()
				state.setTerminal(streamFailed, NewStreamPanicError(r))
				ok = false
			}
		}()

		if state.terminalState() != streamOpen {
			return false
		}

		if !upstream.Next() {
			if state.terminalState() == streamClosed {
				return false
			}

			if err := upstream.Err(); err != nil {
				state.callbackOnce.Do(func() {
					finalErr := err
					if interceptor.OnErr != nil {
						if replacement := interceptor.OnErr(err); replacement != nil {
							finalErr = replacement
						}
					}
					state.setTerminal(streamFailed, finalErr)
				})
				return false
			}

			state.callbackOnce.Do(func() {
				if interceptor.OnDone != nil {
					if err := interceptor.OnDone(); err != nil {
						state.setTerminal(streamFailed, err)
						return
					}
				}
				state.setTerminal(streamDone, nil)
			})
			return false
		}

		ev := upstream.Event()
		if interceptor.OnEvent != nil {
			transformed, err := interceptor.OnEvent(ev)
			if err != nil {
				func() {
					defer func() {
						if recover() != nil {
							// Best-effort cleanup only. The stream is already failing.
						}
					}()
					_ = upstream.Close()
				}()
				state.setTerminal(streamFailed, err)
				return false
			}
			ev = transformed
		}

		state.setCurrent(ev)
		return true
	}

	closeFn := func() error {
		state.closeOnce.Do(func() {
			if state.terminalState() == streamOpen {
				state.callbackOnce.Do(func() {
					state.setTerminal(streamClosed, nil)
					func() {
						defer func() {
							if r := recover(); r != nil {
								state.appendCloseErr(NewStreamPanicError(r))
							}
						}()
						if interceptor.OnClose != nil {
							state.appendCloseErr(interceptor.OnClose())
						}
					}()
				})
			}
			func() {
				defer func() {
					if r := recover(); r != nil {
						state.appendCloseErr(NewStreamPanicError(r))
					}
				}()
				state.appendCloseErr(upstream.Close())
			}()
		})
		return state.getCloseErr()
	}

	return NewStream(
		next,
		state.currentEvent,
		state.terminalErr,
		closeFn,
	)
}
