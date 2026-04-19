package source

import (
	"context"

	"github.com/homepc/atlas-audio-engine/internal/domain"
)

type Playable struct {
	TrackID string `json:"track_id"`
	Path    string `json:"path"`
}

type Library interface {
	ListTracks(ctx context.Context) ([]domain.Track, error)
	GetTrack(ctx context.Context, id string) (domain.Track, error)
	ResolvePlayable(ctx context.Context, id string) (Playable, error)
}
