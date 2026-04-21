package media

import (
	"context"
	"io"
)

type AudioStreamer interface {
	StreamMP3(ctx context.Context, inputPath string, startSeconds float64, output io.Writer) error
	StreamPCM(ctx context.Context, inputPath string, startSeconds float64, output io.Writer) error
}

type VideoMetadata struct {
	Title       string
	Artist      string
	NextTitle   string
	NextArtist  string
	DurationMs  int64
	ElapsedMs   int64
	ArtworkPath string
}

type VideoMetadataProvider func(ctx context.Context) (VideoMetadata, error)

type VideoStreamer interface {
	StreamMPEGTS(ctx context.Context, inputPath string, startSeconds float64, metadata VideoMetadata, output io.Writer) error
	StreamPersistentMPEGTS(ctx context.Context, audioURL string, metadataProvider VideoMetadataProvider, output io.Writer) error
	StreamPersistentVisualMPEGTS(ctx context.Context, audioURL string, metadataProvider VideoMetadataProvider, output io.Writer) error
}

type Streamer interface {
	AudioStreamer
	VideoStreamer
}
