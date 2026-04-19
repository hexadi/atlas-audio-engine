package localfiles

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/homepc/atlas-audio-engine/internal/domain"
	"github.com/homepc/atlas-audio-engine/internal/source"
)

var supportedExtensions = map[string]struct{}{
	".aac":  {},
	".flac": {},
	".m4a":  {},
	".mp3":  {},
	".ogg":  {},
	".wav":  {},
}

type Metadata struct {
	Title      string
	Artist     string
	Album      string
	DurationMs int64
}

type Prober interface {
	Probe(ctx context.Context, path string) (Metadata, error)
}

type Adapter struct {
	root   string
	prober Prober
}

func NewAdapter(root string, prober Prober) *Adapter {
	return &Adapter{
		root:   root,
		prober: prober,
	}
}

func (a *Adapter) ListTracks(ctx context.Context) ([]domain.Track, error) {
	var tracks []domain.Track

	err := filepath.WalkDir(a.root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if _, ok := supportedExtensions[strings.ToLower(filepath.Ext(path))]; !ok {
			return nil
		}

		track, err := a.buildTrack(ctx, path)
		if err != nil {
			return nil
		}
		tracks = append(tracks, track)
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(tracks, func(i, j int) bool {
		if tracks[i].Title == tracks[j].Title {
			return tracks[i].FilePath < tracks[j].FilePath
		}
		return tracks[i].Title < tracks[j].Title
	})

	if len(tracks) == 0 {
		return nil, errors.New("no playable local tracks found")
	}
	return tracks, nil
}

func (a *Adapter) GetTrack(ctx context.Context, id string) (domain.Track, error) {
	tracks, err := a.ListTracks(ctx)
	if err != nil {
		return domain.Track{}, err
	}
	for _, track := range tracks {
		if track.ID == id {
			return track, nil
		}
	}
	return domain.Track{}, errors.New("track not found")
}

func (a *Adapter) ResolvePlayable(ctx context.Context, id string) (source.Playable, error) {
	track, err := a.GetTrack(ctx, id)
	if err != nil {
		return source.Playable{}, err
	}
	return source.Playable{
		TrackID: track.ID,
		Path:    track.FilePath,
	}, nil
}

func (a *Adapter) buildTrack(ctx context.Context, path string) (domain.Track, error) {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return domain.Track{}, err
	}

	metadata, err := a.prober.Probe(ctx, absolutePath)
	if err != nil {
		return domain.Track{}, err
	}

	title := metadata.Title
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(absolutePath), filepath.Ext(absolutePath))
	}

	artist := metadata.Artist
	if artist == "" {
		artist = "Unknown Artist"
	}

	return domain.Track{
		ID:         stableID(absolutePath),
		Title:      title,
		Artist:     artist,
		Album:      metadata.Album,
		DurationMs: metadata.DurationMs,
		SourceType: domain.SourceTypeLocal,
		FilePath:   absolutePath,
	}, nil
}

func stableID(value string) string {
	sum := sha1.Sum([]byte(value))
	return hex.EncodeToString(sum[:])
}
