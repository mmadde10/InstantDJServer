package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/gorilla/mux"
	"github.com/zmb3/spotify"
	"go.mongodb.org/mongo-driver/bson"
	"golang.org/x/oauth2"
)

var stateToken, err = generateRandomString(32)

//TODO: change dep on env, check if null
const redirectURI = "http://localhost:8080/api/callback"

var clientID = os.Getenv("spotifyClientID")
var secretKey = os.Getenv("spotifySecretKey")

var (
	auth  = spotify.NewAuthenticator(redirectURI, spotify.ScopeUserReadEmail)
	ch    = make(chan *spotify.Client)
	state = stateToken
	token = make(chan *oauth2.Token)
)

// Will use token and get user info, or return an error
func validateUser(token string) (bool, interface{}, error) {
	client := &http.Client{}
	url := "https://api.spotify.com/v1/me"

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Add("Authorization", token)
	resp, _ := client.Do(req)

	var f interface{}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)
	error := json.Unmarshal(body, &f)

	if error != nil {
		log.Fatal(error)
	}

	if resp.StatusCode == 200 {
		return true, f, nil
	}

	return false, nil, nil
}

// AuthenticateUser oauth endpoint
func authenticateUser(w http.ResponseWriter, r *http.Request) {
	auth.SetAuthInfo(clientID, secretKey)
	url := auth.AuthURL(state)
	fmt.Println("Please log in to Spotify by visiting the following page in your browser:", url)
	// wait for auth to complete
	client := <-ch
	tok := <-token

	// use the client to make calls that require authorization
	user, err := client.CurrentUser()
	if err != nil {
		log.Fatal(err)
	}

	var AuthUser User
	filter := bson.D{{
		"spotifyid",
		bson.D{{
			"$in",
			bson.A{user.ID},
		}},
	}}

	db := mongoClient.Database("instant_dj_dev")
	collection := db.Collection("users")
	ctx, _ := context.WithTimeout(context.Background(), 30*time.Second)
	error := collection.FindOne(ctx, filter).Decode(&AuthUser)

	if error != nil {
		log.Fatal(error)
	}
	AuthInfo := User{
		SpotifyID:    user.ID,
		Name:         user.DisplayName,
		AccessToken:  tok.AccessToken,
		RefreshToken: tok.RefreshToken,
		Email:        user.Email,
	}

	fmt.Println("\n You are logged in as:", user.ID)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(AuthInfo)
}

// CompleteAuth Auth user
func completeAuth(w http.ResponseWriter, r *http.Request) {
	tok, err := auth.Token(state, r)
	if err != nil {
		http.Error(w, "Couldn't get token", http.StatusForbidden)
		log.Fatal(err)
	}
	if st := r.FormValue("state"); st != state {
		http.NotFound(w, r)
		log.Fatalf("State mismatch: %s != %s\n", st, state)
	}
	// use the token to get an authenticated client
	client := auth.NewClient(tok)
	fmt.Fprintf(w, "Login Completed!")
	ch <- &client
	token <- tok
}

//GetTrack Checks for token, then gets track by ID
func getTrack(w http.ResponseWriter, r *http.Request) {
	client := &http.Client{}
	tok := r.Header["Authorization"][0]

	params := mux.Vars(r)
	id := params["id"]

	url := "https://api.spotify.com/v1/tracks/" + id

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Add("Authorization", tok)
	resp, _ := client.Do(req)

	if resp.StatusCode == 200 {

		defer resp.Body.Close()

		body, _ := ioutil.ReadAll(resp.Body)

		var TrackResult Track
		err := json.Unmarshal(body, &TrackResult)

		if err != nil {
			log.Fatal(err)
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(TrackResult)
	}
	if resp.StatusCode == 401 {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("401 - Unauthorized"))
		return
	}
	if resp.StatusCode == 400 {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("400 - Bad Request"))
	}
}

// GetSearchResults - Takes in a query,
func getSearchResults(w http.ResponseWriter, r *http.Request) {

	params := mux.Vars(r)
	query := params["query"]

	encodedQuery := url.QueryEscape(query)

	baseURL, err := url.Parse("https://api.spotify.com/v1/search")
	if err != nil {
		fmt.Println("Malformed URL: ", err.Error())
		return
	}

	client := &http.Client{}
	tok := r.Header["Authorization"][0]

	newParams := url.Values{}
	newParams.Add("q", encodedQuery)
	newParams.Add("type", "track,artist")
	newParams.Add("market", "US")
	newParams.Add("limit", "10")
	newParams.Add("offset", "5")

	baseURL.RawQuery = newParams.Encode()

	req, _ := http.NewRequest("GET", baseURL.String(), nil)
	req.Header.Add("Authorization", tok)
	resp, _ := client.Do(req)

	defer resp.Body.Close()

	//var SearchResult map[string]string
	var f interface{}
	body, _ := ioutil.ReadAll(resp.Body)
	error := json.Unmarshal(body, &f)

	if error != nil {
		log.Fatal(error)
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(f)
}

// Queue route handlers
// Post, header needs to contain authorization, takes in token, hit an isAuthorized, https://api.spotify.com/v1/me

func createQueue(w http.ResponseWriter, r *http.Request) {

	// Auth middleware
	//TODO: Handle response
	tok := r.Header["Authorization"][0]
	isAuth, user, error := validateUser(tok)

	//TODO: If there is an error with auth, do not write to the DB
	if error != nil || !isAuth {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("401 - Unauthorized"))
		return
	}

	var f interface{}
	decoder := json.NewDecoder(r.Body)

	err := decoder.Decode(&f)

	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("400 - Bad Request"))
	}

	var tracks []Track
	queueCode, _ := generateUUID()
	//create new queue into db, insert user id into queue
	db := mongoClient.Database("instant_dj_dev")
	collection := db.Collection("queues")
	ctx, _ := context.WithTimeout(context.Background(), 30*time.Second)
	_, error2 := collection.InsertOne(ctx, bson.D{
		{Key: "Name", Value: "Test Name"},
		{Key: "RoomCode", Value: queueCode},
		{Key: "createdBy", Value: user},
		{Key: "Tracks", Value: tracks},
	})

	if error2 != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("400 - Bad Request"))
	} else {
		// Return confirmation of queue with queue id
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(queueCode)
	}
}

func getQueue(w http.ResponseWriter, r *http.Request) {
	tok := r.Header["Authorization"][0]
	isAuth, _, error := validateUser(tok)

	if error != nil || !isAuth {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("401 - Unauthorized"))
		return
	}

	params := mux.Vars(r)
	id := params["id"]

	var f interface{}

	db := mongoClient.Database("instant_dj_dev")
	collection := db.Collection("queues")
	ctx, _ := context.WithTimeout(context.Background(), 30*time.Second)

	filter := bson.D{{
		"RoomCode",
		bson.D{{
			"$in",
			bson.A{id},
		}},
	}}
	decodeError := collection.FindOne(ctx, filter).Decode(&f)

	if decodeError != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("400 - Bad Request"))
	} else {
		// Return confirmation of queue with queue id
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(f)
	}

}

func updateQueue(w http.ResponseWriter, r *http.Request) {
	tok := r.Header["Authorization"][0]
	isAuth, _, error := validateUser(tok)

	if error != nil || !isAuth {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("401 - Unauthorized"))
		return
	}

	params := mux.Vars(r)
	id := params["id"]

	var track Track
	decoder := json.NewDecoder(r.Body)

	err := decoder.Decode(&track)

	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("400 - Bad Request1"))
	}
	filter := bson.D{{
		"RoomCode",
		bson.D{{
			"$in",
			bson.A{id},
		}},
	}}
	update := bson.M{
		"$push": bson.M{"Track": track},
	}

	var f interface{}

	db := mongoClient.Database("instant_dj_dev")
	collection := db.Collection("queues")
	ctx, _ := context.WithTimeout(context.Background(), 30*time.Second)
	decodeError := collection.FindOneAndUpdate(ctx, filter, update).Decode(&f)

	if decodeError != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("400 - Bad Request2"))
	} else {
		// Return confirmation of queue with queue id
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(f)
	}
}
