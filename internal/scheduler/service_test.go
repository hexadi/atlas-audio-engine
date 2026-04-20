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

func TestSkipAdvancesToQueuedTrackImmediately(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 19, 12, 0, 30, 0, time.UTC)
	service := newTestService(t, now)

	if _, err := service.Enqueue(context.Background(), "channel-1", "track-3"); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	playhead, err := service.Skip(context.Background(), "channel-1")
	if err != nil {
		t.Fatalf("skip: %v", err)
	}
	if playhead.TrackID != "track-3" {
		t.Fatalf("expected queued track after skip, got %s", playhead.TrackID)
	}
	if playhead.ElapsedMs != 0 {
		t.Fatalf("expected elapsed to reset after skip, got %d", playhead.ElapsedMs)
	}
}

func TestMoveQueueItemReordersEntries(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 19, 12, 0, 30, 0, time.UTC)
	service := newTestService(t, now)

	if _, err := service.Enqueue(context.Background(), "channel-1", "track-3"); err != nil {
		t.Fatalf("enqueue first: %v", err)
	}
	if _, err := service.Enqueue(context.Background(), "channel-1", "track-4"); err != nil {
		t.Fatalf("enqueue second: %v", err)
	}

	queue, err := service.Queue(context.Background(), "channel-1")
	if err != nil {
		t.Fatalf("queue: %v", err)
	}

	moved, err := service.MoveQueueItem(context.Background(), "channel-1", queue[1].ID, 1)
	if err != nil {
		t.Fatalf("move queue item: %v", err)
	}
	if moved[0].TrackID != "track-4" || moved[1].TrackID != "track-3" {
		t.Fatalf("expected queue order [track-4, track-3], got %#v", moved)
	}
}

func TestReplacePlaylistPersistsNewOrderAndResetsCurrentTrack(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 19, 12, 3, 0, 0, time.UTC)
	service := newTestService(t, now)

	playlist, err := service.ReplacePlaylist(context.Background(), "channel-1", []string{"track-2", "track-1", "track-3"})
	if err != nil {
		t.Fatalf("replace playlist: %v", err)
	}
	if len(playlist) != 3 {
		t.Fatalf("expected 3 playlist entries, got %d", len(playlist))
	}
	if playlist[0].TrackID != "track-2" || playlist[1].TrackID != "track-1" || playlist[2].TrackID != "track-3" {
		t.Fatalf("expected persisted playlist order [track-2, track-1, track-3], got %#v", playlist)
	}

	playhead, err := service.CurrentNow(context.Background(), "channel-1")
	if err != nil {
		t.Fatalf("current now: %v", err)
	}
	if playhead.TrackID != "track-2" {
		t.Fatalf("expected current track to reset to new playlist head, got %s", playhead.TrackID)
	}
}

func TestReplacePlaylistRejectsEmptyAndDuplicateTracks(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 19, 12, 3, 0, 0, time.UTC)
	service := newTestService(t, now)

	if _, err := service.ReplacePlaylist(context.Background(), "channel-1", nil); err == nil {
		t.Fatalf("expected empty playlist replacement to fail")
	}
	if _, err := service.ReplacePlaylist(context.Background(), "channel-1", []string{"track-1", "track-1"}); err == nil {
		t.Fatalf("expected duplicate playlist replacement to fail")
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
			"track-4": {ID: "track-4", Title: "Queued Again", Artist: "Artist", DurationMs: 60000, SourceType: domain.SourceTypeLocal},
		},
	}

	return NewServiceWithClock(repository, library, func() time.Time { return now })
}
