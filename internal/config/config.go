package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
)

// Config holds application level configuration aggregated from env/config files.
type Config struct {
	Server struct {
		Addr string
	}
	Database struct {
		Path string
	}
	Download struct {
		DataDir string
	}
	Storage struct {
		Bucket    string
		KeyPrefix string
		Region    string
		Endpoint  string
	}
	AWS struct {
		Profile string
	}
}

// Load reads configuration from environment variables and optional config files.
func Load() (Config, error) {
	loadDotEnv()

	v := viper.New()
	v.SetEnvPrefix("MAGNET")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	v.SetDefault("server.addr", "0.0.0.0:8080")
	v.SetDefault("database.path", "data/magnet.db")
	v.SetDefault("download.datadir", "data/downloads")
	v.SetDefault("storage.bucket", "")
	v.SetDefault("storage.keyprefix", "magnet-tasks")
	v.SetDefault("storage.region", "us-east-1")
	v.SetDefault("storage.endpoint", "")
	v.SetDefault("aws.profile", "")

	v.SetConfigName("config")
	v.AddConfigPath(".")
	_ = v.ReadInConfig() // optional file

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return Config{}, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return cfg, nil
}

func loadDotEnv() {
	file, err := os.Open(".env")
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

		partsIndex := strings.Index(line, "=")
		if partsIndex <= 0 {
			continue
		}

		key := strings.TrimSpace(line[:partsIndex])
		value := strings.TrimSpace(line[partsIndex+1:])
		value = strings.Trim(value, `"'`)
		if key == "" {
			continue
		}

		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, value)
		}
	}
}
