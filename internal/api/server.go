package api

import (
	"crypto/subtle"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"github.com/homepc/atlas-audio-engine/internal/media"
	"github.com/homepc/atlas-audio-engine/internal/scheduler"
)

const (
	DefaultDashboardUsername = "admin"
	DefaultDashboardPassword = "atlas"
)

type ServerOptions struct {
	DashboardUsername string
	DashboardPassword string
}

func NewServer(service *scheduler.Service) *echo.Echo {
	return NewServerWithStreamer(service, nil)
}

func NewServerWithStreamer(service *scheduler.Service, streamer media.Streamer) *echo.Echo {
	return NewServerWithOptions(service, streamer, ServerOptions{
		DashboardUsername: DefaultDashboardUsername,
		DashboardPassword: DefaultDashboardPassword,
	})
}

func NewServerWithOptions(service *scheduler.Service, streamer media.Streamer, options ServerOptions) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.Use(middleware.Recover())

	handler := &Handler{service: service, streamer: streamer}

	e.GET("/", handler.Home)
	dashboard := e.Group("")
	dashboard.Use(middleware.BasicAuth(dashboardAuthenticator(options)))
	dashboard.GET("/dashboard", handler.Dashboard)
	e.GET("/visual", handler.Visual)
	e.GET("/artwork/:trackId", handler.Artwork)
	e.GET("/health", handler.Health)
	e.GET("/channels", handler.ListChannels)
	e.POST("/channels", handler.CreateChannel)
	e.PATCH("/channels/:id", handler.UpdateChannel)
	e.DELETE("/channels/:id", handler.DeleteChannel)
	e.GET("/channels/:id/state", handler.State)
	e.GET("/channels/:id/ws", handler.StateWebSocket)
	e.GET("/channels/:id/tracks", handler.Tracks)
	e.GET("/channels/:id/library", handler.Library)
	e.GET("/channels/:id/playlist", handler.Playlist)
	e.PUT("/channels/:id/playlist", handler.ReplacePlaylist)
	e.POST("/channels/:id/playlist/shuffle", handler.ShufflePlaylist)
	e.GET("/channels/:id/tracks/:trackId/audio", handler.Audio)
	e.GET("/channels/:id/stream.m3u8", handler.HLSManifest)
	e.GET("/channels/:id/stream.mp3", handler.StreamMP3)
	e.GET("/channels/:id/stream.pcm", handler.StreamPCM)
	e.GET("/channels/:id/stream.ts", handler.StreamVideo)
	e.GET("/channels/:id/broadcast.ts", handler.BroadcastVideo)
	e.GET("/channels/:id/broadcast/status", handler.BroadcastStatus)
	e.GET("/channels/:id/now-playing", handler.NowPlaying)
	e.GET("/channels/:id/queue", handler.Queue)
	e.POST("/channels/:id/queue", handler.Enqueue)
	e.DELETE("/channels/:id/queue/:queueItemId", handler.RemoveQueueItem)
	e.POST("/channels/:id/queue/:queueItemId/move", handler.MoveQueueItem)
	e.POST("/channels/:id/skip", handler.Skip)

	e.HTTPErrorHandler = func(err error, c echo.Context) {
		if c.Response().Committed {
			return
		}

		code := http.StatusInternalServerError
		if httpErr, ok := err.(*echo.HTTPError); ok {
			code = httpErr.Code
		}
		_ = c.JSON(code, map[string]string{"error": err.Error()})
	}

	return e
}

func dashboardAuthenticator(options ServerOptions) middleware.BasicAuthValidator {
	username := options.DashboardUsername
	if username == "" {
		username = DefaultDashboardUsername
	}
	password := options.DashboardPassword
	if password == "" {
		password = DefaultDashboardPassword
	}

	return func(inputUsername, inputPassword string, _ echo.Context) (bool, error) {
		usernameMatch := subtle.ConstantTimeCompare([]byte(inputUsername), []byte(username)) == 1
		passwordMatch := subtle.ConstantTimeCompare([]byte(inputPassword), []byte(password)) == 1
		return usernameMatch && passwordMatch, nil
	}
}
