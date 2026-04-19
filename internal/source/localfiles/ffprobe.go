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
		Title:      payload.Format.Tags["title"],
		Artist:     payload.Format.Tags["artist"],
		Album:      payload.Format.Tags["album"],
		DurationMs: int64(seconds * 1000),
	}, nil
}
