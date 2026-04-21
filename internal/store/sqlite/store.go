package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
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
		SELECT id, name, enabled, created_at, started_at, current_track_id, playlist_cursor, current_schedule_block_id
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
			&channel.Enabled,
			&channel.CreatedAt,
			&channel.StartedAt,
			&channel.CurrentTrackID,
			&channel.PlaylistCursor,
			&channel.CurrentScheduleBlockID,
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
		SELECT id, name, enabled, created_at, started_at, current_track_id, playlist_cursor, current_schedule_block_id
		FROM channels
		WHERE id = ?
	`, channelID).Scan(
		&state.Channel.ID,
		&state.Channel.Name,
		&state.Channel.Enabled,
		&state.Channel.CreatedAt,
		&state.Channel.StartedAt,
		&state.Channel.CurrentTrackID,
		&state.Channel.PlaylistCursor,
		&state.Channel.CurrentScheduleBlockID,
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

	blockRows, err := s.db.QueryContext(ctx, `
		SELECT id, channel_id, name, weekdays, start_minute, end_minute, starts_at, ends_at, loop, shuffle_on_run
		FROM schedule_blocks
		WHERE channel_id = ?
		ORDER BY start_minute ASC, end_minute ASC, id ASC
	`, channelID)
	if err != nil {
		return store.ChannelState{}, err
	}
	defer blockRows.Close()

	for blockRows.Next() {
		var block domain.ScheduleBlock
		var weekdaysRaw []byte
		var legacyStartsAt sql.NullTime
		var legacyEndsAt sql.NullTime
		if err := blockRows.Scan(&block.ID, &block.ChannelID, &block.Name, &weekdaysRaw, &block.StartMinute, &block.EndMinute, &legacyStartsAt, &legacyEndsAt, &block.Loop, &block.ShuffleOnRun); err != nil {
			return store.ChannelState{}, err
		}
		if len(weekdaysRaw) > 0 {
			if err := json.Unmarshal(weekdaysRaw, &block.Weekdays); err != nil {
				return store.ChannelState{}, err
			}
		}
		if len(block.Weekdays) == 0 && legacyStartsAt.Valid && legacyEndsAt.Valid {
			startLocal := legacyStartsAt.Time.UTC()
			endLocal := legacyEndsAt.Time.UTC()
			block.Weekdays = []int{int(startLocal.Weekday())}
			block.StartMinute = startLocal.Hour()*60 + startLocal.Minute()
			block.EndMinute = endLocal.Hour()*60 + endLocal.Minute()
		}

		trackRows, err := s.db.QueryContext(ctx, `
			SELECT track_id
			FROM schedule_block_tracks
			WHERE channel_id = ? AND block_id = ?
			ORDER BY position ASC
		`, channelID, block.ID)
		if err != nil {
			return store.ChannelState{}, err
		}
		for trackRows.Next() {
			var trackID string
			if err := trackRows.Scan(&trackID); err != nil {
				trackRows.Close()
				return store.ChannelState{}, err
			}
			block.TrackIDs = append(block.TrackIDs, trackID)
		}
		if err := trackRows.Err(); err != nil {
			trackRows.Close()
			return store.ChannelState{}, err
		}
		trackRows.Close()

		state.ScheduleBlocks = append(state.ScheduleBlocks, block)
	}
	if err := blockRows.Err(); err != nil {
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
		INSERT INTO channels (id, name, enabled, created_at, started_at, current_track_id, playlist_cursor, current_schedule_block_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			enabled = excluded.enabled,
			created_at = excluded.created_at,
			started_at = excluded.started_at,
			current_track_id = excluded.current_track_id,
			playlist_cursor = excluded.playlist_cursor,
			current_schedule_block_id = excluded.current_schedule_block_id
	`,
		state.Channel.ID,
		state.Channel.Name,
		state.Channel.Enabled,
		state.Channel.CreatedAt.UTC(),
		state.Channel.StartedAt.UTC(),
		state.Channel.CurrentTrackID,
		state.Channel.PlaylistCursor,
		state.Channel.CurrentScheduleBlockID,
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

	if _, err = tx.ExecContext(ctx, `DELETE FROM schedule_block_tracks WHERE channel_id = ?`, state.Channel.ID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM schedule_blocks WHERE channel_id = ?`, state.Channel.ID); err != nil {
		return err
	}
	for _, block := range state.ScheduleBlocks {
		weekdaysJSON, err := json.Marshal(block.Weekdays)
		if err != nil {
			return err
		}
		legacyStartsAt, legacyEndsAt := legacyScheduleBounds(block)
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO schedule_blocks (id, channel_id, name, weekdays, start_minute, end_minute, starts_at, ends_at, loop, shuffle_on_run)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, block.ID, state.Channel.ID, block.Name, string(weekdaysJSON), block.StartMinute, block.EndMinute, legacyStartsAt, legacyEndsAt, block.Loop, block.ShuffleOnRun); err != nil {
			return err
		}
		for position, trackID := range block.TrackIDs {
			if _, err = tx.ExecContext(ctx, `
				INSERT INTO schedule_block_tracks (channel_id, block_id, position, track_id)
				VALUES (?, ?, ?, ?)
			`, state.Channel.ID, block.ID, position, trackID); err != nil {
				return err
			}
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

func (s *Store) DeleteChannel(ctx context.Context, channelID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.ExecContext(ctx, `DELETE FROM playlist_entries WHERE channel_id = ?`, channelID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM schedule_block_tracks WHERE channel_id = ?`, channelID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM schedule_blocks WHERE channel_id = ?`, channelID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM queue_items WHERE channel_id = ?`, channelID); err != nil {
		return err
	}

	result, err := tx.ExecContext(ctx, `DELETE FROM channels WHERE id = ?`, channelID)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errors.New("channel not found")
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
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS channels (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			enabled BOOLEAN NOT NULL DEFAULT 1,
			created_at DATETIME NOT NULL,
			started_at DATETIME NOT NULL,
			current_track_id TEXT NOT NULL,
			playlist_cursor INTEGER NOT NULL DEFAULT 0,
			current_schedule_block_id TEXT NOT NULL DEFAULT ''
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

		CREATE TABLE IF NOT EXISTS schedule_blocks (
			id TEXT NOT NULL,
			channel_id TEXT NOT NULL,
			name TEXT NOT NULL,
			weekdays TEXT NOT NULL DEFAULT '[]',
			start_minute INTEGER NOT NULL DEFAULT 0,
			end_minute INTEGER NOT NULL DEFAULT 0,
			starts_at DATETIME NOT NULL,
			ends_at DATETIME NOT NULL,
			loop BOOLEAN NOT NULL DEFAULT 1,
			shuffle_on_run BOOLEAN NOT NULL DEFAULT 0,
			PRIMARY KEY (channel_id, id)
		);

		CREATE TABLE IF NOT EXISTS schedule_block_tracks (
			channel_id TEXT NOT NULL,
			block_id TEXT NOT NULL,
			position INTEGER NOT NULL,
			track_id TEXT NOT NULL,
			PRIMARY KEY (channel_id, block_id, position)
		);
	`); err != nil {
		return err
	}
	if err := s.ensureColumn("channels", "enabled", "BOOLEAN NOT NULL DEFAULT 1"); err != nil {
		return err
	}
	if err := s.ensureColumn("channels", "current_schedule_block_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := s.ensureColumn("schedule_blocks", "weekdays", "TEXT NOT NULL DEFAULT '[]'"); err != nil {
		return err
	}
	if err := s.ensureColumn("schedule_blocks", "start_minute", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := s.ensureColumn("schedule_blocks", "end_minute", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := s.ensureColumn("schedule_blocks", "loop", "BOOLEAN NOT NULL DEFAULT 1"); err != nil {
		return err
	}
	return s.ensureColumn("schedule_blocks", "shuffle_on_run", "BOOLEAN NOT NULL DEFAULT 0")
}

func (s *Store) ensureColumn(tableName, columnName, definition string) error {
	rows, err := s.db.Query(fmt.Sprintf(`PRAGMA table_info(%s)`, tableName))
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name string
		var columnType string
		var notNull int
		var defaultValue sql.NullString
		var primaryKey int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return err
		}
		if name == columnName {
			return rows.Err()
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	_, err = s.db.Exec(fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s`, tableName, columnName, definition))
	return err
}

func legacyScheduleBounds(block domain.ScheduleBlock) (time.Time, time.Time) {
	startMinute := block.StartMinute
	endMinute := block.EndMinute
	if startMinute < 0 {
		startMinute = 0
	}
	if endMinute < 0 {
		endMinute = 0
	}
	baseWeekStart := time.Date(2000, 1, 2, 0, 0, 0, 0, time.UTC)
	weekday := 0
	if len(block.Weekdays) > 0 {
		weekday = block.Weekdays[0]
	}
	start := baseWeekStart.AddDate(0, 0, weekday).Add(time.Duration(startMinute) * time.Minute)
	end := baseWeekStart.AddDate(0, 0, weekday).Add(time.Duration(endMinute) * time.Minute)
	if endMinute <= startMinute {
		end = end.Add(24 * time.Hour)
	}
	return start, end
}
