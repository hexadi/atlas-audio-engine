package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"github.com/homepc/atlas-audio-engine/internal/domain"
	"github.com/homepc/atlas-audio-engine/internal/store"
)

type Store struct {
	db *sql.DB
}

func NewStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	instance := &Store{db: db}
	if err := instance.ensureSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return instance, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) ListChannels(ctx context.Context) ([]domain.Channel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, created_at, started_at, current_track_id, playlist_cursor
		FROM channels
		ORDER BY name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var channels []domain.Channel
	for rows.Next() {
		var channel domain.Channel
		if err := rows.Scan(
			&channel.ID,
			&channel.Name,
			&channel.CreatedAt,
			&channel.StartedAt,
			&channel.CurrentTrackID,
			&channel.PlaylistCursor,
		); err != nil {
			return nil, err
		}
		channels = append(channels, channel)
	}
	return channels, rows.Err()
}

func (s *Store) GetChannelState(ctx context.Context, channelID string) (store.ChannelState, error) {
	var state store.ChannelState
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, created_at, started_at, current_track_id, playlist_cursor
		FROM channels
		WHERE id = ?
	`, channelID).Scan(
		&state.Channel.ID,
		&state.Channel.Name,
		&state.Channel.CreatedAt,
		&state.Channel.StartedAt,
		&state.Channel.CurrentTrackID,
		&state.Channel.PlaylistCursor,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return store.ChannelState{}, errors.New("channel not found")
		}
		return store.ChannelState{}, err
	}

	playlistRows, err := s.db.QueryContext(ctx, `
		SELECT track_id
		FROM playlist_entries
		WHERE channel_id = ?
		ORDER BY position ASC
	`, channelID)
	if err != nil {
		return store.ChannelState{}, err
	}
	defer playlistRows.Close()

	for playlistRows.Next() {
		var trackID string
		if err := playlistRows.Scan(&trackID); err != nil {
			return store.ChannelState{}, err
		}
		state.PlaylistTrackIDs = append(state.PlaylistTrackIDs, trackID)
	}
	if err := playlistRows.Err(); err != nil {
		return store.ChannelState{}, err
	}

	queueRows, err := s.db.QueryContext(ctx, `
		SELECT id, channel_id, track_id, enqueued_at
		FROM queue_items
		WHERE channel_id = ?
		ORDER BY enqueued_at ASC
	`, channelID)
	if err != nil {
		return store.ChannelState{}, err
	}
	defer queueRows.Close()

	for queueRows.Next() {
		var item domain.QueueItem
		if err := queueRows.Scan(&item.ID, &item.ChannelID, &item.TrackID, &item.EnqueuedAt); err != nil {
			return store.ChannelState{}, err
		}
		state.Queue = append(state.Queue, item)
	}
	return state, queueRows.Err()
}

func (s *Store) UpsertChannelState(ctx context.Context, state store.ChannelState) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.ExecContext(ctx, `
		INSERT INTO channels (id, name, created_at, started_at, current_track_id, playlist_cursor)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			created_at = excluded.created_at,
			started_at = excluded.started_at,
			current_track_id = excluded.current_track_id,
			playlist_cursor = excluded.playlist_cursor
	`,
		state.Channel.ID,
		state.Channel.Name,
		state.Channel.CreatedAt.UTC(),
		state.Channel.StartedAt.UTC(),
		state.Channel.CurrentTrackID,
		state.Channel.PlaylistCursor,
	); err != nil {
		return err
	}

	if _, err = tx.ExecContext(ctx, `DELETE FROM playlist_entries WHERE channel_id = ?`, state.Channel.ID); err != nil {
		return err
	}
	for position, trackID := range state.PlaylistTrackIDs {
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO playlist_entries (channel_id, position, track_id)
			VALUES (?, ?, ?)
		`, state.Channel.ID, position, trackID); err != nil {
			return err
		}
	}

	if _, err = tx.ExecContext(ctx, `DELETE FROM queue_items WHERE channel_id = ?`, state.Channel.ID); err != nil {
		return err
	}
	for _, item := range state.Queue {
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO queue_items (id, channel_id, track_id, enqueued_at)
			VALUES (?, ?, ?, ?)
		`, item.ID, item.ChannelID, item.TrackID, item.EnqueuedAt.UTC()); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *Store) Enqueue(ctx context.Context, channelID, trackID string, enqueuedAt time.Time) (domain.QueueItem, error) {
	item := domain.QueueItem{
		ID:         fmt.Sprintf("%s-%d", trackID, enqueuedAt.UnixNano()),
		ChannelID:  channelID,
		TrackID:    trackID,
		EnqueuedAt: enqueuedAt.UTC(),
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO queue_items (id, channel_id, track_id, enqueued_at)
		VALUES (?, ?, ?, ?)
	`, item.ID, item.ChannelID, item.TrackID, item.EnqueuedAt)
	if err != nil {
		return domain.QueueItem{}, err
	}
	return item, nil
}

func (s *Store) RemoveQueueItem(ctx context.Context, channelID, queueItemID string) error {
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM queue_items
		WHERE id = ? AND channel_id = ?
	`, queueItemID, channelID)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errors.New("queue item not found")
	}
	return nil
}

func (s *Store) ensureSchema() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS channels (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			started_at DATETIME NOT NULL,
			current_track_id TEXT NOT NULL,
			playlist_cursor INTEGER NOT NULL DEFAULT 0
		);

		CREATE TABLE IF NOT EXISTS playlist_entries (
			channel_id TEXT NOT NULL,
			position INTEGER NOT NULL,
			track_id TEXT NOT NULL,
			PRIMARY KEY (channel_id, position)
		);

		CREATE TABLE IF NOT EXISTS queue_items (
			id TEXT PRIMARY KEY,
			channel_id TEXT NOT NULL,
			track_id TEXT NOT NULL,
			enqueued_at DATETIME NOT NULL
		);
	`)
	return err
}
