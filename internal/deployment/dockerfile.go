package deployment

import (
	"fmt"
	logs "github.com/rs/zerolog/log"
	"os"
	"text/template"
)

type Dockerfile interface {
	createDockerfile(option *BuildOption) error
}

type dockerfile struct {
	location   string
	goTemplate *template.Template
}

func NewDockerfileController(location string) Dockerfile {
	return &dockerfile{
		location:   location,
		goTemplate: template.Must(template.ParseFiles("internal/template/dockerfile-go.tmpl")),
	}
}

type BuildOption struct {
	Lang          Language
	Location      string
	GoBuildOption GoBuildOption
}

type Language int

const (
	Go Language = iota
)

type GoBuildOption struct {
	GoVersion string
	Location  string
	DLocation string
	GOOS      string
	GOARCH    string
	CGO       string
	TAG       string
	OUTPUT    string
	FLAGS     string
}

func (df *dockerfile) createDockerfile(option *BuildOption) error {
	file, err := os.Create(option.Location)
	if err != nil {
		return fmt.Errorf("failed to create dockerfile: %w", err)
	}
	defer func() {
		if err = file.Close(); err != nil {
			logs.Error().Err(err).Msg("failed to close dockerfile")
		}
	}()
	switch option.Lang {
	case Go:
		if err = df.goTemplate.Execute(file, option.GoBuildOption); err != nil {
			return fmt.Errorf("failed to execute 'Go' template: %w", err)
		}
	default:
		return fmt.Errorf("invalid language option")
	}
	return nil
}
