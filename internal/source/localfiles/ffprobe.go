package localfiles

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strconv"
)

type FFprobeProber struct {
	binary string
}

func NewFFprobeProber(binary string) *FFprobeProber {
	return &FFprobeProber{binary: binary}
}

func (p *FFprobeProber) Probe(ctx context.Context, path string) (Metadata, error) {
	command := exec.CommandContext(
		ctx,
		p.binary,
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		path,
	)

	output, err := command.Output()
	if err != nil {
		return Metadata{}, err
	}

	var payload struct {
		Format struct {
			Duration string            `json:"duration"`
			Tags     map[string]string `json:"tags"`
		} `json:"format"`
	}
	if err := json.Unmarshal(output, &payload); err != nil {
		return Metadata{}, err
	}

	if payload.Format.Duration == "" {
		return Metadata{}, errors.New("ffprobe did not return a duration")
	}

	seconds, err := strconv.ParseFloat(payload.Format.Duration, 64)
	if err != nil {
		return Metadata{}, err
	}

	return Metadata{
		Title:      payload.Format.Tags["TITLE"],
		Artist:     payload.Format.Tags["ARTIST"],
		Album:      payload.Format.Tags["ALBUM"],
		Disc:       firstTag(payload.Format.Tags, "DISC", "DISCNUMBER", "disc", "discnumber"),
		DurationMs: int64(seconds * 1000),
	}, nil
}

func firstTag(tags map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := tags[key]; value != "" {
			return value
		}
	}
	return ""
}
