package scheduler

import (
	"context"
	"errors"
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

func (s *Service) Queue(ctx context.Context, channelID string) ([]domain.QueueItem, error) {
	state, err := s.store.GetChannelState(ctx, channelID)
	if err != nil {
		return nil, err
	}
	return state.Queue, nil
}

func (s *Service) Enqueue(ctx context.Context, channelID, trackID string) (domain.QueueItem, error) {
	if _, err := s.source.GetTrack(ctx, trackID); err != nil {
		return domain.QueueItem{}, err
	}
	return s.store.Enqueue(ctx, channelID, trackID, s.clock())
}

func (s *Service) CurrentNow(ctx context.Context, channelID string) (domain.PlayheadState, error) {
	return s.Current(ctx, channelID, s.clock())
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
