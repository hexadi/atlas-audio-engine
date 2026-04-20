package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/homepc/atlas-audio-engine/internal/domain"
	"github.com/homepc/atlas-audio-engine/internal/source"
	"github.com/homepc/atlas-audio-engine/internal/store"
)

type Clock func() time.Time

type Service struct {
	store  store.Store
	source source.Library
	clock  Clock
}

func NewService(repository store.Store, library source.Library) *Service {
	return &Service{
		store:  repository,
		source: library,
		clock: func() time.Time {
			return time.Now().UTC()
		},
	}
}

func NewServiceWithClock(repository store.Store, library source.Library, clock Clock) *Service {
	service := NewService(repository, library)
	service.clock = clock
	return service
}

func (s *Service) ListChannels(ctx context.Context) ([]domain.Channel, error) {
	return s.store.ListChannels(ctx)
}

func (s *Service) Queue(ctx context.Context, channelID string) ([]domain.QueueEntry, error) {
	state, err := s.store.GetChannelState(ctx, channelID)
	if err != nil {
		return nil, err
	}

	entries := make([]domain.QueueEntry, 0, len(state.Queue))
	for index, item := range state.Queue {
		track, err := s.source.GetTrack(ctx, item.TrackID)
		if err != nil {
			return nil, err
		}
		entries = append(entries, domain.QueueEntry{
			ID:         item.ID,
			ChannelID:  item.ChannelID,
			TrackID:    item.TrackID,
			EnqueuedAt: item.EnqueuedAt,
			Position:   index + 1,
			Title:      track.Title,
			Artist:     track.Artist,
			Album:      track.Album,
			DurationMs: track.DurationMs,
			SourceType: track.SourceType,
			ArtworkURL: track.ArtworkURL,
		})
	}
	return entries, nil
}

func (s *Service) Tracks(ctx context.Context, channelID string) ([]domain.Track, error) {
	state, err := s.store.GetChannelState(ctx, channelID)
	if err != nil {
		return nil, err
	}

	tracks := make([]domain.Track, 0, len(state.PlaylistTrackIDs))
	for _, trackID := range state.PlaylistTrackIDs {
		track, err := s.source.GetTrack(ctx, trackID)
		if err != nil {
			return nil, err
		}
		tracks = append(tracks, track)
	}
	return tracks, nil
}

func (s *Service) LibraryTracks(ctx context.Context) ([]domain.Track, error) {
	return s.source.ListTracks(ctx)
}

func (s *Service) Playlist(ctx context.Context, channelID string) ([]domain.PlaylistEntry, error) {
	state, err := s.store.GetChannelState(ctx, channelID)
	if err != nil {
		return nil, err
	}

	entries := make([]domain.PlaylistEntry, 0, len(state.PlaylistTrackIDs))
	for index, trackID := range state.PlaylistTrackIDs {
		track, err := s.source.GetTrack(ctx, trackID)
		if err != nil {
			return nil, err
		}
		entries = append(entries, domain.PlaylistEntry{
			TrackID:    track.ID,
			Position:   index + 1,
			Title:      track.Title,
			Artist:     track.Artist,
			Album:      track.Album,
			DurationMs: track.DurationMs,
			SourceType: track.SourceType,
			ArtworkURL: track.ArtworkURL,
		})
	}
	return entries, nil
}

func (s *Service) ReplacePlaylist(ctx context.Context, channelID string, trackIDs []string) ([]domain.PlaylistEntry, error) {
	if len(trackIDs) == 0 {
		return nil, errors.New("playlist must contain at least one track")
	}

	seen := make(map[string]struct{}, len(trackIDs))
	for _, trackID := range trackIDs {
		if trackID == "" {
			return nil, errors.New("playlist track id cannot be empty")
		}
		if _, exists := seen[trackID]; exists {
			return nil, fmt.Errorf("playlist contains duplicate track %q", trackID)
		}
		seen[trackID] = struct{}{}
	}

	state, err := s.store.GetChannelState(ctx, channelID)
	if err != nil {
		return nil, err
	}

	for _, trackID := range trackIDs {
		track, err := s.source.GetTrack(ctx, trackID)
		if err != nil {
			return nil, err
		}
		if track.ID == "" {
			return nil, errors.New("track not found")
		}
	}

	state.PlaylistTrackIDs = append([]string(nil), trackIDs...)
	state.Channel.PlaylistCursor = 0
	state.Channel.CurrentTrackID = trackIDs[0]
	state.Channel.StartedAt = s.clock()

	if err := s.store.UpsertChannelState(ctx, state); err != nil {
		return nil, err
	}
	log.Printf("event=playlist.replace channel_id=%s track_count=%d current_track_id=%s", channelID, len(trackIDs), state.Channel.CurrentTrackID)
	return s.Playlist(ctx, channelID)
}

func (s *Service) Enqueue(ctx context.Context, channelID, trackID string) (domain.QueueItem, error) {
	if _, err := s.source.GetTrack(ctx, trackID); err != nil {
		return domain.QueueItem{}, err
	}
	item, err := s.store.Enqueue(ctx, channelID, trackID, s.clock())
	if err != nil {
		return domain.QueueItem{}, err
	}
	log.Printf("event=queue.enqueue channel_id=%s queue_item_id=%s track_id=%s", channelID, item.ID, trackID)
	return item, nil
}

func (s *Service) RemoveQueueItem(ctx context.Context, channelID, queueItemID string) error {
	if err := s.store.RemoveQueueItem(ctx, channelID, queueItemID); err != nil {
		return err
	}
	log.Printf("event=queue.remove channel_id=%s queue_item_id=%s", channelID, queueItemID)
	return nil
}

func (s *Service) ArtworkPath(ctx context.Context, trackID string) (string, error) {
	track, err := s.source.GetTrack(ctx, trackID)
	if err != nil {
		return "", err
	}
	if track.ArtworkPath == "" {
		return "", errors.New("artwork not found")
	}
	return track.ArtworkPath, nil
}

func (s *Service) MoveQueueItem(ctx context.Context, channelID, queueItemID string, position int) ([]domain.QueueEntry, error) {
	state, err := s.store.GetChannelState(ctx, channelID)
	if err != nil {
		return nil, err
	}
	if len(state.Queue) == 0 {
		return nil, errors.New("queue is empty")
	}

	currentIndex := -1
	for index, item := range state.Queue {
		if item.ID == queueItemID {
			currentIndex = index
			break
		}
	}
	if currentIndex == -1 {
		return nil, errors.New("queue item not found")
	}

	targetIndex := position - 1
	if targetIndex >= len(state.Queue) {
		targetIndex = len(state.Queue) - 1
	}

	item := state.Queue[currentIndex]
	reordered := append([]domain.QueueItem(nil), state.Queue[:currentIndex]...)
	reordered = append(reordered, state.Queue[currentIndex+1:]...)

	if targetIndex > len(reordered) {
		targetIndex = len(reordered)
	}

	reordered = append(reordered[:targetIndex], append([]domain.QueueItem{item}, reordered[targetIndex:]...)...)
	state.Queue = reordered

	if err := s.store.UpsertChannelState(ctx, state); err != nil {
		return nil, err
	}
	log.Printf("event=queue.move channel_id=%s queue_item_id=%s position=%d", channelID, queueItemID, position)
	return s.Queue(ctx, channelID)
}

func (s *Service) Skip(ctx context.Context, channelID string) (domain.PlayheadState, error) {
	state, err := s.store.GetChannelState(ctx, channelID)
	if err != nil {
		return domain.PlayheadState{}, err
	}
	if len(state.PlaylistTrackIDs) == 0 {
		return domain.PlayheadState{}, errors.New("channel playlist is empty")
	}

	nextTrackID, nextCursor, queue := s.pickNext(state)
	if nextTrackID == "" {
		return domain.PlayheadState{}, errors.New("no next track available")
	}

	nextTrack, err := s.source.GetTrack(ctx, nextTrackID)
	if err != nil {
		return domain.PlayheadState{}, err
	}
	if nextTrack.DurationMs <= 0 {
		return domain.PlayheadState{}, errors.New("next track has invalid duration")
	}

	now := s.clock()
	state.Channel.CurrentTrackID = nextTrackID
	state.Channel.StartedAt = now
	state.Channel.PlaylistCursor = nextCursor
	state.Queue = queue

	if err := s.store.UpsertChannelState(ctx, state); err != nil {
		return domain.PlayheadState{}, err
	}
	log.Printf("event=playback.skip channel_id=%s track_id=%s queued_remaining=%d", channelID, nextTrack.ID, len(state.Queue))

	return domain.PlayheadState{
		ChannelID:  channelID,
		TrackID:    nextTrack.ID,
		Title:      nextTrack.Title,
		Artist:     nextTrack.Artist,
		DurationMs: nextTrack.DurationMs,
		ElapsedMs:  0,
		StartedAt:  now,
		SourceType: nextTrack.SourceType,
		ArtworkURL: nextTrack.ArtworkURL,
	}, nil
}

func (s *Service) CurrentNow(ctx context.Context, channelID string) (domain.PlayheadState, error) {
	return s.Current(ctx, channelID, s.clock())
}

func (s *Service) State(ctx context.Context, channelID string) (domain.ChannelStateSnapshot, error) {
	nowPlaying, err := s.CurrentNow(ctx, channelID)
	if err != nil {
		return domain.ChannelStateSnapshot{}, err
	}

	queue, err := s.Queue(ctx, channelID)
	if err != nil {
		return domain.ChannelStateSnapshot{}, err
	}

	nextTrack, err := s.Next(ctx, channelID, nowPlaying.TrackID)
	if err != nil {
		return domain.ChannelStateSnapshot{}, err
	}

	var next *domain.NextTrack
	if nextTrack != nil {
		next = &domain.NextTrack{
			TrackID:    nextTrack.ID,
			Title:      nextTrack.Title,
			Artist:     nextTrack.Artist,
			Album:      nextTrack.Album,
			DurationMs: nextTrack.DurationMs,
			SourceType: nextTrack.SourceType,
			ArtworkURL: nextTrack.ArtworkURL,
		}
	}

	return domain.ChannelStateSnapshot{
		ChannelID:  channelID,
		NowPlaying: nowPlaying,
		Queue:      queue,
		NextTrack:  next,
	}, nil
}

func (s *Service) Current(ctx context.Context, channelID string, at time.Time) (domain.PlayheadState, error) {
	state, err := s.store.GetChannelState(ctx, channelID)
	if err != nil {
		return domain.PlayheadState{}, err
	}

	if len(state.PlaylistTrackIDs) == 0 {
		return domain.PlayheadState{}, errors.New("channel playlist is empty")
	}

	if state.Channel.StartedAt.IsZero() {
		state.Channel.StartedAt = at.UTC()
	}
	if state.Channel.CurrentTrackID == "" {
		state.Channel.CurrentTrackID = state.PlaylistTrackIDs[0]
	}

	currentTrack, err := s.source.GetTrack(ctx, state.Channel.CurrentTrackID)
	if err != nil {
		return domain.PlayheadState{}, err
	}
	if currentTrack.DurationMs <= 0 {
		return domain.PlayheadState{}, errors.New("current track has invalid duration")
	}

	currentStart := state.Channel.StartedAt.UTC()
	changed := false

	for trackEndedAt(currentStart, currentTrack.DurationMs).Before(at) || trackEndedAt(currentStart, currentTrack.DurationMs).Equal(at) {
		nextTrackID, nextCursor, queue := s.pickNext(state)
		if nextTrackID == "" {
			break
		}

		currentStart = trackEndedAt(currentStart, currentTrack.DurationMs)
		state.Channel.StartedAt = currentStart
		state.Channel.CurrentTrackID = nextTrackID
		state.Channel.PlaylistCursor = nextCursor
		state.Queue = queue
		changed = true

		currentTrack, err = s.source.GetTrack(ctx, nextTrackID)
		if err != nil {
			return domain.PlayheadState{}, err
		}
		if currentTrack.DurationMs <= 0 {
			return domain.PlayheadState{}, errors.New("next track has invalid duration")
		}
	}

	if changed {
		if err := s.store.UpsertChannelState(ctx, state); err != nil {
			return domain.PlayheadState{}, err
		}
		log.Printf("event=scheduler.advance channel_id=%s track_id=%s started_at=%s queued_remaining=%d", channelID, currentTrack.ID, state.Channel.StartedAt.UTC().Format(time.RFC3339), len(state.Queue))
	}

	elapsed := at.Sub(state.Channel.StartedAt.UTC()).Milliseconds()
	if elapsed < 0 {
		elapsed = 0
	}

	return domain.PlayheadState{
		ChannelID:  channelID,
		TrackID:    currentTrack.ID,
		Title:      currentTrack.Title,
		Artist:     currentTrack.Artist,
		DurationMs: currentTrack.DurationMs,
		ElapsedMs:  elapsed,
		StartedAt:  state.Channel.StartedAt.UTC(),
		SourceType: currentTrack.SourceType,
		ArtworkURL: currentTrack.ArtworkURL,
	}, nil
}

func (s *Service) Next(ctx context.Context, channelID, afterTrackID string) (*domain.Track, error) {
	state, err := s.store.GetChannelState(ctx, channelID)
	if err != nil {
		return nil, err
	}

	if len(state.Queue) > 0 {
		track, err := s.source.GetTrack(ctx, state.Queue[0].TrackID)
		if err != nil {
			return nil, err
		}
		return &track, nil
	}

	if len(state.PlaylistTrackIDs) == 0 {
		return nil, errors.New("channel playlist is empty")
	}

	index := state.Channel.PlaylistCursor
	if afterTrackID != "" {
		for position, trackID := range state.PlaylistTrackIDs {
			if trackID == afterTrackID {
				index = position
				break
			}
		}
	}

	nextIndex := (index + 1) % len(state.PlaylistTrackIDs)
	track, err := s.source.GetTrack(ctx, state.PlaylistTrackIDs[nextIndex])
	if err != nil {
		return nil, err
	}
	return &track, nil
}

func (s *Service) pickNext(state store.ChannelState) (string, int, []domain.QueueItem) {
	if len(state.Queue) > 0 {
		item := state.Queue[0]
		return item.TrackID, state.Channel.PlaylistCursor, append([]domain.QueueItem(nil), state.Queue[1:]...)
	}
	if len(state.PlaylistTrackIDs) == 0 {
		return "", state.Channel.PlaylistCursor, state.Queue
	}

	nextCursor := (state.Channel.PlaylistCursor + 1) % len(state.PlaylistTrackIDs)
	return state.PlaylistTrackIDs[nextCursor], nextCursor, state.Queue
}

func trackEndedAt(start time.Time, durationMs int64) time.Time {
	return start.Add(time.Duration(durationMs) * time.Millisecond)
}
