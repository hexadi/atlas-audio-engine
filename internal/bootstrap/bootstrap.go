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

	if len(channels) > 0 {
		state, err := repository.GetChannelState(ctx, channelID)
		if err != nil {
			log.Printf("event=bootstrap.skip reason=channels_exist configured_channel_missing channel_id=%s channel_count=%d", channelID, len(channels))
			return nil
		}

		reconciledTrackIDs := reconcilePlaylistTrackIDs(state.PlaylistTrackIDs, playlistTrackIDs)
		currentTrackValid := containsTrackID(reconciledTrackIDs, state.Channel.CurrentTrackID)
		if len(reconciledTrackIDs) == len(state.PlaylistTrackIDs) && currentTrackValid {
			log.Printf("event=bootstrap.skip reason=channel_valid channel_id=%s track_count=%d", channelID, len(reconciledTrackIDs))
			return nil
		}

		state.PlaylistTrackIDs = reconciledTrackIDs
		state.Channel.PlaylistCursor = 0
		state.Channel.CurrentTrackID = reconciledTrackIDs[0]
		state.Channel.StartedAt = startAt.UTC()
		if err := repository.UpsertChannelState(ctx, state); err != nil {
			return err
		}
		log.Printf("event=bootstrap.reconcile channel_id=%s track_count=%d", channelID, len(reconciledTrackIDs))
		return nil
	}

	state := store.ChannelState{
		Channel: domain.Channel{
			ID:             channelID,
			Name:           channelName,
			Enabled:        true,
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

func reconcilePlaylistTrackIDs(existingTrackIDs []string, libraryTrackIDs []string) []string {
	librarySet := make(map[string]struct{}, len(libraryTrackIDs))
	for _, trackID := range libraryTrackIDs {
		librarySet[trackID] = struct{}{}
	}

	reconciled := make([]string, 0, len(libraryTrackIDs))
	seen := make(map[string]struct{}, len(libraryTrackIDs))
	for _, trackID := range existingTrackIDs {
		if _, ok := librarySet[trackID]; !ok {
			continue
		}
		if _, ok := seen[trackID]; ok {
			continue
		}
		reconciled = append(reconciled, trackID)
		seen[trackID] = struct{}{}
	}

	for _, trackID := range libraryTrackIDs {
		if _, ok := seen[trackID]; ok {
			continue
		}
		reconciled = append(reconciled, trackID)
	}
	return reconciled
}

func containsTrackID(trackIDs []string, target string) bool {
	for _, trackID := range trackIDs {
		if trackID == target {
			return true
		}
	}
	return false
}
