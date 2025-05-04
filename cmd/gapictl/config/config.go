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
