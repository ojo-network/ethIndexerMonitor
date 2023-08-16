package config

import (
	"os"
	"time"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

type (
	Config struct {
		NetworkMap map[string]Source `mapstructure:"network_map"`
	}

	Source struct {
		TruthUrl  string `mapstructure:"truth_url"`
		CheckUrl  string `mapstructure:"check_url"`
		BaseAsset string `mapstructure:"base_asset"`

		// should match most of the time
		Deviation float64 `mapstructure:"deviation"`

		CronInterval string `mapstructure:"cron_interval"`
	}

	AccessToken struct {
		SlackToken   string
		SlackChannel string
		AppToken     string
	}
)

func ParseConfig(args []string) (*Config, *AccessToken, error) {
	godotenv.Load(".env") //nolint
	viper.SetConfigFile(args[0])
	viper.AutomaticEnv()

	err := viper.ReadInConfig()
	if err != nil {
		return nil, nil, err
	}

	var config Config
	err = viper.Unmarshal(&config)
	if err != nil {
		return nil, nil, err
	}

	token := viper.GetString("SLACK_TOKEN")
	if token == "" {
		token = os.Getenv("SLACK_TOKEN")
	}

	channel := viper.GetString("SLACK_CHANNEL")
	if channel == "" {
		channel = os.Getenv("SLACK_CHANNEL")
	}

	appToken := viper.GetString("APP_TOKEN")
	if channel == "" {
		channel = os.Getenv("APP_TOKEN")
	}

	accessToken := &AccessToken{
		SlackToken:   token,
		SlackChannel: channel,
		AppToken:     appToken,
	}

	return &config, accessToken, config.validate()
}

func (c *Config) validate() error {
	// check for cron interval parse
	for _, network := range c.NetworkMap {
		if _, err := time.ParseDuration(network.CronInterval); err != nil {
			return err
		}

	}

	return nil
}
