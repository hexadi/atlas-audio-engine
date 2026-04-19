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
}

type Playlist struct {
	ID        string   `json:"id"`
	ChannelID string   `json:"channel_id"`
	Tracks    []string `json:"tracks"`
}

type Channel struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
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

type PlayheadState struct {
	ChannelID  string     `json:"channel_id"`
	TrackID    string     `json:"track_id"`
	Title      string     `json:"title"`
	Artist     string     `json:"artist"`
	DurationMs int64      `json:"duration_ms"`
	ElapsedMs  int64      `json:"elapsed_ms"`
	StartedAt  time.Time  `json:"started_at"`
	SourceType SourceType `json:"source_type"`
}

type ScheduleBlock struct {
	ID        string    `json:"id"`
	ChannelID string    `json:"channel_id"`
	Name      string    `json:"name"`
	StartsAt  time.Time `json:"starts_at"`
	EndsAt    time.Time `json:"ends_at"`
}
