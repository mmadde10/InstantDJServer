package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var mongoClient *mongo.Client

func main() {

	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	clientOptions := options.Client().ApplyURI("")
	mongoClient, err = mongo.Connect(ctx, clientOptions)

	if err != nil {
		log.Fatal("Cannot connext")
	}

	r := router()
	fmt.Println("Starting server on the port 8080...")

	log.Fatal(http.ListenAndServe(":8080", r))
}
