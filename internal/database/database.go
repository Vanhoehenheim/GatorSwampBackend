// internal/database/mongodb.go
package database

import (
	"context"
	"fmt"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MongoDB struct {
	Client     *mongo.Client
	Users      *mongo.Collection
	Posts      *mongo.Collection
	Comments   *mongo.Collection
	Subreddits *mongo.Collection
	Messages   *mongo.Collection
}

func NewMongoDB(uri string) (*MongoDB, error) {
	serverAPI := options.ServerAPI(options.ServerAPIVersion1)
	opts := options.Client().ApplyURI(uri).SetServerAPIOptions(serverAPI)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %v", err)
	}

	// Ping the database to verify connection
	if err := client.Database("admin").RunCommand(ctx, bson.D{{"ping", 1}}).Err(); err != nil {
		return nil, fmt.Errorf("failed to ping MongoDB: %v", err)
	}

	log.Println("Successfully connected to MongoDB!")

	// Initialize database and collections
	db := client.Database("gator_swamp")
	return &MongoDB{
		Client:     client,
		Users:      db.Collection("users"),
		Posts:      db.Collection("posts"),
		Comments:   db.Collection("comments"),
		Subreddits: db.Collection("subreddits"),
		Messages:   db.Collection("messages"),
	}, nil
}

func (m *MongoDB) Close(ctx context.Context) error {
	return m.Client.Disconnect(ctx)
}
