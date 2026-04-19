# Atlas Audio Engine

🚧 **Work in progress**

Atlas Audio Engine is a modern radio automation platform designed to run always-on music channels with synchronized visual metadata.

Inspired by the **dizqueTV** architecture, Atlas Audio Engine brings live-channel concepts to music broadcasting with multi-source ingestion, smart scheduling, and real-time display overlays.

---

## Vision

Atlas Audio Engine aims to create a true "continuous station" experience:

- 🎵 **Audio playback from multiple sources**
  - Local media libraries
  - Jellyfin
  - etc.
- 🎨 **Real-time visual output**
  - Album artwork
  - Artist and track metadata overlays
- 📻 **24/7 channel-style programming**
  - Music playlists behave like live broadcast channels
- 🎲 **Flexible scheduling modes**
  - Automatic/randomized scheduling
  - Manual/curator-defined sequencing
- 🎚️ **Live control features**
  - Queue jumps
  - Manual override during broadcast

---

## Core Features (Planned)

### 1. Multi-Source Music Ingestion
- Unified track model across streaming and local sources
- Metadata normalization (artist, album, title, duration, artwork)
- Source priority and fallback behavior

### 2. Channel-Based Playback Engine
- Continuous channel timelines (similar to linear TV channels)
- Persistent "what should be playing now" state
- Seamless transition between tracks and scheduled blocks

### 3. Intelligent Scheduler
- Rule-based and random block generation
- Time-slot programming (genre, mood, era, curator themes)
- Recurrence patterns (daily, weekly, event-based)

### 4. Broadcast Visual Layer
- Dynamic rendering of now-playing information
- Artwork and branding overlays
- Output suitable for livestream platforms (e.g., RTMP pipelines)

### 5. Live Operations Console
- Monitor active channel state
- Force-next / skip / inject track
- Emergency fallback playlist control

---

## Architecture Direction

Atlas Audio Engine follows a modular architecture:

- **Source Connectors**: adapters for local media libraries, Jellyfin, and additional providers
- **Metadata Service**: canonical metadata + enrichment
- **Scheduling Service**: channel timeline generation and update logic
- **Playback Orchestrator**: resolves current playhead and emits playback instructions
- **Visual Composer**: generates synchronized video/overlay output
- **Control API/UI**: tools for operators and curators

---

## Use Cases

- Internet radio stations with branded visual streams
- Community or hobby broadcasters wanting TV-like music channels
- Curator-driven channels with occasional automation
- Hands-off ambient channels with intelligent randomization

---

## Status

🚧 **Early project stage** — architecture and implementation roadmap are in progress.

If you want to contribute, start by proposing:
- source adapter interfaces,
- scheduler rule schemas,
- playback state models,
- overlay rendering approaches.

---

## Roadmap (High-Level)

- [ ] Define core domain model (Track, Channel, ScheduleBlock, Playhead)
- [ ] Implement local-file source adapter
- [ ] Implement first scheduler prototype
- [ ] Implement now-playing state service
- [ ] Add overlay renderer for artwork + metadata
- [ ] Add operator controls for manual override

---

## Inspiration

This project is conceptually inspired by **dizqueTV** and its channel/scheduling paradigm, adapted for a music-first radio automation workflow.

---

## License

This project is licensed under the terms described in [LICENSE](./LICENSE).
