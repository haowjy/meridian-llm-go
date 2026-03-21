package llmprovider

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func testStreamEvent(id string) StreamEvent {
	return StreamEvent{
		Metadata: &StreamMetadata{
			GenerationID: id,
			Model:        "test-model-" + id,
		},
	}
}

func assertPanics(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic, got nil")
		}
	}()
	fn()
}

func TestNewStreamFromChan_CleanCompletion(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := make(chan StreamEvent, 3)
	want := []StreamEvent{
		testStreamEvent("1"),
		testStreamEvent("2"),
		testStreamEvent("3"),
	}
	for _, ev := range want {
		ch <- ev
	}
	close(ch)

	stream := NewStreamFromChan(ctx, ch, cancel)
	defer stream.Close()

	var got []StreamEvent
	for stream.Next() {
		got = append(got, stream.Event())
	}

	if len(got) != len(want) {
		t.Fatalf("len(got)=%d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Metadata == nil || want[i].Metadata == nil {
			t.Fatalf("unexpected nil metadata at index %d", i)
		}
		if got[i].Metadata.GenerationID != want[i].Metadata.GenerationID {
			t.Fatalf("event %d generation_id=%q, want %q", i, got[i].Metadata.GenerationID, want[i].Metadata.GenerationID)
		}
	}

	if err := stream.Err(); err != nil {
		t.Fatalf("Err()=%v, want nil", err)
	}
}

func TestNewStreamFromChan_InBandError(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	providerErr := errors.New("provider failed")
	ch := make(chan StreamEvent, 1)
	ch <- StreamEvent{Error: providerErr}
	close(ch)

	stream := NewStreamFromChan(ctx, ch, cancel)
	defer stream.Close()

	if stream.Next() {
		t.Fatal("Next()=true, want false")
	}
	if !errors.Is(stream.Err(), providerErr) {
		t.Fatalf("Err()=%v, want %v", stream.Err(), providerErr)
	}
}

func TestNewStreamFromChan_ContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan StreamEvent)
	stream := NewStreamFromChan(ctx, ch, cancel)

	cancel()

	if stream.Next() {
		t.Fatal("Next()=true, want false")
	}
	if !errors.Is(stream.Err(), context.Canceled) {
		t.Fatalf("Err()=%v, want context.Canceled", stream.Err())
	}
}

func TestNewStreamFromChan_EarlyClose(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := make(chan StreamEvent, 2)
	ch <- testStreamEvent("early-1")
	ch <- testStreamEvent("early-2")
	close(ch)

	stream := NewStreamFromChan(ctx, ch, cancel)

	closeDone := make(chan error, 1)
	go func() {
		closeDone <- stream.Close()
	}()

	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close() error: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Close() timed out (possible deadlock)")
	}

	if stream.Next() {
		t.Fatal("Next()=true after Close(), want false")
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("Err()=%v, want nil after early Close()", err)
	}
}

func TestNewStreamFromChan_DrainPath(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := make(chan StreamEvent)
	stream := NewStreamFromChan(ctx, ch, cancel)

	producerDone := make(chan struct{})
	go func() {
		defer close(producerDone)
		ch <- testStreamEvent("drain-1")
		ch <- testStreamEvent("drain-2")
		close(ch)
	}()

	closeDone := make(chan error, 1)
	go func() {
		closeDone <- stream.Close()
	}()

	select {
	case <-producerDone:
	case <-time.After(1 * time.Second):
		t.Fatal("producer remained blocked after Close(); drain path did not unblock sender")
	}

	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close() error: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Close() did not return after producer completion")
	}
}

func TestNewStreamFromChan_PanicConversion(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := make(chan StreamEvent, 1)
	ch <- StreamEvent{Error: NewStreamPanicError("boom")}
	close(ch)

	stream := NewStreamFromChan(ctx, ch, cancel)
	defer stream.Close()

	if stream.Next() {
		t.Fatal("Next()=true, want false on panic error event")
	}

	var panicErr *StreamPanicError
	if !errors.As(stream.Err(), &panicErr) {
		t.Fatalf("Err()=%T, want *StreamPanicError", stream.Err())
	}
	if panicErr.Recovered != "boom" {
		t.Fatalf("panic recovered value=%v, want boom", panicErr.Recovered)
	}
}

func TestNewStreamFromChan_NilPanics(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ch := make(chan StreamEvent)
	cancel := func() {}

	t.Run("nil ctx", func(t *testing.T) {
		assertPanics(t, func() {
			NewStreamFromChan(nil, ch, cancel)
		})
	})

	t.Run("nil ch", func(t *testing.T) {
		assertPanics(t, func() {
			NewStreamFromChan(ctx, nil, cancel)
		})
	})

	t.Run("nil cancel", func(t *testing.T) {
		assertPanics(t, func() {
			NewStreamFromChan(ctx, ch, nil)
		})
	})
}

func TestNewStreamFromChan_ConcurrentCloseAndNext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := make(chan StreamEvent)
	stream := NewStreamFromChan(ctx, ch, cancel)

	nextDone := make(chan bool, 1)
	nextPanic := make(chan any, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				nextPanic <- r
			}
		}()
		nextDone <- stream.Next()
	}()

	// Let Next() enter its blocking receive path.
	time.Sleep(20 * time.Millisecond)

	closeDone := make(chan error, 1)
	go func() {
		closeDone <- stream.Close()
	}()

	go func() {
		time.Sleep(40 * time.Millisecond)
		close(ch)
	}()

	select {
	case r := <-nextPanic:
		t.Fatalf("Next() panicked: %v", r)
	case ok := <-nextDone:
		if ok {
			t.Fatal("Next()=true, want false")
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Next() timed out (possible deadlock)")
	}

	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close() error: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Close() timed out (possible deadlock)")
	}

	if err := stream.Err(); err != nil {
		t.Fatalf("Err()=%v, want nil after Close() wins terminal state", err)
	}
}

func TestNewStreamFromChan_EventPriorityOverCancel(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := make(chan StreamEvent, 2)
	ev1 := testStreamEvent("priority-1")
	ev2 := testStreamEvent("priority-2")
	ch <- ev1
	ch <- ev2

	stream := NewStreamFromChan(ctx, ch, cancel)

	cancel()

	if !stream.Next() {
		t.Fatal("first Next()=false, want true")
	}
	got1 := stream.Event()
	if got1.Metadata == nil || got1.Metadata.GenerationID != ev1.Metadata.GenerationID {
		t.Fatalf("first event generation_id=%v, want %q", got1.Metadata, ev1.Metadata.GenerationID)
	}

	if !stream.Next() {
		t.Fatal("second Next()=false, want true")
	}
	got2 := stream.Event()
	if got2.Metadata == nil || got2.Metadata.GenerationID != ev2.Metadata.GenerationID {
		t.Fatalf("second event generation_id=%v, want %q", got2.Metadata, ev2.Metadata.GenerationID)
	}

	if stream.Next() {
		t.Fatal("third Next()=true, want false")
	}
	if !errors.Is(stream.Err(), context.Canceled) {
		t.Fatalf("Err()=%v, want context.Canceled after buffered events are consumed", stream.Err())
	}

	close(ch)
}

func TestStreamFromSlice_DeterministicIteration(t *testing.T) {
	t.Parallel()

	want := []StreamEvent{
		testStreamEvent("slice-1"),
		testStreamEvent("slice-2"),
		testStreamEvent("slice-3"),
	}
	stream := StreamFromSlice(want, nil)
	defer stream.Close()

	var got []StreamEvent
	for stream.Next() {
		got = append(got, stream.Event())
	}

	if len(got) != len(want) {
		t.Fatalf("len(got)=%d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Metadata == nil || want[i].Metadata == nil {
			t.Fatalf("unexpected nil metadata at index %d", i)
		}
		if got[i].Metadata.GenerationID != want[i].Metadata.GenerationID {
			t.Fatalf("event %d generation_id=%q, want %q", i, got[i].Metadata.GenerationID, want[i].Metadata.GenerationID)
		}
	}
}

func TestStreamFromSlice_TerminalError(t *testing.T) {
	t.Parallel()

	terminalErr := errors.New("terminal error")
	stream := StreamFromSlice([]StreamEvent{testStreamEvent("only")}, terminalErr)
	defer stream.Close()

	if !stream.Next() {
		t.Fatal("first Next()=false, want true")
	}
	if stream.Next() {
		t.Fatal("second Next()=true, want false")
	}
	if !errors.Is(stream.Err(), terminalErr) {
		t.Fatalf("Err()=%v, want %v", stream.Err(), terminalErr)
	}
}

func TestStreamFromSlice_NilTerminalError(t *testing.T) {
	t.Parallel()

	stream := StreamFromSlice([]StreamEvent{testStreamEvent("only")}, nil)
	defer stream.Close()

	if !stream.Next() {
		t.Fatal("first Next()=false, want true")
	}
	if stream.Next() {
		t.Fatal("second Next()=true, want false")
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("Err()=%v, want nil", err)
	}
}

func TestStreamFromSlice_CloseStopsIteration(t *testing.T) {
	t.Parallel()

	stream := StreamFromSlice([]StreamEvent{
		testStreamEvent("a"),
		testStreamEvent("b"),
	}, errors.New("terminal"))

	if err := stream.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}
	if stream.Next() {
		t.Fatal("Next()=true after Close(), want false")
	}
}

func TestStreamFromSlice_EmptySlice(t *testing.T) {
	t.Parallel()

	stream := StreamFromSlice(nil, nil)
	defer stream.Close()

	if stream.Next() {
		t.Fatal("Next()=true, want false for empty stream")
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("Err()=%v, want nil", err)
	}
}

func TestTransformStream_OnEventPassthrough(t *testing.T) {
	t.Parallel()

	want := []StreamEvent{
		testStreamEvent("pass-1"),
		testStreamEvent("pass-2"),
	}
	stream := TransformStream(StreamFromSlice(want, nil), StreamInterceptor{})
	defer stream.Close()

	var got []StreamEvent
	for stream.Next() {
		got = append(got, stream.Event())
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("Err()=%v, want nil", err)
	}

	if len(got) != len(want) {
		t.Fatalf("len(got)=%d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Metadata == nil || want[i].Metadata == nil {
			t.Fatalf("unexpected nil metadata at index %d", i)
		}
		if got[i].Metadata.GenerationID != want[i].Metadata.GenerationID {
			t.Fatalf("event %d generation_id=%q, want %q", i, got[i].Metadata.GenerationID, want[i].Metadata.GenerationID)
		}
	}
}

func TestTransformStream_OnEventTransform(t *testing.T) {
	t.Parallel()

	stream := TransformStream(
		StreamFromSlice([]StreamEvent{testStreamEvent("orig")}, nil),
		StreamInterceptor{
			OnEvent: func(ev StreamEvent) (StreamEvent, error) {
				if ev.Metadata == nil {
					return ev, errors.New("expected metadata")
				}
				ev.Metadata.GenerationID = "transformed"
				return ev, nil
			},
		},
	)
	defer stream.Close()

	if !stream.Next() {
		t.Fatal("Next()=false, want true")
	}
	got := stream.Event()
	if got.Metadata == nil || got.Metadata.GenerationID != "transformed" {
		t.Fatalf("GenerationID=%v, want transformed", got.Metadata)
	}

	if stream.Next() {
		t.Fatal("second Next()=true, want false")
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("Err()=%v, want nil", err)
	}
}

func TestTransformStream_OnEventError(t *testing.T) {
	t.Parallel()

	interceptorErr := errors.New("interceptor failed")
	upstream := StreamFromSlice([]StreamEvent{
		testStreamEvent("event-1"),
		testStreamEvent("event-2"),
	}, nil)

	stream := TransformStream(
		upstream,
		StreamInterceptor{
			OnEvent: func(StreamEvent) (StreamEvent, error) {
				return testStreamEvent("ignored"), interceptorErr
			},
		},
	)
	defer stream.Close()

	if stream.Next() {
		t.Fatal("Next()=true, want false when OnEvent returns error")
	}
	if !errors.Is(stream.Err(), interceptorErr) {
		t.Fatalf("Err()=%v, want %v", stream.Err(), interceptorErr)
	}

	// OnEvent error path must close upstream immediately.
	if upstream.Next() {
		t.Fatal("upstream.Next()=true, want false because upstream should be closed")
	}
}

func TestTransformStream_OnDoneSuccess(t *testing.T) {
	t.Parallel()

	var onDoneCalls atomic.Int32
	stream := TransformStream(
		StreamFromSlice([]StreamEvent{testStreamEvent("done")}, nil),
		StreamInterceptor{
			OnDone: func() error {
				onDoneCalls.Add(1)
				return nil
			},
		},
	)
	defer stream.Close()

	if !stream.Next() {
		t.Fatal("first Next()=false, want true")
	}
	if stream.Next() {
		t.Fatal("second Next()=true, want false")
	}
	if got := onDoneCalls.Load(); got != 1 {
		t.Fatalf("OnDone calls=%d, want 1", got)
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("Err()=%v, want nil", err)
	}
}

func TestTransformStream_OnDoneError(t *testing.T) {
	t.Parallel()

	doneErr := errors.New("ondone failed")
	stream := TransformStream(
		StreamFromSlice([]StreamEvent{testStreamEvent("done")}, nil),
		StreamInterceptor{
			OnDone: func() error { return doneErr },
		},
	)
	defer stream.Close()

	if !stream.Next() {
		t.Fatal("first Next()=false, want true")
	}
	if stream.Next() {
		t.Fatal("second Next()=true, want false")
	}
	if !errors.Is(stream.Err(), doneErr) {
		t.Fatalf("Err()=%v, want %v", stream.Err(), doneErr)
	}
}

func TestTransformStream_OnErrPassthrough(t *testing.T) {
	t.Parallel()

	upstreamErr := errors.New("upstream failed")
	var onErrCalls atomic.Int32

	stream := TransformStream(
		StreamFromSlice([]StreamEvent{testStreamEvent("event")}, upstreamErr),
		StreamInterceptor{
			OnErr: func(err error) error {
				onErrCalls.Add(1)
				if !errors.Is(err, upstreamErr) {
					t.Fatalf("OnErr arg=%v, want %v", err, upstreamErr)
				}
				return nil
			},
		},
	)
	defer stream.Close()

	if !stream.Next() {
		t.Fatal("first Next()=false, want true")
	}
	if stream.Next() {
		t.Fatal("second Next()=true, want false")
	}
	if got := onErrCalls.Load(); got != 1 {
		t.Fatalf("OnErr calls=%d, want 1", got)
	}
	if !errors.Is(stream.Err(), upstreamErr) {
		t.Fatalf("Err()=%v, want %v", stream.Err(), upstreamErr)
	}
}

func TestTransformStream_OnErrReplacement(t *testing.T) {
	t.Parallel()

	upstreamErr := errors.New("upstream failed")
	replacementErr := errors.New("replacement failed")

	stream := TransformStream(
		StreamFromSlice([]StreamEvent{testStreamEvent("event")}, upstreamErr),
		StreamInterceptor{
			OnErr: func(err error) error {
				if !errors.Is(err, upstreamErr) {
					t.Fatalf("OnErr arg=%v, want %v", err, upstreamErr)
				}
				return replacementErr
			},
		},
	)
	defer stream.Close()

	if !stream.Next() {
		t.Fatal("first Next()=false, want true")
	}
	if stream.Next() {
		t.Fatal("second Next()=true, want false")
	}
	if !errors.Is(stream.Err(), replacementErr) {
		t.Fatalf("Err()=%v, want %v", stream.Err(), replacementErr)
	}
}

func TestTransformStream_OnCloseFiresOnEarlyClose(t *testing.T) {
	t.Parallel()

	var onCloseCalls atomic.Int32
	var onDoneCalls atomic.Int32
	stream := TransformStream(
		StreamFromSlice([]StreamEvent{
			testStreamEvent("event-1"),
			testStreamEvent("event-2"),
		}, nil),
		StreamInterceptor{
			OnClose: func() error {
				onCloseCalls.Add(1)
				return nil
			},
			OnDone: func() error {
				onDoneCalls.Add(1)
				return nil
			},
		},
	)

	if err := stream.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}
	if got := onCloseCalls.Load(); got != 1 {
		t.Fatalf("OnClose calls=%d, want 1", got)
	}
	if got := onDoneCalls.Load(); got != 0 {
		t.Fatalf("OnDone calls=%d, want 0", got)
	}
	if stream.Next() {
		t.Fatal("Next()=true after Close(), want false")
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("Err()=%v, want nil", err)
	}
}

func TestTransformStream_OnDoneAndOnCloseMutualExclusion(t *testing.T) {
	t.Parallel()

	var onDoneCalls atomic.Int32
	var onCloseCalls atomic.Int32
	stream := TransformStream(
		StreamFromSlice([]StreamEvent{testStreamEvent("event")}, nil),
		StreamInterceptor{
			OnDone: func() error {
				onDoneCalls.Add(1)
				return nil
			},
			OnClose: func() error {
				onCloseCalls.Add(1)
				return nil
			},
		},
	)

	if !stream.Next() {
		t.Fatal("first Next()=false, want true")
	}
	if stream.Next() {
		t.Fatal("second Next()=true, want false")
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("Err()=%v, want nil", err)
	}

	if err := stream.Close(); err != nil {
		t.Fatalf("Close() error after done: %v", err)
	}

	if got := onDoneCalls.Load(); got != 1 {
		t.Fatalf("OnDone calls=%d, want 1", got)
	}
	if got := onCloseCalls.Load(); got != 0 {
		t.Fatalf("OnClose calls=%d, want 0", got)
	}
}

func TestTransformStream_ConcurrentCloseRacesNext_MutualExclusion(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := make(chan StreamEvent)
	allowClose := make(chan struct{})
	raceStart := make(chan struct{})
	go func() {
		events <- testStreamEvent("race")
		<-allowClose
		<-raceStart
		close(events)
	}()

	var doneCount atomic.Int32
	var closeCount atomic.Int32
	stream := TransformStream(
		NewStreamFromChan(ctx, events, cancel),
		StreamInterceptor{
			OnDone: func() error {
				doneCount.Add(1)
				return nil
			},
			OnClose: func() error {
				closeCount.Add(1)
				return nil
			},
		},
	)
	defer stream.Close()

	if !stream.Next() {
		t.Fatal("first Next()=false, want true")
	}

	close(allowClose)

	nextDone := make(chan bool, 1)
	go func() {
		<-raceStart
		nextDone <- stream.Next()
	}()

	closeDone := make(chan error, 1)
	go func() {
		<-raceStart
		closeDone <- stream.Close()
	}()

	close(raceStart)

	select {
	case ok := <-nextDone:
		if ok {
			t.Fatal("racing Next()=true, want false")
		}
	case <-time.After(1 * time.Second):
		t.Fatal("racing Next() timed out")
	}

	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("racing Close() error: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("racing Close() timed out")
	}

	total := doneCount.Load() + closeCount.Load()
	if total != 1 {
		t.Fatalf("OnDone+OnClose callbacks=%d, want 1 (OnDone=%d OnClose=%d)", total, doneCount.Load(), closeCount.Load())
	}
}

func TestTransformStream_OnDonePanic_ConcurrentClose(t *testing.T) {
	t.Parallel()

	var closeCount atomic.Int32
	stream := TransformStream(
		StreamFromSlice([]StreamEvent{testStreamEvent("event")}, nil),
		StreamInterceptor{
			OnDone: func() error {
				panic("panic in OnDone")
			},
			OnClose: func() error {
				closeCount.Add(1)
				return nil
			},
		},
	)
	defer stream.Close()

	if !stream.Next() {
		t.Fatal("first Next()=false, want true")
	}
	if stream.Next() {
		t.Fatal("second Next()=true, want false after OnDone panic")
	}

	var panicErr *StreamPanicError
	if !errors.As(stream.Err(), &panicErr) {
		t.Fatalf("Err()=%T, want *StreamPanicError", stream.Err())
	}

	if err := stream.Close(); err != nil {
		t.Fatalf("Close() error=%v, want nil", err)
	}
	if got := closeCount.Load(); got != 0 {
		t.Fatalf("OnClose calls=%d, want 0", got)
	}
}

func TestTransformStream_MultiLayerComposition(t *testing.T) {
	t.Parallel()

	baseErr := errors.New("base error")
	innerErr := errors.New("inner replacement")
	var outerOnErrCalls atomic.Int32
	var outerOnDoneCalls atomic.Int32

	inner := TransformStream(
		StreamFromSlice([]StreamEvent{testStreamEvent("event")}, baseErr),
		StreamInterceptor{
			OnErr: func(err error) error {
				if !errors.Is(err, baseErr) {
					t.Fatalf("inner OnErr arg=%v, want %v", err, baseErr)
				}
				return innerErr
			},
		},
	)

	outer := TransformStream(
		inner,
		StreamInterceptor{
			OnErr: func(err error) error {
				outerOnErrCalls.Add(1)
				if !errors.Is(err, innerErr) {
					t.Fatalf("outer OnErr arg=%v, want %v", err, innerErr)
				}
				return nil
			},
			OnDone: func() error {
				outerOnDoneCalls.Add(1)
				return nil
			},
		},
	)
	defer outer.Close()

	if !outer.Next() {
		t.Fatal("first Next()=false, want true")
	}
	if outer.Next() {
		t.Fatal("second Next()=true, want false")
	}
	if got := outerOnErrCalls.Load(); got != 1 {
		t.Fatalf("outer OnErr calls=%d, want 1", got)
	}
	if got := outerOnDoneCalls.Load(); got != 0 {
		t.Fatalf("outer OnDone calls=%d, want 0", got)
	}
	if !errors.Is(outer.Err(), innerErr) {
		t.Fatalf("Err()=%v, want %v", outer.Err(), innerErr)
	}
}

func TestTransformStream_PanicInOnEvent(t *testing.T) {
	t.Parallel()

	stream := TransformStream(
		StreamFromSlice([]StreamEvent{testStreamEvent("event")}, nil),
		StreamInterceptor{
			OnEvent: func(StreamEvent) (StreamEvent, error) {
				panic("panic in OnEvent")
			},
		},
	)
	defer stream.Close()

	if stream.Next() {
		t.Fatal("Next()=true, want false after panic")
	}

	var panicErr *StreamPanicError
	if !errors.As(stream.Err(), &panicErr) {
		t.Fatalf("Err()=%T, want *StreamPanicError", stream.Err())
	}
}

func TestTransformStream_PanicInOnClose(t *testing.T) {
	t.Parallel()

	stream := TransformStream(
		StreamFromSlice([]StreamEvent{
			testStreamEvent("event-1"),
			testStreamEvent("event-2"),
		}, nil),
		StreamInterceptor{
			OnClose: func() error {
				panic("panic in OnClose")
			},
		},
	)

	err := stream.Close()
	if err == nil {
		t.Fatal("Close() error=nil, want panic conversion error")
	}

	var panicErr *StreamPanicError
	if !errors.As(err, &panicErr) {
		t.Fatalf("Close() error type=%T, want *StreamPanicError", err)
	}

	if stream.Next() {
		t.Fatal("Next()=true after Close(), want false")
	}
	if stream.Err() != nil {
		t.Fatalf("Err()=%v, want nil for early close terminal state", stream.Err())
	}
}

func TestNewStream_NilCallbackPanics(t *testing.T) {
	t.Parallel()

	next := func() bool { return false }
	event := func() StreamEvent { return StreamEvent{} }
	errFn := func() error { return nil }
	closeFn := func() error { return nil }

	tests := []struct {
		name    string
		next    func() bool
		event   func() StreamEvent
		errFn   func() error
		closeFn func() error
	}{
		{
			name:    "nil next",
			next:    nil,
			event:   event,
			errFn:   errFn,
			closeFn: closeFn,
		},
		{
			name:    "nil event",
			next:    next,
			event:   nil,
			errFn:   errFn,
			closeFn: closeFn,
		},
		{
			name:    "nil errFn",
			next:    next,
			event:   event,
			errFn:   nil,
			closeFn: closeFn,
		},
		{
			name:    "nil closeFn",
			next:    next,
			event:   event,
			errFn:   errFn,
			closeFn: nil,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertPanics(t, func() {
				NewStream(tc.next, tc.event, tc.errFn, tc.closeFn)
			})
		})
	}
}
