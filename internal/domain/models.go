package domain

import "time"

type SourceType string

const (
	SourceTypeLocal SourceType = "local"
)

type Track struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Artist      string     `json:"artist"`
	Album       string     `json:"album,omitempty"`
	DurationMs  int64      `json:"duration_ms"`
	SourceType  SourceType `json:"source_type"`
	FilePath    string     `json:"file_path,omitempty"`
	ArtworkPath string     `json:"artwork_path,omitempty"`
	ArtworkURL  string     `json:"artwork_url,omitempty"`
}

type Playlist struct {
	ID        string   `json:"id"`
	ChannelID string   `json:"channel_id"`
	Tracks    []string `json:"tracks"`
}

type PlaylistEntry struct {
	TrackID    string     `json:"track_id"`
	Position   int        `json:"position"`
	Title      string     `json:"title"`
	Artist     string     `json:"artist"`
	Album      string     `json:"album,omitempty"`
	DurationMs int64      `json:"duration_ms"`
	SourceType SourceType `json:"source_type"`
	ArtworkURL string     `json:"artwork_url,omitempty"`
}

type Channel struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Enabled        bool      `json:"enabled"`
	CreatedAt      time.Time `json:"created_at"`
	StartedAt      time.Time `json:"started_at"`
	CurrentTrackID string    `json:"current_track_id,omitempty"`
	PlaylistCursor int       `json:"playlist_cursor"`
}

type QueueItem struct {
	ID         string    `json:"id"`
	ChannelID  string    `json:"channel_id"`
	TrackID    string    `json:"track_id"`
	EnqueuedAt time.Time `json:"enqueued_at"`
}

type QueueEntry struct {
	ID         string     `json:"id"`
	ChannelID  string     `json:"channel_id"`
	TrackID    string     `json:"track_id"`
	EnqueuedAt time.Time  `json:"enqueued_at"`
	Position   int        `json:"position"`
	Title      string     `json:"title"`
	Artist     string     `json:"artist"`
	Album      string     `json:"album,omitempty"`
	DurationMs int64      `json:"duration_ms"`
	SourceType SourceType `json:"source_type"`
	ArtworkURL string     `json:"artwork_url,omitempty"`
}

type PlayheadState struct {
	ChannelID  string     `json:"channel_id"`
	TrackID    string     `json:"track_id"`
	Title      string     `json:"title"`
	Artist     string     `json:"artist"`
	DurationMs int64      `json:"duration_ms"`
	ElapsedMs  int64      `json:"elapsed_ms"`
	StartedAt  time.Time  `json:"started_at"`
	SourceType SourceType `json:"source_type"`
	ArtworkURL string     `json:"artwork_url,omitempty"`
}

type NextTrack struct {
	TrackID    string     `json:"track_id"`
	Title      string     `json:"title"`
	Artist     string     `json:"artist"`
	Album      string     `json:"album,omitempty"`
	DurationMs int64      `json:"duration_ms"`
	SourceType SourceType `json:"source_type"`
	ArtworkURL string     `json:"artwork_url,omitempty"`
}

type ChannelStateSnapshot struct {
	ChannelID  string        `json:"channel_id"`
	NowPlaying PlayheadState `json:"now_playing"`
	Queue      []QueueEntry  `json:"queue"`
	NextTrack  *NextTrack    `json:"next_track,omitempty"`
}

type ScheduleBlock struct {
	ID        string    `json:"id"`
	ChannelID string    `json:"channel_id"`
	Name      string    `json:"name"`
	StartsAt  time.Time `json:"starts_at"`
	EndsAt    time.Time `json:"ends_at"`
}
