package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"golang.org/x/net/websocket"

	"github.com/homepc/atlas-audio-engine/internal/media"
	"github.com/homepc/atlas-audio-engine/internal/scheduler"
)

type Handler struct {
	service  *scheduler.Service
	streamer media.Streamer
}

type enqueueRequest struct {
	TrackID string `json:"track_id"`
}

type createChannelRequest struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	TrackIDs []string `json:"track_ids"`
}

type updateChannelRequest struct {
	Name    *string `json:"name"`
	Enabled *bool   `json:"enabled"`
}

type moveQueueItemRequest struct {
	Position int `json:"position"`
}

type replacePlaylistRequest struct {
	TrackIDs []string `json:"track_ids"`
}

type broadcastStatusResponse struct {
	ChannelID      string               `json:"channel_id"`
	Mode           string               `json:"mode"`
	Video          broadcastVideoStatus `json:"video"`
	Audio          broadcastAudioStatus `json:"audio"`
	URLs           broadcastURLs        `json:"urls"`
	NowPlaying     media.VideoMetadata  `json:"now_playing"`
	HasStreamer    bool                 `json:"has_streamer"`
	RecommendedURL string               `json:"recommended_url"`
}

type broadcastVideoStatus struct {
	Width  int `json:"width"`
	Height int `json:"height"`
	FPS    int `json:"fps"`
}

type broadcastAudioStatus struct {
	Format     string `json:"format"`
	SampleRate int    `json:"sample_rate"`
	Channels   int    `json:"channels"`
}

type broadcastURLs struct {
	BroadcastTS string `json:"broadcast_ts"`
	StreamTS    string `json:"stream_ts"`
	MP3         string `json:"mp3"`
	PCM         string `json:"pcm"`
}

func (h *Handler) Health(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) Home(c echo.Context) error {
	html, err := homePageHTML()
	if err != nil {
		return err
	}
	return c.HTML(http.StatusOK, html)
}

func (h *Handler) Dashboard(c echo.Context) error {
	html, err := dashboardPageHTML()
	if err != nil {
		return err
	}
	return c.HTML(http.StatusOK, html)
}

func (h *Handler) Visual(c echo.Context) error {
	html, err := visualPageHTML()
	if err != nil {
		return err
	}
	return c.HTML(http.StatusOK, html)
}

func (h *Handler) ListChannels(c echo.Context) error {
	channels, err := h.service.ListChannels(c.Request().Context())
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, channels)
}

func (h *Handler) CreateChannel(c echo.Context) error {
	var request createChannelRequest
	if err := c.Bind(&request); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if strings.TrimSpace(request.Name) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "channel name is required")
	}

	channel, err := h.service.CreateChannel(c.Request().Context(), request.ID, request.Name, request.TrackIDs)
	if err != nil {
		if errors.Is(err, scheduler.ErrChannelExists) {
			return echo.NewHTTPError(http.StatusConflict, "channel already exists")
		}
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.JSON(http.StatusCreated, channel)
}

func (h *Handler) UpdateChannel(c echo.Context) error {
	var request updateChannelRequest
	if err := c.Bind(&request); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if request.Name == nil && request.Enabled == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "at least one field is required")
	}

	channel, err := h.service.UpdateChannel(c.Request().Context(), c.Param("id"), request.Name, request.Enabled)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.JSON(http.StatusOK, channel)
}

func (h *Handler) DeleteChannel(c echo.Context) error {
	if err := h.service.DeleteChannel(c.Request().Context(), c.Param("id")); err != nil {
		if errors.Is(err, scheduler.ErrCannotDeleteLastChannel) {
			return echo.NewHTTPError(http.StatusConflict, err.Error())
		}
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *Handler) Tracks(c echo.Context) error {
	tracks, err := h.service.Tracks(c.Request().Context(), c.Param("id"))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, tracks)
}

func (h *Handler) Library(c echo.Context) error {
	tracks, err := h.service.LibraryTracks(c.Request().Context())
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, tracks)
}

func (h *Handler) Playlist(c echo.Context) error {
	playlist, err := h.service.Playlist(c.Request().Context(), c.Param("id"))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, playlist)
}

func (h *Handler) ReplacePlaylist(c echo.Context) error {
	var request replacePlaylistRequest
	if err := c.Bind(&request); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if len(request.TrackIDs) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "playlist must contain at least one track")
	}
	seen := make(map[string]struct{}, len(request.TrackIDs))
	for _, trackID := range request.TrackIDs {
		if strings.TrimSpace(trackID) == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "playlist track id cannot be empty")
		}
		if _, exists := seen[trackID]; exists {
			return echo.NewHTTPError(http.StatusBadRequest, "playlist cannot contain duplicate tracks")
		}
		seen[trackID] = struct{}{}
	}

	playlist, err := h.service.ReplacePlaylist(c.Request().Context(), c.Param("id"), request.TrackIDs)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, playlist)
}

func (h *Handler) ShufflePlaylist(c echo.Context) error {
	playlist, err := h.service.ShufflePlaylist(c.Request().Context(), c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.JSON(http.StatusOK, playlist)
}

func (h *Handler) State(c echo.Context) error {
	state, err := h.service.State(c.Request().Context(), c.Param("id"))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, state)
}

func (h *Handler) StateWebSocket(c echo.Context) error {
	channelID := c.Param("id")
	websocket.Handler(func(ws *websocket.Conn) {
		defer ws.Close()

		log.Printf("event=websocket.connect channel_id=%s remote_addr=%s", channelID, c.RealIP())
		defer log.Printf("event=websocket.disconnect channel_id=%s remote_addr=%s", channelID, c.RealIP())

		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for {
			state, err := h.service.State(c.Request().Context(), channelID)
			if err != nil {
				_ = websocket.Message.Send(ws, `{"error":"unable to load state"}`)
				return
			}

			payload, err := json.Marshal(state)
			if err != nil {
				_ = websocket.Message.Send(ws, `{"error":"unable to encode state"}`)
				return
			}
			if err := websocket.Message.Send(ws, string(payload)); err != nil {
				return
			}

			select {
			case <-c.Request().Context().Done():
				return
			case <-ticker.C:
			}
		}
	}).ServeHTTP(c.Response(), c.Request())
	return nil
}

func (h *Handler) Artwork(c echo.Context) error {
	path, err := h.service.ArtworkPath(c.Request().Context(), c.Param("trackId"))
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "artwork not found")
	}
	return c.File(path)
}

func (h *Handler) Audio(c echo.Context) error {
	playable, err := h.service.ResolvePlayable(c.Request().Context(), c.Param("id"), c.Param("trackId"))
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "audio not found")
	}

	if contentType := mime.TypeByExtension(filepath.Ext(playable.Path)); contentType != "" {
		c.Response().Header().Set(echo.HeaderContentType, contentType)
	}
	c.Response().Header().Set("Accept-Ranges", "bytes")
	return c.File(playable.Path)
}

func (h *Handler) HLSManifest(c echo.Context) error {
	channelID := c.Param("id")
	playhead, err := h.service.CurrentNow(c.Request().Context(), channelID)
	if err != nil {
		return err
	}

	durationSeconds := float64(playhead.DurationMs) / 1000
	if durationSeconds <= 0 {
		return echo.NewHTTPError(http.StatusUnprocessableEntity, "current track has invalid duration")
	}

	audioURL := fmt.Sprintf("/channels/%s/tracks/%s/audio", channelID, playhead.TrackID)
	manifest := fmt.Sprintf(`#EXTM3U
#EXT-X-VERSION:3
#EXT-X-PLAYLIST-TYPE:VOD
#EXT-X-TARGETDURATION:%d
#EXTINF:%.3f,
%s
#EXT-X-ENDLIST
`, int(durationSeconds+0.999), durationSeconds, audioURL)

	c.Response().Header().Set(echo.HeaderContentType, "application/vnd.apple.mpegurl")
	c.Response().Header().Set("Cache-Control", "no-store")
	return c.String(http.StatusOK, manifest)
}

func (h *Handler) StreamMP3(c echo.Context) error {
	if h.streamer == nil {
		return echo.NewHTTPError(http.StatusNotImplemented, "audio streaming is not configured")
	}

	channelID := c.Param("id")
	c.Response().Header().Set(echo.HeaderContentType, "audio/mpeg")
	c.Response().Header().Set("Cache-Control", "no-store")
	c.Response().Header().Set("X-Accel-Buffering", "no")
	c.Response().WriteHeader(http.StatusOK)

	once := c.QueryParam("once") == "1" || strings.EqualFold(c.QueryParam("once"), "true")
	firstSegment := true

	for {
		playhead, err := h.service.CurrentNow(c.Request().Context(), channelID)
		if err != nil {
			return nil
		}

		playable, err := h.service.ResolvePlayable(c.Request().Context(), channelID, playhead.TrackID)
		if err != nil {
			log.Printf("event=stream.mp3.resolve_error channel_id=%s track_id=%s error=%q", channelID, playhead.TrackID, err)
			return nil
		}

		startSeconds := float64(playhead.ElapsedMs) / 1000
		if firstSegment {
			if value := c.QueryParam("start"); value != "" {
				parsed, err := strconv.ParseFloat(value, 64)
				if err != nil || parsed < 0 {
					return nil
				}
				startSeconds = parsed
			}
		}

		segmentCtx, cancelSegment := context.WithCancel(c.Request().Context())
		go h.cancelSegmentWhenPlayheadChanges(segmentCtx, cancelSegment, channelID, playhead.TrackID, playhead.StartedAt)

		log.Printf("event=stream.mp3.segment channel_id=%s track_id=%s start_seconds=%.3f", channelID, playhead.TrackID, startSeconds)
		err = h.streamer.StreamMP3(segmentCtx, playable.Path, startSeconds, c.Response())
		cancelSegment()
		if err != nil {
			if c.Request().Context().Err() != nil {
				return nil
			}
			if segmentCtx.Err() != nil || errors.Is(err, context.Canceled) {
				firstSegment = false
				continue
			}
			log.Printf("event=stream.mp3.error channel_id=%s track_id=%s error=%q", channelID, playhead.TrackID, err)
			return nil
		}

		if flusher, ok := c.Response().Writer.(http.Flusher); ok {
			flusher.Flush()
		}
		if once {
			return nil
		}

		firstSegment = false
		select {
		case <-c.Request().Context().Done():
			return nil
		default:
		}
	}
}

func (h *Handler) StreamPCM(c echo.Context) error {
	if h.streamer == nil {
		return echo.NewHTTPError(http.StatusNotImplemented, "audio streaming is not configured")
	}

	channelID := c.Param("id")
	c.Response().Header().Set(echo.HeaderContentType, "audio/L16; rate=48000; channels=2")
	c.Response().Header().Set("Cache-Control", "no-store")
	c.Response().Header().Set("X-Accel-Buffering", "no")
	c.Response().WriteHeader(http.StatusOK)

	return h.streamAudioSegments(c, channelID, func(ctx context.Context, inputPath string, startSeconds float64) error {
		return h.streamer.StreamPCM(ctx, inputPath, startSeconds, c.Response())
	}, "stream.pcm")
}

func (h *Handler) streamAudioSegments(c echo.Context, channelID string, streamSegment func(context.Context, string, float64) error, eventName string) error {
	once := c.QueryParam("once") == "1" || strings.EqualFold(c.QueryParam("once"), "true")
	firstSegment := true

	for {
		playhead, err := h.service.CurrentNow(c.Request().Context(), channelID)
		if err != nil {
			return nil
		}

		playable, err := h.service.ResolvePlayable(c.Request().Context(), channelID, playhead.TrackID)
		if err != nil {
			log.Printf("event=%s.resolve_error channel_id=%s track_id=%s error=%q", eventName, channelID, playhead.TrackID, err)
			return nil
		}

		startSeconds := float64(playhead.ElapsedMs) / 1000
		if firstSegment {
			if value := c.QueryParam("start"); value != "" {
				parsed, err := strconv.ParseFloat(value, 64)
				if err != nil || parsed < 0 {
					return nil
				}
				startSeconds = parsed
			}
		}

		segmentCtx, cancelSegment := context.WithCancel(c.Request().Context())
		go h.cancelSegmentWhenPlayheadChanges(segmentCtx, cancelSegment, channelID, playhead.TrackID, playhead.StartedAt)

		log.Printf("event=%s.segment channel_id=%s track_id=%s start_seconds=%.3f", eventName, channelID, playhead.TrackID, startSeconds)
		err = streamSegment(segmentCtx, playable.Path, startSeconds)
		cancelSegment()
		if err != nil {
			if c.Request().Context().Err() != nil {
				return nil
			}
			if segmentCtx.Err() != nil || errors.Is(err, context.Canceled) {
				firstSegment = false
				continue
			}
			log.Printf("event=%s.error channel_id=%s track_id=%s error=%q", eventName, channelID, playhead.TrackID, err)
			return nil
		}

		if flusher, ok := c.Response().Writer.(http.Flusher); ok {
			flusher.Flush()
		}
		if once {
			return nil
		}

		firstSegment = false
		select {
		case <-c.Request().Context().Done():
			return nil
		default:
		}
	}
}

func (h *Handler) StreamVideo(c echo.Context) error {
	if h.streamer == nil {
		return echo.NewHTTPError(http.StatusNotImplemented, "video streaming is not configured")
	}

	channelID := c.Param("id")
	c.Response().Header().Set(echo.HeaderContentType, "video/MP2T")
	c.Response().Header().Set("Cache-Control", "no-store")
	c.Response().Header().Set("X-Accel-Buffering", "no")
	c.Response().Header().Set("X-Atlas-Video-Buffer-KB", strconv.Itoa(videoBufferBytes(c)/1024))
	c.Response().WriteHeader(http.StatusOK)
	output := newPrebufferWriter(c.Response(), videoBufferBytes(c))

	once := c.QueryParam("once") == "1" || strings.EqualFold(c.QueryParam("once"), "true")
	firstSegment := true

	for {
		state, err := h.service.State(c.Request().Context(), channelID)
		if err != nil {
			return nil
		}
		playhead := state.NowPlaying

		playable, err := h.service.ResolvePlayable(c.Request().Context(), channelID, playhead.TrackID)
		if err != nil {
			log.Printf("event=stream.video.resolve_error channel_id=%s track_id=%s error=%q", channelID, playhead.TrackID, err)
			return nil
		}

		startSeconds := float64(playhead.ElapsedMs) / 1000
		if firstSegment {
			if value := c.QueryParam("start"); value != "" {
				parsed, err := strconv.ParseFloat(value, 64)
				if err != nil || parsed < 0 {
					return nil
				}
				startSeconds = parsed
			}
		}

		metadata := media.VideoMetadata{
			Title:      playhead.Title,
			Artist:     playhead.Artist,
			DurationMs: playhead.DurationMs,
			ElapsedMs:  int64(startSeconds * 1000),
		}
		if state.NextTrack != nil {
			metadata.NextTitle = state.NextTrack.Title
			metadata.NextArtist = state.NextTrack.Artist
		}
		if artworkPath, err := h.service.ArtworkPath(c.Request().Context(), playhead.TrackID); err == nil {
			metadata.ArtworkPath = artworkPath
		}

		segmentCtx, cancelSegment := context.WithCancel(c.Request().Context())
		go h.cancelSegmentWhenPlayheadChanges(segmentCtx, cancelSegment, channelID, playhead.TrackID, playhead.StartedAt)

		log.Printf("event=stream.video.segment channel_id=%s track_id=%s start_seconds=%.3f", channelID, playhead.TrackID, startSeconds)
		err = h.streamer.StreamMPEGTS(segmentCtx, playable.Path, startSeconds, metadata, output)
		cancelSegment()
		if flushErr := output.FlushBuffered(); flushErr != nil && err == nil {
			err = flushErr
		}
		if err != nil {
			if c.Request().Context().Err() != nil {
				return nil
			}
			if segmentCtx.Err() != nil || errors.Is(err, context.Canceled) {
				firstSegment = false
				continue
			}
			log.Printf("event=stream.video.error channel_id=%s track_id=%s error=%q", channelID, playhead.TrackID, err)
			return nil
		}

		if flusher, ok := c.Response().Writer.(http.Flusher); ok {
			flusher.Flush()
		}
		if once {
			return nil
		}

		firstSegment = false
		select {
		case <-c.Request().Context().Done():
			return nil
		default:
		}
	}
}

func (h *Handler) BroadcastVideo(c echo.Context) error {
	if h.streamer == nil {
		return echo.NewHTTPError(http.StatusNotImplemented, "broadcast streaming is not configured")
	}

	channelID := c.Param("id")
	c.Response().Header().Set(echo.HeaderContentType, "video/MP2T")
	c.Response().Header().Set("Cache-Control", "no-store")
	c.Response().Header().Set("X-Accel-Buffering", "no")
	c.Response().Header().Set("X-Atlas-Broadcast-Mode", "persistent-visual-encoder")
	c.Response().WriteHeader(http.StatusOK)

	audioURL := streamPCMURL(c, channelID)
	metadataProvider := func(ctx context.Context) (media.VideoMetadata, error) {
		state, err := h.service.State(ctx, channelID)
		if err != nil {
			return media.VideoMetadata{}, err
		}
		playhead := state.NowPlaying
		metadata := media.VideoMetadata{
			Title:      playhead.Title,
			Artist:     playhead.Artist,
			DurationMs: playhead.DurationMs,
			ElapsedMs:  playhead.ElapsedMs,
		}
		if state.NextTrack != nil {
			metadata.NextTitle = state.NextTrack.Title
			metadata.NextArtist = state.NextTrack.Artist
		}
		if artworkPath, err := h.service.ArtworkPath(ctx, playhead.TrackID); err == nil {
			metadata.ArtworkPath = artworkPath
		}
		return metadata, nil
	}

	log.Printf("event=stream.broadcast.start channel_id=%s audio_url=%s", channelID, audioURL)
	err := h.streamer.StreamPersistentVisualMPEGTS(c.Request().Context(), audioURL, metadataProvider, c.Response())
	if err != nil && c.Request().Context().Err() == nil {
		log.Printf("event=stream.broadcast.error channel_id=%s error=%q", channelID, err)
	}
	return nil
}

func (h *Handler) BroadcastStatus(c echo.Context) error {
	channelID := c.Param("id")
	state, err := h.service.State(c.Request().Context(), channelID)
	if err != nil {
		return err
	}

	metadata := media.VideoMetadata{
		Title:      state.NowPlaying.Title,
		Artist:     state.NowPlaying.Artist,
		DurationMs: state.NowPlaying.DurationMs,
		ElapsedMs:  state.NowPlaying.ElapsedMs,
	}
	if state.NextTrack != nil {
		metadata.NextTitle = state.NextTrack.Title
		metadata.NextArtist = state.NextTrack.Artist
	}
	if artworkPath, err := h.service.ArtworkPath(c.Request().Context(), state.NowPlaying.TrackID); err == nil {
		metadata.ArtworkPath = artworkPath
	}

	urls := broadcastURLs{
		BroadcastTS: absoluteChannelURL(c, channelID, "broadcast.ts"),
		StreamTS:    absoluteChannelURL(c, channelID, "stream.ts"),
		MP3:         absoluteChannelURL(c, channelID, "stream.mp3"),
		PCM:         absoluteChannelURL(c, channelID, "stream.pcm"),
	}

	return c.JSON(http.StatusOK, broadcastStatusResponse{
		ChannelID:      channelID,
		Mode:           "persistent-visual-encoder",
		Video:          broadcastVideoStatus{Width: 1280, Height: 720, FPS: 24},
		Audio:          broadcastAudioStatus{Format: "s16le", SampleRate: 48000, Channels: 2},
		URLs:           urls,
		NowPlaying:     metadata,
		HasStreamer:    h.streamer != nil,
		RecommendedURL: urls.BroadcastTS,
	})
}

func streamPCMURL(c echo.Context, channelID string) string {
	return absoluteChannelURL(c, channelID, "stream.pcm")
}

func absoluteChannelURL(c echo.Context, channelID, endpoint string) string {
	scheme := "http"
	if c.IsTLS() {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s/channels/%s/%s", scheme, c.Request().Host, channelID, endpoint)
}

func videoBufferBytes(c echo.Context) int {
	const defaultBufferKB = 0
	value := strings.TrimSpace(c.QueryParam("buffer_kb"))
	if value == "" {
		return defaultBufferKB * 1024
	}

	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return defaultBufferKB * 1024
	}
	if parsed > 4096 {
		parsed = 4096
	}
	return parsed * 1024
}

type prebufferWriter struct {
	output interface {
		Write([]byte) (int, error)
		Flush()
	}
	targetBytes int
	buffer      bytes.Buffer
	released    bool
}

func newPrebufferWriter(output interface {
	Write([]byte) (int, error)
	Flush()
}, targetBytes int) *prebufferWriter {
	return &prebufferWriter{output: output, targetBytes: targetBytes}
}

func (w *prebufferWriter) Write(data []byte) (int, error) {
	if w.released || w.targetBytes <= 0 {
		if _, err := w.output.Write(data); err != nil {
			return 0, err
		}
		w.output.Flush()
		return len(data), nil
	}

	w.buffer.Write(data)
	if w.buffer.Len() < w.targetBytes {
		return len(data), nil
	}
	if err := w.FlushBuffered(); err != nil {
		return 0, err
	}
	return len(data), nil
}

func (w *prebufferWriter) Flush() {
	if w.released {
		w.output.Flush()
	}
}

func (w *prebufferWriter) FlushBuffered() error {
	if w.released {
		w.output.Flush()
		return nil
	}

	w.released = true
	if w.buffer.Len() > 0 {
		if _, err := w.output.Write(w.buffer.Bytes()); err != nil {
			return err
		}
		w.buffer.Reset()
	}
	w.output.Flush()
	return nil
}

func (h *Handler) cancelSegmentWhenPlayheadChanges(ctx context.Context, cancel context.CancelFunc, channelID, trackID string, startedAt time.Time) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			playhead, err := h.service.CurrentNow(ctx, channelID)
			if err != nil {
				continue
			}
			if playhead.TrackID != trackID || !playhead.StartedAt.Equal(startedAt) {
				log.Printf("event=stream.mp3.segment_cancel channel_id=%s old_track_id=%s new_track_id=%s", channelID, trackID, playhead.TrackID)
				cancel()
				return
			}
		}
	}
}

func (h *Handler) NowPlaying(c echo.Context) error {
	playhead, err := h.service.CurrentNow(c.Request().Context(), c.Param("id"))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, playhead)
}

func (h *Handler) Queue(c echo.Context) error {
	queue, err := h.service.Queue(c.Request().Context(), c.Param("id"))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, queue)
}

func (h *Handler) Enqueue(c echo.Context) error {
	var request enqueueRequest
	if err := c.Bind(&request); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if request.TrackID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "track_id is required")
	}

	item, err := h.service.Enqueue(c.Request().Context(), c.Param("id"), request.TrackID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, item)
}

func (h *Handler) RemoveQueueItem(c echo.Context) error {
	if err := h.service.RemoveQueueItem(c.Request().Context(), c.Param("id"), c.Param("queueItemId")); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *Handler) MoveQueueItem(c echo.Context) error {
	var request moveQueueItemRequest
	if err := c.Bind(&request); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if request.Position < 1 {
		return echo.NewHTTPError(http.StatusBadRequest, "position must be at least 1")
	}

	queue, err := h.service.MoveQueueItem(c.Request().Context(), c.Param("id"), c.Param("queueItemId"), request.Position)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, queue)
}

func (h *Handler) Skip(c echo.Context) error {
	playhead, err := h.service.Skip(c.Request().Context(), c.Param("id"))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, playhead)
}
