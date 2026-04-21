package store

import (
	"context"
	"time"

	"github.com/homepc/atlas-audio-engine/internal/domain"
)

type ChannelState struct {
	Channel          domain.Channel
	PlaylistTrackIDs []string
	ScheduleBlocks   []domain.ScheduleBlock
	Queue            []domain.QueueItem
}

type Store interface {
	ListChannels(ctx context.Context) ([]domain.Channel, error)
	GetChannelState(ctx context.Context, channelID string) (ChannelState, error)
	UpsertChannelState(ctx context.Context, state ChannelState) error
	DeleteChannel(ctx context.Context, channelID string) error
	Enqueue(ctx context.Context, channelID, trackID string, enqueuedAt time.Time) (domain.QueueItem, error)
	RemoveQueueItem(ctx context.Context, channelID, queueItemID string) error
}
