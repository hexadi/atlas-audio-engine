package api

const homePageHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Atlas Audio Engine</title>
  <style>
    :root {
      color-scheme: dark;
      --bg: #0b1020;
      --panel: rgba(18, 27, 52, 0.88);
      --panel-border: rgba(132, 154, 214, 0.24);
      --text: #eef4ff;
      --muted: #98a6c7;
      --accent: #6ed6ff;
      --accent-2: #7cffbf;
      --track: rgba(255, 255, 255, 0.12);
    }

    * { box-sizing: border-box; }

    body {
      margin: 0;
      min-height: 100vh;
      font-family: "Segoe UI", "Aptos", sans-serif;
      background:
        radial-gradient(circle at top left, rgba(110, 214, 255, 0.18), transparent 30%),
        radial-gradient(circle at bottom right, rgba(124, 255, 191, 0.14), transparent 32%),
        linear-gradient(160deg, #07101f 0%, #0d1730 52%, #08111f 100%);
      color: var(--text);
      display: grid;
      place-items: center;
      padding: 24px;
    }

    .shell {
      width: min(720px, 100%);
      background: var(--panel);
      border: 1px solid var(--panel-border);
      border-radius: 24px;
      box-shadow: 0 24px 80px rgba(0, 0, 0, 0.35);
      overflow: hidden;
      backdrop-filter: blur(20px);
    }

    .hero {
      display: grid;
      grid-template-columns: 136px 1fr;
      gap: 22px;
      align-items: center;
      padding: 28px 28px 20px;
      border-bottom: 1px solid rgba(255, 255, 255, 0.08);
    }

    .cover {
      width: 136px;
      aspect-ratio: 1;
      border-radius: 20px;
      background:
        linear-gradient(135deg, rgba(110, 214, 255, 0.32), rgba(124, 255, 191, 0.18)),
        rgba(255, 255, 255, 0.06);
      border: 1px solid rgba(255, 255, 255, 0.1);
      box-shadow: 0 18px 42px rgba(0, 0, 0, 0.28);
      object-fit: cover;
      display: block;
    }

    .cover.is-hidden {
      display: none;
    }

    .fallback-cover {
      width: 136px;
      aspect-ratio: 1;
      border-radius: 20px;
      display: grid;
      place-items: center;
      color: rgba(255, 255, 255, 0.72);
      font-size: 42px;
      font-weight: 900;
      background:
        radial-gradient(circle at 35% 25%, rgba(255, 255, 255, 0.22), transparent 34%),
        linear-gradient(135deg, rgba(110, 214, 255, 0.34), rgba(124, 255, 191, 0.18));
      border: 1px solid rgba(255, 255, 255, 0.1);
      box-shadow: 0 18px 42px rgba(0, 0, 0, 0.28);
    }

    .fallback-cover.is-hidden {
      display: none;
    }

    .hero-copy {
      min-width: 0;
    }

    .eyebrow {
      margin: 0 0 10px;
      color: var(--accent);
      font-size: 12px;
      font-weight: 700;
      letter-spacing: 0.14em;
      text-transform: uppercase;
    }

    .title {
      margin: 0;
      font-size: clamp(28px, 5vw, 48px);
      line-height: 1.05;
    }

    .artist {
      margin: 10px 0 0;
      font-size: clamp(18px, 3vw, 24px);
      color: var(--muted);
    }

    .meta {
      display: grid;
      gap: 18px;
      padding: 22px 28px 28px;
    }

    .progress-header,
    .next-row {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 12px;
    }

    .label {
      font-size: 12px;
      font-weight: 700;
      letter-spacing: 0.12em;
      text-transform: uppercase;
      color: var(--muted);
    }

    .time {
      font-variant-numeric: tabular-nums;
      color: var(--text);
      font-size: 14px;
    }

    .bar {
      width: 100%;
      height: 14px;
      background: var(--track);
      border-radius: 999px;
      overflow: hidden;
      border: 1px solid rgba(255, 255, 255, 0.05);
    }

    .bar-fill {
      height: 100%;
      width: 0%;
      background: linear-gradient(90deg, var(--accent), var(--accent-2));
      transition: width 0.35s ease;
    }

    .next-card {
      padding: 18px 20px;
      border-radius: 18px;
      background: rgba(255, 255, 255, 0.05);
      border: 1px solid rgba(255, 255, 255, 0.06);
    }

    .next-title {
      margin: 6px 0 4px;
      font-size: 22px;
    }

    .next-artist {
      margin: 0;
      color: var(--muted);
      font-size: 16px;
    }

    .status {
      padding: 14px 28px 22px;
      color: var(--muted);
      font-size: 14px;
    }

    .controls {
      display: flex;
      gap: 12px;
      flex-wrap: wrap;
    }

    button {
      appearance: none;
      border: 0;
      border-radius: 999px;
      padding: 12px 18px;
      color: #06111f;
      background: linear-gradient(90deg, var(--accent), var(--accent-2));
      font-weight: 800;
      cursor: pointer;
      box-shadow: 0 10px 28px rgba(110, 214, 255, 0.2);
    }

    button:disabled {
      cursor: wait;
      opacity: 0.62;
    }

    .queue-card {
      padding: 18px 20px;
      border-radius: 18px;
      background: rgba(0, 0, 0, 0.18);
      border: 1px solid rgba(255, 255, 255, 0.06);
    }

    .library-card,
    .playlist-card {
      padding: 18px 20px;
      border-radius: 18px;
      background: rgba(255, 255, 255, 0.04);
      border: 1px solid rgba(255, 255, 255, 0.06);
    }

    .queue-list,
    .track-list,
    .library-list {
      list-style: none;
      padding: 0;
      margin: 12px 0 0;
      display: grid;
      gap: 10px;
    }

    .queue-item,
    .track-item {
      display: grid;
      grid-template-columns: auto 1fr auto;
      gap: 10px;
      align-items: center;
      color: var(--text);
    }

    .queue-position {
      color: var(--accent-2);
      font-variant-numeric: tabular-nums;
      font-weight: 800;
    }

    .track-text {
      display: grid;
      gap: 2px;
    }

    .track-title {
      font-weight: 800;
    }

    .track-artist {
      color: var(--muted);
      font-size: 14px;
    }

    .secondary-button {
      padding: 9px 12px;
      background: rgba(255, 255, 255, 0.11);
      color: var(--text);
      box-shadow: none;
      border: 1px solid rgba(255, 255, 255, 0.1);
    }

    .button-row {
      display: flex;
      gap: 8px;
      flex-wrap: wrap;
      justify-content: flex-end;
    }

    .empty {
      color: var(--muted);
    }

    @media (max-width: 620px) {
      .hero {
        grid-template-columns: 1fr;
      }

      .cover,
      .fallback-cover {
        width: 112px;
      }
    }
  </style>
</head>
<body>
  <main class="shell">
    <section class="hero">
      <img id="cover-image" class="cover is-hidden" alt="Album artwork">
      <div id="fallback-cover" class="fallback-cover">AA</div>
      <div class="hero-copy">
        <p class="eyebrow">Now Playing</p>
        <h1 id="now-title" class="title">Loading...</h1>
        <p id="now-artist" class="artist">Connecting to channel</p>
      </div>
    </section>

    <section class="meta">
      <div>
        <div class="progress-header">
          <span class="label">Progress</span>
          <span id="time-display" class="time">0:00 / 0:00</span>
        </div>
        <div class="bar" aria-hidden="true">
          <div id="progress-fill" class="bar-fill"></div>
        </div>
      </div>

      <div class="next-card">
        <div class="next-row">
          <span class="label">Next Song</span>
          <span id="channel-id" class="time"></span>
        </div>
        <h2 id="next-title" class="next-title">Loading...</h2>
        <p id="next-artist" class="next-artist">Please wait</p>
      </div>

      <div class="controls">
        <button id="skip-button" type="button">Skip Track</button>
      </div>

      <div class="queue-card">
        <span class="label">Queue</span>
        <ul id="queue-list" class="queue-list">
          <li class="empty">No queued tracks</li>
        </ul>
      </div>

      <div class="playlist-card">
        <span class="label">Playlist Editor</span>
        <ul id="track-list" class="track-list">
          <li class="empty">Loading playlist...</li>
        </ul>
      </div>

      <div class="library-card">
        <span class="label">Library</span>
        <ul id="library-list" class="library-list">
          <li class="empty">Loading library...</li>
        </ul>
      </div>
    </section>

    <footer id="status" class="status">Waiting for state...</footer>
  </main>

  <script>
    const elements = {
      title: document.getElementById('now-title'),
      artist: document.getElementById('now-artist'),
      time: document.getElementById('time-display'),
      fill: document.getElementById('progress-fill'),
      nextTitle: document.getElementById('next-title'),
      nextArtist: document.getElementById('next-artist'),
      status: document.getElementById('status'),
      channelId: document.getElementById('channel-id'),
      skipButton: document.getElementById('skip-button'),
      queueList: document.getElementById('queue-list'),
      trackList: document.getElementById('track-list'),
      libraryList: document.getElementById('library-list'),
      coverImage: document.getElementById('cover-image'),
      fallbackCover: document.getElementById('fallback-cover'),
    };

    let snapshot = null;
    let channelId = null;
    let playlistTracks = [];
    let libraryTracks = [];
    let displayedNowPlaying = {
      artworkUrl: null,
      title: null,
      artist: null,
    };

    function formatTime(ms) {
      const totalSeconds = Math.max(0, Math.floor((ms || 0) / 1000));
      const minutes = Math.floor(totalSeconds / 60);
      const seconds = totalSeconds % 60;
      return minutes + ':' + String(seconds).padStart(2, '0');
    }

    function render() {
      if (!snapshot) {
        return;
      }

      const now = snapshot.now_playing || {};
      const startedAt = now.started_at ? new Date(now.started_at).getTime() : Date.now();
      const duration = now.duration_ms || 0;
      const liveElapsed = Math.max(0, Date.now() - startedAt);
      const elapsed = Math.min(duration, liveElapsed);
      const progress = duration > 0 ? (elapsed / duration) * 100 : 0;

      elements.time.textContent = formatTime(elapsed) + ' / ' + formatTime(duration);
      elements.fill.style.width = progress + '%';
      elements.channelId.textContent = snapshot.channel_id || '';

      renderNowPlayingIdentity(now);

      if (snapshot.next_track) {
        elements.nextTitle.textContent = snapshot.next_track.title || 'Unknown track';
        elements.nextArtist.textContent = snapshot.next_track.artist || 'Unknown artist';
      } else {
        elements.nextTitle.textContent = 'No next song';
        elements.nextArtist.textContent = 'Queue and playlist are empty';
      }

      const queueCount = Array.isArray(snapshot.queue) ? snapshot.queue.length : 0;
      elements.status.textContent = 'Channel ready. ' + queueCount + ' queued track' + (queueCount === 1 ? '' : 's') + '.';

      elements.queueList.innerHTML = '';
      if (queueCount === 0) {
        const item = document.createElement('li');
        item.className = 'empty';
        item.textContent = 'No queued tracks';
        elements.queueList.appendChild(item);
      } else {
        snapshot.queue.slice(0, 5).forEach((track) => {
          const item = document.createElement('li');
          item.className = 'queue-item';

          const position = document.createElement('span');
          position.className = 'queue-position';
          position.textContent = '#' + track.position;

          const label = document.createElement('span');
          label.textContent = (track.title || 'Unknown track') + ' - ' + (track.artist || 'Unknown artist');

          item.appendChild(position);
          item.appendChild(label);
          elements.queueList.appendChild(item);
        });
      }

      renderTracks();
    }

    function renderNowPlayingIdentity(now) {
      const title = now.title || 'Nothing playing';
      const artist = now.artist || 'No artist available';
      const artworkUrl = now.artwork_url || '';

      if (displayedNowPlaying.title !== title) {
        elements.title.textContent = title;
        displayedNowPlaying.title = title;
      }

      if (displayedNowPlaying.artist !== artist) {
        elements.artist.textContent = artist;
        displayedNowPlaying.artist = artist;
      }

      if (displayedNowPlaying.artworkUrl !== artworkUrl) {
        if (artworkUrl) {
          elements.coverImage.src = artworkUrl;
          elements.coverImage.classList.remove('is-hidden');
          elements.fallbackCover.classList.add('is-hidden');
        } else {
          elements.coverImage.removeAttribute('src');
          elements.coverImage.classList.add('is-hidden');
          elements.fallbackCover.classList.remove('is-hidden');
        }
        displayedNowPlaying.artworkUrl = artworkUrl;
      }
    }

    function renderTracks() {
      elements.trackList.innerHTML = '';

      if (!Array.isArray(playlistTracks) || playlistTracks.length === 0) {
        const item = document.createElement('li');
        item.className = 'empty';
        item.textContent = 'Playlist is empty';
        elements.trackList.appendChild(item);
      } else {
        playlistTracks.forEach((track, index) => {
          const item = document.createElement('li');
          item.className = 'track-item';

          const position = document.createElement('span');
          position.className = 'queue-position';
          position.textContent = String(index + 1).padStart(2, '0');

          const text = document.createElement('span');
          text.className = 'track-text';

          const title = document.createElement('span');
          title.className = 'track-title';
          title.textContent = track.title || 'Unknown track';

          const artist = document.createElement('span');
          artist.className = 'track-artist';
          artist.textContent = track.artist || 'Unknown artist';

          const actions = document.createElement('span');
          actions.className = 'button-row';

          const queueButton = document.createElement('button');
          queueButton.type = 'button';
          queueButton.className = 'secondary-button';
          queueButton.textContent = 'Queue';
          queueButton.addEventListener('click', () => addTrackToQueue(track.track_id || track.id, queueButton));

          const upButton = document.createElement('button');
          upButton.type = 'button';
          upButton.className = 'secondary-button';
          upButton.textContent = 'Up';
          upButton.disabled = index === 0;
          upButton.addEventListener('click', () => movePlaylistTrack(index, index - 1));

          const downButton = document.createElement('button');
          downButton.type = 'button';
          downButton.className = 'secondary-button';
          downButton.textContent = 'Down';
          downButton.disabled = index === playlistTracks.length - 1;
          downButton.addEventListener('click', () => movePlaylistTrack(index, index + 1));

          const removeButton = document.createElement('button');
          removeButton.type = 'button';
          removeButton.className = 'secondary-button';
          removeButton.textContent = 'Remove';
          removeButton.addEventListener('click', () => removePlaylistTrack(index));

          actions.appendChild(queueButton);
          actions.appendChild(upButton);
          actions.appendChild(downButton);
          actions.appendChild(removeButton);
          text.appendChild(title);
          text.appendChild(artist);
          item.appendChild(position);
          item.appendChild(text);
          item.appendChild(actions);
          elements.trackList.appendChild(item);
        });
      }

      elements.libraryList.innerHTML = '';
      if (!Array.isArray(libraryTracks) || libraryTracks.length === 0) {
        const item = document.createElement('li');
        item.className = 'empty';
        item.textContent = 'No library tracks loaded';
        elements.libraryList.appendChild(item);
        return;
      }

      const playlistIDs = new Set(playlistTracks.map((track) => track.track_id || track.id));
      libraryTracks.slice(0, 12).forEach((track, index) => {
        const item = document.createElement('li');
        item.className = 'track-item';

        const position = document.createElement('span');
        position.className = 'queue-position';
        position.textContent = String(index + 1).padStart(2, '0');

        const text = document.createElement('span');
        text.className = 'track-text';

        const title = document.createElement('span');
        title.className = 'track-title';
        title.textContent = track.title || 'Unknown track';

        const artist = document.createElement('span');
        artist.className = 'track-artist';
        artist.textContent = track.artist || 'Unknown artist';

        const button = document.createElement('button');
        button.type = 'button';
        button.className = 'secondary-button';
        button.textContent = playlistIDs.has(track.id) ? 'Added' : 'Add';
        button.disabled = playlistIDs.has(track.id);
        button.addEventListener('click', () => addPlaylistTrack(track.id, button));

        text.appendChild(title);
        text.appendChild(artist);
        item.appendChild(position);
        item.appendChild(text);
        item.appendChild(button);
        elements.libraryList.appendChild(item);
      });
    }

    async function loadChannelId() {
      const response = await fetch('/channels');
      if (!response.ok) {
        throw new Error('Unable to load channels');
      }
      const channels = await response.json();
      if (!Array.isArray(channels) || channels.length === 0) {
        throw new Error('No channels available');
      }
      return channels[0].id;
    }

    async function refreshState() {
      try {
        if (!channelId) {
          channelId = await loadChannelId();
        }
        if (playlistTracks.length === 0 && libraryTracks.length === 0) {
          await refreshTracks();
        }
        const response = await fetch('/channels/' + encodeURIComponent(channelId) + '/state');
        if (!response.ok) {
          throw new Error('Unable to load state');
        }
        snapshot = await response.json();
        render();
      } catch (error) {
        elements.status.textContent = error.message || 'Failed to load state';
        elements.title.textContent = 'Unavailable';
        elements.artist.textContent = 'Check server and media library';
        elements.nextTitle.textContent = 'Unavailable';
        elements.nextArtist.textContent = '';
      }
    }

    async function refreshTracks() {
      if (!channelId) {
        return;
      }
      const playlistResponse = await fetch('/channels/' + encodeURIComponent(channelId) + '/playlist');
      if (!playlistResponse.ok) {
        throw new Error('Unable to load playlist');
      }
      playlistTracks = await playlistResponse.json();

      const libraryResponse = await fetch('/channels/' + encodeURIComponent(channelId) + '/library');
      if (!libraryResponse.ok) {
        throw new Error('Unable to load library');
      }
      libraryTracks = await libraryResponse.json();
      renderTracks();
    }

    async function savePlaylist(trackIds) {
      if (!channelId) {
        return;
      }
      elements.status.textContent = 'Saving playlist...';
      const response = await fetch('/channels/' + encodeURIComponent(channelId) + '/playlist', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ track_ids: trackIds }),
      });
      if (!response.ok) {
        throw new Error('Unable to save playlist');
      }
      playlistTracks = await response.json();
      renderTracks();
      await refreshState();
    }

    async function addPlaylistTrack(trackId, button) {
      if (!trackId) {
        return;
      }
      button.disabled = true;
      try {
        const trackIds = playlistTracks.map((track) => track.track_id || track.id);
        await savePlaylist(trackIds.concat(trackId));
      } catch (error) {
        elements.status.textContent = error.message || 'Failed to add track to playlist';
      } finally {
        button.disabled = false;
      }
    }

    async function removePlaylistTrack(index) {
      try {
        const trackIds = playlistTracks.map((track) => track.track_id || track.id);
        trackIds.splice(index, 1);
        await savePlaylist(trackIds);
      } catch (error) {
        elements.status.textContent = error.message || 'Failed to remove track from playlist';
      }
    }

    async function movePlaylistTrack(fromIndex, toIndex) {
      if (toIndex < 0 || toIndex >= playlistTracks.length) {
        return;
      }
      try {
        const trackIds = playlistTracks.map((track) => track.track_id || track.id);
        const moved = trackIds.splice(fromIndex, 1)[0];
        trackIds.splice(toIndex, 0, moved);
        await savePlaylist(trackIds);
      } catch (error) {
        elements.status.textContent = error.message || 'Failed to reorder playlist';
      }
    }

    async function addTrackToQueue(trackId, button) {
      if (!channelId || !trackId) {
        return;
      }
      button.disabled = true;
      elements.status.textContent = 'Adding track to queue...';
      try {
        const response = await fetch('/channels/' + encodeURIComponent(channelId) + '/queue', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ track_id: trackId }),
        });
        if (!response.ok) {
          throw new Error('Unable to add track to queue');
        }
        await refreshState();
      } catch (error) {
        elements.status.textContent = error.message || 'Failed to add track to queue';
      } finally {
        button.disabled = false;
      }
    }

    async function skipTrack() {
      if (!channelId) {
        return;
      }
      elements.skipButton.disabled = true;
      elements.status.textContent = 'Skipping track...';
      try {
        const response = await fetch('/channels/' + encodeURIComponent(channelId) + '/skip', { method: 'POST' });
        if (!response.ok) {
          throw new Error('Unable to skip track');
        }
        snapshot = await response.json();
        render();
      } catch (error) {
        elements.status.textContent = error.message || 'Failed to skip track';
      } finally {
        elements.skipButton.disabled = false;
      }
    }

    elements.skipButton.addEventListener('click', skipTrack);
    refreshState();
    setInterval(refreshState, 5000);
    setInterval(render, 250);
  </script>
</body>
</html>
`
