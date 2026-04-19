package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/homepc/atlas-audio-engine/internal/domain"
	"github.com/homepc/atlas-audio-engine/internal/scheduler"
	"github.com/homepc/atlas-audio-engine/internal/source"
	"github.com/homepc/atlas-audio-engine/internal/store"
	"github.com/homepc/atlas-audio-engine/internal/store/memory"
)

type apiFakeLibrary struct {
	tracks map[string]domain.Track
}

func (f apiFakeLibrary) ListTracks(context.Context) ([]domain.Track, error) {
	items := make([]domain.Track, 0, len(f.tracks))
	for _, track := range f.tracks {
		items = append(items, track)
	}
	return items, nil
}

func (f apiFakeLibrary) GetTrack(_ context.Context, id string) (domain.Track, error) {
	return f.tracks[id], nil
}

func (f apiFakeLibrary) ResolvePlayable(_ context.Context, id string) (source.Playable, error) {
	return source.Playable{TrackID: id, Path: "/tmp/" + id}, nil
}

func TestNowPlayingAndQueueFlow(t *testing.T) {
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	repository := memory.NewStore()
	state := store.ChannelState{
		Channel: domain.Channel{
			ID:             "channel-1",
			Name:           "Test Channel",
			CreatedAt:      now.Add(-time.Hour),
			StartedAt:      now,
			CurrentTrackID: "track-1",
			PlaylistCursor: 0,
		},
		PlaylistTrackIDs: []string{"track-1", "track-2"},
	}
	if err := repository.UpsertChannelState(context.Background(), state); err != nil {
		t.Fatalf("seed store: %v", err)
	}

	library := apiFakeLibrary{
		tracks: map[string]domain.Track{
			"track-1": {ID: "track-1", Title: "Track One", Artist: "Artist", DurationMs: 1000, SourceType: domain.SourceTypeLocal},
			"track-2": {ID: "track-2", Title: "Track Two", Artist: "Artist", DurationMs: 1000, SourceType: domain.SourceTypeLocal},
			"track-3": {ID: "track-3", Title: "Queued", Artist: "Artist", DurationMs: 1000, SourceType: domain.SourceTypeLocal},
		},
	}

	service := scheduler.NewServiceWithClock(repository, library, func() time.Time { return now })
	server := NewServer(service)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/channels/channel-1/now-playing", nil)
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200 from now-playing, got %d", recorder.Code)
	}

	var playhead domain.PlayheadState
	if err := json.Unmarshal(recorder.Body.Bytes(), &playhead); err != nil {
		t.Fatalf("unmarshal playhead: %v", err)
	}
	if playhead.TrackID != "track-1" {
		t.Fatalf("expected seeded track, got %s", playhead.TrackID)
	}

	body, _ := json.Marshal(map[string]string{"track_id": "track-3"})
	queueRecorder := httptest.NewRecorder()
	queueRequest := httptest.NewRequest(http.MethodPost, "/channels/channel-1/queue", bytes.NewReader(body))
	queueRequest.Header.Set("Content-Type", "application/json")
	server.ServeHTTP(queueRecorder, queueRequest)

	if queueRecorder.Code != http.StatusCreated {
		t.Fatalf("expected 201 from queue create, got %d", queueRecorder.Code)
	}

	service = scheduler.NewServiceWithClock(repository, library, func() time.Time { return now.Add(1100 * time.Millisecond) })
	server = NewServer(service)

	advancedRecorder := httptest.NewRecorder()
	advancedRequest := httptest.NewRequest(http.MethodGet, "/channels/channel-1/now-playing", nil)
	server.ServeHTTP(advancedRecorder, advancedRequest)

	var advancedPlayhead domain.PlayheadState
	if err := json.Unmarshal(advancedRecorder.Body.Bytes(), &advancedPlayhead); err != nil {
		t.Fatalf("unmarshal advanced playhead: %v", err)
	}
	if advancedPlayhead.TrackID != "track-3" {
		t.Fatalf("expected queued track after boundary, got %s", advancedPlayhead.TrackID)
	}
}
