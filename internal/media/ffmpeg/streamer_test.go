package ffmpeg

import (
	"bytes"
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
		"x=620:y=190",
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
	if !strings.Contains(overlay, "drawbox=x=620:y=610:w=560:h=12") {
		t.Fatalf("expected progress base bar, got %q", overlay)
	}
	if !strings.Contains(overlay, "[progress_base][progress]overlay=620:610") {
		t.Fatalf("expected progress overlay, got %q", overlay)
	}
}

func TestVideoFilterIncludesProgressWhenDurationIsKnown(t *testing.T) {
	t.Parallel()

	filter := videoFilter("title.txt", "artist.txt", "next.txt", true, "", media.VideoMetadata{
		DurationMs: 120_000,
		ElapsedMs:  30_000,
	})

	if !strings.Contains(filter, "drawbox=x=620:y=610") {
		t.Fatalf("expected video filter to include progress bar, got %q", filter)
	}
}
