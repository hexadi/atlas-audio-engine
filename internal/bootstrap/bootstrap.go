package bootstrap

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/homepc/atlas-audio-engine/internal/domain"
	"github.com/homepc/atlas-audio-engine/internal/source"
	"github.com/homepc/atlas-audio-engine/internal/store"
)

func SeedLocalChannel(
	ctx context.Context,
	repository store.Store,
	library source.Library,
	channelID string,
	channelName string,
	startAt time.Time,
) error {
	channels, err := repository.ListChannels(ctx)
	if err != nil {
		return err
	}
	if len(channels) > 0 {
		log.Printf("event=bootstrap.skip reason=channels_exist channel_count=%d", len(channels))
		return nil
	}

	tracks, err := library.ListTracks(ctx)
	if err != nil {
		return err
	}
	if len(tracks) == 0 {
		return errors.New("cannot seed channel without local tracks")
	}

	playlistTrackIDs := make([]string, 0, len(tracks))
	for _, track := range tracks {
		playlistTrackIDs = append(playlistTrackIDs, track.ID)
	}

	state := store.ChannelState{
		Channel: domain.Channel{
			ID:             channelID,
			Name:           channelName,
			CreatedAt:      startAt.UTC(),
			StartedAt:      startAt.UTC(),
			CurrentTrackID: playlistTrackIDs[0],
			PlaylistCursor: 0,
		},
		PlaylistTrackIDs: playlistTrackIDs,
	}
	if err := repository.UpsertChannelState(ctx, state); err != nil {
		return err
	}
	log.Printf("event=bootstrap.seed channel_id=%s track_count=%d", channelID, len(playlistTrackIDs))
	return nil
}
