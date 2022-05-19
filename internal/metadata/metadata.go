package metadata

import (
	"runtime/debug"
	"sync"
)

// https://cloud.google.com/run/docs/container-contract#env-vars

type Config struct {
	Service string `kong:"env=K_SERVICE"`
	Version string `kong:""`
}

type metadata struct {
	lock   sync.Mutex
	config Config
}

// global variable - not happy, but :shrug:
var globalMetadata = &metadata{}

// useful for tests
func Reset() {
	globalMetadata.lock.Lock()
	defer globalMetadata.lock.Unlock()

	globalMetadata.config = Config{}
}

func FromConfig(config Config) {
	globalMetadata.lock.Lock()
	defer globalMetadata.lock.Unlock()

	globalMetadata.config = config
}

func Service() string {
	globalMetadata.lock.Lock()
	defer globalMetadata.lock.Unlock()

	return globalMetadata.config.Service
}

func Version() string {
	globalMetadata.lock.Lock()
	defer globalMetadata.lock.Unlock()

	if globalMetadata.config.Version != "" {
		return globalMetadata.config.Version
	}

	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}

	globalMetadata.config.Version = info.Main.Version

	return globalMetadata.config.Version
}
