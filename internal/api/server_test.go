package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/websocket"

	"github.com/homepc/atlas-audio-engine/internal/domain"
	"github.com/homepc/atlas-audio-engine/internal/media"
	"github.com/homepc/atlas-audio-engine/internal/scheduler"
	"github.com/homepc/atlas-audio-engine/internal/source"
	"github.com/homepc/atlas-audio-engine/internal/store"
	"github.com/homepc/atlas-audio-engine/internal/store/memory"
)

type apiFakeLibrary struct {
	tracks map[string]domain.Track
}

type fakeAudioStreamer struct {
	inputPath          string
	videoInputPath     string
	startSeconds       float64
	videoStartSeconds  float64
	metadata           media.VideoMetadata
	output             string
	videoOutput        string
	persistentAudioURL string
	persistentOutput   string
	persistentMetadata media.VideoMetadata
	persistentCalls    int
	cancelVideoAfter   int
	cancelVideo        context.CancelFunc
	calls              int
	videoCalls         int
}

type fakeFlushWriter struct {
	bytes.Buffer
	flushes int
}

func (f *fakeFlushWriter) Flush() {
	f.flushes++
}

func (f *fakeAudioStreamer) StreamMP3(_ context.Context, inputPath string, startSeconds float64, output io.Writer) error {
	f.inputPath = inputPath
	f.startSeconds = startSeconds
	f.calls++
	_, err := io.WriteString(output, f.output)
	return err
}

func (f *fakeAudioStreamer) StreamMPEGTS(_ context.Context, inputPath string, startSeconds float64, metadata media.VideoMetadata, output io.Writer) error {
	f.videoInputPath = inputPath
	f.videoStartSeconds = startSeconds
	f.metadata = metadata
	f.videoCalls++
	_, err := io.WriteString(output, f.videoOutput)
	if f.cancelVideoAfter > 0 && f.videoCalls >= f.cancelVideoAfter && f.cancelVideo != nil {
		f.cancelVideo()
		return context.Canceled
	}
	return err
}

func (f *fakeAudioStreamer) StreamPersistentMPEGTS(ctx context.Context, audioURL string, metadataProvider media.VideoMetadataProvider, output io.Writer) error {
	f.persistentAudioURL = audioURL
	f.persistentCalls++
	metadata, err := metadataProvider(ctx)
	if err != nil {
		return err
	}
	f.persistentMetadata = metadata
	_, err = io.WriteString(output, f.persistentOutput)
	return err
}

func (f *fakeAudioStreamer) StreamPersistentVisualMPEGTS(ctx context.Context, audioURL string, metadataProvider media.VideoMetadataProvider, output io.Writer) error {
	return f.StreamPersistentMPEGTS(ctx, audioURL, metadataProvider, output)
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
	track := f.tracks[id]
	return source.Playable{TrackID: id, Path: track.FilePath}, nil
}

func TestNowPlayingAndQueueFlow(t *testing.T) {
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	tempDir := t.TempDir()
	coverPath := filepath.Join(tempDir, "cover.jpg")
	if err := os.WriteFile(coverPath, []byte("fake-cover"), 0o644); err != nil {
		t.Fatalf("write cover: %v", err)
	}
	audioPath := filepath.Join(tempDir, "track-1.mp3")
	if err := os.WriteFile(audioPath, []byte("fake-audio"), 0o644); err != nil {
		t.Fatalf("write audio: %v", err)
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
			"track-1": {ID: "track-1", Title: "Track One", Artist: "Artist", DurationMs: 1000, SourceType: domain.SourceTypeLocal, FilePath: audioPath, ArtworkPath: coverPath, ArtworkURL: "/artwork/track-1"},
			"track-2": {ID: "track-2", Title: "Track Two", Artist: "Artist", DurationMs: 1000, SourceType: domain.SourceTypeLocal},
			"track-3": {ID: "track-3", Title: "Queued", Artist: "Artist", DurationMs: 1000, SourceType: domain.SourceTypeLocal},
			"track-4": {ID: "track-4", Title: "Queued Again", Artist: "Artist", DurationMs: 1000, SourceType: domain.SourceTypeLocal},
		},
	}

	streamer := &fakeAudioStreamer{output: "fake-mp3", videoOutput: "fake-mpegts", persistentOutput: "fake-broadcast"}
	service := scheduler.NewServiceWithClock(repository, library, func() time.Time { return now })
	server := NewServerWithStreamer(service, streamer)

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
	if !bytes.Contains(homeRecorder.Body.Bytes(), []byte("listen-player")) {
		t.Fatalf("expected home page HTML to include audio player")
	}
	if !bytes.Contains(homeRecorder.Body.Bytes(), []byte("Play Stream")) {
		t.Fatalf("expected home page HTML to include play stream button")
	}
	if !bytes.Contains(homeRecorder.Body.Bytes(), []byte("stopStream")) {
		t.Fatalf("expected home page HTML to include stop stream behavior")
	}
	if bytes.Contains(homeRecorder.Body.Bytes(), []byte("autoplay")) {
		t.Fatalf("expected home page audio player not to force autoplay")
	}
	if !bytes.Contains(homeRecorder.Body.Bytes(), []byte("stream.mp3")) {
		t.Fatalf("expected home page audio player to use continuous stream")
	}
	if !bytes.Contains(homeRecorder.Body.Bytes(), []byte("WebSocket")) {
		t.Fatalf("expected home page HTML to include websocket live updates")
	}

	visualRecorder := httptest.NewRecorder()
	visualRequest := httptest.NewRequest(http.MethodGet, "/visual", nil)
	server.ServeHTTP(visualRecorder, visualRequest)

	if visualRecorder.Code != http.StatusOK {
		t.Fatalf("expected 200 from visual page, got %d", visualRecorder.Code)
	}
	if !bytes.Contains(visualRecorder.Body.Bytes(), []byte("Atlas Visual Output")) {
		t.Fatalf("expected visual page title")
	}
	if !bytes.Contains(visualRecorder.Body.Bytes(), []byte("connectSocket")) {
		t.Fatalf("expected visual page to use live websocket state")
	}

	artworkRecorder := httptest.NewRecorder()
	artworkRequest := httptest.NewRequest(http.MethodGet, "/artwork/track-1", nil)
	server.ServeHTTP(artworkRecorder, artworkRequest)

	if artworkRecorder.Code != http.StatusOK {
		t.Fatalf("expected 200 from artwork route, got %d", artworkRecorder.Code)
	}

	audioRecorder := httptest.NewRecorder()
	audioRequest := httptest.NewRequest(http.MethodGet, "/channels/channel-1/tracks/track-1/audio", nil)
	server.ServeHTTP(audioRecorder, audioRequest)

	if audioRecorder.Code != http.StatusOK {
		t.Fatalf("expected 200 from audio route, got %d", audioRecorder.Code)
	}
	if audioRecorder.Body.String() != "fake-audio" {
		t.Fatalf("expected audio file body, got %q", audioRecorder.Body.String())
	}

	unattachedAudioRecorder := httptest.NewRecorder()
	unattachedAudioRequest := httptest.NewRequest(http.MethodGet, "/channels/channel-1/tracks/track-4/audio", nil)
	server.ServeHTTP(unattachedAudioRecorder, unattachedAudioRequest)

	if unattachedAudioRecorder.Code != http.StatusNotFound {
		t.Fatalf("expected 404 from unattached audio route, got %d", unattachedAudioRecorder.Code)
	}

	hlsRecorder := httptest.NewRecorder()
	hlsRequest := httptest.NewRequest(http.MethodGet, "/channels/channel-1/stream.m3u8", nil)
	server.ServeHTTP(hlsRecorder, hlsRequest)

	if hlsRecorder.Code != http.StatusOK {
		t.Fatalf("expected 200 from hls manifest, got %d", hlsRecorder.Code)
	}
	hlsBody := hlsRecorder.Body.String()
	if !strings.Contains(hlsBody, "#EXTM3U") || !strings.Contains(hlsBody, "/channels/channel-1/tracks/track-1/audio") {
		t.Fatalf("expected hls manifest to reference current track audio, got %q", hlsBody)
	}
	if contentType := hlsRecorder.Header().Get("Content-Type"); !strings.Contains(contentType, "application/vnd.apple.mpegurl") {
		t.Fatalf("expected hls content type, got %q", contentType)
	}

	streamRecorder := httptest.NewRecorder()
	streamRequest := httptest.NewRequest(http.MethodGet, "/channels/channel-1/stream.mp3?once=1&start=0.25", nil)
	server.ServeHTTP(streamRecorder, streamRequest)

	if streamRecorder.Code != http.StatusOK {
		t.Fatalf("expected 200 from mp3 stream, got %d", streamRecorder.Code)
	}
	if streamRecorder.Body.String() != "fake-mp3" {
		t.Fatalf("expected fake stream body, got %q", streamRecorder.Body.String())
	}
	if streamer.inputPath != audioPath {
		t.Fatalf("expected streamer input %q, got %q", audioPath, streamer.inputPath)
	}
	if streamer.startSeconds != 0.25 {
		t.Fatalf("expected start offset 0.25, got %f", streamer.startSeconds)
	}
	if streamer.calls != 1 {
		t.Fatalf("expected once stream to call streamer once, got %d", streamer.calls)
	}

	videoRecorder := httptest.NewRecorder()
	videoRequest := httptest.NewRequest(http.MethodGet, "/channels/channel-1/stream.ts?once=1&start=0.5", nil)
	server.ServeHTTP(videoRecorder, videoRequest)

	if videoRecorder.Code != http.StatusOK {
		t.Fatalf("expected 200 from video stream, got %d", videoRecorder.Code)
	}
	if videoRecorder.Body.String() != "fake-mpegts" {
		t.Fatalf("expected fake video stream body, got %q", videoRecorder.Body.String())
	}
	if contentType := videoRecorder.Header().Get("Content-Type"); !strings.Contains(contentType, "video/MP2T") {
		t.Fatalf("expected video stream content type, got %q", contentType)
	}
	if videoRecorder.Header().Get("X-Atlas-Video-Buffer-KB") != "0" {
		t.Fatalf("expected default video buffer header, got %q", videoRecorder.Header().Get("X-Atlas-Video-Buffer-KB"))
	}
	if streamer.videoInputPath != audioPath {
		t.Fatalf("expected video streamer input %q, got %q", audioPath, streamer.videoInputPath)
	}
	if streamer.videoStartSeconds != 0.5 {
		t.Fatalf("expected video start offset 0.5, got %f", streamer.videoStartSeconds)
	}
	if streamer.metadata.Title != "Track One" || streamer.metadata.Artist != "Artist" {
		t.Fatalf("expected video metadata for current track, got %#v", streamer.metadata)
	}
	if streamer.metadata.DurationMs != 1000 || streamer.metadata.ElapsedMs != 500 {
		t.Fatalf("expected video metadata to include playhead progress, got %#v", streamer.metadata)
	}
	if streamer.metadata.NextTitle != "Track Two" || streamer.metadata.ArtworkPath != coverPath {
		t.Fatalf("expected video metadata to include next track and artwork path, got %#v", streamer.metadata)
	}
	if streamer.videoCalls != 1 {
		t.Fatalf("expected once video stream to call streamer once, got %d", streamer.videoCalls)
	}

	broadcastRecorder := httptest.NewRecorder()
	broadcastRequest := httptest.NewRequest(http.MethodGet, "/channels/channel-1/broadcast.ts", nil)
	server.ServeHTTP(broadcastRecorder, broadcastRequest)

	if broadcastRecorder.Code != http.StatusOK {
		t.Fatalf("expected 200 from broadcast stream, got %d", broadcastRecorder.Code)
	}
	if broadcastRecorder.Body.String() != "fake-broadcast" {
		t.Fatalf("expected fake broadcast stream body, got %q", broadcastRecorder.Body.String())
	}
	if !strings.Contains(broadcastRecorder.Header().Get("Content-Type"), "video/MP2T") {
		t.Fatalf("expected broadcast content type, got %q", broadcastRecorder.Header().Get("Content-Type"))
	}
	if broadcastRecorder.Header().Get("X-Atlas-Broadcast-Mode") != "persistent-visual-encoder" {
		t.Fatalf("expected persistent encoder header, got %q", broadcastRecorder.Header().Get("X-Atlas-Broadcast-Mode"))
	}
	if streamer.persistentAudioURL != "http://example.com/channels/channel-1/stream.mp3" {
		t.Fatalf("expected broadcast to consume station audio URL, got %q", streamer.persistentAudioURL)
	}
	if streamer.persistentMetadata.Title != "Track One" || streamer.persistentMetadata.NextTitle != "Track Two" {
		t.Fatalf("expected broadcast metadata snapshot, got %#v", streamer.persistentMetadata)
	}
	if streamer.persistentMetadata.ArtworkPath != coverPath {
		t.Fatalf("expected broadcast metadata to include artwork path, got %#v", streamer.persistentMetadata)
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

	emptyPlaylistBody, _ := json.Marshal(map[string][]string{"track_ids": []string{}})
	emptyPlaylistRecorder := httptest.NewRecorder()
	emptyPlaylistRequest := httptest.NewRequest(http.MethodPut, "/channels/channel-1/playlist", bytes.NewReader(emptyPlaylistBody))
	emptyPlaylistRequest.Header.Set("Content-Type", "application/json")
	server.ServeHTTP(emptyPlaylistRecorder, emptyPlaylistRequest)

	if emptyPlaylistRecorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 from empty playlist replace, got %d", emptyPlaylistRecorder.Code)
	}

	duplicatePlaylistBody, _ := json.Marshal(map[string][]string{"track_ids": []string{"track-1", "track-1"}})
	duplicatePlaylistRecorder := httptest.NewRecorder()
	duplicatePlaylistRequest := httptest.NewRequest(http.MethodPut, "/channels/channel-1/playlist", bytes.NewReader(duplicatePlaylistBody))
	duplicatePlaylistRequest.Header.Set("Content-Type", "application/json")
	server.ServeHTTP(duplicatePlaylistRecorder, duplicatePlaylistRequest)

	if duplicatePlaylistRecorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 from duplicate playlist replace, got %d", duplicatePlaylistRecorder.Code)
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

func TestVideoStreamContinuesAcrossSegmentsOnOneResponse(t *testing.T) {
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	tempDir := t.TempDir()
	audioPath := filepath.Join(tempDir, "track-1.mp3")
	if err := os.WriteFile(audioPath, []byte("fake-audio"), 0o644); err != nil {
		t.Fatalf("write audio: %v", err)
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
		PlaylistTrackIDs: []string{"track-1"},
	}
	if err := repository.UpsertChannelState(context.Background(), state); err != nil {
		t.Fatalf("seed store: %v", err)
	}

	library := apiFakeLibrary{
		tracks: map[string]domain.Track{
			"track-1": {ID: "track-1", Title: "Track One", Artist: "Artist", DurationMs: 60_000, SourceType: domain.SourceTypeLocal, FilePath: audioPath},
		},
	}

	requestContext, cancel := context.WithCancel(context.Background())
	streamer := &fakeAudioStreamer{videoOutput: "video-segment-", cancelVideoAfter: 2, cancelVideo: cancel}
	service := scheduler.NewServiceWithClock(repository, library, func() time.Time { return now })
	server := NewServerWithStreamer(service, streamer)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/channels/channel-1/stream.ts", nil).WithContext(requestContext)
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200 from video stream, got %d", recorder.Code)
	}
	if streamer.videoCalls != 2 {
		t.Fatalf("expected video stream to continue to a second segment, got %d calls", streamer.videoCalls)
	}
	if recorder.Body.String() != "video-segment-video-segment-" {
		t.Fatalf("expected both video segments on one response, got %q", recorder.Body.String())
	}
}

func TestPrebufferWriterDelaysUntilTargetThenFlushes(t *testing.T) {
	output := &fakeFlushWriter{}
	writer := newPrebufferWriter(output, 6)

	if _, err := writer.Write([]byte("abc")); err != nil {
		t.Fatalf("write first chunk: %v", err)
	}
	if output.String() != "" {
		t.Fatalf("expected first chunk to be buffered, got %q", output.String())
	}

	if _, err := writer.Write([]byte("def")); err != nil {
		t.Fatalf("write second chunk: %v", err)
	}
	if output.String() != "abcdef" {
		t.Fatalf("expected buffered chunks to flush at target size, got %q", output.String())
	}
	if output.flushes == 0 {
		t.Fatalf("expected output to be flushed after releasing buffer")
	}
}

func TestPrebufferWriterFlushBufferedReleasesPartialBuffer(t *testing.T) {
	output := &fakeFlushWriter{}
	writer := newPrebufferWriter(output, 1024)

	if _, err := writer.Write([]byte("tiny")); err != nil {
		t.Fatalf("write tiny chunk: %v", err)
	}
	if output.String() != "" {
		t.Fatalf("expected tiny chunk to stay buffered, got %q", output.String())
	}

	if err := writer.FlushBuffered(); err != nil {
		t.Fatalf("flush buffered: %v", err)
	}
	if output.String() != "tiny" {
		t.Fatalf("expected partial buffer to flush, got %q", output.String())
	}
}

func TestStateWebSocketSendsSnapshot(t *testing.T) {
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
		},
	}

	service := scheduler.NewServiceWithClock(repository, library, func() time.Time { return now })
	server := httptest.NewServer(NewServer(service))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/channels/channel-1/ws"
	ws, err := websocket.Dial(wsURL, "", server.URL)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer ws.Close()

	var raw string
	if err := websocket.Message.Receive(ws, &raw); err != nil {
		t.Fatalf("receive websocket message: %v", err)
	}

	var snapshot domain.ChannelStateSnapshot
	if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
		t.Fatalf("unmarshal websocket snapshot: %v", err)
	}
	if snapshot.ChannelID != "channel-1" {
		t.Fatalf("expected channel-1 snapshot, got %s", snapshot.ChannelID)
	}
	if snapshot.NowPlaying.TrackID != "track-1" {
		t.Fatalf("expected websocket now-playing track-1, got %s", snapshot.NowPlaying.TrackID)
	}
	if snapshot.NextTrack == nil || snapshot.NextTrack.TrackID != "track-2" {
		t.Fatalf("expected websocket next track track-2, got %#v", snapshot.NextTrack)
	}
}
