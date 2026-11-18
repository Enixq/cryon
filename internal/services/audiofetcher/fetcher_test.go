package audiofetcher

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRaceAudioStreamsReturnsWorkingResolverWithoutWaitingForFailure(t *testing.T) {
	started := time.Now()
	stream, err := raceAudioStreams(context.Background(), []audioStreamAttempt{
		{
			name:    "blocked",
			timeout: time.Second,
			resolve: func(ctx context.Context) (*AudioStream, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			},
		},
		{
			name:    "working",
			timeout: time.Second,
			resolve: func(context.Context) (*AudioStream, error) {
				return &AudioStream{URL: "https://audio.example/stream"}, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("raceAudioStreams: %v", err)
	}
	if stream.URL == "" {
		t.Fatal("working stream was not returned")
	}
	if time.Since(started) > 200*time.Millisecond {
		t.Fatal("race waited for the blocked resolver")
	}
}

func TestRaceAudioStreamsReturnsAttemptErrors(t *testing.T) {
	_, err := raceAudioStreams(context.Background(), []audioStreamAttempt{
		{name: "first", timeout: time.Second, resolve: func(context.Context) (*AudioStream, error) { return nil, errors.New("first failure") }},
		{name: "second", timeout: time.Second, resolve: func(context.Context) (*AudioStream, error) { return nil, errors.New("second failure") }},
	})
	if err == nil {
		t.Fatal("expected resolver error")
	}
}
