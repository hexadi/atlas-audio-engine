package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/homepc/atlas-audio-engine/internal/source/localfiles"
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
