package model

import "time"

type Deployment struct {
	Id          string    `bson:"_id"`
	CreatedAt   time.Time `bson:"created_at"`
	UpdatedAt   time.Time `bson:"updated_at"`
	DeletedAt   time.Time `bson:"deleted_at"`
	Name        string    `bson:"name"`
	Location    string    `bson:"location"`
	Dockerfile  string    `bson:"dockerfile,omitempty"`
	ImageId     string    `bson:"image_id,omitempty"`
	Stage       stage     `bson:"stage"`
	ContainerId string    `bson:"container_id,omitempty"`
}

type stage int

const (
	None stage = iota * 2
	FileUpload
	DockerfileUpload
	ImageCreated
	ContainerCreated
	Run
)

func (s stage) String() string {
	switch s {
	case None:
		return "None"
	case FileUpload:
		return "File Uploaded"
	case DockerfileUpload:
		return "Dockerfile Uploaded/Created"
	case ImageCreated:
		return "ImageId Created"
	case ContainerCreated:
		return "Container Created"
	case Run:
		return "Run"
	default:
		return "Unknown"
	}

}

type Dockerfile struct {
	Id   string `bson:"_id"`
	Data string `bson:"data"`
}
