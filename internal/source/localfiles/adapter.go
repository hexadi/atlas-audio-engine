package localfiles

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

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
	Disc       string
	DurationMs int64
}

type Prober interface {
	Probe(ctx context.Context, path string) (Metadata, error)
}

type Adapter struct {
	root   string
	prober Prober

	mu      sync.RWMutex
	tracks  []domain.Track
	byID    map[string]domain.Track
	scanned bool
}

func NewAdapter(root string, prober Prober) *Adapter {
	return &Adapter{
		root:   root,
		prober: prober,
	}
}

func (a *Adapter) ListTracks(ctx context.Context) ([]domain.Track, error) {
	if tracks, ok := a.cachedTracks(); ok {
		return tracks, nil
	}

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

	a.storeCache(tracks)
	return tracks, nil
}

func (a *Adapter) GetTrack(ctx context.Context, id string) (domain.Track, error) {
	if track, ok := a.cachedTrack(id); ok {
		return track, nil
	}

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

	fallbackArtist, fallbackTitle := parseFilenameMetadata(absolutePath)

	title := metadata.Title
	if title == "" {
		title = fallbackTitle
	}

	artist := strings.Join(strings.Split(metadata.Artist, ";"), ", ")
	if artist == "" {
		artist = fallbackArtist
	}
	if artist == "" {
		artist = "Unknown Artist"
	}

	trackID := stableID(absolutePath)
	artworkPath := findCoverArtwork(absolutePath, metadata)
	artworkURL := ""
	if artworkPath != "" {
		artworkURL = "/artwork/" + trackID
	}

	return domain.Track{
		ID:          trackID,
		Title:       title,
		Artist:      artist,
		Album:       metadata.Album,
		DurationMs:  metadata.DurationMs,
		SourceType:  domain.SourceTypeLocal,
		FilePath:    absolutePath,
		ArtworkPath: artworkPath,
		ArtworkURL:  artworkURL,
	}, nil
}

func stableID(value string) string {
	sum := sha1.Sum([]byte(value))
	return hex.EncodeToString(sum[:])
}

func parseFilenameMetadata(path string) (artist string, title string) {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	parts := strings.Split(base, " - ")

	switch len(parts) {
	case 0:
		return "", base
	case 1:
		return "", parts[0]
	case 2:
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	default:
		possibleTrackNumber := strings.TrimSpace(parts[0])
		if isNumericPrefix(possibleTrackNumber) {
			return strings.TrimSpace(parts[1]), strings.TrimSpace(strings.Join(parts[2:], " - "))
		}
		return strings.TrimSpace(parts[0]), strings.TrimSpace(strings.Join(parts[1:], " - "))
	}
}

func isNumericPrefix(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func findCoverArtwork(trackPath string, metadata Metadata) string {
	coverPath := filepath.Join(filepath.Dir(trackPath), "cover.jpg")
	if info, err := os.Stat(coverPath); err == nil && !info.IsDir() {
		return coverPath
	}

	if metadata.Disc != "" {
		parentCoverPath := filepath.Join(filepath.Dir(trackPath), "..", "cover.jpg")
		if info, err := os.Stat(parentCoverPath); err == nil && !info.IsDir() {
			absolutePath, absErr := filepath.Abs(parentCoverPath)
			if absErr == nil {
				return absolutePath
			}
			return parentCoverPath
		}
	}

	return ""
}

func (a *Adapter) cachedTracks() ([]domain.Track, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if !a.scanned || len(a.tracks) == 0 {
		return nil, false
	}
	return append([]domain.Track(nil), a.tracks...), true
}

func (a *Adapter) cachedTrack(id string) (domain.Track, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if !a.scanned || a.byID == nil {
		return domain.Track{}, false
	}
	track, ok := a.byID[id]
	return track, ok
}

func (a *Adapter) storeCache(tracks []domain.Track) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.tracks = append([]domain.Track(nil), tracks...)
	a.byID = make(map[string]domain.Track, len(tracks))
	for _, track := range tracks {
		a.byID[track.ID] = track
	}
	a.scanned = true
}
