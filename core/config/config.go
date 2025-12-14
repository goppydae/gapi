package config

import (
	"fmt"
	"os"

	"github.com/spf13/viper"
)

type TransportConfig struct {
	Type     string `mapstructure:"type"`
	Address  string `mapstructure:"address"`
	CertFile string `mapstructure:"certFile"`
	KeyFile  string `mapstructure:"keyFile"`
}

type Config struct {
	Transport TransportConfig `mapstructure:"transport"`
}

func Load() (*Config, error) {
	if env := os.Getenv("GAPI_CONFIG"); env != "" {
		viper.SetConfigFile(env)
	} else {
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
		addDefaultPaths() // uses build tag-specific implementation
	}
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config error: %w", err)
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal error: %w", err)
	}

	return &cfg, nil
}
