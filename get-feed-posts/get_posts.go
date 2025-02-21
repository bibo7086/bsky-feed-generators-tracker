package main

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bluesky-social/indigo/api/atproto"
	"github.com/bluesky-social/indigo/api/bsky"
	"github.com/bluesky-social/indigo/xrpc"
	_ "github.com/lib/pq"
)

type FeedData struct {
	Cursor *string                       `json:"cursor"`
	Feed   []*bsky.FeedDefs_FeedViewPost `json:"feed"`
}

type FeedGeneratorMetadata struct {
	PostURIs []string `json:"post_uris"`
	// Can be easily extended to include other metadata fields, perhaps make use of response from getFeedGenerator??
}

type Config struct {
	Identifier string `json:"username"`
	Password   string `json:"password"`
}

var db *sql.DB

func initDB() {
	var err error
	connStr := "user=username password=password dbname=feed-generators host=localhost port=5432 sslmode=disable"

	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatalf("Error connecting to the database: %v", err)
	}
	// Maximum number of open connections to the database.
	db.SetMaxOpenConns(90)

	err = db.Ping()
	if err != nil {
		log.Fatalf("Error pinging the database: %v", err)
	}

	log.Println("Connected to the database")
}

func main() {
	initDB()

	configFile, err := os.Open("../config.json")
	if err != nil {
		log.Fatal("Error opening config file:", err)
	}
	defer configFile.Close()

	configDecoder := json.NewDecoder(configFile)
	var config Config
	err = configDecoder.Decode(&config)
	if err != nil {
		log.Fatal("Error decoding config file:", err)
	}

	latestFilePath := "../data/test_dids.csv"
	// Normally, we would get the latest file path from the data directory
	// latestFilePath := getLatestFilePath("../data/dids_and_paths")
	if latestFilePath == "" {
		log.Fatal("No dids_and_paths file found")
	}

	file, err := os.Open(latestFilePath)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		log.Fatal(err)
	}

	logFile, err := os.Create("../data/getfeeds-posts.log")
	if err != nil {
		log.Fatal(err)
	}
	defer logFile.Close()

	log.SetOutput(io.MultiWriter(os.Stdout, logFile))

	client := &xrpc.Client{
		Host: "https://bsky.social",
	}

	sessionInput := &atproto.ServerCreateSession_Input{
		Identifier: config.Identifier,
		Password:   config.Password,
	}
	sessionOutput, err := atproto.ServerCreateSession(context.Background(), client, sessionInput)
	if err != nil {
		log.Fatal(err)
	}

	client.Auth = &xrpc.AuthInfo{
		AccessJwt:  sessionOutput.AccessJwt,
		RefreshJwt: sessionOutput.RefreshJwt,
	}

	go func() {
		for {
			time.Sleep(118 * time.Minute)
			refreshSession(client)
		}
	}()

	rateLimiter := time.NewTicker(time.Second / 10)
	defer rateLimiter.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	uriCh := make(chan string)
	errCh := make(chan error)

	var wg sync.WaitGroup

	numWorkers := 10
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for uri := range uriCh {
				log.Printf("Processing URI: %s\n", uri)
				err := worker(ctx, client, uri, errCh)
				if err != nil {
					log.Printf("Error processing URI %s: %v", uri, err)
				}
			}
			log.Println("Worker goroutine done")
		}()
	}

	for _, record := range records {
		uri := fmt.Sprintf("at://%s/%s", record[0], record[1])
		log.Printf("Sending URI to worker: %s\n", uri)
		select {
		case uriCh <- uri:
			log.Printf("URI sent to worker: %s\n", uri)
			<-rateLimiter.C
		case <-ctx.Done():
			break
		}
	}

	close(uriCh)

	go func() {
		wg.Wait()
		close(errCh)
	}()

	for err := range errCh {
		log.Printf("Worker error: %v", err)
	}

	log.Println("All URIs processed")
}

func worker(ctx context.Context, client *xrpc.Client, uri string, errCh chan<- error) error {
	log.Println("Worker processing started for URI:", uri)
	defer log.Println("Worker processing ended for URI:", uri)

	limit := int64(100)
	cursor := ""
	prevCursors := make([]string, 0)

	// Fetch the existing post URIs from the metadata for the specific aturi
	metadata, err := fetchMetadata(ctx, nil, uri)
	if err != nil {
		return err
	}

	for {
		output, err := bsky.FeedGetFeed(ctx, client, cursor, uri, limit)
		if err != nil {
			log.Println("Error fetching feed for URI:", uri, "-", err)
			return err
		}

		feedData := FeedData{
			Cursor: output.Cursor,
			Feed:   output.Feed,
		}

		if len(output.Feed) == 0 && output.Cursor != nil {
			log.Printf("Feed is empty for URI: %s\n", uri)
			break
		}

		// Collect all new post data that are not already in the existing posts
		var newPosts []*bsky.FeedDefs_FeedViewPost
		for _, post := range feedData.Feed {
			if !contains(metadata.PostURIs, post.Post.Uri) {
				newPosts = append(newPosts, post)
				metadata.PostURIs = append(metadata.PostURIs, post.Post.Uri)
			}
		}

		// If no new posts found, stop fetching for this URI
		if len(newPosts) == 0 && len(feedData.Feed) > 0 {
			log.Printf("No new posts found for URI: %s\n", uri)
			break
		}

		// Save the collected new posts to the database for the current feed generator
		err = savePostsToDB(ctx, uri, newPosts, &metadata)
		if err != nil {
			log.Printf("Error saving new posts to DB for URI %s: %v", uri, err)
			return err
		}

		if output.Cursor != nil {
			if containsNull(*output.Cursor) {
				log.Println("Cursor contains null, stopping loop.")
				break
			}

			if contains(prevCursors, *output.Cursor) {
				log.Println("Cursor matches previous value, stopping loop.")
				break
			}

			prevCursors = append(prevCursors, *output.Cursor)
			if len(prevCursors) > 2 {
				prevCursors = prevCursors[1:]
			}

			cursor = *output.Cursor
		} else {
			log.Println("Cursor is null, stopping loop.")
			break
		}
	}

	return nil
}

func (feedData *FeedData) getPostURIs() []interface{} {
	var postURIs []interface{}
	for _, post := range feedData.Feed {
		postURIs = append(postURIs, post.Post.Uri)
	}
	return postURIs
}

func contains(slice []string, val string) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}

func containsNull(cursor string) bool {
	return strings.Contains(cursor, "null")
}

func savePostsToDB(ctx context.Context, aturi string, newPosts []*bsky.FeedDefs_FeedViewPost, metadata *FeedGeneratorMetadata) error {
	log.Println("Starting savePostsToDB for aturi:", aturi)
	tx, err := db.Begin()
	if err != nil {
		log.Printf("Error beginning transaction for aturi %s: %v\n", aturi, err)
		return fmt.Errorf("error beginning transaction: %v", err)
	}

	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
			log.Printf("Transaction rollback due to panic for aturi %s: %v\n", aturi, p)
			panic(p) // re-throw panic after Rollback
		} else if err != nil {
			log.Printf("Transaction rollback for aturi %s due to error: %v\n", aturi, err)
			tx.Rollback()
		} else {
			err = tx.Commit()
			if err != nil {
				log.Printf("Error committing transaction for aturi %s: %v\n", aturi, err)
			}
		}
	}()

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		log.Printf("Error marshalling metadata to JSON for aturi %s: %v", aturi, err)
		return fmt.Errorf("error marshalling metadata: %v", err)
	}

	// Update feed_generators table with the updated metadata JSONB
	query := `INSERT INTO feed_generators (aturi, metadata) VALUES ($1, $2)
             ON CONFLICT (aturi) DO UPDATE SET metadata = $2`
	log.Printf("Executing query to update feed_generators for aturi %s\n", aturi)
	_, err = tx.Exec(query, aturi, metadataJSON)
	if err != nil {
		log.Printf("Error updating feed_generators for aturi %s: %v\n", aturi, err)
		return fmt.Errorf("error updating feed_generators: %v", err)
	}

	// Insert new posts into the posts table if they don't already exist
	for _, post := range newPosts {
		// log.Printf("Inserting post %s created at %s into posts\n", post.Post.Uri, post.Post.IndexedAt)

		postData, err := json.Marshal(post)
		if err != nil {
			log.Printf("Error marshalling post data for post %s: %v\n", post.Post.Uri, err)
			return fmt.Errorf("error marshalling post data: %v", err)
		}

		query := `INSERT INTO posts (uri, post_data) VALUES ($1, $2) ON CONFLICT DO NOTHING`
		_, err = tx.Exec(query, post.Post.Uri, postData)
		if err != nil {
			log.Printf("Error inserting post %s into posts table: %v\n", post.Post.Uri, err)
			return fmt.Errorf("error inserting into posts: %v", err)
		}
	}

	log.Println("Successfully completed savePostsToDB for aturi:", aturi)
	return nil
}

func fetchMetadata(ctx context.Context, tx *sql.Tx, uri string) (FeedGeneratorMetadata, error) {
	var metadataJSON []byte
	var metadata FeedGeneratorMetadata

	query := `SELECT metadata FROM feed_generators WHERE aturi = $1`
	var err error
	if tx != nil {
		err = tx.QueryRowContext(ctx, query, uri).Scan(&metadataJSON)
	} else {
		err = db.QueryRowContext(ctx, query, uri).Scan(&metadataJSON)
	}

	if err != nil {
		if err == sql.ErrNoRows {
			// No metadata found, return an empty FeedGeneratorMetadata
			return FeedGeneratorMetadata{PostURIs: []string{}}, nil
		}
		return FeedGeneratorMetadata{}, fmt.Errorf("error fetching metadata for URI %s: %v", uri, err)
	}

	err = json.Unmarshal(metadataJSON, &metadata)
	if err != nil {
		return FeedGeneratorMetadata{}, fmt.Errorf("error unmarshalling metadata for URI %s: %v", uri, err)
	}

	return metadata, nil
}

func refreshSession(client *xrpc.Client) {
	url := "https://bsky.social/xrpc/com.atproto.server.refreshSession"
	method := "POST"

	httpClient := &http.Client{}

	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		log.Println("Error creating request:", err)
		return
	}

	req.Header.Add("Accept", "application/json")
	req.Header.Add("Authorization", "Bearer "+client.Auth.RefreshJwt)

	res, err := httpClient.Do(req)
	if err != nil {
		log.Println("Error refreshing session:", err)
		return
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		log.Println("Error reading response body:", err)
		return
	}

	log.Println(string(body))

	var refreshOutput MyServerRefreshSessionOutput
	if err := json.Unmarshal(body, &refreshOutput); err != nil {
		log.Println("Error unmarshaling response body:", err)
		return
	}

	client.Auth.AccessJwt = refreshOutput.AccessJwt
	client.Auth.RefreshJwt = refreshOutput.RefreshJwt

	log.Println("Session refreshed")
}

// Get the most recent file based on the date in the filename
func getLatestFilePath(dir string) string {
	var latestFile string
	var latestDate int

	files, err := filepath.Glob(filepath.Join(dir, "dids_and_paths_*.csv"))
	if err != nil {
		log.Fatal(err)
	}

	for _, file := range files {
		parts := filepath.Base(file)
		date := parts[len("dids_and_paths_") : len(parts)-len(".csv")]

		fileDate, err := strconv.Atoi(date)
		if err != nil {
			log.Printf("Error parsing date from filename %s: %v\n", file, err)
			continue
		}

		if fileDate > latestDate {
			latestDate = fileDate
			latestFile = file
		}
	}

	return latestFile
}

type MyServerRefreshSessionOutput struct {
	AccessJwt  string       `json:"accessJwt"`
	Did        string       `json:"did"`
	DidDoc     *interface{} `json:"didDoc,omitempty"`
	Handle     string       `json:"handle"`
	RefreshJwt string       `json:"refreshJwt"`
}
