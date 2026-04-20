package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/homepc/atlas-audio-engine/internal/domain"
	"github.com/homepc/atlas-audio-engine/internal/source/localfiles"
	"github.com/homepc/atlas-audio-engine/internal/store"
	"github.com/homepc/atlas-audio-engine/internal/store/memory"
)

type bootstrapProber struct {
	metadataByPath map[string]localfiles.Metadata
}

func (b bootstrapProber) Probe(_ context.Context, path string) (localfiles.Metadata, error) {
	return b.metadataByPath[path], nil
}

func TestSeedLocalChannelScansAndPersistsPlaylist(t *testing.T) {
	t.Parallel()

	mediaDir := t.TempDir()
	firstTrack := filepath.Join(mediaDir, "alpha.mp3")
	secondTrack := filepath.Join(mediaDir, "beta.mp3")
	for _, item := range []string{firstTrack, secondTrack} {
		if err := os.WriteFile(item, []byte("audio"), 0o644); err != nil {
			t.Fatalf("write track: %v", err)
		}
	}

	firstAbs, _ := filepath.Abs(firstTrack)
	secondAbs, _ := filepath.Abs(secondTrack)

	source := localfiles.NewAdapter(mediaDir, bootstrapProber{
		metadataByPath: map[string]localfiles.Metadata{
			firstAbs:  {Title: "Alpha", Artist: "Atlas", DurationMs: 120000},
			secondAbs: {Title: "Beta", Artist: "Atlas", DurationMs: 120000},
		},
	})
	repository := memory.NewStore()

	if err := SeedLocalChannel(context.Background(), repository, source, "local-library", "Local Library", time.Now().UTC()); err != nil {
		t.Fatalf("seed local channel: %v", err)
	}

	state, err := repository.GetChannelState(context.Background(), "local-library")
	if err != nil {
		t.Fatalf("get channel state: %v", err)
	}
	if len(state.PlaylistTrackIDs) != 2 {
		t.Fatalf("expected 2 playlist tracks, got %d", len(state.PlaylistTrackIDs))
	}
	if state.Channel.CurrentTrackID == "" {
		t.Fatalf("expected current track to be initialized")
	}
}

func TestSeedLocalChannelReconcilesStaleExistingPlaylist(t *testing.T) {
	t.Parallel()

	mediaDir := t.TempDir()
	firstTrack := filepath.Join(mediaDir, "alpha.mp3")
	secondTrack := filepath.Join(mediaDir, "beta.mp3")
	for _, item := range []string{firstTrack, secondTrack} {
		if err := os.WriteFile(item, []byte("audio"), 0o644); err != nil {
			t.Fatalf("write track: %v", err)
		}
	}

	firstAbs, _ := filepath.Abs(firstTrack)
	secondAbs, _ := filepath.Abs(secondTrack)

	source := localfiles.NewAdapter(mediaDir, bootstrapProber{
		metadataByPath: map[string]localfiles.Metadata{
			firstAbs:  {Title: "Alpha", Artist: "Atlas", DurationMs: 120000},
			secondAbs: {Title: "Beta", Artist: "Atlas", DurationMs: 120000},
		},
	})
	repository := memory.NewStore()
	startedAt := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	staleState := store.ChannelState{
		Channel: domain.Channel{
			ID:             "local-library",
			Name:           "Local Library",
			CreatedAt:      startedAt.Add(-time.Hour),
			StartedAt:      startedAt.Add(-time.Hour),
			CurrentTrackID: "stale-track",
			PlaylistCursor: 4,
		},
		PlaylistTrackIDs: []string{"stale-track"},
	}
	if err := repository.UpsertChannelState(context.Background(), staleState); err != nil {
		t.Fatalf("seed stale state: %v", err)
	}

	if err := SeedLocalChannel(context.Background(), repository, source, "local-library", "Local Library", startedAt); err != nil {
		t.Fatalf("reconcile local channel: %v", err)
	}

	state, err := repository.GetChannelState(context.Background(), "local-library")
	if err != nil {
		t.Fatalf("get channel state: %v", err)
	}
	if len(state.PlaylistTrackIDs) != 2 {
		t.Fatalf("expected reconciled playlist to include 2 current tracks, got %d", len(state.PlaylistTrackIDs))
	}
	if state.Channel.CurrentTrackID != state.PlaylistTrackIDs[0] {
		t.Fatalf("expected current track to reset to playlist head, got %q against %#v", state.Channel.CurrentTrackID, state.PlaylistTrackIDs)
	}
	if state.Channel.PlaylistCursor != 0 {
		t.Fatalf("expected playlist cursor reset to 0, got %d", state.Channel.PlaylistCursor)
	}
	if !state.Channel.StartedAt.Equal(startedAt) {
		t.Fatalf("expected started_at reset to %s, got %s", startedAt, state.Channel.StartedAt)
	}
}
