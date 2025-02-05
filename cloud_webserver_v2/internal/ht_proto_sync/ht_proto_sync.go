package ht_proto_sync

import (
	"context"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"time"

	"github.com/google/go-github/github"
)

// SyncService --> syncs files with github
type SyncService struct {
	storedHash  string
	stopChannel chan bool
}

var (
	owner  = "hytech-racing" // GitHub username or organization
	repo   = "HT_proto"      // Repository name
	branch = "master"        // Branch to pull commits from
)

// Retrieves chosen asset from latest release
func (s *SyncService) retrieveData(client *github.Client, latestHash string) error {
	// regexAnalyzer (regexp.Regexp) is a Regex object that can be used to match patterns against text
	// Our target file is in the following format, regexPattern, which we want to download
	regexPattern := `^\d{4}-\d{2}-\d{2}T\d{2}_\d{2}_\d{2}\.html$`
	regexAnalyzer, err := regexp.Compile(regexPattern)
	if err != nil {
		return err
	}

	// returns the latest release
	release, _, err := client.Repositories.GetLatestRelease(
		context.Background(), owner, repo)
	if err != nil {
		return err
	}

	for _, asset := range release.Assets {
		// Reports whether or not the asset.Name is a match
		if regexAnalyzer.Match([]byte(*asset.Name)) {
			// Create File
			out, err := os.Create("/app/files/" + *asset.Name)
			if err != nil {
				return err
			}
			defer out.Close()

			resp, err := http.Get(*asset.BrowserDownloadURL)
			if err != nil {
				return err
			}
			defer resp.Body.Close()

			// Write contents onto the created file
			_, err = io.Copy(out, resp.Body)
			if err != nil {
				return err
			}

			// Only updates stored Hash if we successfully retrieve target file
			// --> checks edge case if the target file is not in release yet
			s.storedHash = latestHash
		}
	}
	return nil
}

// HT_Proto Listener
// Listens to see if any new commits have been made
func (s *SyncService) ht_protoListen(client *github.Client) error {
	// Comparing commit hashes
	commits, _, err := client.Repositories.ListCommits(context.Background(), owner, repo, &github.CommitsListOptions{
		SHA:         branch,
		ListOptions: github.ListOptions{PerPage: 1},
	})
	if err != nil {
		return err
	}

	latestCommit := commits[0]
	latestHash := *latestCommit.SHA

	if latestHash != s.storedHash {
		err := s.retrieveData(client, latestHash)

		if err != nil {
			return err
		}
	}
	return nil
}

// Starts Listening...
func (s *SyncService) StartListening(client *github.Client) {
	// Tickers use channels to receive values periodically
	// In this case, every 5 minutes
	ticker := time.NewTicker(5 * time.Minute) // Runs every 5 minutes 
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChannel: // Wait for the stop signal
			log.Println("Stopping listener...")
			return
		case <-ticker.C: // Wait for next tick
			err := s.ht_protoListen(client)
			if err != nil {
				log.Printf("Error while listening: %v", err)
			}
			return
		}
	}
}

func (s *SyncService) Stop() {
	close(s.stopChannel)
}

// Creates a SyncService and STARTS it
func Initializer() *SyncService {
	// Initialize client for github
	client := github.NewClient(nil)

	// Start listening..
	s := &SyncService{
		stopChannel: make(chan bool),
		storedHash:  "",
	}

	log.Println("Starting.. Listener...")

	go s.StartListening(client)

	return s
}
