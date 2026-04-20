package ffmpeg

import (
	"context"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/homepc/atlas-audio-engine/internal/media"
)

type Streamer struct {
	binary   string
	fontPath string
}

const videoFPS = 24
const (
	frameWidth  = 1280
	frameHeight = 720
	artSize     = 440
)

func NewStreamer(binary string) *Streamer {
	return &Streamer{binary: binary}
}

func NewStreamerWithFont(binary, fontPath string) *Streamer {
	return &Streamer{binary: binary, fontPath: fontPath}
}

func (s *Streamer) StreamMP3(ctx context.Context, inputPath string, startSeconds float64, output io.Writer) error {
	args := []string{
		"-hide_banner",
		"-loglevel", "error",
	}
	if startSeconds > 0 {
		args = append(args, "-ss", strconv.FormatFloat(startSeconds, 'f', 3, 64))
	}
	args = append(args,
		"-re",
		"-i", inputPath,
		"-vn",
		"-acodec", "libmp3lame",
		"-b:a", "192k",
		"-flush_packets", "1",
		"-write_xing", "0",
		"-f", "mp3",
		"pipe:1",
	)

	cmd := exec.CommandContext(ctx, s.binary, args...)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	stderrDone := readAllAsync(stderr)
	copyErr := copyChunks(output, stdout)
	waitErr := cmd.Wait()
	stderrOutput := <-stderrDone
	if copyErr != nil {
		return copyErr
	}
	if waitErr != nil {
		return fmt.Errorf("%w: %s", waitErr, strings.TrimSpace(stderrOutput))
	}
	return nil
}

func (s *Streamer) StreamMPEGTS(ctx context.Context, inputPath string, startSeconds float64, metadata media.VideoMetadata, output io.Writer) error {
	tempDir, err := os.MkdirTemp("", "atlas-video-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	titleFile, err := writeTextFile(tempDir, "title.txt", wrapText(fallback(metadata.Title, "Nothing playing"), 18))
	if err != nil {
		return err
	}
	artistFile, err := writeTextFile(tempDir, "artist.txt", wrapText(fallback(metadata.Artist, "Unknown artist"), 28))
	if err != nil {
		return err
	}
	nextFile, err := writeTextFile(tempDir, "next.txt", wrapText(formatNext(metadata.NextTitle, metadata.NextArtist), 42))
	if err != nil {
		return err
	}

	args := []string{
		"-hide_banner",
		"-loglevel", "error",
	}
	if startSeconds > 0 {
		args = append(args, "-ss", strconv.FormatFloat(startSeconds, 'f', 3, 64))
	}
	args = append(args,
		"-re",
		"-i", inputPath,
	)

	if metadata.ArtworkPath != "" {
		args = append(args,
			"-re",
			"-loop", "1",
			"-framerate", strconv.Itoa(videoFPS),
			"-i", metadata.ArtworkPath,
		)
	} else {
		args = append(args,
			"-re",
			"-f", "lavfi",
			"-i", fmt.Sprintf("color=c=#071426:s=1280x720:r=%d", videoFPS),
		)
	}

	filter := videoFilter(titleFile, artistFile, nextFile, metadata.ArtworkPath != "", usableFontPath(s.fontPath), metadata)
	args = append(args,
		"-filter_complex", filter,
		"-map", "[v]",
		"-map", "0:a:0",
		"-c:v", "libx264",
		"-preset", "ultrafast",
		"-tune", "zerolatency",
		"-bf", "0",
		"-pix_fmt", "yuv420p",
		"-r", strconv.Itoa(videoFPS),
		"-g", strconv.Itoa(videoFPS*2),
		"-c:a", "aac",
		"-b:a", "160k",
		"-shortest",
		"-flush_packets", "1",
		"-muxdelay", "0",
		"-muxpreload", "0",
		"-mpegts_flags", "resend_headers",
		"-f", "mpegts",
		"pipe:1",
	)

	cmd := exec.CommandContext(ctx, s.binary, args...)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	stderrDone := readAllAsync(stderr)
	copyErr := copyChunks(output, stdout)
	waitErr := cmd.Wait()
	stderrOutput := <-stderrDone
	if copyErr != nil {
		return copyErr
	}
	if waitErr != nil {
		return fmt.Errorf("%w: %s", waitErr, strings.TrimSpace(stderrOutput))
	}
	return nil
}

func (s *Streamer) StreamPersistentMPEGTS(ctx context.Context, audioURL string, metadataProvider media.VideoMetadataProvider, output io.Writer) error {
	tempDir, err := os.MkdirTemp("", "atlas-broadcast-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	files := persistentMetadataFiles{
		title:  filepath.Join(tempDir, "title.txt"),
		artist: filepath.Join(tempDir, "artist.txt"),
		next:   filepath.Join(tempDir, "next.txt"),
		clock:  filepath.Join(tempDir, "clock.txt"),
	}
	if err := writePersistentMetadata(ctx, metadataProvider, files); err != nil {
		return err
	}

	metadataCtx, cancelMetadata := context.WithCancel(ctx)
	defer cancelMetadata()
	go refreshPersistentMetadata(metadataCtx, metadataProvider, files)

	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-reconnect", "1",
		"-reconnect_streamed", "1",
		"-reconnect_delay_max", "2",
		"-i", audioURL,
		"-re",
		"-f", "lavfi",
		"-i", fmt.Sprintf("color=c=#071426:s=1280x720:r=%d", videoFPS),
	}

	filter := persistentVideoFilter(files, usableFontPath(s.fontPath))
	args = append(args,
		"-filter_complex", filter,
		"-map", "[v]",
		"-map", "0:a:0",
		"-c:v", "libx264",
		"-preset", "ultrafast",
		"-tune", "zerolatency",
		"-bf", "0",
		"-pix_fmt", "yuv420p",
		"-r", strconv.Itoa(videoFPS),
		"-g", strconv.Itoa(videoFPS*2),
		"-c:a", "aac",
		"-b:a", "160k",
		"-flush_packets", "1",
		"-muxdelay", "0",
		"-muxpreload", "0",
		"-mpegts_flags", "resend_headers",
		"-f", "mpegts",
		"pipe:1",
	)

	cmd := exec.CommandContext(ctx, s.binary, args...)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	stderrDone := readAllAsync(stderr)
	copyErr := copyChunks(output, stdout)
	waitErr := cmd.Wait()
	stderrOutput := <-stderrDone
	if copyErr != nil {
		return copyErr
	}
	if waitErr != nil {
		return fmt.Errorf("%w: %s", waitErr, strings.TrimSpace(stderrOutput))
	}
	return nil
}

func (s *Streamer) StreamPersistentVisualMPEGTS(ctx context.Context, audioURL string, metadataProvider media.VideoMetadataProvider, output io.Writer) error {
	tempDir, err := os.MkdirTemp("", "atlas-broadcast-visual-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	files := persistentMetadataFiles{
		title:  filepath.Join(tempDir, "title.txt"),
		artist: filepath.Join(tempDir, "artist.txt"),
		next:   filepath.Join(tempDir, "next.txt"),
		clock:  filepath.Join(tempDir, "clock.txt"),
	}
	initialMetadata, err := metadataProvider(ctx)
	if err != nil {
		return err
	}
	if err := writePersistentMetadataValue(initialMetadata, files); err != nil {
		return err
	}

	metadataCtx, cancelMetadata := context.WithCancel(ctx)
	defer cancelMetadata()
	go refreshPersistentMetadata(metadataCtx, metadataProvider, files)

	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-f", "rawvideo",
		"-pix_fmt", "rgba",
		"-s", fmt.Sprintf("%dx%d", frameWidth, frameHeight),
		"-r", strconv.Itoa(videoFPS),
		"-i", "pipe:0",
		"-reconnect", "1",
		"-reconnect_streamed", "1",
		"-reconnect_delay_max", "2",
		"-i", audioURL,
	}

	filter := persistentVisualFilter(files, usableFontPath(s.fontPath))
	args = append(args,
		"-filter_complex", filter,
		"-map", "[v]",
		"-map", "1:a:0",
		"-c:v", "libx264",
		"-preset", "ultrafast",
		"-tune", "zerolatency",
		"-bf", "0",
		"-pix_fmt", "yuv420p",
		"-r", strconv.Itoa(videoFPS),
		"-g", strconv.Itoa(videoFPS*2),
		"-c:a", "aac",
		"-b:a", "160k",
		"-flush_packets", "1",
		"-muxdelay", "0",
		"-muxpreload", "0",
		"-mpegts_flags", "resend_headers",
		"-f", "mpegts",
		"pipe:1",
	)

	cmd := exec.CommandContext(ctx, s.binary, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	frameErr := make(chan error, 1)
	go func() {
		frameErr <- writeVisualFrames(ctx, stdin, metadataProvider, initialMetadata)
	}()

	stderrDone := readAllAsync(stderr)
	copyErr := copyChunks(output, stdout)
	waitErr := cmd.Wait()
	stderrOutput := <-stderrDone
	cancelMetadata()

	select {
	case err := <-frameErr:
		if err != nil && copyErr == nil && ctx.Err() == nil {
			copyErr = err
		}
	default:
	}

	if copyErr != nil {
		return copyErr
	}
	if waitErr != nil {
		return fmt.Errorf("%w: %s", waitErr, strings.TrimSpace(stderrOutput))
	}
	return nil
}

func copyChunks(dst io.Writer, src io.Reader) error {
	buffer := make([]byte, 16*1024)
	for {
		n, readErr := src.Read(buffer)
		if n > 0 {
			if _, writeErr := dst.Write(buffer[:n]); writeErr != nil {
				return writeErr
			}
			if flusher, ok := dst.(interface{ Flush() }); ok {
				flusher.Flush()
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				return nil
			}
			return readErr
		}
	}
}

func readAllAsync(reader io.Reader) <-chan string {
	done := make(chan string, 1)
	go func() {
		data, _ := io.ReadAll(reader)
		done <- string(data)
	}()
	return done
}

func writeTextFile(dir, name, content string) (string, error) {
	path := filepath.Join(dir, name)
	return path, os.WriteFile(path, []byte(content), 0o644)
}

type persistentMetadataFiles struct {
	title  string
	artist string
	next   string
	clock  string
}

func refreshPersistentMetadata(ctx context.Context, provider media.VideoMetadataProvider, files persistentMetadataFiles) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = writePersistentMetadata(ctx, provider, files)
		}
	}
}

func writePersistentMetadata(ctx context.Context, provider media.VideoMetadataProvider, files persistentMetadataFiles) error {
	metadata, err := provider(ctx)
	if err != nil {
		return err
	}
	return writePersistentMetadataValue(metadata, files)
}

func writePersistentMetadataValue(metadata media.VideoMetadata, files persistentMetadataFiles) error {
	values := map[string]string{
		files.title:  wrapText(fallback(metadata.Title, "Nothing playing"), 18),
		files.artist: wrapText(fallback(metadata.Artist, "Unknown artist"), 28),
		files.next:   wrapText(formatNext(metadata.NextTitle, metadata.NextArtist), 42),
		files.clock:  formatClock(metadata.ElapsedMs, metadata.DurationMs),
	}
	for path, value := range values {
		if err := os.WriteFile(path, []byte(value), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func writeVisualFrames(ctx context.Context, output io.WriteCloser, provider media.VideoMetadataProvider, initial media.VideoMetadata) error {
	defer output.Close()

	ticker := time.NewTicker(time.Second / time.Duration(videoFPS))
	defer ticker.Stop()

	renderer := newFrameRenderer()
	metadata := initial
	nextRefresh := time.Now()

	for {
		now := time.Now()
		if !now.Before(nextRefresh) {
			if nextMetadata, err := provider(ctx); err == nil {
				metadata = nextMetadata
			}
			nextRefresh = now.Add(500 * time.Millisecond)
		}

		frame := renderer.Render(metadata)
		if _, err := output.Write(frame.Pix); err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

type frameRenderer struct {
	lastArtworkPath string
	artwork         image.Image
	frame           *image.RGBA
}

func newFrameRenderer() *frameRenderer {
	return &frameRenderer{
		frame: image.NewRGBA(image.Rect(0, 0, frameWidth, frameHeight)),
	}
}

func (r *frameRenderer) Render(metadata media.VideoMetadata) *image.RGBA {
	if metadata.ArtworkPath != r.lastArtworkPath {
		r.artwork = loadArtwork(metadata.ArtworkPath)
		r.lastArtworkPath = metadata.ArtworkPath
	}

	fillBackground(r.frame)
	drawPanel(r.frame)
	if r.artwork != nil {
		drawCover(r.frame, r.artwork)
	} else {
		drawCoverPlaceholder(r.frame)
	}
	drawProgress(r.frame, metadata.ElapsedMs, metadata.DurationMs)
	return r.frame
}

func loadArtwork(path string) image.Image {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()
	img, _, err := image.Decode(file)
	if err != nil {
		return nil
	}
	return img
}

func fillBackground(frame *image.RGBA) {
	for y := 0; y < frameHeight; y++ {
		for x := 0; x < frameWidth; x++ {
			r := uint8(3 + (x * 13 / frameWidth))
			g := uint8(8 + (y * 17 / frameHeight))
			b := uint8(22 + ((x + y) * 24 / (frameWidth + frameHeight)))
			frame.SetRGBA(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
		}
	}
}

func drawPanel(frame *image.RGBA) {
	fillRect(frame, image.Rect(64, 96, 1216, 624), color.RGBA{R: 255, G: 255, B: 255, A: 14})
}

func drawCover(frame *image.RGBA, artwork image.Image) {
	dst := image.Rect(96, 140, 96+artSize, 140+artSize)
	drawScaled(frame, dst, artwork)
}

func drawCoverPlaceholder(frame *image.RGBA) {
	dst := image.Rect(96, 140, 96+artSize, 140+artSize)
	for y := dst.Min.Y; y < dst.Max.Y; y++ {
		for x := dst.Min.X; x < dst.Max.X; x++ {
			frame.SetRGBA(x, y, color.RGBA{
				R: uint8(24 + (x-dst.Min.X)*50/artSize),
				G: uint8(74 + (y-dst.Min.Y)*80/artSize),
				B: 120,
				A: 255,
			})
		}
	}
}

func drawProgress(frame *image.RGBA, elapsedMs, durationMs int64) {
	base := image.Rect(620, 610, 1180, 622)
	fillRect(frame, base, color.RGBA{R: 255, G: 255, B: 255, A: 36})
	if durationMs <= 0 {
		return
	}
	progress := float64(maxInt64(0, elapsedMs)) / float64(durationMs)
	if progress > 1 {
		progress = 1
	}
	fill := base
	fill.Max.X = fill.Min.X + int(float64(base.Dx())*progress)
	fillRect(frame, fill, color.RGBA{R: 124, G: 255, B: 191, A: 230})
}

func fillRect(frame *image.RGBA, rect image.Rectangle, c color.RGBA) {
	rect = rect.Intersect(frame.Bounds())
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			blendPixel(frame, x, y, c)
		}
	}
}

func blendPixel(frame *image.RGBA, x, y int, c color.RGBA) {
	if c.A == 255 {
		frame.SetRGBA(x, y, c)
		return
	}

	existing := frame.RGBAAt(x, y)
	alpha := uint32(c.A)
	inverse := uint32(255 - c.A)
	frame.SetRGBA(x, y, color.RGBA{
		R: uint8((uint32(c.R)*alpha + uint32(existing.R)*inverse) / 255),
		G: uint8((uint32(c.G)*alpha + uint32(existing.G)*inverse) / 255),
		B: uint8((uint32(c.B)*alpha + uint32(existing.B)*inverse) / 255),
		A: 255,
	})
}

func drawScaled(dst *image.RGBA, dstRect image.Rectangle, src image.Image) {
	srcBounds := src.Bounds()
	for y := dstRect.Min.Y; y < dstRect.Max.Y; y++ {
		for x := dstRect.Min.X; x < dstRect.Max.X; x++ {
			srcX := srcBounds.Min.X + (x-dstRect.Min.X)*srcBounds.Dx()/dstRect.Dx()
			srcY := srcBounds.Min.Y + (y-dstRect.Min.Y)*srcBounds.Dy()/dstRect.Dy()
			dst.Set(x, y, src.At(srcX, srcY))
		}
	}
}

func fallback(value, fallbackValue string) string {
	if strings.TrimSpace(value) == "" {
		return fallbackValue
	}
	return value
}

func formatNext(title, artist string) string {
	title = strings.TrimSpace(title)
	artist = strings.TrimSpace(artist)
	if title == "" {
		return "Next: playlist unavailable"
	}
	if artist == "" {
		return "Next: " + title
	}
	return "Next: " + title + " - " + artist
}

func formatClock(elapsedMs, durationMs int64) string {
	return formatDuration(elapsedMs) + " / " + formatDuration(durationMs)
}

func formatDuration(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	totalSeconds := ms / 1000
	minutes := totalSeconds / 60
	seconds := totalSeconds % 60
	return fmt.Sprintf("%d:%02d", minutes, seconds)
}

func wrapText(value string, maxRunes int) string {
	if maxRunes <= 0 {
		return value
	}

	paragraphs := strings.Split(strings.TrimSpace(value), "\n")
	wrapped := make([]string, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		wrapped = append(wrapped, wrapParagraph(paragraph, maxRunes)...)
	}
	return strings.Join(wrapped, "\n")
}

func wrapParagraph(value string, maxRunes int) []string {
	words := strings.Fields(value)
	if len(words) == 0 {
		return []string{""}
	}

	lines := make([]string, 0, len(words))
	current := ""
	for _, word := range words {
		if current == "" {
			current = word
			continue
		}
		if runeLen(current)+1+runeLen(word) > maxRunes {
			lines = append(lines, current)
			current = word
			continue
		}
		current += " " + word
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

func runeLen(value string) int {
	return len([]rune(value))
}

func usableFontPath(fontPath string) string {
	if strings.TrimSpace(fontPath) == "" {
		return ""
	}
	if _, err := os.Stat(fontPath); err != nil {
		return ""
	}
	return fontPath
}

func videoFilter(titleFile, artistFile, nextFile string, hasArtwork bool, fontPath string, metadata media.VideoMetadata) string {
	title := drawText(titleFile, fontPath, "72", "x=620:y=174", "fontcolor=white", "14")
	artist := drawText(artistFile, fontPath, "40", "x=620:y=340", "fontcolor=white@0.72", "10")
	next := drawText(nextFile, fontPath, "26", "x=620:y=464", "fontcolor=white@0.82:box=1:boxcolor=white@0.10:boxborderw=18", "8")
	progressSource, progressOverlay := progressFilters(metadata.ElapsedMs, metadata.DurationMs)

	if !hasArtwork {
		return progressSource + "[1:v]format=yuv420p," + title + "," + artist + "," + next + progressOverlay + "[v]"
	}

	return progressSource +
		"[1:v]scale=1280:720:force_original_aspect_ratio=increase,crop=1280:720,eq=brightness=-0.28:saturation=1.18[bg];" +
		"[1:v]scale=440:440:force_original_aspect_ratio=decrease,pad=440:440:(ow-iw)/2:(oh-ih)/2:color=black@0,format=rgba[art];" +
		"[bg][art]overlay=96:140," + title + "," + artist + "," + next + progressOverlay + "[v]"
}

func persistentVideoFilter(files persistentMetadataFiles, fontPath string) string {
	title := drawText(files.title, fontPath, "72", "x=96:y=150", "fontcolor=white", "14", true)
	artist := drawText(files.artist, fontPath, "40", "x=96:y=318", "fontcolor=white@0.72", "10", true)
	next := drawText(files.next, fontPath, "26", "x=96:y=456", "fontcolor=white@0.82:box=1:boxcolor=white@0.10:boxborderw=18", "8", true)
	clock := drawText(files.clock, fontPath, "24", "x=96:y=610", "fontcolor=white@0.70", "8", true)

	return "[1:v]format=yuv420p," +
		"drawbox=x=0:y=0:w=1280:h=720:color=0x061126@1:t=fill," +
		"drawbox=x=64:y=96:w=1152:h=528:color=white@0.055:t=fill," +
		title + "," + artist + "," + next + "," + clock + "[v]"
}

func persistentVisualFilter(files persistentMetadataFiles, fontPath string) string {
	title := drawText(files.title, fontPath, "72", "x=620:y=150", "fontcolor=white", "14", true)
	artist := drawText(files.artist, fontPath, "40", "x=620:y=318", "fontcolor=white@0.72", "10", true)
	next := drawText(files.next, fontPath, "26", "x=620:y=456", "fontcolor=white@0.82:box=1:boxcolor=white@0.10:boxborderw=18", "8", true)
	clock := drawText(files.clock, fontPath, "24", "x=1030:y=636", "fontcolor=white@0.70", "8", true)

	return "[0:v]format=yuv420p," + title + "," + artist + "," + next + "," + clock + "[v]"
}

func progressFilters(elapsedMs, durationMs int64) (string, string) {
	if durationMs <= 0 {
		return "", ""
	}

	elapsedSeconds := float64(maxInt64(0, elapsedMs)) / 1000
	durationSeconds := float64(durationMs) / 1000
	alphaExpression := fmt.Sprintf("if(lte(X,W*min(1\\,(%.3f+T)/%.3f)),230,0)", elapsedSeconds, durationSeconds)
	source := fmt.Sprintf("color=c=0x7cffbf:s=560x12:r=%d,format=rgba,geq=r='124':g='255':b='191':a='%s'[progress];", videoFPS, alphaExpression)
	overlay := ",drawbox=x=620:y=610:w=560:h=12:color=white@0.14:t=fill[progress_base];[progress_base][progress]overlay=620:610:shortest=1"
	return source, overlay
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func drawText(textFile, fontPath, fontSize, position, extra, lineSpacing string, reload ...bool) string {
	parts := []string{}
	if fontPath != "" {
		parts = append(parts, "fontfile='"+escapeFilterPath(fontPath)+"'")
	}
	parts = append(parts,
		"textfile='"+escapeFilterPath(textFile)+"'",
		"fontsize="+fontSize,
		position,
		extra,
		"line_spacing="+lineSpacing,
	)
	if len(reload) > 0 && reload[0] {
		parts = append(parts, "reload=1")
	}
	return "drawtext=" + strings.Join(parts, ":")
}

func escapeFilterPath(path string) string {
	escaped := filepath.ToSlash(path)
	escaped = strings.ReplaceAll(escaped, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, ":", "\\:")
	escaped = strings.ReplaceAll(escaped, "'", "\\'")
	return escaped
}
