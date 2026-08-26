package config

import (
	"os"
	"path/filepath"
)

type Config struct {
	Listen    string
	DataDir   string
	Dev       bool
	PublicURL string
}

func Default() Config {
	data := os.Getenv("HALLO_DATA")
	if data == "" {
		if _, err := os.Stat("/var/lib/hallo"); err == nil {
			data = "/var/lib/hallo"
		} else {
			data = "data"
		}
	}
	listen := os.Getenv("HALLO_LISTEN")
	if listen == "" {
		listen = ":18080"
	}
	return Config{
		Listen:    listen,
		DataDir:   data,
		Dev:       os.Getenv("HALLO_DEV") == "1",
		PublicURL: os.Getenv("HALLO_PUBLIC_URL"),
	}
}

func (c Config) DBPath() string {
	return filepath.Join(c.DataDir, "hallo.db")
}

func (c Config) XrayDir() string {
	return filepath.Join(c.DataDir, "xray")
}

func (c Config) XrayConfigPath() string {
	return filepath.Join(c.XrayDir(), "config.json")
}
