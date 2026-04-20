package localfiles

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

type fakeProber struct {
	metadataByPath map[string]Metadata
	callCount      int
}

func (f fakeProber) Probe(_ context.Context, path string) (Metadata, error) {
	return f.metadataByPath[path], nil
}

type countingProber struct {
	metadataByPath map[string]Metadata
	callCount      *int
}

func (f countingProber) Probe(_ context.Context, path string) (Metadata, error) {
	*f.callCount = *f.callCount + 1
	return f.metadataByPath[path], nil
}

func TestListTracksNormalizesLocalFiles(t *testing.T) {
	t.Parallel()

	mediaDir := t.TempDir()
	songPath := filepath.Join(mediaDir, "folder", "sample-song.mp3")
	if err := os.MkdirAll(filepath.Dir(songPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(songPath, []byte("not-real-audio"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	absolutePath, err := filepath.Abs(songPath)
	if err != nil {
		t.Fatalf("abs: %v", err)
	}

	adapter := NewAdapter(mediaDir, fakeProber{
		metadataByPath: map[string]Metadata{
			absolutePath: {
				Title:      "Sample Song",
				Artist:     "Test Artist",
				Album:      "Pilot",
				DurationMs: 123000,
			},
		},
	})

	tracks, err := adapter.ListTracks(context.Background())
	if err != nil {
		t.Fatalf("list tracks: %v", err)
	}
	if len(tracks) != 1 {
		t.Fatalf("expected 1 track, got %d", len(tracks))
	}

	track := tracks[0]
	if track.Title != "Sample Song" {
		t.Fatalf("expected normalized title, got %q", track.Title)
	}
	if track.Artist != "Test Artist" {
		t.Fatalf("expected normalized artist, got %q", track.Artist)
	}
	if track.DurationMs != 123000 {
		t.Fatalf("expected duration to be preserved, got %d", track.DurationMs)
	}
	if track.FilePath != absolutePath {
		t.Fatalf("expected absolute file path, got %q", track.FilePath)
	}
	if track.ID == "" {
		t.Fatalf("expected stable id to be generated")
	}
}

func TestListTracksFallsBackToFilenameMetadata(t *testing.T) {
	t.Parallel()

	mediaDir := t.TempDir()
	songPath := filepath.Join(mediaDir, "01 - Jetset'er - Oh Baby.flac")
	if err := os.WriteFile(songPath, []byte("not-real-audio"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	absolutePath, err := filepath.Abs(songPath)
	if err != nil {
		t.Fatalf("abs: %v", err)
	}

	adapter := NewAdapter(mediaDir, fakeProber{
		metadataByPath: map[string]Metadata{
			absolutePath: {
				DurationMs: 215000,
			},
		},
	})

	tracks, err := adapter.ListTracks(context.Background())
	if err != nil {
		t.Fatalf("list tracks: %v", err)
	}

	track := tracks[0]
	if track.Artist != "Jetset'er" {
		t.Fatalf("expected filename artist fallback, got %q", track.Artist)
	}
	if track.Title != "Oh Baby" {
		t.Fatalf("expected filename title fallback, got %q", track.Title)
	}
}

func TestGetTrackUsesCachedLibraryAfterInitialScan(t *testing.T) {
	t.Parallel()

	mediaDir := t.TempDir()
	songPath := filepath.Join(mediaDir, "01 - Jetset'er - Oh Baby.flac")
	if err := os.WriteFile(songPath, []byte("not-real-audio"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	absolutePath, err := filepath.Abs(songPath)
	if err != nil {
		t.Fatalf("abs: %v", err)
	}

	callCount := 0
	adapter := NewAdapter(mediaDir, countingProber{
		callCount: &callCount,
		metadataByPath: map[string]Metadata{
			absolutePath: {
				DurationMs: 215000,
			},
		},
	})

	tracks, err := adapter.ListTracks(context.Background())
	if err != nil {
		t.Fatalf("list tracks: %v", err)
	}
	if callCount != 1 {
		t.Fatalf("expected 1 probe call after initial scan, got %d", callCount)
	}

	if _, err := adapter.GetTrack(context.Background(), tracks[0].ID); err != nil {
		t.Fatalf("get track: %v", err)
	}
	if callCount != 1 {
		t.Fatalf("expected cached get track to avoid extra probe calls, got %d", callCount)
	}
}

func TestListTracksUsesSiblingCoverArtwork(t *testing.T) {
	t.Parallel()

	mediaDir := t.TempDir()
	albumDir := filepath.Join(mediaDir, "album")
	songPath := filepath.Join(albumDir, "sample-song.mp3")
	coverPath := filepath.Join(albumDir, "cover.jpg")
	if err := os.MkdirAll(albumDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(songPath, []byte("not-real-audio"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := os.WriteFile(coverPath, []byte("cover"), 0o644); err != nil {
		t.Fatalf("write cover: %v", err)
	}

	absolutePath, err := filepath.Abs(songPath)
	if err != nil {
		t.Fatalf("abs song: %v", err)
	}
	absoluteCoverPath, err := filepath.Abs(coverPath)
	if err != nil {
		t.Fatalf("abs cover: %v", err)
	}

	adapter := NewAdapter(mediaDir, fakeProber{
		metadataByPath: map[string]Metadata{
			absolutePath: {
				Title:      "Sample Song",
				Artist:     "Test Artist",
				DurationMs: 123000,
			},
		},
	})

	tracks, err := adapter.ListTracks(context.Background())
	if err != nil {
		t.Fatalf("list tracks: %v", err)
	}

	if tracks[0].ArtworkPath != absoluteCoverPath {
		t.Fatalf("expected sibling cover path %q, got %q", absoluteCoverPath, tracks[0].ArtworkPath)
	}
	if tracks[0].ArtworkURL != "/artwork/"+tracks[0].ID {
		t.Fatalf("expected artwork url for track, got %q", tracks[0].ArtworkURL)
	}
}

func TestListTracksUsesParentCoverForDiscMetadata(t *testing.T) {
	t.Parallel()

	mediaDir := t.TempDir()
	discDir := filepath.Join(mediaDir, "album", "CD1")
	songPath := filepath.Join(discDir, "sample-song.mp3")
	parentCoverPath := filepath.Join(mediaDir, "album", "cover.jpg")
	if err := os.MkdirAll(discDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(songPath, []byte("not-real-audio"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := os.WriteFile(parentCoverPath, []byte("cover"), 0o644); err != nil {
		t.Fatalf("write parent cover: %v", err)
	}

	absolutePath, err := filepath.Abs(songPath)
	if err != nil {
		t.Fatalf("abs song: %v", err)
	}
	absoluteCoverPath, err := filepath.Abs(parentCoverPath)
	if err != nil {
		t.Fatalf("abs cover: %v", err)
	}

	adapter := NewAdapter(mediaDir, fakeProber{
		metadataByPath: map[string]Metadata{
			absolutePath: {
				Title:      "Sample Song",
				Artist:     "Test Artist",
				Disc:       "1",
				DurationMs: 123000,
			},
		},
	})

	tracks, err := adapter.ListTracks(context.Background())
	if err != nil {
		t.Fatalf("list tracks: %v", err)
	}

	if tracks[0].ArtworkPath != absoluteCoverPath {
		t.Fatalf("expected parent cover path %q, got %q", absoluteCoverPath, tracks[0].ArtworkPath)
	}
}
