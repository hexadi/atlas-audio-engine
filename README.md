# Atlas Audio Engine

Atlas Audio Engine is a Go-based radio automation backend for always-on music channels with deterministic playhead state and operator queue control.

This first implementation focuses on a thin vertical slice:

- local-file ingestion
- canonical track/channel/playhead models
- a deterministic scheduler with queue priority at track boundaries
- an HTTP API for health, channels, now-playing, and queue control
- a public listener homepage for now playing, progress, next song, and browser playback
- an authenticated operator dashboard for skip control, queue visibility, queue adds/removes/reordering, persisted playlist edits, and broadcast tools

## Current MVP

The current backend proves the core loop:

`local media -> normalized tracks -> channel playlist -> scheduler -> playhead state -> HTTP API`

Phase 1 intentionally does not include:

- Jellyfin or Spotify integrations
- RTMP output
- external operator identity providers

## Project Layout

```text
cmd/atlas-audio-engine     application entrypoint
internal/api               Echo HTTP handlers
internal/bootstrap         startup seeding for the first local channel
internal/config            environment-driven configuration
internal/domain            core types
internal/scheduler         playhead and next-track logic
internal/source            source interfaces
internal/source/localfiles local-media adapter and ffprobe probing
internal/store             storage interfaces
internal/store/sqlite      SQLite-backed persistence
internal/store/memory      in-memory test store
```

## Configuration

Set these environment variables as needed:

- `ATLAS_LISTEN_ADDR` default `:8080`
- `ATLAS_DATABASE_PATH` default `atlas.db`
- `ATLAS_MEDIA_DIR` default `./testdata/media`
- `ATLAS_CHANNEL_ID` default `local-library`
- `ATLAS_CHANNEL_NAME` default `Local Library`
- `ATLAS_FFMPEG_PATH` default `ffmpeg`
- `ATLAS_VIDEO_FONT_PATH` optional `.ttf` or `.otf` file for `stream.ts` text rendering
- `ATLAS_DASHBOARD_USERNAME` default `admin`
- `ATLAS_DASHBOARD_PASSWORD` default `atlas`

`ffprobe` must be available on the system path because the local source adapter uses it to read duration and tags.
If a repo-local `.env` file exists, the app loads it automatically before applying defaults. Existing shell environment variables still take precedence.

## API

- `GET /` public homepage with now playing, audio player, progress bar, and next song
- `GET /dashboard` authenticated operator dashboard with channel create/switch, queue, playlist, skip, library, and broadcast controls
- `GET /visual` browser-source visual output with cover art, title, artist, next track, and progress
- `GET /artwork/:trackId` cover image for local tracks that have `cover.jpg` beside the audio file
- `GET /health`
- `GET /channels`
- `POST /channels`
- `PATCH /channels/:id`
- `DELETE /channels/:id`
- `GET /channels/:id/library`
- `GET /channels/:id/playlist`
- `PUT /channels/:id/playlist`
- `POST /channels/:id/playlist/shuffle`
- `GET /channels/:id/schedule-blocks`
- `PUT /channels/:id/schedule-blocks`
- `GET /channels/:id/tracks`
- `GET /channels/:id/tracks/:trackId/audio`
- `GET /channels/:id/stream.m3u8`
- `GET /channels/:id/stream.mp3`
- `GET /channels/:id/stream.pcm`
- `GET /channels/:id/stream.ts`
- `GET /channels/:id/broadcast.ts`
- `GET /channels/:id/broadcast/status`
- `GET /channels/:id/state`
- `GET /channels/:id/ws`
- `GET /channels/:id/now-playing`
- `GET /channels/:id/queue`
- `POST /channels/:id/queue`
- `DELETE /channels/:id/queue/:queueItemId`
- `POST /channels/:id/queue/:queueItemId/move`
- `POST /channels/:id/skip`

`GET /channels/:id/queue` returns enriched queue entries with track metadata and queue position, not just raw track ids.
`POST /channels` creates a new local-library-backed channel. If `track_ids` is omitted, the channel starts with an empty playlist so operators can build it from the dashboard library.
`PATCH /channels/:id` updates channel metadata such as `name` or `enabled`; disabled channels remain visible in the dashboard but cannot play.
`DELETE /channels/:id` deletes a channel plus its playlist and queue. The API refuses to delete the last remaining channel.
`POST /channels/:id/playlist/shuffle` shuffles the selected channel playlist, resets the playhead to the first shuffled track, and persists the new order.
`GET /channels/:id/state` returns a single operator snapshot with `now_playing`, `queue`, and `next_track`.
`GET /channels/:id/ws` upgrades to a WebSocket and streams the same state snapshot for live now-playing updates.
`GET /channels/:id/tracks/:trackId/audio` serves local audio for tracks attached to that channel playlist, current playhead, or queue.
`GET /channels/:id/stream.m3u8` returns an initial HLS proof-of-concept manifest for the current local track.
`GET /channels/:id/stream.mp3` uses FFmpeg to transcode a continuous browser-friendly MP3 station stream, advancing through tracks as the scheduler moves the playhead. The response is flushed in small chunks and paced in realtime so browsers can listen continuously instead of downloading whole songs at once.
`GET /channels/:id/stream.pcm` exposes the same continuous station audio as raw 48kHz stereo PCM for internal broadcast composition.
`GET /channels/:id/stream.ts` uses FFmpeg to compose the current audio with a live visual layer based on the same now-playing data as `/visual`: cover art, title, artist, and next track. It returns an MPEG-TS stream suitable for players such as VLC, ffplay, or OBS media sources.
`GET /channels/:id/broadcast.ts` keeps one FFmpeg video encoder alive while Go continuously renders visual frames with current artwork and progress. FFmpeg overlays reloaded title, artist, next track, and clock text while consuming the channel's continuous PCM stream. Use this for smoother broadcast-style video output without per-track video encoder restarts.
`GET /channels/:id/broadcast/status` returns the recommended broadcast URL, encoder mode, video shape, audio feed format, and current broadcast metadata.

Example queue request:

```json
{
  "track_id": "your-track-id"
}
```

Example channel create request:

```json
{
  "name": "Second Channel",
  "track_ids": ["track-id-1", "track-id-2"]
}
```

## Running

```bash
go mod tidy
go test ./...
go run ./cmd/atlas-audio-engine
```

For local development on Windows PowerShell, copy `.env.example` to `.env`, update `ATLAS_MEDIA_DIR`, and then run:

```powershell
go run ./cmd/atlas-audio-engine
```

Run the local smoke test after setting `ATLAS_MEDIA_DIR` or passing `-MediaDir`:

```powershell
.\scripts\smoke.ps1 -MediaDir 'C:\path\to\music'
```

Before starting the server, place supported audio files under the media directory referenced by `ATLAS_MEDIA_DIR`.
Using `testdata/media` keeps Go tooling happy; a repo-root `media/` folder can interfere with `go test ./...` when it contains artist or album directories with characters that are invalid in Go import paths.

## Roadmap

- Add more source adapters
- Extend scheduler rules beyond a single seeded playlist
- Add richer schedule editor controls for block creation and editing
- Add broadcast output and visual composition
- Add live operator workflows and richer playback controls

## License

This project is licensed under the terms described in [LICENSE](./LICENSE).
