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
      padding: 28px 28px 20px;
      border-bottom: 1px solid rgba(255, 255, 255, 0.08);
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

    .empty {
      color: var(--muted);
    }
  </style>
</head>
<body>
  <main class="shell">
    <section class="hero">
      <p class="eyebrow">Now Playing</p>
      <h1 id="now-title" class="title">Loading...</h1>
      <p id="now-artist" class="artist">Connecting to channel</p>
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
    };

    let snapshot = null;
    let channelId = null;

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

      elements.title.textContent = now.title || 'Nothing playing';
      elements.artist.textContent = now.artist || 'No artist available';
      elements.time.textContent = formatTime(elapsed) + ' / ' + formatTime(duration);
      elements.fill.style.width = progress + '%';
      elements.channelId.textContent = snapshot.channel_id || '';

      if (snapshot.next_track) {
        elements.nextTitle.textContent = snapshot.next_track.title || 'Unknown track';
        elements.nextArtist.textContent = snapshot.next_track.artist || 'Unknown artist';
      } else {
        elements.nextTitle.textContent = 'No next song';
        elements.nextArtist.textContent = 'Queue and playlist are empty';
      }

      const queueCount = Array.isArray(snapshot.queue) ? snapshot.queue.length : 0;
      elements.status.textContent = 'Channel ready. ' + queueCount + ' queued track' + (queueCount === 1 ? '' : 's') + '.';
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

    refreshState();
    setInterval(refreshState, 5000);
    setInterval(render, 250);
  </script>
</body>
</html>
`
