package ffmpeg

import (
	"bytes"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/homepc/atlas-audio-engine/internal/media"
)

type flushingBuffer struct {
	bytes.Buffer
	flushes int
}

func (b *flushingBuffer) Flush() {
	b.flushes++
}

func TestCopyChunksFlushesEachWrite(t *testing.T) {
	t.Parallel()

	input := bytes.Repeat([]byte("a"), 40*1024)
	output := &flushingBuffer{}

	if err := copyChunks(output, bytes.NewReader(input)); err != nil {
		t.Fatalf("copy chunks: %v", err)
	}
	if !bytes.Equal(output.Bytes(), input) {
		t.Fatalf("expected copied output to match input")
	}
	if output.flushes < 2 {
		t.Fatalf("expected multiple flushes for chunked output, got %d", output.flushes)
	}
}

func TestDrawTextEscapesWindowsFontPath(t *testing.T) {
	t.Parallel()

	filter := drawText(
		`C:\Users\HomePC\AppData\Local\Temp\atlas-video-123\title.txt`,
		`C:\Users\HomePC\Downloads\FC Subject Fontset ver 2.01\Application Files ver 2.01\FCSubject-Medium.ttf`,
		"72",
		"x=930:y=285",
		"fontcolor=white",
		"8",
	)

	if !strings.HasPrefix(filter, "drawtext=") {
		t.Fatalf("expected drawtext filter assignment, got %q", filter)
	}
	if !strings.Contains(filter, `fontfile='C\:/Users/HomePC/Downloads/FC Subject Fontset ver 2.01/Application Files ver 2.01/FCSubject-Medium.ttf'`) {
		t.Fatalf("expected escaped font path, got %q", filter)
	}
	if !strings.Contains(filter, `textfile='C\:/Users/HomePC/AppData/Local/Temp/atlas-video-123/title.txt'`) {
		t.Fatalf("expected escaped text path, got %q", filter)
	}
}

func TestWrapTextUsesRuneLengthAndPreservesExistingBreaks(t *testing.T) {
	t.Parallel()

	wrapped := wrapText("This Is A Very Long Song Title\nเพลงภาษาไทยยาวมาก", 12)

	expected := "This Is A\nVery Long\nSong Title\nเพลงภาษาไทยยาวมาก"
	if wrapped != expected {
		t.Fatalf("expected wrapped text %q, got %q", expected, wrapped)
	}
}

func TestWrapTextKeepsShortTextSingleLine(t *testing.T) {
	t.Parallel()

	wrapped := wrapText("Oh Baby", 18)

	if wrapped != "Oh Baby" {
		t.Fatalf("expected short text to stay single line, got %q", wrapped)
	}
}

func TestProgressFiltersDrawAnimatedFill(t *testing.T) {
	t.Parallel()

	source, overlay := progressFilters(15_000, 60_000)

	if !strings.Contains(source, "geq=") {
		t.Fatalf("expected progress source to use per-frame geq alpha, got %q", source)
	}
	if !strings.Contains(source, "(15.000+T)/60.000") {
		t.Fatalf("expected progress source to include elapsed and duration expression, got %q", source)
	}
	if !strings.Contains(overlay, "drawbox=x=96:y=948:w=1728:h=18") {
		t.Fatalf("expected progress base bar, got %q", overlay)
	}
	if !strings.Contains(overlay, "[progress_base][progress]overlay=96:948") {
		t.Fatalf("expected progress overlay, got %q", overlay)
	}
}

func TestVideoFilterIncludesProgressWhenDurationIsKnown(t *testing.T) {
	t.Parallel()

	filter := videoFilter("title.txt", "artist.txt", "radio.txt", "schedule.txt", "next.txt", true, "", media.VideoMetadata{
		DurationMs: 120_000,
		ElapsedMs:  30_000,
	})

	if !strings.Contains(filter, "drawbox=x=96:y=948") {
		t.Fatalf("expected video filter to include progress bar, got %q", filter)
	}
	if !strings.Contains(filter, "radio.txt") {
		t.Fatalf("expected video filter to include radio overlay, got %q", filter)
	}
	if !strings.Contains(filter, "schedule.txt") {
		t.Fatalf("expected video filter to include schedule overlay, got %q", filter)
	}
}

func TestFrameRendererReusesScaledArtworkForSamePath(t *testing.T) {
	t.Parallel()

	renderer := newFrameRenderer()
	renderer.scaledArtwork = image.NewRGBA(image.Rect(0, 0, artSize, artSize))
	renderer.scaledArtwork.SetRGBA(0, 0, color.RGBA{R: 255, A: 255})
	renderer.lastArtworkPath = "cover.jpg"
	firstArtwork := renderer.scaledArtwork

	frame := renderer.Render(media.VideoMetadata{
		ArtworkPath: "cover.jpg",
		DurationMs:  1000,
		ElapsedMs:   500,
	})

	if renderer.scaledArtwork != firstArtwork {
		t.Fatalf("expected renderer to reuse scaled artwork when path is unchanged")
	}
	if frame.Bounds().Dx() != frameWidth || frame.Bounds().Dy() != frameHeight {
		t.Fatalf("expected frame size %dx%d, got %dx%d", frameWidth, frameHeight, frame.Bounds().Dx(), frame.Bounds().Dy())
	}
}

func TestWriteStableTextFileNeverLeavesEmptyFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "artist.txt")
	if err := writeStableTextFile(path, "Artist"); err != nil {
		t.Fatalf("write stable text file: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read stable text file: %v", err)
	}
	if len(content) != textFileSize {
		t.Fatalf("expected padded file size %d, got %d", textFileSize, len(content))
	}
	if !strings.HasPrefix(string(content), "Artist") {
		t.Fatalf("expected file to start with content, got %q", string(content[:16]))
	}

	if err := writeStableTextFile(path, "A"); err != nil {
		t.Fatalf("rewrite stable text file: %v", err)
	}
	content, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("read rewritten stable text file: %v", err)
	}
	if strings.TrimSpace(string(content)) != "A" {
		t.Fatalf("expected rewritten content to clear old bytes, got %q", strings.TrimSpace(string(content)))
	}
}
