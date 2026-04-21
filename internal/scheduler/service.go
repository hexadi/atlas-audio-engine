package scheduler

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"log"
	"math/rand/v2"
	"sort"
	"strings"
	"time"
	"unicode"

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

var ErrChannelExists = errors.New("channel already exists")
var ErrCannotDeleteLastChannel = errors.New("cannot delete the last channel")
var scheduleTimeZone = time.FixedZone("GMT+7", 7*60*60)

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

func (s *Service) CreateChannel(ctx context.Context, id, name string, trackIDs []string) (domain.Channel, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return domain.Channel{}, errors.New("channel name is required")
	}

	id = normalizeChannelID(id)
	if id == "" {
		id = normalizeChannelID(name)
	}
	if id == "" {
		return domain.Channel{}, errors.New("channel id is required")
	}

	channels, err := s.store.ListChannels(ctx)
	if err != nil {
		return domain.Channel{}, err
	}
	for _, channel := range channels {
		if channel.ID == id {
			return domain.Channel{}, ErrChannelExists
		}
	}

	playlistTrackIDs, err := s.resolvePlaylistTrackIDs(ctx, trackIDs)
	if err != nil {
		return domain.Channel{}, err
	}

	now := s.clock().UTC()
	channel := domain.Channel{
		ID:        id,
		Name:      name,
		Enabled:   true,
		CreatedAt: now,
		StartedAt: now,
	}
	if len(playlistTrackIDs) > 0 {
		channel.CurrentTrackID = playlistTrackIDs[0]
	}

	if err := s.store.UpsertChannelState(ctx, store.ChannelState{
		Channel:          channel,
		PlaylistTrackIDs: playlistTrackIDs,
	}); err != nil {
		return domain.Channel{}, err
	}
	log.Printf("event=channel.create channel_id=%s track_count=%d", channel.ID, len(playlistTrackIDs))
	return channel, nil
}

func (s *Service) UpdateChannel(ctx context.Context, channelID string, name *string, enabled *bool) (domain.Channel, error) {
	state, err := s.store.GetChannelState(ctx, channelID)
	if err != nil {
		return domain.Channel{}, err
	}

	if name != nil {
		trimmed := strings.TrimSpace(*name)
		if trimmed == "" {
			return domain.Channel{}, errors.New("channel name is required")
		}
		state.Channel.Name = trimmed
	}
	if enabled != nil {
		state.Channel.Enabled = *enabled
	}

	if err := s.store.UpsertChannelState(ctx, state); err != nil {
		return domain.Channel{}, err
	}
	log.Printf("event=channel.update channel_id=%s enabled=%t", channelID, state.Channel.Enabled)
	return state.Channel, nil
}

func (s *Service) DeleteChannel(ctx context.Context, channelID string) error {
	channels, err := s.store.ListChannels(ctx)
	if err != nil {
		return err
	}
	if len(channels) <= 1 {
		return ErrCannotDeleteLastChannel
	}
	if err := s.store.DeleteChannel(ctx, channelID); err != nil {
		return err
	}
	log.Printf("event=channel.delete channel_id=%s", channelID)
	return nil
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

func (s *Service) ShufflePlaylist(ctx context.Context, channelID string) ([]domain.PlaylistEntry, error) {
	state, err := s.store.GetChannelState(ctx, channelID)
	if err != nil {
		return nil, err
	}
	if len(state.PlaylistTrackIDs) == 0 {
		return nil, errors.New("playlist must contain at least one track")
	}

	shuffled := append([]string(nil), state.PlaylistTrackIDs...)
	if len(shuffled) > 1 {
		original := append([]string(nil), shuffled...)
		for attempt := 0; attempt < 8; attempt++ {
			rand.Shuffle(len(shuffled), func(i, j int) {
				shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
			})
			if !sameTrackOrder(original, shuffled) {
				break
			}
		}
	}

	state.PlaylistTrackIDs = shuffled
	state.Channel.PlaylistCursor = 0
	state.Channel.CurrentTrackID = shuffled[0]
	state.Channel.StartedAt = s.clock().UTC()

	if err := s.store.UpsertChannelState(ctx, state); err != nil {
		return nil, err
	}
	log.Printf("event=playlist.shuffle channel_id=%s track_count=%d current_track_id=%s", channelID, len(shuffled), state.Channel.CurrentTrackID)
	return s.Playlist(ctx, channelID)
}

func (s *Service) ScheduleBlocks(ctx context.Context, channelID string) ([]domain.ScheduleBlock, error) {
	state, err := s.store.GetChannelState(ctx, channelID)
	if err != nil {
		return nil, err
	}

	blocks := make([]domain.ScheduleBlock, 0, len(state.ScheduleBlocks))
	for _, block := range state.ScheduleBlocks {
		cloned := domain.ScheduleBlock{
			ID:           block.ID,
			ChannelID:    block.ChannelID,
			Name:         block.Name,
			Weekdays:     append([]int(nil), block.Weekdays...),
			StartMinute:  block.StartMinute,
			EndMinute:    block.EndMinute,
			TrackIDs:     append([]string(nil), block.TrackIDs...),
			Loop:         block.Loop,
			ShuffleOnRun: block.ShuffleOnRun,
		}
		blocks = append(blocks, cloned)
	}
	return blocks, nil
}

func (s *Service) ReplaceScheduleBlocks(ctx context.Context, channelID string, blocks []domain.ScheduleBlock) ([]domain.ScheduleBlock, error) {
	validated, err := s.validateScheduleBlocks(ctx, channelID, blocks)
	if err != nil {
		return nil, err
	}

	state, err := s.store.GetChannelState(ctx, channelID)
	if err != nil {
		return nil, err
	}
	state.ScheduleBlocks = validated
	if err := s.store.UpsertChannelState(ctx, state); err != nil {
		return nil, err
	}
	log.Printf("event=schedule.blocks.replace channel_id=%s block_count=%d", channelID, len(validated))
	return s.ScheduleBlocks(ctx, channelID)
}

func sameTrackOrder(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func (s *Service) validateScheduleBlocks(ctx context.Context, channelID string, blocks []domain.ScheduleBlock) ([]domain.ScheduleBlock, error) {
	if len(blocks) == 0 {
		return []domain.ScheduleBlock{}, nil
	}

	validated := make([]domain.ScheduleBlock, 0, len(blocks))
	seenIDs := make(map[string]struct{}, len(blocks))
	for _, block := range blocks {
		block.ChannelID = channelID
		block.ID = strings.TrimSpace(block.ID)
		block.Name = strings.TrimSpace(block.Name)
		if block.ID == "" {
			return nil, errors.New("schedule block id is required")
		}
		if _, exists := seenIDs[block.ID]; exists {
			return nil, fmt.Errorf("schedule blocks contain duplicate id %q", block.ID)
		}
		seenIDs[block.ID] = struct{}{}
		if block.Name == "" {
			return nil, fmt.Errorf("schedule block %q name is required", block.ID)
		}
		weekdays, err := normalizeWeekdays(block.Weekdays)
		if err != nil {
			return nil, fmt.Errorf("schedule block %q: %w", block.ID, err)
		}
		block.Weekdays = weekdays
		if len(block.Weekdays) == 0 {
			return nil, fmt.Errorf("schedule block %q must include at least one weekday", block.ID)
		}
		if block.StartMinute < 0 || block.StartMinute >= minutesPerDay {
			return nil, fmt.Errorf("schedule block %q start_minute must be between 0 and 1439", block.ID)
		}
		if block.EndMinute < 0 || block.EndMinute >= minutesPerDay {
			return nil, fmt.Errorf("schedule block %q end_minute must be between 0 and 1439", block.ID)
		}
		if block.StartMinute == block.EndMinute {
			return nil, fmt.Errorf("schedule block %q must span at least one minute", block.ID)
		}

		if len(block.TrackIDs) == 0 {
			return nil, fmt.Errorf("schedule block %q must contain at least one track", block.ID)
		}
		if err := s.validateTrackIDs(ctx, block.TrackIDs); err != nil {
			return nil, fmt.Errorf("schedule block %q: %w", block.ID, err)
		}

		block.TrackIDs = append([]string(nil), block.TrackIDs...)
		validated = append(validated, block)
	}

	sort.Slice(validated, func(i, j int) bool {
		if validated[i].StartMinute == validated[j].StartMinute {
			if validated[i].EndMinute == validated[j].EndMinute {
				return validated[i].ID < validated[j].ID
			}
			return validated[i].EndMinute < validated[j].EndMinute
		}
		return validated[i].StartMinute < validated[j].StartMinute
	})

	intervals := make([]scheduleInterval, 0, len(validated)*2)
	for _, block := range validated {
		for _, interval := range scheduleIntervals(block) {
			for _, existing := range intervals {
				if intervalsOverlap(existing, interval) {
					return nil, fmt.Errorf("schedule block %q overlaps with %q", existing.blockID, block.ID)
				}
			}
			intervals = append(intervals, interval)
		}
	}

	return validated, nil
}

const (
	minutesPerDay  = 24 * 60
	minutesPerWeek = 7 * minutesPerDay
)

type scheduleInterval struct {
	blockID string
	start   int
	end     int
}

func normalizeWeekdays(weekdays []int) ([]int, error) {
	if len(weekdays) == 0 {
		return nil, nil
	}

	seen := make(map[int]struct{}, len(weekdays))
	normalized := make([]int, 0, len(weekdays))
	for _, weekday := range weekdays {
		if weekday < 0 || weekday > 6 {
			return nil, fmt.Errorf("weekday %d is out of range", weekday)
		}
		if _, exists := seen[weekday]; exists {
			continue
		}
		seen[weekday] = struct{}{}
		normalized = append(normalized, weekday)
	}
	sort.Ints(normalized)
	return normalized, nil
}

func scheduleIntervals(block domain.ScheduleBlock) []scheduleInterval {
	intervals := make([]scheduleInterval, 0, len(block.Weekdays)*2)
	duration := scheduleBlockDuration(block)
	for _, weekday := range block.Weekdays {
		start := weekday*minutesPerDay + block.StartMinute
		end := start + duration
		if end <= minutesPerWeek {
			intervals = append(intervals, scheduleInterval{blockID: block.ID, start: start, end: end})
			continue
		}
		intervals = append(intervals, scheduleInterval{blockID: block.ID, start: start, end: minutesPerWeek})
		intervals = append(intervals, scheduleInterval{blockID: block.ID, start: 0, end: end - minutesPerWeek})
	}
	sort.Slice(intervals, func(i, j int) bool {
		if intervals[i].start == intervals[j].start {
			if intervals[i].end == intervals[j].end {
				return intervals[i].blockID < intervals[j].blockID
			}
			return intervals[i].end < intervals[j].end
		}
		return intervals[i].start < intervals[j].start
	})
	return intervals
}

func intervalsOverlap(left, right scheduleInterval) bool {
	if left.blockID == right.blockID {
		return false
	}
	return left.start < right.end && right.start < left.end
}

func scheduleBlockDuration(block domain.ScheduleBlock) int {
	duration := block.EndMinute - block.StartMinute
	if duration > 0 {
		return duration
	}
	return minutesPerDay - block.StartMinute + block.EndMinute
}

func (s *Service) validateTrackIDs(ctx context.Context, trackIDs []string) error {
	seen := make(map[string]struct{}, len(trackIDs))
	for _, trackID := range trackIDs {
		trackID = strings.TrimSpace(trackID)
		if trackID == "" {
			return errors.New("track id cannot be empty")
		}
		if _, exists := seen[trackID]; exists {
			return fmt.Errorf("contains duplicate track %q", trackID)
		}
		track, err := s.source.GetTrack(ctx, trackID)
		if err != nil {
			return err
		}
		if track.ID == "" {
			return errors.New("track not found")
		}
		seen[trackID] = struct{}{}
	}
	return nil
}

func (s *Service) activeTrackIDsAt(state store.ChannelState, at time.Time) ([]string, string) {
	block, occurrenceStart, _, ok := activeScheduleBlock(state, at)
	if !ok {
		return state.PlaylistTrackIDs, ""
	}
	return scheduleTrackOrder(*block, occurrenceStart), block.ID
}

func startOfWeek(at time.Time) time.Time {
	local := at.In(scheduleTimeZone)
	offset := int(local.Weekday())
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, scheduleTimeZone)
	return start.AddDate(0, 0, -offset)
}

func blockOccurrenceAt(block domain.ScheduleBlock, at time.Time) (time.Time, time.Time, bool) {
	localAt := at.In(scheduleTimeZone)
	weekStart := startOfWeek(localAt)
	weeks := []time.Time{weekStart, weekStart.AddDate(0, 0, -7)}
	for _, candidateWeekStart := range weeks {
		for _, weekday := range block.Weekdays {
			start := candidateWeekStart.AddDate(0, 0, weekday).Add(time.Duration(block.StartMinute) * time.Minute)
			end := start.Add(time.Duration(scheduleBlockDuration(block)) * time.Minute)
			if !localAt.Before(start) && localAt.Before(end) {
				return start.UTC(), end.UTC(), true
			}
		}
	}
	return time.Time{}, time.Time{}, false
}

func activeScheduleBlock(state store.ChannelState, at time.Time) (*domain.ScheduleBlock, time.Time, time.Time, bool) {
	var selected *domain.ScheduleBlock
	var selectedStart time.Time
	var selectedEnd time.Time
	for index := range state.ScheduleBlocks {
		block := &state.ScheduleBlocks[index]
		start, end, ok := blockOccurrenceAt(*block, at)
		if !ok {
			continue
		}
		if selected == nil || start.After(selectedStart) || (start.Equal(selectedStart) && end.Before(selectedEnd)) || (start.Equal(selectedStart) && end.Equal(selectedEnd) && block.ID > selected.ID) {
			selected = block
			selectedStart = start
			selectedEnd = end
		}
	}
	return selected, selectedStart, selectedEnd, selected != nil
}

func scheduleTrackOrder(block domain.ScheduleBlock, occurrenceStart time.Time) []string {
	order := append([]string(nil), block.TrackIDs...)
	if len(order) <= 1 || !block.ShuffleOnRun {
		return order
	}

	seed := fnv.New64a()
	_, _ = seed.Write([]byte(block.ID))
	_, _ = seed.Write([]byte("|"))
	_, _ = seed.Write([]byte(occurrenceStart.UTC().Format(time.RFC3339Nano)))
	rng := rand.New(rand.NewPCG(seed.Sum64(), seed.Sum64()^0x9e3779b97f4a7c15))
	rng.Shuffle(len(order), func(i, j int) {
		order[i], order[j] = order[j], order[i]
	})
	return order
}

func (s *Service) currentScheduleTrackAt(ctx context.Context, state store.ChannelState, at time.Time) (domain.ScheduleBlock, []string, string, time.Time, bool, error) {
	block, occurrenceStart, _, ok := activeScheduleBlock(state, at)
	if !ok {
		return domain.ScheduleBlock{}, nil, "", time.Time{}, false, nil
	}

	order := scheduleTrackOrder(*block, occurrenceStart)
	if len(order) == 0 {
		return *block, order, "", time.Time{}, false, nil
	}

	totalDuration := time.Duration(0)
	durations := make([]time.Duration, len(order))
	for index, trackID := range order {
		track, err := s.source.GetTrack(ctx, trackID)
		if err != nil {
			return domain.ScheduleBlock{}, nil, "", time.Time{}, false, err
		}
		if track.DurationMs <= 0 {
			return domain.ScheduleBlock{}, nil, "", time.Time{}, false, fmt.Errorf("schedule block %q has invalid track %q duration", block.ID, trackID)
		}
		durations[index] = time.Duration(track.DurationMs) * time.Millisecond
		totalDuration += durations[index]
	}
	if totalDuration <= 0 {
		return domain.ScheduleBlock{}, nil, "", time.Time{}, false, fmt.Errorf("schedule block %q has no playable duration", block.ID)
	}

	elapsed := at.Sub(occurrenceStart)
	if elapsed < 0 {
		return *block, order, "", time.Time{}, false, nil
	}
	if !block.Loop && elapsed >= totalDuration {
		return *block, order, "", time.Time{}, false, nil
	}
	if block.Loop {
		elapsed %= totalDuration
	}

	startedAt := occurrenceStart.UTC()
	for index, duration := range durations {
		if elapsed < duration {
			return *block, order, order[index], startedAt, true, nil
		}
		elapsed -= duration
		startedAt = startedAt.Add(duration)
	}

	return *block, order, "", time.Time{}, false, nil
}

func nextScheduleTrack(order []string, currentTrackID string, loop bool) (string, bool) {
	if len(order) == 0 {
		return "", false
	}
	index := indexOfTrack(order, currentTrackID)
	if index == -1 {
		return order[0], true
	}
	nextIndex := index + 1
	if nextIndex < len(order) {
		return order[nextIndex], true
	}
	if loop {
		return order[0], true
	}
	return "", false
}

func nextTrackAfter(trackIDs []string, currentTrackID string, cursor int, reset bool) (string, int) {
	if len(trackIDs) == 0 {
		return "", cursor
	}
	if index := indexOfTrack(trackIDs, currentTrackID); index >= 0 {
		nextIndex := (index + 1) % len(trackIDs)
		return trackIDs[nextIndex], nextIndex
	}
	if reset || cursor < 0 || cursor >= len(trackIDs) {
		return trackIDs[0], 0
	}
	nextIndex := (cursor + 1) % len(trackIDs)
	return trackIDs[nextIndex], nextIndex
}

func (s *Service) resolvePlaylistTrackIDs(ctx context.Context, trackIDs []string) ([]string, error) {
	if len(trackIDs) == 0 {
		return nil, nil
	}

	seen := make(map[string]struct{}, len(trackIDs))
	resolved := make([]string, 0, len(trackIDs))
	for _, trackID := range trackIDs {
		trackID = strings.TrimSpace(trackID)
		if trackID == "" {
			return nil, errors.New("playlist track id cannot be empty")
		}
		if _, exists := seen[trackID]; exists {
			return nil, fmt.Errorf("playlist contains duplicate track %q", trackID)
		}
		track, err := s.source.GetTrack(ctx, trackID)
		if err != nil {
			return nil, err
		}
		if track.ID == "" {
			return nil, errors.New("track not found")
		}
		seen[trackID] = struct{}{}
		resolved = append(resolved, trackID)
	}
	return resolved, nil
}

func normalizeChannelID(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	var builder strings.Builder
	lastDash := false
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash && builder.Len() > 0 {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(builder.String(), "-")
}

func (s *Service) Enqueue(ctx context.Context, channelID, trackID string) (domain.QueueItem, error) {
	state, err := s.store.GetChannelState(ctx, channelID)
	if err != nil {
		return domain.QueueItem{}, err
	}
	if !state.Channel.Enabled {
		return domain.QueueItem{}, errors.New("channel is disabled")
	}
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

func (s *Service) ResolvePlayable(ctx context.Context, channelID, trackID string) (source.Playable, error) {
	state, err := s.store.GetChannelState(ctx, channelID)
	if err != nil {
		return source.Playable{}, err
	}

	found := false
	if state.Channel.CurrentTrackID == trackID {
		found = true
	}
	for _, playlistTrackID := range state.PlaylistTrackIDs {
		if playlistTrackID == trackID {
			found = true
			break
		}
	}
	if !found {
		for _, item := range state.Queue {
			if item.TrackID == trackID {
				found = true
				break
			}
		}
	}
	if !found {
		for _, block := range state.ScheduleBlocks {
			for _, blockTrackID := range block.TrackIDs {
				if blockTrackID == trackID {
					found = true
					break
				}
			}
			if found {
				break
			}
		}
	}
	if !found {
		return source.Playable{}, errors.New("track is not attached to channel")
	}

	return s.source.ResolvePlayable(ctx, trackID)
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
	if !state.Channel.Enabled {
		return domain.PlayheadState{}, errors.New("channel is disabled")
	}

	block, order, scheduleTrackID, _, scheduleActive, err := s.currentScheduleTrackAt(ctx, state, s.clock())
	if err != nil {
		return domain.PlayheadState{}, err
	}
	if len(state.Queue) == 0 && len(state.PlaylistTrackIDs) == 0 && !scheduleActive {
		return domain.PlayheadState{}, errors.New("channel playlist is empty")
	}

	var nextID string
	queue := state.Queue
	if len(state.Queue) > 0 {
		nextID = state.Queue[0].TrackID
		queue = append([]domain.QueueItem(nil), state.Queue[1:]...)
		state.Channel.CurrentScheduleBlockID = ""
	} else {
		if scheduleActive {
			nextID, _ = nextScheduleTrack(order, scheduleTrackID, block.Loop)
			if nextID != "" {
				state.Channel.CurrentScheduleBlockID = block.ID
			}
		}
		if nextID == "" {
			nextID, _ = nextTrackAfter(state.PlaylistTrackIDs, state.Channel.CurrentTrackID, state.Channel.PlaylistCursor, false)
			state.Channel.CurrentScheduleBlockID = ""
		}
	}
	if nextID == "" {
		return domain.PlayheadState{}, errors.New("no next track available")
	}

	nextTrack, err := s.source.GetTrack(ctx, nextID)
	if err != nil {
		return domain.PlayheadState{}, err
	}
	if nextTrack.DurationMs <= 0 {
		return domain.PlayheadState{}, errors.New("next track has invalid duration")
	}

	now := s.clock()
	state.Channel.CurrentTrackID = nextID
	state.Channel.StartedAt = now
	state.Channel.PlaylistCursor = 0
	state.Queue = queue

	if err := s.store.UpsertChannelState(ctx, state); err != nil {
		return domain.PlayheadState{}, err
	}
	log.Printf("event=playback.skip channel_id=%s track_id=%s queued_remaining=%d", channelID, nextTrack.ID, len(state.Queue))

	return domain.PlayheadState{
		ChannelID:         channelID,
		TrackID:           nextTrack.ID,
		Title:             nextTrack.Title,
		Artist:            nextTrack.Artist,
		DurationMs:        nextTrack.DurationMs,
		ElapsedMs:         0,
		StartedAt:         now,
		SourceType:        nextTrack.SourceType,
		ArtworkURL:        nextTrack.ArtworkURL,
		ScheduleBlockName: block.Name,
		ScheduleBlockID:   block.ID,
	}, nil
}

func (s *Service) CurrentNow(ctx context.Context, channelID string) (domain.PlayheadState, error) {
	return s.Current(ctx, channelID, s.clock())
}

func (s *Service) CurrentSnapshot(ctx context.Context, channelID string) (domain.PlayheadState, error) {
	return s.currentAt(ctx, channelID, s.clock(), false)
}

func (s *Service) State(ctx context.Context, channelID string) (domain.ChannelStateSnapshot, error) {
	state, err := s.store.GetChannelState(ctx, channelID)
	if err != nil {
		return domain.ChannelStateSnapshot{}, err
	}

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
		ChannelID:   channelID,
		ChannelName: state.Channel.Name,
		NowPlaying:  nowPlaying,
		Queue:       queue,
		NextTrack:   next,
	}, nil
}

func (s *Service) StateSnapshot(ctx context.Context, channelID string) (domain.ChannelStateSnapshot, error) {
	state, err := s.store.GetChannelState(ctx, channelID)
	if err != nil {
		return domain.ChannelStateSnapshot{}, err
	}

	nowPlaying, err := s.CurrentSnapshot(ctx, channelID)
	if err != nil {
		return domain.ChannelStateSnapshot{}, err
	}

	queue, err := s.Queue(ctx, channelID)
	if err != nil {
		return domain.ChannelStateSnapshot{}, err
	}

	nextTrack, err := s.NextSnapshot(ctx, channelID, nowPlaying.TrackID)
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
		ChannelID:   channelID,
		ChannelName: state.Channel.Name,
		NowPlaying:  nowPlaying,
		Queue:       queue,
		NextTrack:   next,
	}, nil
}

func (s *Service) Current(ctx context.Context, channelID string, at time.Time) (domain.PlayheadState, error) {
	return s.currentAt(ctx, channelID, at, true)
}

func (s *Service) currentAt(ctx context.Context, channelID string, at time.Time, persist bool) (domain.PlayheadState, error) {
	state, err := s.store.GetChannelState(ctx, channelID)
	if err != nil {
		return domain.PlayheadState{}, err
	}
	if !state.Channel.Enabled {
		return domain.PlayheadState{}, errors.New("channel is disabled")
	}
	if state.Channel.StartedAt.IsZero() {
		state.Channel.StartedAt = at.UTC()
	}

	if block, _, scheduleTrackID, scheduleStartedAt, active, err := s.currentScheduleTrackAt(ctx, state, at); err != nil {
		return domain.PlayheadState{}, err
	} else if active {
		currentTrack, err := s.source.GetTrack(ctx, scheduleTrackID)
		if err != nil {
			return domain.PlayheadState{}, err
		}

		changed := state.Channel.CurrentTrackID != scheduleTrackID ||
			!state.Channel.StartedAt.Equal(scheduleStartedAt) ||
			state.Channel.CurrentScheduleBlockID != block.ID
		state.Channel.CurrentTrackID = scheduleTrackID
		state.Channel.StartedAt = scheduleStartedAt
		state.Channel.CurrentScheduleBlockID = block.ID
		if persist && changed {
			if err := s.store.UpsertChannelState(ctx, state); err != nil {
				return domain.PlayheadState{}, err
			}
		}

		elapsed := at.Sub(scheduleStartedAt).Milliseconds()
		if elapsed < 0 {
			elapsed = 0
		}
		return domain.PlayheadState{
			ChannelID:         channelID,
			TrackID:           currentTrack.ID,
			Title:             currentTrack.Title,
			Artist:            currentTrack.Artist,
			DurationMs:        currentTrack.DurationMs,
			ElapsedMs:         elapsed,
			StartedAt:         scheduleStartedAt,
			SourceType:        currentTrack.SourceType,
			ArtworkURL:        currentTrack.ArtworkURL,
			ScheduleBlockName: block.Name,
			ScheduleBlockID:   block.ID,
		}, nil
	}

	stateDirty := state.Channel.CurrentScheduleBlockID != ""
	if stateDirty {
		state.Channel.CurrentScheduleBlockID = ""
	}

	sequence := state.PlaylistTrackIDs
	if state.Channel.CurrentTrackID == "" {
		if len(sequence) == 0 {
			return domain.PlayheadState{}, errors.New("channel playlist is empty")
		}
		state.Channel.CurrentTrackID = sequence[0]
		state.Channel.PlaylistCursor = 0
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

	for !trackEndedAt(currentStart, currentTrack.DurationMs).After(at) {
		nextID, nextCursor, queue, nextBlockID, err := s.pickNext(ctx, state, trackEndedAt(currentStart, currentTrack.DurationMs))
		if err != nil {
			return domain.PlayheadState{}, err
		}
		if nextID == "" {
			break
		}

		currentStart = trackEndedAt(currentStart, currentTrack.DurationMs)
		state.Channel.StartedAt = currentStart
		state.Channel.CurrentTrackID = nextID
		state.Channel.PlaylistCursor = nextCursor
		state.Channel.CurrentScheduleBlockID = nextBlockID
		state.Queue = queue
		changed = true

		currentTrack, err = s.source.GetTrack(ctx, nextID)
		if err != nil {
			return domain.PlayheadState{}, err
		}
		if currentTrack.DurationMs <= 0 {
			return domain.PlayheadState{}, errors.New("next track has invalid duration")
		}
	}

	if changed {
		if persist {
			if err := s.store.UpsertChannelState(ctx, state); err != nil {
				return domain.PlayheadState{}, err
			}
		}
		log.Printf("event=scheduler.advance channel_id=%s track_id=%s started_at=%s queued_remaining=%d", channelID, currentTrack.ID, state.Channel.StartedAt.UTC().Format(time.RFC3339), len(state.Queue))
	} else if stateDirty && persist {
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
		ArtworkURL: currentTrack.ArtworkURL,
	}, nil
}

func (s *Service) Next(ctx context.Context, channelID, afterTrackID string) (*domain.Track, error) {
	playhead, err := s.CurrentNow(ctx, channelID)
	if err != nil {
		return nil, err
	}

	state, err := s.store.GetChannelState(ctx, channelID)
	if err != nil {
		return nil, err
	}
	if !state.Channel.Enabled {
		return nil, errors.New("channel is disabled")
	}

	if len(state.Queue) > 0 {
		track, err := s.source.GetTrack(ctx, state.Queue[0].TrackID)
		if err != nil {
			return nil, err
		}
		return &track, nil
	}

	if block, order, scheduleTrackID, _, scheduleActive, err := s.currentScheduleTrackAt(ctx, state, s.clock()); err != nil {
		return nil, err
	} else if scheduleActive {
		if afterTrackID == "" {
			afterTrackID = scheduleTrackID
		}
		if nextID, ok := nextScheduleTrack(order, afterTrackID, block.Loop); ok {
			track, err := s.source.GetTrack(ctx, nextID)
			if err != nil {
				return nil, err
			}
			return &track, nil
		}
	}

	currentTrack, err := s.source.GetTrack(ctx, playhead.TrackID)
	if err != nil {
		return nil, err
	}
	if currentTrack.DurationMs <= 0 {
		return nil, errors.New("current track has invalid duration")
	}

	nextTrackAt := state.Channel.StartedAt.UTC().Add(time.Duration(currentTrack.DurationMs) * time.Millisecond)
	sequence, activeBlockID := s.activeTrackIDsAt(state, nextTrackAt)
	if len(sequence) == 0 {
		return nil, errors.New("channel playlist is empty")
	}

	if afterTrackID == "" {
		afterTrackID = playhead.TrackID
	}
	reset := activeBlockID != state.Channel.CurrentScheduleBlockID
	nextID, _ := nextTrackAfter(sequence, afterTrackID, state.Channel.PlaylistCursor, reset)
	if nextID == "" {
		return nil, errors.New("no next track available")
	}

	track, err := s.source.GetTrack(ctx, nextID)
	if err != nil {
		return nil, err
	}
	return &track, nil
}

func (s *Service) NextSnapshot(ctx context.Context, channelID, afterTrackID string) (*domain.Track, error) {
	playhead, err := s.CurrentSnapshot(ctx, channelID)
	if err != nil {
		return nil, err
	}

	state, err := s.store.GetChannelState(ctx, channelID)
	if err != nil {
		return nil, err
	}
	if !state.Channel.Enabled {
		return nil, errors.New("channel is disabled")
	}

	if len(state.Queue) > 0 {
		track, err := s.source.GetTrack(ctx, state.Queue[0].TrackID)
		if err != nil {
			return nil, err
		}
		return &track, nil
	}

	if block, order, scheduleTrackID, _, scheduleActive, err := s.currentScheduleTrackAt(ctx, state, s.clock()); err != nil {
		return nil, err
	} else if scheduleActive {
		if afterTrackID == "" {
			afterTrackID = scheduleTrackID
		}
		if nextID, ok := nextScheduleTrack(order, afterTrackID, block.Loop); ok {
			track, err := s.source.GetTrack(ctx, nextID)
			if err != nil {
				return nil, err
			}
			return &track, nil
		}
	}

	currentTrack, err := s.source.GetTrack(ctx, playhead.TrackID)
	if err != nil {
		return nil, err
	}
	if currentTrack.DurationMs <= 0 {
		return nil, errors.New("current track has invalid duration")
	}

	nextTrackAt := state.Channel.StartedAt.UTC().Add(time.Duration(currentTrack.DurationMs) * time.Millisecond)
	sequence, activeBlockID := s.activeTrackIDsAt(state, nextTrackAt)
	if len(sequence) == 0 {
		return nil, errors.New("channel playlist is empty")
	}

	if afterTrackID == "" {
		afterTrackID = playhead.TrackID
	}
	reset := activeBlockID != state.Channel.CurrentScheduleBlockID
	nextID, _ := nextTrackAfter(sequence, afterTrackID, state.Channel.PlaylistCursor, reset)
	if nextID == "" {
		return nil, errors.New("no next track available")
	}

	track, err := s.source.GetTrack(ctx, nextID)
	if err != nil {
		return nil, err
	}
	return &track, nil
}

func (s *Service) pickNext(ctx context.Context, state store.ChannelState, at time.Time) (string, int, []domain.QueueItem, string, error) {
	if len(state.Queue) > 0 {
		item := state.Queue[0]
		block, _, _, _, active, err := s.currentScheduleTrackAt(ctx, state, at)
		if err != nil {
			return "", 0, nil, "", err
		}
		activeBlockID := ""
		if active {
			activeBlockID = block.ID
		}
		return item.TrackID, state.Channel.PlaylistCursor, append([]domain.QueueItem(nil), state.Queue[1:]...), activeBlockID, nil
	}
	if block, order, scheduleTrackID, _, scheduleActive, err := s.currentScheduleTrackAt(ctx, state, at); err == nil && scheduleActive {
		if nextID, ok := nextScheduleTrack(order, scheduleTrackID, block.Loop); ok {
			return nextID, indexOfTrack(order, nextID), state.Queue, block.ID, nil
		}
	}

	sequence, activeBlockID := state.PlaylistTrackIDs, ""
	if len(sequence) == 0 {
		return "", state.Channel.PlaylistCursor, state.Queue, activeBlockID, nil
	}

	reset := activeBlockID != state.Channel.CurrentScheduleBlockID
	nextID, nextCursor := nextTrackAfter(sequence, state.Channel.CurrentTrackID, state.Channel.PlaylistCursor, reset)
	return nextID, nextCursor, state.Queue, activeBlockID, nil
}

func indexOfTrack(trackIDs []string, trackID string) int {
	for index, candidate := range trackIDs {
		if candidate == trackID {
			return index
		}
	}
	return -1
}

func trackEndedAt(start time.Time, durationMs int64) time.Time {
	return start.Add(time.Duration(durationMs) * time.Millisecond)
}
