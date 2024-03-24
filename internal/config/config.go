package config

import (
	"errors"
	"github.com/go-playground/validator/v10"
	logs "github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

const (
	defaultVersion  = "/v1"
	defaultPort     = 8080
	defaultLocation = "data"
)

type Config struct {
	APIPath string `json:"api_path"`
	Port    int    `json:"port"`

	LogLevel string `json:"log_level"`

	DatabaseHost string `json:"database_host" validate:"required"`
	Location     string `json:"location"`
}

func getConfigValueAsString(key string) (value string) {
	return viper.GetString(key)
}

func getConfigValueAsInt(key string) (value int) {
	return viper.GetInt(key)
}

func configureViperDefaults() {
	viper.SetDefault("api_path", defaultVersion)
	viper.SetDefault("port", defaultPort)
	viper.SetDefault("location", defaultLocation)
	viper.AutomaticEnv()
}

func populateConfigurations(conf *Config) {
	conf.APIPath = getConfigValueAsString("api_path")
	conf.Port = getConfigValueAsInt("port")

	conf.LogLevel = getConfigValueAsString("log_level")

	conf.DatabaseHost = getConfigValueAsString("database_host")
	conf.Location = getConfigValueAsString("location")
}

func GetConfig() (*Config, error) {
	viper.SetConfigName("configuration")
	viper.SetConfigType("json")
	viper.AddConfigPath(".")
	configureViperDefaults()

	err := viper.ReadInConfig()
	if err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if errors.As(err, &configFileNotFoundError) {
			logs.Warn().Err(err).Msg("configuration file not found")
		}
	}

	var conf Config
	populateConfigurations(&conf)

	validate := validator.New()
	if err = validate.Struct(conf); err != nil {
		return nil, err
	}
	return &conf, nil
}
