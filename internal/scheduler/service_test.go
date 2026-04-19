package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/homepc/atlas-audio-engine/internal/domain"
	"github.com/homepc/atlas-audio-engine/internal/source"
	"github.com/homepc/atlas-audio-engine/internal/store"
	"github.com/homepc/atlas-audio-engine/internal/store/memory"
)

type fakeLibrary struct {
	tracks map[string]domain.Track
}

func (f fakeLibrary) ListTracks(context.Context) ([]domain.Track, error) {
	items := make([]domain.Track, 0, len(f.tracks))
	for _, track := range f.tracks {
		items = append(items, track)
	}
	return items, nil
}

func (f fakeLibrary) GetTrack(_ context.Context, id string) (domain.Track, error) {
	return f.tracks[id], nil
}

func (f fakeLibrary) ResolvePlayable(_ context.Context, id string) (source.Playable, error) {
	return source.Playable{TrackID: id, Path: "/tmp/" + id}, nil
}

func TestCurrentFallsBackToPlaylistWhenQueueIsEmpty(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 19, 12, 0, 30, 0, time.UTC)
	service := newTestService(t, now)

	playhead, err := service.Current(context.Background(), "channel-1", now)
	if err != nil {
		t.Fatalf("current: %v", err)
	}
	if playhead.TrackID != "track-1" {
		t.Fatalf("expected current playlist track, got %s", playhead.TrackID)
	}
}

func TestCurrentAdvancesToQueuedTrackAfterBoundary(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 19, 12, 1, 1, 0, time.UTC)
	service := newTestService(t, now)

	if _, err := service.Enqueue(context.Background(), "channel-1", "track-3"); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	playhead, err := service.Current(context.Background(), "channel-1", now)
	if err != nil {
		t.Fatalf("current: %v", err)
	}
	if playhead.TrackID != "track-3" {
		t.Fatalf("expected queued track to take over after boundary, got %s", playhead.TrackID)
	}
}

func TestCurrentCalculatesElapsedTime(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 19, 12, 0, 45, 0, time.UTC)
	service := newTestService(t, now)

	playhead, err := service.Current(context.Background(), "channel-1", now)
	if err != nil {
		t.Fatalf("current: %v", err)
	}
	if playhead.ElapsedMs != 45000 {
		t.Fatalf("expected elapsed 45000ms, got %d", playhead.ElapsedMs)
	}
}

func newTestService(t *testing.T, now time.Time) *Service {
	t.Helper()

	repository := memory.NewStore()
	state := store.ChannelState{
		Channel: domain.Channel{
			ID:             "channel-1",
			Name:           "Test Channel",
			CreatedAt:      now.Add(-time.Hour),
			StartedAt:      time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC),
			CurrentTrackID: "track-1",
			PlaylistCursor: 0,
		},
		PlaylistTrackIDs: []string{"track-1", "track-2"},
	}
	if err := repository.UpsertChannelState(context.Background(), state); err != nil {
		t.Fatalf("seed store: %v", err)
	}

	library := fakeLibrary{
		tracks: map[string]domain.Track{
			"track-1": {ID: "track-1", Title: "Track One", Artist: "Artist", DurationMs: 60000, SourceType: domain.SourceTypeLocal},
			"track-2": {ID: "track-2", Title: "Track Two", Artist: "Artist", DurationMs: 60000, SourceType: domain.SourceTypeLocal},
			"track-3": {ID: "track-3", Title: "Queued", Artist: "Artist", DurationMs: 60000, SourceType: domain.SourceTypeLocal},
		},
	}

	return NewServiceWithClock(repository, library, func() time.Time { return now })
}
