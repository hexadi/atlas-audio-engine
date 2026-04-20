package api

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/homepc/atlas-audio-engine/internal/scheduler"
)

type Handler struct {
	service *scheduler.Service
}

type enqueueRequest struct {
	TrackID string `json:"track_id"`
}

type moveQueueItemRequest struct {
	Position int `json:"position"`
}

type replacePlaylistRequest struct {
	TrackIDs []string `json:"track_ids"`
}

func (h *Handler) Health(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) Home(c echo.Context) error {
	return c.HTML(http.StatusOK, homePageHTML)
}

func (h *Handler) ListChannels(c echo.Context) error {
	channels, err := h.service.ListChannels(c.Request().Context())
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, channels)
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

func (h *Handler) State(c echo.Context) error {
	state, err := h.service.State(c.Request().Context(), c.Param("id"))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, state)
}

func (h *Handler) Artwork(c echo.Context) error {
	path, err := h.service.ArtworkPath(c.Request().Context(), c.Param("trackId"))
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "artwork not found")
	}
	return c.File(path)
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
