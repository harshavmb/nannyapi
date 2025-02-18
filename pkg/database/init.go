package database

import (
	"context"
	"log"
	"os"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func InitDB() (*mongo.Database, error) {
	var mongodb_uri string
	if mongodb_uri = os.Getenv("MONGODB_URI"); mongodb_uri == "" {
		log.Fatal("You must set your 'MONGODB_URI' environment variable. See\n\t https://www.mongodb.com/docs/drivers/go/current/usage-examples/#environment-variable")
	}

	client, err := mongo.Connect(options.Client().ApplyURI(mongodb_uri))
	if err != nil {
		log.Fatal("error while connecting to mongodb: ", err)
		return nil, err
	}
	// defer func() {
	// 	if err = client.Disconnect(context.Background()); err != nil {
	// 		log.Fatal("error while disconnecting from mongodb: ", err)
	// 	}
	// }()

	mongoDB := client.Database("nannyai")

	err = client.Ping(context.Background(), nil)
	if err != nil {
		log.Fatal("error while pinging mongodb: ", err)
		return nil, err
	}
	log.Println("Connected to MongoDB!")
	return mongoDB, nil
}
