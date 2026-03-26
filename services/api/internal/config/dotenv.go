package config

import (
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

// LoadEnvFiles loads .env files into the process environment.
// Later files override earlier ones for duplicate keys.
// Variables already set in the OS environment are not overwritten (godotenv default).
//
// From repository root: loads .env then services/api/.env (API-specific wins).
// From services/api: loads ../../.env then .env (local wins).
func LoadEnvFiles() {
	wd, err := os.Getwd()
	if err != nil {
		return
	}
	var ordered []string
	if filepath.Base(wd) == "api" && filepath.Base(filepath.Dir(wd)) == "services" {
		ordered = []string{
			filepath.Join(wd, "..", "..", ".env"),
			filepath.Join(wd, ".env"),
		}
	} else {
		ordered = []string{
			filepath.Join(wd, ".env"),
			filepath.Join(wd, "services", "api", ".env"),
		}
	}
	var files []string
	for _, p := range ordered {
		p = filepath.Clean(p)
		if _, err := os.Stat(p); err != nil {
			continue
		}
		files = append(files, p)
	}
	if len(files) == 0 {
		return
	}
	_ = godotenv.Load(files...)
}
