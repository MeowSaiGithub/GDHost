package api

import (
	"GDHost/internal/database"
	"GDHost/internal/deployment"
	"context"
	"errors"
	"github.com/gin-contrib/logger"
	"github.com/gin-contrib/requestid"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"net/http"
	"os"
	"time"
)

type Server interface {
	SetUpRouter(db database.Database) error
	Run() error
	Shutdown(ctx context.Context, cancel context.CancelFunc, sig chan os.Signal)
}

type server struct {
	path     string
	srv      *http.Server
	host     string
	logger   *zerolog.Logger
	location string
}

func NewServer(path, host, location string, logger *zerolog.Logger) Server {
	return &server{
		path:     path,
		host:     host,
		logger:   logger,
		location: location,
	}
}

func (s *server) SetUpRouter(db database.Database) error {
	r := gin.New()
	r.Use(requestid.New())
	r.Use(logger.SetLogger())
	r.Use(gin.Recovery())

	dcontroller, err := deployment.NewDeploymentController(s.location, db, s.logger)
	if err != nil {
		return err
	}

	dep := r.Group(s.path + "/deployments")
	{
		dep.POST("/create", dcontroller.CreateDeployment)
		dep.POST("/:id/dockerfile/go", dcontroller.GenerateGoDockerfile)
		dep.POST("/:id/image", dcontroller.CreateDeploymentImage)
		dep.POST("/:id/run", dcontroller.RunDeployment)
		dep.POST("/:id/stop", dcontroller.StopDeployment)
		dep.DELETE("/:id/container", dcontroller.DeleteDeploymentContainer)
		dep.GET("/:id/log", dcontroller.GetLogs)
		dep.GET("/:id", dcontroller.GetDeployment)
		dep.GET("/", dcontroller.GetDeployments)
		dep.DELETE("/:id", dcontroller.DeleteDeployment)
		dep.GET("/:id/dockerfile", dcontroller.DownloadDockerfile)
		dep.POST("/:id/dockerfile", dcontroller.UploadDockerfile)
	}

	s.srv = &http.Server{
		Addr:              s.host,
		Handler:           r,
		ReadHeaderTimeout: 30 * time.Second,
		WriteTimeout:      30 * time.Second,
	}
	return nil
}

func (s *server) Run() error {
	return s.srv.ListenAndServe()
}

func (s *server) Shutdown(ctx context.Context, cancel context.CancelFunc, sig chan os.Signal) {
	<-sig
	defer cancel()

	go func() {
		<-ctx.Done()
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			s.logger.Fatal().Err(ctx.Err()).Msg("graceful shutdown timed out.. forcing exit")
		}
	}()
	if err := s.srv.Shutdown(ctx); err != nil {
		s.logger.Fatal().Err(err).Msg("failed to shutdown server")
	}
}
