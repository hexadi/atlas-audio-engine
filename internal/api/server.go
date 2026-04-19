package api

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"github.com/homepc/atlas-audio-engine/internal/scheduler"
)

func NewServer(service *scheduler.Service) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.Use(middleware.Recover())

	handler := &Handler{service: service}

	e.GET("/health", handler.Health)
	e.GET("/channels", handler.ListChannels)
	e.GET("/channels/:id/now-playing", handler.NowPlaying)
	e.GET("/channels/:id/queue", handler.Queue)
	e.POST("/channels/:id/queue", handler.Enqueue)

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
