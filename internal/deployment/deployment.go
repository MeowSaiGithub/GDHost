package deployment

import (
	"GDHost/internal/database"
	"GDHost/internal/model"
	"GDHost/internal/response"
	"GDHost/internal/utility"
	"context"
	"errors"
	"fmt"
	"github.com/docker/docker/client"
	"github.com/gin-contrib/requestid"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

type Deployment interface {
	GenerateGoDockerfile(c *gin.Context)
	CreateDeployment(c *gin.Context)
	CreateDeploymentImage(c *gin.Context)
	RunDeployment(c *gin.Context)
	StopDeployment(c *gin.Context)
	DeleteDeploymentContainer(c *gin.Context)
	GetDeployment(c *gin.Context)
	GetDeployments(c *gin.Context)
	GetLogs(c *gin.Context)
	DeleteDeployment(c *gin.Context)
	UploadDockerfile(c *gin.Context)
	DownloadDockerfile(c *gin.Context)
}

type deployment struct {
	location string
	df       Dockerfile
	db       database.Database
	ctr      *container
	logger   *zerolog.Logger
}

// NewDeploymentController creates a new container controller and dockerfile controller and return Deployment
func NewDeploymentController(location string, db database.Database, logger *zerolog.Logger) (Deployment, error) {
	ctr, err := newContainerController()

	return &deployment{
		location: location,
		df:       NewDockerfileController(location),
		db:       db,
		ctr:      ctr,
		logger:   logger,
	}, err
}

type CreateDeploymentReq struct {
	Version string `json:"version" validate:"required"`
	OS      string `json:"os,omitempty"`
	GOARCH  string `json:"arch,omitempty"`
	CGO     bool   `json:"cgo,omitempty"`
	TAG     string `json:"tag,omitempty"`
	OUTPUT  string `json:"output,omitempty"`
	FLAGS   string `json:"flags,omitempty"`
	CMD     string `json:"cmd,omitempty"`
}

const (
	goDefaultOS     = "linux"
	goDefaultArch   = "amd64"
	goDefaultOutput = "application"
	goDefaultCMD    = "./application"
)

func (d *deployment) CreateDeployment(c *gin.Context) {
	logger := d.logger.With().Str("request_id", requestid.Get(c)).Logger()

	name := c.PostForm("name")
	if name == "" {
		logger.Error().Msg("no name in request")
		response.StatusBadRequest(c, "name missing in request")
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		logger.Error().Err(err).Msg("failed to get file from request")
		response.StatusBadRequest(c, "failed to get file from request")
		return
	}

	ext := filepath.Ext(file.Filename)
	if ext != ".zip" {
		logger.Error().Str("ext", ext).Msg("file is not zip type")
		response.StatusBadRequest(c, "file must be 'zip' type")
		return
	}

	ctx := c.Request.Context()

	filter := bson.D{
		{"name", name},
		{"deleted_at", time.Time{}},
	}

	_, err = d.db.FindDeployment(ctx, &filter, nil)
	if err != nil {
		if !errors.Is(err, mongo.ErrNoDocuments) {
			logger.Error().Err(err).Msg("failed to find deployment")
			response.StatusInternalServerError(c)
			return
		}
	} else {
		logger.Error().Msg("duplicated name")
		response.StatusConflicted(c, "duplicated name")
		return
	}

	session, txnOptions, err := d.db.CreateSession()
	if err != nil {
		logger.Error().Err(err).Msg("failed to create database session")
		response.StatusInternalServerError(c)
		return
	}
	defer session.EndSession(ctx)
	depId := uuid.NewString()

	path := filepath.Join(d.location, depId)
	fpath := filepath.Join(d.location, depId, file.Filename)

	callback := func(sc mongo.SessionContext) (interface{}, error) {

		sc = mongo.NewSessionContext(ctx, session)

		dep := model.Deployment{
			Id:        depId,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			DeletedAt: time.Time{},
			Name:      name,
			Location:  fpath,
			Stage:     model.FileUpload,
		}

		if err = d.db.CreateDeployment(sc, &dep); err != nil {
			return nil, err
		}

		if err = utility.CreateFile(filepath.Join(path, "application")); err != nil {
			return nil, fmt.Errorf("failed to create deployment folder: %w", err)
		}

		if err = c.SaveUploadedFile(file, fpath); err != nil {
			return nil, fmt.Errorf("failed to save file in directory: %w", err)
		}

		return nil, nil
	}

	if _, err = session.WithTransaction(ctx, callback, txnOptions); err != nil {
		logger.Error().Err(err).Msg("failed to create deployment")
		if err2 := utility.DeleteAll(path); err2 != nil {
			if !errors.Is(err2, fs.ErrNotExist) {
				logger.Error().Err(err2).Msg("failed to clean up file")
			}
		}
		response.StatusBadRequest(c, "duplicated name")
		return
	}

	logger.Info().Str("deployment_id", depId).Msg("deployment created")
	response.StatusCommonOK(c, depId)
	return
}

func (d *deployment) GenerateGoDockerfile(c *gin.Context) {
	logger := d.logger.With().Str("request_id", requestid.Get(c)).Logger()

	deId := c.Param("id")
	if deId == "" {
		logger.Error().Msg("deployment not found")
		response.StatusBadRequest(c, "deployment not found")
		return
	}

	msg := CreateDeploymentReq{}
	if err := c.BindJSON(&msg); err != nil {
		logger.Error().Err(err).Msg("bad request")
		return
	}

	validate := validator.New()
	if err := validate.Struct(msg); err != nil {
		logger.Error().Err(err).Msg("request validation error")
		return
	}

	ctx := c.Request.Context()
	filter := bson.D{
		{"_id", deId},
		{"deleted_at", time.Time{}},
	}
	projection := bson.M{"_id": 1, "location": 1}
	opts := options.FindOne().SetProjection(projection)

	dep, err := d.db.FindDeployment(ctx, &filter, opts)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			logger.Error().Err(err).Str("deployment_id", deId).Msg("deployment not found")
			response.StatusNotFound(c, "deployment not found")
			return
		}
		logger.Error().Err(err).Str("deployment_id", deId).Msg("failed to find deployment")
		response.StatusInternalServerError(c)
		return
	}

	switch {
	case msg.OS == "":
		msg.OS = goDefaultOS
	case msg.GOARCH == "":
		msg.GOARCH = goDefaultArch
	case msg.OUTPUT == "":
		msg.OUTPUT = goDefaultOutput
	case msg.CMD == "":
		msg.CMD = goDefaultCMD
	}

	path := filepath.Join(filepath.Dir(dep.Location), "application", "Dockerfile")

	option := BuildOption{
		Lang:     Go,
		Location: path,
		GoBuildOption: GoBuildOption{
			GoVersion: msg.Version,
			Location:  "application",
			DLocation: "application",
			GOOS:      msg.OS,
			GOARCH:    msg.GOARCH,
			TAG:       msg.TAG,
			OUTPUT:    msg.OUTPUT,
			FLAGS:     msg.FLAGS,
		},
	}
	if msg.CGO {
		option.GoBuildOption.CGO = "CGO_ENABLED=1"
	} else {
		option.GoBuildOption.CGO = "CGO_ENABLED=0"
	}

	update := bson.D{
		{"$set",
			bson.D{
				{"updated_at", time.Now()},
				{"dockerfile", path},
				{"stage", model.DockerfileUpload},
			},
		},
	}

	session, txnOptions, err := d.db.CreateSession()
	if err != nil {
		logger.Error().Err(err).Msg("failed to create database session")
		response.StatusInternalServerError(c)
		return
	}
	defer session.EndSession(ctx)

	callback := func(sc mongo.SessionContext) (interface{}, error) {
		if err = d.db.UpdateDeployment(sc, &filter, &update); err != nil {
			return nil, fmt.Errorf("failed to update deployment: %w", err)
		}

		if err = d.df.createDockerfile(&option); err != nil {
			return nil, fmt.Errorf("failed to create a dockerfile: %w", err)
		}
		return nil, nil
	}

	if _, err = session.WithTransaction(ctx, callback, txnOptions); err != nil {
		logger.Error().Err(err).Msg("failed to create dockerfile")
		if err = utility.DeleteFile(path); err != nil {
			if !os.IsNotExist(err) {
				logger.Error().Err(err).Msg("failed to clean up dockerfile")
			}
		}
		response.StatusInternalServerError(c)
		return
	}

	logger.Info().Str("deployment_id", dep.Id).Msg("dockerfile created")
	response.StatusCommonOK(c, "dockerfile created")
	return
}

func (d *deployment) CreateDeploymentImage(c *gin.Context) {
	logger := d.logger.With().Str("request_id", requestid.Get(c)).Logger()

	depId := c.Param("id")
	if depId == "" {
		logger.Error().Msg("no deployment id in URL")
		response.StatusBadRequest(c, "no deployment id in URL")
		return
	}

	ctx := c.Request.Context()
	filter := bson.D{
		{"_id", depId},
		{"deleted_at", time.Time{}},
	}

	projection := bson.M{"_id": 1, "name": 1, "location": 1, "dockerfile": 1, "stage": 1, "image_id": 1}
	opts := options.FindOne().SetProjection(projection)
	dep, err := d.db.FindDeployment(ctx, &filter, opts)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			logger.Error().Err(err).Str("deployment_id", depId).Msg("deployment not found")
			response.StatusNotFound(c, "deployment not found")
			return
		}
		logger.Error().Err(err).Str("deployment_id", depId).Msg("failed to find deployment")
		response.StatusInternalServerError(c)
		return
	}

	if dep.Stage < model.DockerfileUpload {
		logger.Error().Str("deployment_id", depId).Msg("dockerfile has not been created/uploaded yet")
		response.StatusBadRequest(c, "dockerfile has not been created/upload yet")
		return
	}

	if dep.ImageId != "" {
		if dep.ContainerId != "" {
			logger.Error().Str("deployment_id", depId).Msg("found container, cannot delete old image")
			response.StatusUnProcessed(c, "found container, cannot create new image without deleting the container")
			return
		}

		if err = d.ctr.deleteImage(ctx, dep.ImageId); err != nil {
			logger.Error().Err(err).Str("deployment_id", depId).Msg("failed to remove old image")
			response.StatusInternalServerError(c)
			return
		}
	}

	dest := filepath.Join(filepath.Dir(dep.Location), "application")

	if err = utility.Unzip(dep.Location, dest); err != nil {
		logger.Error().Err(err).Str("deployment_id", depId).Msg("failed to unzip the source file")
		response.StatusInternalServerError(c)
		return
	}

	abfp, err := filepath.Abs(dest)
	if err != nil {
		logger.Error().Err(err).Str("deployment_id", depId).Msg("failed to get absolute file path")
		return
	}

	ilogs, err := d.ctr.buildImage(ctx, dep.Name, abfp, logger)
	if err != nil {
		logger.Error().Err(err).Str("deployment_id", depId).Msg("failed to build image")
		response.StatusInternalServerError(c)
		return
	}
	defer func() {
		if err2 := ilogs.Close(); err2 != nil {
			logger.Error().Err(err2).Msg("failed to close build logs")
		}
	}()

	buf := make([]byte, 1024)

	for {
		n, err := ilogs.Read(buf)
		if err != nil {
			if errors.Is(err, io.EOF) {
				c.SSEvent("message", "finished")
				c.Writer.Flush()
				break
			}
			logger.Error().Err(err).Str("deployment_id", depId).Msg("failed to read image build response")
			response.StatusInternalServerError(c)
			return
		}
		msg := string(buf[:n])
		c.SSEvent("message", msg)
		c.Writer.Flush()
	}

	ctx = context.Background()

	id, err := d.ctr.getImageId(ctx, dep.Name)
	if err != nil {
		logger.Error().Err(err).Str("deployment_id", depId).Msg("failed to get image id")
		return
	}

	update := bson.D{
		{"$set",
			bson.D{
				{"updated_at", time.Now()},
				{"stage", model.ImageCreated},
				{"image_id", id},
			}},
	}

	if err = d.db.UpdateDeployment(ctx, &filter, &update); err != nil {
		logger.Error().Err(err).Str("deployment_id", depId).Msg("failed to update deployment")
		return
	}

	if err = utility.RemoveExceptDockerfile(dest); err != nil {
		logger.Error().Err(err).Str("deployment_id", depId).Msg("failed to clean up extracted zip file")
	}

	logger.Info().Str("deployment_id", depId).Msg("image created for deployment")
	return
}

type runDeploymentReq struct {
	HostPort      int `json:"host_port,omitempty"`
	ContainerPort int `json:"container_port,omitempty"`
}

func (d *deployment) RunDeployment(c *gin.Context) {
	logger := d.logger.With().Str("request_id", requestid.Get(c)).Logger()

	depId := c.Param("id")
	if depId == "" {
		logger.Error().Msg("no deployment id in URL")
		response.StatusBadRequest(c, "no deployment id in URL")
		return
	}
	var req runDeploymentReq
	if err := c.BindJSON(&req); err != nil {
		logger.Error().Err(err).Msg("failed to bind request")
		response.StatusBadRequest(c, "failed to bind request")
		return
	}

	ctx := c.Request.Context()
	filter := bson.D{
		{"_id", depId},
		{"deleted_at", time.Time{}},
	}
	projection := bson.M{"_id": 1, "stage": 1, "name": 1, "container_id": 1}
	opts := options.FindOne().SetProjection(projection)
	dep, err := d.db.FindDeployment(ctx, &filter, opts)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			logger.Error().Err(err).Str("deployment_id", depId).Msg("deployment not found")
			response.StatusNotFound(c, "deployment not found")
			return
		}
		logger.Error().Err(err).Str("deployment_id", depId).Msg("failed to get deployment")
		response.StatusInternalServerError(c)
		return
	}

	if dep.Stage < model.ImageCreated {
		logger.Error().Str("deployment_id", depId).Msg("need to create image first")
		response.StatusInternalServerError(c)
		return
	}
	cid := dep.ContainerId

	if dep.ContainerId == "" {
		if req.HostPort == 0 {
			logger.Error().Str("deployment_id", depId).Msg("host port missing in request")
			response.StatusBadRequest(c, "host_port missing. host_port is required for the first time running")
			return
		}
		if req.ContainerPort == 0 {
			logger.Error().Str("deployment_id", depId).Msg("container_port missing in request")
			response.StatusBadRequest(c, "container_port missing, container_port is required for the first time running")
			return
		}

		hport := strconv.Itoa(req.HostPort)
		cport := strconv.Itoa(req.ContainerPort)

		cid, err = d.ctr.createContainer(ctx, dep.Name, hport, cport)
		if err != nil {
			logger.Error().Err(err).Str("deployment_id", depId).Msg("failed to create container")
			response.StatusInternalServerError(c)
			return

		}
		update := bson.D{
			{"$set", bson.D{
				{"updated_at", time.Now()},
				{"stage", model.ContainerCreated},
				{"container_id", cid},
			}},
		}
		if err = d.db.UpdateDeployment(ctx, &filter, &update); err != nil {
			logger.Error().Err(err).Str("deployment_id", depId).Msg("failed to update deployment")
			response.StatusInternalServerError(c)
			return
		}
	}

	if err = d.ctr.startContainer(ctx, cid); err != nil {
		logger.Error().Err(err).Str("deployment_id", depId).Msg("failed to start container")
		response.StatusInternalServerError(c)
		return
	}
	logger.Info().Str("deployment_id", depId).Msg("container started")
	response.StatusCommonOK(c, "deployment started")
	return
}

func (d *deployment) StopDeployment(c *gin.Context) {
	logger := d.logger.With().Str("request_id", requestid.Get(c)).Logger()

	depId := c.Param("id")
	if depId == "" {
		logger.Error().Msg("deployment id not found")
		response.StatusBadRequest(c, "deployment id not found")
		return
	}

	ctx := c.Request.Context()
	filter := bson.D{
		{"_id", depId},
		{"deleted_at", time.Time{}},
	}
	projection := bson.M{"_id": 1, "container_id": 1}
	opts := options.FindOne().SetProjection(projection)
	dep, err := d.db.FindDeployment(ctx, &filter, opts)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			logger.Error().Err(err).Str("deployment_id", depId).Msg("deployment not found")
			response.StatusNotFound(c, "deployment not found")
			return
		}
		logger.Error().Err(err).Str("deployment_id", depId).Msg("failed to find deployment")
		response.StatusInternalServerError(c)
		return
	}

	if err = d.ctr.stopContainer(ctx, dep.ContainerId); err != nil {
		logger.Error().Err(err).Str("deployment_id", depId).Msg("failed to stop deployment")
		response.StatusInternalServerError(c)
		return
	}

	logger.Info().Str("deployment_id", depId).Msg("deployment stopped")
	response.StatusCommonOK(c, "deployment stopped")
	return
}

func (d *deployment) DeleteDeploymentContainer(c *gin.Context) {
	logger := d.logger.With().Str("request_id", requestid.Get(c)).Logger()

	depId := c.Param("id")
	if depId == "" {
		logger.Error().Msg("deployment id not found")
		response.StatusBadRequest(c, "deployment id not found")
		return
	}

	ctx := c.Request.Context()
	filter := bson.D{
		{"_id", depId},
		{"deleted_at", time.Time{}},
	}
	projection := bson.M{"_id": 1, "container_id": 1}
	opts := options.FindOne().SetProjection(projection)
	dep, err := d.db.FindDeployment(ctx, &filter, opts)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			logger.Error().Err(err).Str("deployment_id", depId).Msg("deployment not found")
			response.StatusNotFound(c, "deployment not found")
			return
		}
		logger.Error().Err(err).Str("deployment_id", depId).Msg("failed to find deployment")
		response.StatusInternalServerError(c)
		return
	}

	if dep.ContainerId == "" {
		logger.Error().Str("deployment_id", depId).Msg("deployment container has not been created yet")
		response.StatusBadRequest(c, "deployment container has not been created yet")
		return
	}

	isRun, err := d.ctr.isContainerRunning(ctx, dep.ContainerId)
	if err != nil {
		logger.Error().Err(err).Str("deployment_id", depId).Msg("failed to inspect container")
		response.StatusInternalServerError(c)
		return

	}

	if isRun {
		logger.Error().Err(err).Str("deployment_id", depId).Msg("container is still running")
		response.StatusUnProcessed(c, "container is still running")
		return
	}

	sess, tnxOption, err := d.db.CreateSession()
	if err != nil {
		logger.Error().Err(err).Str("deployment_id", depId).Msg("failed to create session")
		response.StatusInternalServerError(c)
		return
	}
	defer sess.EndSession(ctx)

	callback := func(sc mongo.SessionContext) (interface{}, error) {

		sc = mongo.NewSessionContext(ctx, sess)

		update := bson.D{
			{"$set", bson.D{
				{"updated_at", time.Now()},
			}},
			{"$unset", bson.D{
				{"container_id", ""},
			}},
		}
		if err = d.db.UpdateDeployment(sc, &filter, &update); err != nil {
			return nil, fmt.Errorf("failed to update deployment: %w", err)
		}

		if err = d.ctr.removeContainer(sc, dep.ContainerId); err != nil {
			return nil, fmt.Errorf("failed to remove container: %w", err)
		}
		return nil, nil
	}

	if _, err = sess.WithTransaction(ctx, callback, tnxOption); err != nil {
		logger.Error().Err(err).Str("deployment_id", depId).Msg("failed to remove deployment container")
		response.StatusInternalServerError(c)
		return
	}

	logger.Info().Str("deployment_id", depId).Msg("deployment container removed")
	response.StatusCommonOK(c, "deployment container removed")
	return
}

func (d *deployment) GetDeployments(c *gin.Context) {
	logger := d.logger.With().Str("request_id", requestid.Get(c)).Logger()

	ctx := c.Request.Context()

	limitStr := c.DefaultQuery("limit", "10")
	limit, err := strconv.Atoi(limitStr)
	if err != nil {
		logger.Error().Err(err).Msg("invalid limit")
		response.StatusBadRequest(c, "invalid limit")
		return
	}

	pageStr := c.DefaultQuery("page", "1")
	page, err := strconv.Atoi(pageStr)
	if err != nil {
		logger.Error().Err(err).Msg("invalid page")
		response.StatusBadRequest(c, "invalid page")
		return
	}

	filter := bson.D{
		{"deleted_at", time.Time{}},
	}
	projection := bson.M{"_id": 1, "created_at": 1, "updated_at": 1, "name": 1, "stage": 1}
	opts := options.Find().SetProjection(projection).SetLimit(int64(limit)).SetSkip(int64(page - 1))

	deps, err := d.db.FindDeployments(ctx, &filter, opts)
	if err != nil {
		logger.Error().Err(err).Msg("failed to find deployments")
		response.StatusInternalServerError(c)
		return
	}

	if len(*deps) == 0 {
		logger.Info().Msg("no deployments")
		response.StatusNoContent(c)
		return
	}

	logger.Info().Int("deployments", len(*deps)).Msg("deployments sent")
	response.StatusDeployments(c, deps)
	return
}

func (d *deployment) GetDeployment(c *gin.Context) {
	logger := d.logger.With().Str("request_id", requestid.Get(c)).Logger()

	depId := c.Param("id")
	if depId == "" {
		logger.Error().Msg("no deployment id")
		response.StatusBadRequest(c, "no deployment id")
		return
	}

	ctx := c.Request.Context()
	filter := bson.D{
		{"_id", depId},
		{"deleted_at", time.Time{}},
	}
	projection := bson.M{"_id": 1, "created_at": 1, "updated_at": 1, "name": 1, "stage": 1}
	opts := options.FindOne().SetProjection(projection)
	dep, err := d.db.FindDeployment(ctx, &filter, opts)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			logger.Error().Err(err).Str("deployment_id", depId).Msg("deployment not found")
			response.StatusNotFound(c, "deployment not found")
			return
		}
		logger.Error().Err(err).Str("deployment_id", depId).Msg("failed to find deployment")
		response.StatusInternalServerError(c)
		return
	}
	logger.Info().Str("deployment_id", depId).Msg("deployment sent")
	response.StatusDeployment(c, dep)
	return
}

func (d *deployment) GetLogs(c *gin.Context) {
	logger := d.logger.With().Str("request_id", requestid.Get(c)).Logger()

	depId := c.Param("id")
	if depId == "" {
		logger.Error().Msg("no deployment id")
		response.StatusBadRequest(c, "no deployment id")
		return
	}

	ctx := c.Request.Context()
	filter := bson.D{
		{"_id", depId},
		{"deleted_at", time.Time{}},
	}
	projection := bson.M{"_id": 1, "container_id": 1, "stage": 1}
	opts := options.FindOne().SetProjection(projection)
	dep, err := d.db.FindDeployment(ctx, &filter, opts)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			logger.Error().Err(err).Str("deployment_id", depId).Msg("deployment not found")
			response.StatusNotFound(c, "deployment not found")
			return
		}
		logger.Error().Err(err).Str("deployment_id", depId).Msg("failed to find deployment")
		response.StatusInternalServerError(c)
		return
	}

	if dep.Stage < model.ContainerCreated {
		logger.Info().Str("deployment_id", depId).Msg("container has not been created yet")
		response.StatusUnProcessed(c, "container has not been created yet")
		return
	}

	if dep.ContainerId == "" {
		logger.Error().Err(err).Str("deployment_id", depId).Msg("container id missing")
		response.StatusInternalServerError(c)
		return
	}

	clogs, err := d.ctr.getContainerLogs(ctx, dep.ContainerId)
	if err != nil {
		logger.Error().Err(err).Str("deployment_id", depId).Msg("failed to get container logs")
		response.StatusInternalServerError(c)
		return
	}

	defer func() {
		if err = clogs.Close(); err != nil {
			logger.Error().Err(err).Msg("failed to close container log")
		}
	}()
	buf := make([]byte, 1024)
	for {
		n, err := clogs.Read(buf)
		if err != nil {
			logger.Error().Err(err).Str("deployment_id", depId).Msg("failed to read container log")
			response.StatusInternalServerError(c)
			break
		}
		if n == 0 {
			msg := "No log data available"
			c.SSEvent("message", msg)
			c.Writer.Flush()
			continue
		}
		msg := string(buf[:n])
		c.SSEvent("message", msg)
		c.Writer.Flush()
	}
}

func (d *deployment) DeleteDeployment(c *gin.Context) {
	logger := d.logger.With().Str("request_id", requestid.Get(c)).Logger()

	depId := c.Param("id")
	if depId == "" {
		logger.Error().Msg("no deployment id")
		response.StatusBadRequest(c, "no deployment id")
		return
	}

	ctx := c.Request.Context()
	filter := bson.D{
		{"_id", depId},
		{"deleted_at", time.Time{}},
	}
	projection := bson.M{"_id": 1, "container_id": 1, "stage": 1, "image_id": 1}
	opts := options.FindOne().SetProjection(projection)
	dep, err := d.db.FindDeployment(ctx, &filter, opts)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			logger.Error().Err(err).Str("deployment_id", depId).Msg("deployment not found")
			response.StatusNotFound(c, "deployment not found")
			return
		}
		logger.Error().Err(err).Str("deployment_id", depId).Msg("failed to find deployment")
		response.StatusInternalServerError(c)
		return
	}

	sess, txnOptions, err := d.db.CreateSession()
	if err != nil {
		logger.Error().Err(err).Str("deployment_id", depId).Msg("failed to create session")
		response.StatusInternalServerError(c)
		return
	}
	defer sess.EndSession(ctx)

	callback := func(sc mongo.SessionContext) (interface{}, error) {

		sc = mongo.NewSessionContext(ctx, sess)

		update := bson.D{
			{"$set", bson.D{
				{"updated_at", time.Now()},
				{"deleted_at", time.Now()},
			}},
		}

		if err = d.db.UpdateDeployment(sc, &filter, &update); err != nil {
			logger.Error().Err(err).Str("deployment_id", depId).Msg("failed to delete deployment")
			response.StatusInternalServerError(c)
			return nil, err
		}

		path := filepath.Join(d.location, depId)
		if err = utility.DeleteAll(path); err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				return nil, fmt.Errorf("failed to remove files: %w", err)
			}
		}

		if dep.ContainerId != "" {
			running, err := d.ctr.isContainerRunning(sc, dep.ContainerId)
			if err != nil {
				if client.IsErrNotFound(err) {
					logger.Warn().Str("deployment_id", depId).Str("container_id", dep.ContainerId).Msg("container not found")
				}
				return nil, fmt.Errorf("failed to get container stage: %w", err)
			}
			if running {
				if err = d.ctr.stopContainer(sc, dep.ContainerId); err != nil {
					return nil, fmt.Errorf("failed to stop container: %w", err)
				}
				if err = d.ctr.removeContainer(sc, dep.ContainerId); err != nil {
					return nil, fmt.Errorf("failed to remove container: %w", err)
				}
			}
		}

		if dep.Stage > model.ImageCreated {
			if dep.ImageId == "" {
				logger.Warn().Str("deployment_id", depId).Msg("image id is empty")
			} else {
				if err = d.ctr.deleteImage(sc, dep.ImageId); err != nil {
					return nil, fmt.Errorf("failed to delete image: %w", err)
				}
			}

		}
		return nil, nil
	}

	if _, err = sess.WithTransaction(ctx, callback, txnOptions); err != nil {
		logger.Error().Err(err).Str("deployment_id", depId).Msg("failed to delete deployment")
		response.StatusInternalServerError(c)
		return
	}

	logger.Info().Str("deployment_id", depId).Str("deployment_id", depId).Msg("deployment deleted")
	response.StatusCommonOK(c, "deployment deleted")
	return
}

func (d *deployment) UploadDockerfile(c *gin.Context) {
	logger := d.logger.With().Str("request_id", requestid.Get(c)).Logger()

	depId := c.Param("id")
	if depId == "" {
		logger.Error().Msg("deployment id not found")
		response.StatusBadRequest(c, "deployment id not found")
		return
	}

	ctx := c.Request.Context()
	filter := bson.D{
		{"_id", depId},
		{"deleted_at", time.Time{}},
	}

	_, err := d.db.FindDeployment(ctx, &filter, nil)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			logger.Error().Err(err).Str("deployment_id", depId).Msg("deployment not found")
			response.StatusInternalServerError(c)
			return
		}
		logger.Error().Err(err).Str("deployment_id", depId).Msg("failed to find deployment")
		response.StatusInternalServerError(c)
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		logger.Error().Err(err).Str("deployment_id", depId).Msg("failed to get file from request")
		response.StatusBadRequest(c, "failed to get file from request")
		return
	}
	if file.Filename != "Dockerfile" {
		logger.Error().Str("deployment_id", depId).Msg("file name must be Dockerfile")
		response.StatusBadRequest(c, "file name must be Dockerfile")
		return
	}

	path := filepath.Join(d.location, depId, "application", "Dockerfile")
	sess, txnOptions, err := d.db.CreateSession()
	if err != nil {
		logger.Error().Err(err).Str("deployment_id", depId).Msg("failed to create session")
		response.StatusInternalServerError(c)
		return
	}
	defer sess.EndSession(ctx)
	callback := func(sc mongo.SessionContext) (interface{}, error) {
		update := bson.D{
			{"$set", bson.D{
				{"updated_at", time.Now()},
				{"stage", model.DockerfileUpload},
				{"dockerfile", path},
			}},
		}
		if err = d.db.UpdateDeployment(sc, &filter, &update); err != nil {
			return nil, fmt.Errorf("failed to update deployment: %w", err)
		}

		if err = c.SaveUploadedFile(file, path); err != nil {
			return nil, fmt.Errorf("failed to save dockerfile: %w", err)
		}

		return nil, nil
	}
	if _, err = sess.WithTransaction(ctx, callback, txnOptions); err != nil {
		logger.Error().Err(err).Str("deployment_id", depId).Msg("failed to upload dockerfile")
		response.StatusInternalServerError(c)
		return
	}

	logger.Info().Str("deployment_id", depId).Msg("dockerfile uploaded")
	response.StatusCommonOK(c, "dockerfile uploaded")
	return
}

func (d *deployment) DownloadDockerfile(c *gin.Context) {
	logger := d.logger.With().Str("request_id", requestid.Get(c)).Logger()

	depId := c.Param("id")
	if depId == "" {
		logger.Error().Msg("deployment id not found")
		response.StatusBadRequest(c, "deployment id not found")
		return
	}

	ctx := c.Request.Context()
	filter := bson.D{
		{"_id", depId},
		{"deleted_at", time.Time{}},
	}
	projection := bson.M{"stage": 1, "dockerfile": 1}
	opts := options.FindOne().SetProjection(projection)
	dep, err := d.db.FindDeployment(ctx, &filter, opts)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			logger.Error().Err(err).Str("deployment_id", depId).Msg("deployment not found")
			response.StatusNotFound(c, "deployment not found")
			return
		}
		logger.Error().Err(err).Str("deployment_id", depId).Msg("failed to find deployment")
		response.StatusInternalServerError(c)
		return
	}

	if dep.Dockerfile == "" || dep.Stage < model.DockerfileUpload {
		logger.Error().Str("deployment_id", depId).Msg("dockerfile has not been created/uploaded yet")
		response.StatusUnProcessed(c, "dockerfile has not been created/uploaded yet")
		return
	}

	c.File(dep.Dockerfile)
	logger.Info().Str("deployment_id", depId).Msg("dockerfile sent")
	return
}
