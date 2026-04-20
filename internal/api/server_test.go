package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	coverPath := filepath.Join(t.TempDir(), "cover.jpg")
	if err := os.WriteFile(coverPath, []byte("fake-cover"), 0o644); err != nil {
		t.Fatalf("write cover: %v", err)
	}

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
			"track-1": {ID: "track-1", Title: "Track One", Artist: "Artist", DurationMs: 1000, SourceType: domain.SourceTypeLocal, ArtworkPath: coverPath, ArtworkURL: "/artwork/track-1"},
			"track-2": {ID: "track-2", Title: "Track Two", Artist: "Artist", DurationMs: 1000, SourceType: domain.SourceTypeLocal},
			"track-3": {ID: "track-3", Title: "Queued", Artist: "Artist", DurationMs: 1000, SourceType: domain.SourceTypeLocal},
			"track-4": {ID: "track-4", Title: "Queued Again", Artist: "Artist", DurationMs: 1000, SourceType: domain.SourceTypeLocal},
		},
	}

	service := scheduler.NewServiceWithClock(repository, library, func() time.Time { return now })
	server := NewServer(service)

	homeRecorder := httptest.NewRecorder()
	homeRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	server.ServeHTTP(homeRecorder, homeRequest)

	if homeRecorder.Code != http.StatusOK {
		t.Fatalf("expected 200 from home page, got %d", homeRecorder.Code)
	}
	if !bytes.Contains(homeRecorder.Body.Bytes(), []byte("Now Playing")) {
		t.Fatalf("expected home page HTML to include now playing heading")
	}
	if !bytes.Contains(homeRecorder.Body.Bytes(), []byte("Skip Track")) {
		t.Fatalf("expected home page HTML to include skip control")
	}
	if !bytes.Contains(homeRecorder.Body.Bytes(), []byte("Add to Queue")) && !bytes.Contains(homeRecorder.Body.Bytes(), []byte("Library")) {
		t.Fatalf("expected home page HTML to include library queue controls")
	}
	if !bytes.Contains(homeRecorder.Body.Bytes(), []byte("Playlist Editor")) {
		t.Fatalf("expected home page HTML to include playlist editor")
	}

	artworkRecorder := httptest.NewRecorder()
	artworkRequest := httptest.NewRequest(http.MethodGet, "/artwork/track-1", nil)
	server.ServeHTTP(artworkRecorder, artworkRequest)

	if artworkRecorder.Code != http.StatusOK {
		t.Fatalf("expected 200 from artwork route, got %d", artworkRecorder.Code)
	}

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
	if playhead.ArtworkURL != "/artwork/track-1" {
		t.Fatalf("expected now playing artwork url, got %q", playhead.ArtworkURL)
	}

	tracksRecorder := httptest.NewRecorder()
	tracksRequest := httptest.NewRequest(http.MethodGet, "/channels/channel-1/tracks", nil)
	server.ServeHTTP(tracksRecorder, tracksRequest)

	if tracksRecorder.Code != http.StatusOK {
		t.Fatalf("expected 200 from tracks, got %d", tracksRecorder.Code)
	}

	var tracks []domain.Track
	if err := json.Unmarshal(tracksRecorder.Body.Bytes(), &tracks); err != nil {
		t.Fatalf("unmarshal tracks: %v", err)
	}
	if len(tracks) != 2 {
		t.Fatalf("expected 2 channel tracks, got %d", len(tracks))
	}
	if tracks[0].ID != "track-1" || tracks[1].ID != "track-2" {
		t.Fatalf("expected playlist order to be preserved, got %#v", tracks)
	}

	libraryRecorder := httptest.NewRecorder()
	libraryRequest := httptest.NewRequest(http.MethodGet, "/channels/channel-1/library", nil)
	server.ServeHTTP(libraryRecorder, libraryRequest)

	if libraryRecorder.Code != http.StatusOK {
		t.Fatalf("expected 200 from library, got %d", libraryRecorder.Code)
	}

	var libraryTracks []domain.Track
	if err := json.Unmarshal(libraryRecorder.Body.Bytes(), &libraryTracks); err != nil {
		t.Fatalf("unmarshal library tracks: %v", err)
	}
	if len(libraryTracks) != 4 {
		t.Fatalf("expected full library to include 4 tracks, got %d", len(libraryTracks))
	}

	playlistRecorder := httptest.NewRecorder()
	playlistRequest := httptest.NewRequest(http.MethodGet, "/channels/channel-1/playlist", nil)
	server.ServeHTTP(playlistRecorder, playlistRequest)

	if playlistRecorder.Code != http.StatusOK {
		t.Fatalf("expected 200 from playlist, got %d", playlistRecorder.Code)
	}

	var playlist []domain.PlaylistEntry
	if err := json.Unmarshal(playlistRecorder.Body.Bytes(), &playlist); err != nil {
		t.Fatalf("unmarshal playlist: %v", err)
	}
	if len(playlist) != 2 || playlist[0].TrackID != "track-1" || playlist[1].TrackID != "track-2" {
		t.Fatalf("expected initial playlist order [track-1, track-2], got %#v", playlist)
	}

	playlistBody, _ := json.Marshal(map[string][]string{"track_ids": []string{"track-2", "track-3", "track-1"}})
	replacePlaylistRecorder := httptest.NewRecorder()
	replacePlaylistRequest := httptest.NewRequest(http.MethodPut, "/channels/channel-1/playlist", bytes.NewReader(playlistBody))
	replacePlaylistRequest.Header.Set("Content-Type", "application/json")
	server.ServeHTTP(replacePlaylistRecorder, replacePlaylistRequest)

	if replacePlaylistRecorder.Code != http.StatusOK {
		t.Fatalf("expected 200 from replace playlist, got %d", replacePlaylistRecorder.Code)
	}

	var replacedPlaylist []domain.PlaylistEntry
	if err := json.Unmarshal(replacePlaylistRecorder.Body.Bytes(), &replacedPlaylist); err != nil {
		t.Fatalf("unmarshal replaced playlist: %v", err)
	}
	if len(replacedPlaylist) != 3 || replacedPlaylist[0].TrackID != "track-2" || replacedPlaylist[1].TrackID != "track-3" || replacedPlaylist[2].TrackID != "track-1" {
		t.Fatalf("expected replaced playlist order [track-2, track-3, track-1], got %#v", replacedPlaylist)
	}

	stateRecorder := httptest.NewRecorder()
	stateRequest := httptest.NewRequest(http.MethodGet, "/channels/channel-1/state", nil)
	server.ServeHTTP(stateRecorder, stateRequest)

	if stateRecorder.Code != http.StatusOK {
		t.Fatalf("expected 200 from state, got %d", stateRecorder.Code)
	}

	var stateSnapshot domain.ChannelStateSnapshot
	if err := json.Unmarshal(stateRecorder.Body.Bytes(), &stateSnapshot); err != nil {
		t.Fatalf("unmarshal state snapshot: %v", err)
	}
	if stateSnapshot.NowPlaying.TrackID != "track-2" {
		t.Fatalf("expected state now playing reset to track-2, got %s", stateSnapshot.NowPlaying.TrackID)
	}
	if stateSnapshot.NextTrack == nil || stateSnapshot.NextTrack.TrackID != "track-3" {
		t.Fatalf("expected next track track-3 after playlist replace, got %#v", stateSnapshot.NextTrack)
	}

	body, _ := json.Marshal(map[string]string{"track_id": "track-3"})
	queueRecorder := httptest.NewRecorder()
	queueRequest := httptest.NewRequest(http.MethodPost, "/channels/channel-1/queue", bytes.NewReader(body))
	queueRequest.Header.Set("Content-Type", "application/json")
	server.ServeHTTP(queueRecorder, queueRequest)

	if queueRecorder.Code != http.StatusCreated {
		t.Fatalf("expected 201 from queue create, got %d", queueRecorder.Code)
	}

	queueListRecorder := httptest.NewRecorder()
	queueListRequest := httptest.NewRequest(http.MethodGet, "/channels/channel-1/queue", nil)
	server.ServeHTTP(queueListRecorder, queueListRequest)

	if queueListRecorder.Code != http.StatusOK {
		t.Fatalf("expected 200 from queue list, got %d", queueListRecorder.Code)
	}

	var queueEntries []domain.QueueEntry
	if err := json.Unmarshal(queueListRecorder.Body.Bytes(), &queueEntries); err != nil {
		t.Fatalf("unmarshal queue entries: %v", err)
	}
	if len(queueEntries) != 1 {
		t.Fatalf("expected 1 queue entry, got %d", len(queueEntries))
	}
	if queueEntries[0].TrackID != "track-3" || queueEntries[0].Title != "Queued" {
		t.Fatalf("expected queue entry track metadata, got %#v", queueEntries[0])
	}
	if queueEntries[0].Position != 1 {
		t.Fatalf("expected queue position 1, got %d", queueEntries[0].Position)
	}

	deleteRecorder := httptest.NewRecorder()
	deleteRequest := httptest.NewRequest(http.MethodDelete, "/channels/channel-1/queue/"+queueEntries[0].ID, nil)
	server.ServeHTTP(deleteRecorder, deleteRequest)

	if deleteRecorder.Code != http.StatusNoContent {
		t.Fatalf("expected 204 from queue delete, got %d", deleteRecorder.Code)
	}

	queueAfterDeleteRecorder := httptest.NewRecorder()
	queueAfterDeleteRequest := httptest.NewRequest(http.MethodGet, "/channels/channel-1/queue", nil)
	server.ServeHTTP(queueAfterDeleteRecorder, queueAfterDeleteRequest)

	var queueAfterDelete []domain.QueueEntry
	if err := json.Unmarshal(queueAfterDeleteRecorder.Body.Bytes(), &queueAfterDelete); err != nil {
		t.Fatalf("unmarshal queue after delete: %v", err)
	}
	if len(queueAfterDelete) != 0 {
		t.Fatalf("expected queue to be empty after delete, got %d entries", len(queueAfterDelete))
	}

	body, _ = json.Marshal(map[string]string{"track_id": "track-3"})
	queueAgainRecorder := httptest.NewRecorder()
	queueAgainRequest := httptest.NewRequest(http.MethodPost, "/channels/channel-1/queue", bytes.NewReader(body))
	queueAgainRequest.Header.Set("Content-Type", "application/json")
	server.ServeHTTP(queueAgainRecorder, queueAgainRequest)

	if queueAgainRecorder.Code != http.StatusCreated {
		t.Fatalf("expected 201 from second queue create, got %d", queueAgainRecorder.Code)
	}

	secondBody, _ := json.Marshal(map[string]string{"track_id": "track-4"})
	secondQueueRecorder := httptest.NewRecorder()
	secondQueueRequest := httptest.NewRequest(http.MethodPost, "/channels/channel-1/queue", bytes.NewReader(secondBody))
	secondQueueRequest.Header.Set("Content-Type", "application/json")
	server.ServeHTTP(secondQueueRecorder, secondQueueRequest)

	if secondQueueRecorder.Code != http.StatusCreated {
		t.Fatalf("expected 201 from third queue create, got %d", secondQueueRecorder.Code)
	}

	queueBeforeMoveRecorder := httptest.NewRecorder()
	queueBeforeMoveRequest := httptest.NewRequest(http.MethodGet, "/channels/channel-1/queue", nil)
	server.ServeHTTP(queueBeforeMoveRecorder, queueBeforeMoveRequest)

	var queueBeforeMove []domain.QueueEntry
	if err := json.Unmarshal(queueBeforeMoveRecorder.Body.Bytes(), &queueBeforeMove); err != nil {
		t.Fatalf("unmarshal queue before move: %v", err)
	}
	if len(queueBeforeMove) != 2 {
		t.Fatalf("expected 2 queue entries before move, got %d", len(queueBeforeMove))
	}

	moveBody, _ := json.Marshal(map[string]int{"position": 1})
	moveRecorder := httptest.NewRecorder()
	moveRequest := httptest.NewRequest(http.MethodPost, "/channels/channel-1/queue/"+queueBeforeMove[1].ID+"/move", bytes.NewReader(moveBody))
	moveRequest.Header.Set("Content-Type", "application/json")
	server.ServeHTTP(moveRecorder, moveRequest)

	if moveRecorder.Code != http.StatusOK {
		t.Fatalf("expected 200 from queue move, got %d", moveRecorder.Code)
	}

	var movedQueue []domain.QueueEntry
	if err := json.Unmarshal(moveRecorder.Body.Bytes(), &movedQueue); err != nil {
		t.Fatalf("unmarshal moved queue: %v", err)
	}
	if len(movedQueue) != 2 {
		t.Fatalf("expected 2 queue entries after move, got %d", len(movedQueue))
	}
	if movedQueue[0].TrackID != "track-4" || movedQueue[1].TrackID != "track-3" {
		t.Fatalf("expected moved queue order [track-4, track-3], got %#v", movedQueue)
	}

	stateAfterMoveRecorder := httptest.NewRecorder()
	stateAfterMoveRequest := httptest.NewRequest(http.MethodGet, "/channels/channel-1/state", nil)
	server.ServeHTTP(stateAfterMoveRecorder, stateAfterMoveRequest)

	var stateAfterMove domain.ChannelStateSnapshot
	if err := json.Unmarshal(stateAfterMoveRecorder.Body.Bytes(), &stateAfterMove); err != nil {
		t.Fatalf("unmarshal state after move: %v", err)
	}
	if len(stateAfterMove.Queue) != 2 {
		t.Fatalf("expected 2 queued tracks in snapshot, got %d", len(stateAfterMove.Queue))
	}
	if stateAfterMove.NextTrack == nil || stateAfterMove.NextTrack.TrackID != "track-4" {
		t.Fatalf("expected next track to reflect front of queue, got %#v", stateAfterMove.NextTrack)
	}

	skipRecorder := httptest.NewRecorder()
	skipRequest := httptest.NewRequest(http.MethodPost, "/channels/channel-1/skip", nil)
	server.ServeHTTP(skipRecorder, skipRequest)

	if skipRecorder.Code != http.StatusOK {
		t.Fatalf("expected 200 from skip, got %d", skipRecorder.Code)
	}

	var skippedPlayhead domain.PlayheadState
	if err := json.Unmarshal(skipRecorder.Body.Bytes(), &skippedPlayhead); err != nil {
		t.Fatalf("unmarshal skipped playhead: %v", err)
	}
	if skippedPlayhead.TrackID != "track-4" {
		t.Fatalf("expected skip to jump to queued track, got %s", skippedPlayhead.TrackID)
	}
	if skippedPlayhead.ElapsedMs != 0 {
		t.Fatalf("expected skipped playhead elapsed to reset, got %d", skippedPlayhead.ElapsedMs)
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
		t.Fatalf("expected remaining queued track after skipped reordered item, got %s", advancedPlayhead.TrackID)
	}
}
