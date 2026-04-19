package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnvSetsMissingValuesOnly(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	envPath := filepath.Join(tempDir, ".env")
	content := "# comment\nATLAS_MEDIA_DIR=C:\\Music\nATLAS_CHANNEL_NAME=\"Night Shift\"\nINVALID_LINE\n"
	if err := os.WriteFile(envPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write env: %v", err)
	}

	originalMedia := os.Getenv("ATLAS_MEDIA_DIR")
	originalChannel := os.Getenv("ATLAS_CHANNEL_NAME")
	defer restoreEnv("ATLAS_MEDIA_DIR", originalMedia)
	defer restoreEnv("ATLAS_CHANNEL_NAME", originalChannel)

	_ = os.Unsetenv("ATLAS_MEDIA_DIR")
	if err := os.Setenv("ATLAS_CHANNEL_NAME", "Already Set"); err != nil {
		t.Fatalf("set env: %v", err)
	}

	loadDotEnv(envPath)

	if got := os.Getenv("ATLAS_MEDIA_DIR"); got != `C:\Music` {
		t.Fatalf("expected ATLAS_MEDIA_DIR from dotenv, got %q", got)
	}
	if got := os.Getenv("ATLAS_CHANNEL_NAME"); got != "Already Set" {
		t.Fatalf("expected existing env to win, got %q", got)
	}
}

func restoreEnv(key, value string) {
	if value == "" {
		_ = os.Unsetenv(key)
		return
	}
	_ = os.Setenv(key, value)
}
