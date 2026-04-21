package config

import (
	"bufio"
	"os"
	"strings"
)

type Config struct {
	ListenAddress     string
	DatabasePath      string
	MediaDir          string
	ChannelID         string
	ChannelName       string
	FFmpegPath        string
	VideoFontPath     string
	DashboardUsername string
	DashboardPassword string
}

func Load() Config {
	loadDotEnv(".env")

	return Config{
		ListenAddress:     getEnv("ATLAS_LISTEN_ADDR", ":8080"),
		DatabasePath:      getEnv("ATLAS_DATABASE_PATH", "atlas.db"),
		MediaDir:          getEnv("ATLAS_MEDIA_DIR", "./testdata/media"),
		ChannelID:         getEnv("ATLAS_CHANNEL_ID", "local-library"),
		ChannelName:       getEnv("ATLAS_CHANNEL_NAME", "Local Library"),
		FFmpegPath:        getEnv("ATLAS_FFMPEG_PATH", "ffmpeg"),
		VideoFontPath:     getEnv("ATLAS_VIDEO_FONT_PATH", ""),
		DashboardUsername: getEnv("ATLAS_DASHBOARD_USERNAME", "admin"),
		DashboardPassword: getEnv("ATLAS_DASHBOARD_PASSWORD", "atlas"),
	}
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func loadDotEnv(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}

		key = strings.TrimSpace(key)
		if key == "" || os.Getenv(key) != "" {
			continue
		}

		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		_ = os.Setenv(key, value)
	}
}
