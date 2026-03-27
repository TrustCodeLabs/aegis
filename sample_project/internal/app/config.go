package app

import (
	"os"
	"strings"
)

const (
	ServiceName            = "aegis-sample-project"
	RuntimeName            = "Aegis Core sample API"
	EnvironmentName        = "sample_project"
	VisibilityTierInternal = "internal"
	DefaultPort            = "8090"
	DefaultTenant          = "team-a"
	DefaultSubjectID       = "demo-user"
	DefaultRole            = "editor"
	SkillsOutputPath       = "./generated/SKILLS.md"
	StorageModeLayered     = "layered"
	StorageModeDirect      = "direct"
)

func EnvOrDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func NormalizeStorageMode(mode string) string {
	return strings.ToLower(strings.TrimSpace(mode))
}

func IsValidStorageMode(mode string) bool {
	switch NormalizeStorageMode(mode) {
	case StorageModeLayered, StorageModeDirect:
		return true
	default:
		return false
	}
}
