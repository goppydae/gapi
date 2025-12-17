package config

import (
	"fmt"

	"github.com/spf13/viper"
)

type Config struct {
	Target    string `mapstructure:"target"`    // e.g. "localhost:4242"
	Namespace string `mapstructure:"namespace"` // e.g. "prod"
	Token     string `mapstructure:"token"`     // optional auth token
}

func Load() (*Config, error) {
	viper.SetConfigName("ctlconfig")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("$HOME/.config/gapi")
	viper.AddConfigPath("$HOME/.config/gapi")
	viper.AutomaticEnv()

	// Zero-config defaults
	viper.SetDefault("target", "127.0.0.1:14242")
	viper.SetDefault("namespace", "default")

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("read config error: %w", err)
		}
		// Config file not found; proceed with defaults
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal error: %w", err)
	}

	return &cfg, nil
}
