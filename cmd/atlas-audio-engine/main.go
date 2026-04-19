package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/homepc/atlas-audio-engine/internal/api"
	"github.com/homepc/atlas-audio-engine/internal/bootstrap"
	"github.com/homepc/atlas-audio-engine/internal/config"
	"github.com/homepc/atlas-audio-engine/internal/scheduler"
	"github.com/homepc/atlas-audio-engine/internal/source/localfiles"
	"github.com/homepc/atlas-audio-engine/internal/store/sqlite"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()

	trackSource := localfiles.NewAdapter(cfg.MediaDir, localfiles.NewFFprobeProber("ffprobe"))
	store, err := sqlite.NewStore(cfg.DatabasePath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer store.Close()

	if err := bootstrap.SeedLocalChannel(ctx, store, trackSource, cfg.ChannelID, cfg.ChannelName, time.Now().UTC()); err != nil {
		log.Fatalf("seed local channel: %v", err)
	}

	service := scheduler.NewService(store, trackSource)
	server := api.NewServer(service)

	httpServer := &http.Server{
		Addr:              cfg.ListenAddress,
		Handler:           server,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("serve: %v", err)
		}
	}()

	log.Printf("atlas-audio-engine listening on %s", cfg.ListenAddress)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
}
