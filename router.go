package main

import (
	"github.com/gorilla/mux"
)

// Router is exported and used in main.go
func router() *mux.Router {

	router := mux.NewRouter()

	router.HandleFunc("/api/info", info).Methods("GET", "OPTIONS")

	router.HandleFunc("/api/autenticate", authenticateUser).Methods("GET", "OPTIONS")
	router.HandleFunc("/api/callback", completeAuth).Methods("GET", "OPTIONS")

	//Track routes
	router.HandleFunc("/api/tracks/{id}", getTrack).Methods("GET", "OPTIONS")

	//Search Route
	router.HandleFunc("/api/search/{query}", getSearchResults).Methods("GET", "OPTIONS")

	//Queue routes
	// Create Queue
	router.HandleFunc("/api/queue", createQueue).Methods("POST", "OPTIONS")
	router.HandleFunc("/api/queue/{id}", getQueue).Methods("GET", "OPTIONS")
	router.HandleFunc("/api/queue/{id}", updateQueue).Methods("POST", "OPTIONS")

	return router
}
