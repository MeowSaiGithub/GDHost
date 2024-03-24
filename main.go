package main

import (
	"GDHost/api"
	"GDHost/internal/config"
	"GDHost/internal/database"
	"GDHost/internal/logger"
	"context"
	"errors"
	"flag"
	"github.com/rs/zerolog"
	logs "github.com/rs/zerolog/log"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

type app struct {
	logger *zerolog.Logger
	conf   *config.Config
	server api.Server
	db     database.Database
}

func main() {
	flag.String("conf", `configuration.json`, "Configuration file path")
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()
	_ = viper.BindPFlags(pflag.CommandLine)

	var err error
	a := app{}
	if a.conf, err = config.GetConfig(); err != nil {
		logs.Fatal().Err(err).Msg("configuration error")
	}
	logs.Info().Msg("configuration loaded")

	a.logger = logger.InitLog(a.conf.LogLevel)

	if a.db, err = database.NewDatabaseConnection(a.conf.DatabaseHost); err != nil {
		a.logger.Fatal().Err(err).Msg("database error")
	}
	a.logger.Info().Msg("database connected")

	host := ":" + strconv.Itoa(a.conf.Port)
	a.server = api.NewServer(a.conf.APIPath, host, a.conf.Location, a.logger)
	if err = a.server.SetUpRouter(a.db); err != nil {
		a.logger.Fatal().Err(err).Msg("failed to set up the router")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	a.logger.Info().Msgf("basic path: %s", a.conf.APIPath)
	a.logger.Info().Msgf("listening at port: %s", host)

	go func() {
		if err = a.server.Run(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			a.logger.Error().Err(err).Msg("server error occurred")
		}
	}()
	a.server.Shutdown(ctx, cancel, sig)
	if err = a.db.CloseConnection(); err != nil {
		a.logger.Error().Err(err).Msg("failed to close database connection")
	}
	a.logger.Info().Msg("server exited properly.")
}
