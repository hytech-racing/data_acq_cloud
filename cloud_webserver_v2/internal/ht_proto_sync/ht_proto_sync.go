package ht_proto_sync

import (
	"context"
	"time"
	"log"
	"sync"
	"fmt"


	"github.com/aesteri/go-getrelease"
	"golang.org/x/oauth2"
	"github.com/google/go-github/v68/github"
)

// SyncService --> syncs files with github
type SyncService struct {
    storedHash string
	mutt sync.Mutex
	stopChannel chan bool
}

const (
    owner   = "" // GitHub username or organization
    repo    = "" // Repository name
    token   = "" // GitHub Personal Access Token
    branch  = "" // Branch to pull commits from
)

// Retrieves chosen asset from Llatest release
// https://github.com/dhillondeep/go-getrelease?tab=readme-ov-file
func (s *SyncService) retrieveData(releaseClient *getrelease.GithubClient, retries *int) {
	time.Sleep(100 * time.Millisecond)
	log.Println("Downloading...")

	regexPattern := `^\d{4}-\d{2}-\d{2}T\d{2}_\d{2}_\d{2}\.html$`

	// Get the html file from the latest release in HT_proto
	if assetName, err := getrelease.GetLatestAsset(releaseClient, "./download", regexPattern, owner, repo, func(config *getrelease.Configuration) error {
		return nil
	}); err != nil {
		// Edge case if html is not in the assets yet (GH Actions takes some time to process -- for the html files to process)
		if *retries > 0 {

			log.Println(err, "trying again...");
			time.Sleep(2 * time.Minute)
			*retries--
			s.retrieveData(releaseClient, retries)

		} else {
			log.Println("Stopping listener ... Encountered Error: ", err);
			s.Stop()
		}
	} else {
		log.Println(assetName)

		// TODO: downloading files into appropriate location 
	}

}

// HT_Proto Listener
// Listens to see if any new commits have been made
func (s *SyncService) ht_protoListen(client *github.Client, releaseClient *getrelease.GithubClient) { 
	log.Println("Pulling Commits...")

	//Comparing commit hashes
	commits, _, err := client.Repositories.ListCommits(context.Background(), owner, repo, &github.CommitsListOptions{
        SHA: branch,
        ListOptions: github.ListOptions{PerPage: 1},
    })
    if err != nil {
		log.Fatal(err)
    }
	latestCommit := commits[0]
	latestHash := *latestCommit.SHA

	// Make sure only one goroutine modifies stored hash at a time
	s.mutt.Lock()
	if latestHash != s.storedHash {
		log.Println("Hash is not equal!")
		
		s.storedHash = latestHash
		retries := 5
		time.Sleep(100 * time.Millisecond)
		s.retrieveData(releaseClient, &retries)
	}
	s.mutt.Unlock()
	time.Sleep(100 * time.Millisecond)
}

// Starts Listening...
func (s *SyncService) StartListening(client *github.Client, releaseClient *getrelease.GithubClient) {
	ticker := time.NewTicker(1 * time.Minute) // Runs every 5 minutes
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChannel: // Wait for the stop signal
			log.Println("Stopping listener...")
			return
		case <-ticker.C: // Wait for next tick
			s.ht_protoListen(client, releaseClient)
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func (s *SyncService) Stop() {
	close(s.stopChannel)
}

func Initializer() (*SyncService){	
	ts := oauth2.StaticTokenSource(
        &oauth2.Token{AccessToken: token},
    )
    tc := oauth2.NewClient(context.Background(), ts)

	client := github.NewClient(tc)
	releaseClient := getrelease.NewGithubClient(nil)

	// Start listening..
	s := &SyncService{}

	s.stopChannel = make(chan bool)
	s.storedHash = "default"

	log.Println("Starting.. Listener...")
	
	go s.StartListening(client, releaseClient)

	return s
}
