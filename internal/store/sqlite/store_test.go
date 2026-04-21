package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/homepc/atlas-audio-engine/internal/domain"
	"github.com/homepc/atlas-audio-engine/internal/store"
)

func TestChannelStatePersistsAcrossSQLiteReopen(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "atlas-state.db")
	now := time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC)

	repository, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}

	state := store.ChannelState{
		Channel: domain.Channel{
			ID:             "channel-1",
			Name:           "SQLite Channel",
			Enabled:        true,
			CreatedAt:      now.Add(-time.Hour),
			StartedAt:      now,
			CurrentTrackID: "track-2",
			PlaylistCursor: 1,
		},
		PlaylistTrackIDs: []string{"track-1", "track-2", "track-3"},
		Queue: []domain.QueueItem{
			{ID: "queue-1", ChannelID: "channel-1", TrackID: "track-4", EnqueuedAt: now.Add(time.Second)},
		},
	}
	if err := repository.UpsertChannelState(ctx, state); err != nil {
		t.Fatalf("upsert state: %v", err)
	}
	if err := repository.Close(); err != nil {
		t.Fatalf("close sqlite store: %v", err)
	}

	reopened, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("reopen sqlite store: %v", err)
	}
	defer reopened.Close()

	got, err := reopened.GetChannelState(ctx, "channel-1")
	if err != nil {
		t.Fatalf("get reopened state: %v", err)
	}
	if got.Channel.CurrentTrackID != "track-2" || got.Channel.PlaylistCursor != 1 {
		t.Fatalf("expected current track/cursor to persist, got %#v", got.Channel)
	}
	if !got.Channel.Enabled {
		t.Fatalf("expected channel enabled flag to persist")
	}
	if len(got.PlaylistTrackIDs) != 3 || got.PlaylistTrackIDs[0] != "track-1" || got.PlaylistTrackIDs[2] != "track-3" {
		t.Fatalf("expected playlist order to persist, got %#v", got.PlaylistTrackIDs)
	}
	if len(got.Queue) != 1 || got.Queue[0].TrackID != "track-4" {
		t.Fatalf("expected queue to persist, got %#v", got.Queue)
	}
}

func TestQueueItemsReadBackInEnqueueOrder(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "atlas-queue.db")
	now := time.Date(2026, 4, 20, 10, 30, 0, 0, time.UTC)

	repository, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer repository.Close()

	state := store.ChannelState{
		Channel: domain.Channel{
			ID:             "channel-1",
			Name:           "SQLite Channel",
			Enabled:        true,
			CreatedAt:      now.Add(-time.Hour),
			StartedAt:      now,
			CurrentTrackID: "track-1",
			PlaylistCursor: 0,
		},
		PlaylistTrackIDs: []string{"track-1"},
	}
	if err := repository.UpsertChannelState(ctx, state); err != nil {
		t.Fatalf("upsert state: %v", err)
	}

	if _, err := repository.Enqueue(ctx, "channel-1", "track-2", now.Add(2*time.Second)); err != nil {
		t.Fatalf("enqueue second: %v", err)
	}
	if _, err := repository.Enqueue(ctx, "channel-1", "track-3", now.Add(time.Second)); err != nil {
		t.Fatalf("enqueue first: %v", err)
	}

	got, err := repository.GetChannelState(ctx, "channel-1")
	if err != nil {
		t.Fatalf("get state: %v", err)
	}
	if len(got.Queue) != 2 {
		t.Fatalf("expected 2 queue items, got %d", len(got.Queue))
	}
	if got.Queue[0].TrackID != "track-3" || got.Queue[1].TrackID != "track-2" {
		t.Fatalf("expected queue ordered by enqueue time [track-3, track-2], got %#v", got.Queue)
	}
}

func TestDeleteChannelRemovesState(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "atlas-delete.db")
	now := time.Date(2026, 4, 20, 11, 0, 0, 0, time.UTC)

	repository, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer repository.Close()

	state := store.ChannelState{
		Channel: domain.Channel{
			ID:             "channel-1",
			Name:           "Delete Me",
			Enabled:        true,
			CreatedAt:      now.Add(-time.Hour),
			StartedAt:      now,
			CurrentTrackID: "track-1",
			PlaylistCursor: 0,
		},
		PlaylistTrackIDs: []string{"track-1"},
		Queue: []domain.QueueItem{
			{ID: "queue-1", ChannelID: "channel-1", TrackID: "track-2", EnqueuedAt: now},
		},
	}
	if err := repository.UpsertChannelState(ctx, state); err != nil {
		t.Fatalf("upsert state: %v", err)
	}
	if err := repository.DeleteChannel(ctx, "channel-1"); err != nil {
		t.Fatalf("delete channel: %v", err)
	}
	if _, err := repository.GetChannelState(ctx, "channel-1"); err == nil {
		t.Fatalf("expected deleted channel to be unavailable")
	}
}
