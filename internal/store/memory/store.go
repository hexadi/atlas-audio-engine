package memory

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/homepc/atlas-audio-engine/internal/domain"
	"github.com/homepc/atlas-audio-engine/internal/store"
)

type Store struct {
	mu       sync.RWMutex
	channels map[string]store.ChannelState
}

func NewStore() *Store {
	return &Store{
		channels: map[string]store.ChannelState{},
	}
}

func (s *Store) ListChannels(_ context.Context) ([]domain.Channel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	channels := make([]domain.Channel, 0, len(s.channels))
	for _, state := range s.channels {
		channels = append(channels, state.Channel)
	}
	return channels, nil
}

func (s *Store) GetChannelState(_ context.Context, channelID string) (store.ChannelState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	state, ok := s.channels[channelID]
	if !ok {
		return store.ChannelState{}, errors.New("channel not found")
	}
	return cloneState(state), nil
}

func (s *Store) UpsertChannelState(_ context.Context, state store.ChannelState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.channels[state.Channel.ID] = cloneState(state)
	return nil
}

func (s *Store) DeleteChannel(_ context.Context, channelID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.channels[channelID]; !ok {
		return errors.New("channel not found")
	}
	delete(s.channels, channelID)
	return nil
}

func (s *Store) Enqueue(_ context.Context, channelID, trackID string, enqueuedAt time.Time) (domain.QueueItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, ok := s.channels[channelID]
	if !ok {
		return domain.QueueItem{}, errors.New("channel not found")
	}

	item := domain.QueueItem{
		ID:         fmt.Sprintf("%s-%d", trackID, enqueuedAt.UnixNano()),
		ChannelID:  channelID,
		TrackID:    trackID,
		EnqueuedAt: enqueuedAt.UTC(),
	}
	state.Queue = append(state.Queue, item)
	s.channels[channelID] = cloneState(state)
	return item, nil
}

func (s *Store) RemoveQueueItem(_ context.Context, channelID, queueItemID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, ok := s.channels[channelID]
	if !ok {
		return errors.New("channel not found")
	}

	filtered := make([]domain.QueueItem, 0, len(state.Queue))
	removed := false
	for _, item := range state.Queue {
		if item.ID == queueItemID {
			removed = true
			continue
		}
		filtered = append(filtered, item)
	}
	if !removed {
		return errors.New("queue item not found")
	}

	state.Queue = filtered
	s.channels[channelID] = cloneState(state)
	return nil
}

func cloneState(state store.ChannelState) store.ChannelState {
	clonedPlaylist := append([]string(nil), state.PlaylistTrackIDs...)
	clonedBlocks := make([]domain.ScheduleBlock, 0, len(state.ScheduleBlocks))
	for _, block := range state.ScheduleBlocks {
		clonedBlocks = append(clonedBlocks, domain.ScheduleBlock{
			ID:           block.ID,
			ChannelID:    block.ChannelID,
			Name:         block.Name,
			Weekdays:     append([]int(nil), block.Weekdays...),
			StartMinute:  block.StartMinute,
			EndMinute:    block.EndMinute,
			TrackIDs:     append([]string(nil), block.TrackIDs...),
			Loop:         block.Loop,
			ShuffleOnRun: block.ShuffleOnRun,
		})
	}
	clonedQueue := append([]domain.QueueItem(nil), state.Queue...)
	state.PlaylistTrackIDs = clonedPlaylist
	state.ScheduleBlocks = clonedBlocks
	state.Queue = clonedQueue
	return state
}
