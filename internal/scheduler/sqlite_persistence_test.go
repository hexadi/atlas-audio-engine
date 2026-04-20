package scheduler

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/homepc/atlas-audio-engine/internal/domain"
	"github.com/homepc/atlas-audio-engine/internal/store"
	"github.com/homepc/atlas-audio-engine/internal/store/sqlite"
)

func TestReplacePlaylistPersistsAcrossSQLiteReopen(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "atlas-playlist.db")
	now := time.Date(2026, 4, 20, 9, 30, 0, 0, time.UTC)
	library := fakeLibrary{
		tracks: map[string]domain.Track{
			"track-1": {ID: "track-1", Title: "Track One", Artist: "Artist", DurationMs: 60000, SourceType: domain.SourceTypeLocal},
			"track-2": {ID: "track-2", Title: "Track Two", Artist: "Artist", DurationMs: 60000, SourceType: domain.SourceTypeLocal},
			"track-3": {ID: "track-3", Title: "Track Three", Artist: "Artist", DurationMs: 60000, SourceType: domain.SourceTypeLocal},
		},
	}

	repository, err := sqlite.NewStore(dbPath)
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}

	initialState := store.ChannelState{
		Channel: domain.Channel{
			ID:             "channel-1",
			Name:           "SQLite Channel",
			CreatedAt:      now.Add(-time.Hour),
			StartedAt:      now.Add(-time.Minute),
			CurrentTrackID: "track-1",
			PlaylistCursor: 0,
		},
		PlaylistTrackIDs: []string{"track-1", "track-2"},
	}
	if err := repository.UpsertChannelState(ctx, initialState); err != nil {
		t.Fatalf("seed sqlite state: %v", err)
	}

	service := NewServiceWithClock(repository, library, func() time.Time { return now })
	playlist, err := service.ReplacePlaylist(ctx, "channel-1", []string{"track-3", "track-1", "track-2"})
	if err != nil {
		t.Fatalf("replace playlist: %v", err)
	}
	if len(playlist) != 3 || playlist[0].TrackID != "track-3" || playlist[1].TrackID != "track-1" || playlist[2].TrackID != "track-2" {
		t.Fatalf("expected replaced playlist order [track-3, track-1, track-2], got %#v", playlist)
	}

	if err := repository.Close(); err != nil {
		t.Fatalf("close sqlite store: %v", err)
	}

	reopenedRepository, err := sqlite.NewStore(dbPath)
	if err != nil {
		t.Fatalf("reopen sqlite store: %v", err)
	}
	defer func() {
		if err := reopenedRepository.Close(); err != nil {
			t.Fatalf("close reopened sqlite store: %v", err)
		}
	}()

	reopenedService := NewServiceWithClock(reopenedRepository, library, func() time.Time { return now })
	reopenedPlaylist, err := reopenedService.Playlist(ctx, "channel-1")
	if err != nil {
		t.Fatalf("read reopened playlist: %v", err)
	}
	if len(reopenedPlaylist) != 3 || reopenedPlaylist[0].TrackID != "track-3" || reopenedPlaylist[1].TrackID != "track-1" || reopenedPlaylist[2].TrackID != "track-2" {
		t.Fatalf("expected persisted playlist order [track-3, track-1, track-2], got %#v", reopenedPlaylist)
	}

	playhead, err := reopenedService.CurrentNow(ctx, "channel-1")
	if err != nil {
		t.Fatalf("current after reopen: %v", err)
	}
	if playhead.TrackID != "track-3" {
		t.Fatalf("expected current track to persist as playlist head track-3, got %s", playhead.TrackID)
	}
	if !playhead.StartedAt.Equal(now) {
		t.Fatalf("expected restarted playhead timestamp %s, got %s", now, playhead.StartedAt)
	}
}
