package config

import "os"

type Config struct {
	ListenAddress string
	DatabasePath  string
	MediaDir      string
	ChannelID     string
	ChannelName   string
}

func Load() Config {
	return Config{
		ListenAddress: getEnv("ATLAS_LISTEN_ADDR", ":8080"),
		DatabasePath:  getEnv("ATLAS_DATABASE_PATH", "atlas.db"),
		MediaDir:      getEnv("ATLAS_MEDIA_DIR", "./testdata/media"),
		ChannelID:     getEnv("ATLAS_CHANNEL_ID", "local-library"),
		ChannelName:   getEnv("ATLAS_CHANNEL_NAME", "Local Library"),
	}
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
