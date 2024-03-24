package database

import (
	"GDHost/internal/model"
	"context"
	"fmt"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"
)

type Database interface {
	CloseConnection() error
	CreateDeployment(ctx context.Context, deployment *model.Deployment) error
	FindDeployment(ctx context.Context, filter *bson.D, opts *options.FindOneOptions) (*model.Deployment, error)
	UpdateDeployment(ctx context.Context, filter *bson.D, update *bson.D) error
	CreateSession() (mongo.Session, *options.TransactionOptions, error)
	FindDeployments(ctx context.Context, filter *bson.D, opts *options.FindOptions) (*[]model.Deployment, error)
}
type database struct {
	client      *mongo.Client
	deployments *mongo.Collection
}

func NewDatabaseConnection(host string) (Database, error) {
	db := database{}
	if err := db.connectDatabase(host); err != nil {
		return nil, err
	}
	return &db, nil
}

func (d *database) connectDatabase(host string) error {
	var err error
	ctx := context.Background()
	if d.client, err = mongo.Connect(ctx, options.Client().ApplyURI(host)); err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	if err = d.client.Ping(ctx, readpref.Primary()); err != nil {
		return fmt.Errorf("failed to ping to database: %w", err)
	}
	d.deployments = d.client.Database("gdhost").Collection("deployments")

	indexModel := mongo.IndexModel{
		Keys: bson.M{"deleted_at": 1},
	}
	if _, err = d.deployments.Indexes().CreateOne(ctx, indexModel); err != nil {
		return fmt.Errorf("failed to create index: %w", err)
	}

	return nil
}

func (d *database) CloseConnection() error {
	return d.client.Disconnect(context.Background())
}

func (d *database) CreateDeployment(ctx context.Context, deployment *model.Deployment) error {
	_, err := d.deployments.InsertOne(ctx, deployment)
	return err
}

func (d *database) FindDeployments(ctx context.Context, filter *bson.D, opts *options.FindOptions) (*[]model.Deployment, error) {
	deployments := &[]model.Deployment{}
	cursor, err := d.deployments.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("find error: %w", err)
	}
	defer cursor.Close(ctx)
	if err = cursor.All(ctx, deployments); err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}
	return deployments, nil

}

func (d *database) FindDeployment(ctx context.Context, filter *bson.D, opts *options.FindOneOptions) (*model.Deployment, error) {
	deployment := &model.Deployment{}
	err := d.deployments.FindOne(ctx, filter, opts).Decode(deployment)
	return deployment, err
}

func (d *database) UpdateDeployment(ctx context.Context, filter *bson.D, update *bson.D) error {
	_, err := d.deployments.UpdateOne(ctx, filter, update)
	return err
}

func (d *database) CreateSession() (mongo.Session, *options.TransactionOptions, error) {
	wc := writeconcern.Majority()
	txnOptions := options.Transaction().SetWriteConcern(wc)
	session, err := d.client.StartSession()
	return session, txnOptions, err
}
