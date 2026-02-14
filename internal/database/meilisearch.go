package database

import (
	"errors"
	"fmt"
	"log"

	"github.com/meilisearch/meilisearch-go"
)

// InitMeilisearch initializes the Meilisearch client and ensures the index exists.
// It returns the client, index name, or an error if initialization fails.
func InitMeilisearch() (meilisearch.ServiceManager, string, error) {
	// NOTE: godotenv.Load() is called once in config.Load() at startup.

	host := getenv("MEILISEARCH_HOST", "http://localhost:7700")
	apiKey := getenv("MEILISEARCH_API_KEY", "")
	if apiKey == "" {
		apiKey = getenv("MEILISEARCH_MASTER_KEY", "")
	}
	indexName := getenv("MEILISEARCH_INDEX", "notes")

	var client meilisearch.ServiceManager
	if apiKey != "" {
		client = meilisearch.New(host, meilisearch.WithAPIKey(apiKey))
	} else {
		client = meilisearch.New(host)
	}

	if err := TestMeilisearchConnection(client); err != nil {
		return nil, "", err
	}
	if err := EnsureMeilisearchIndex(client, indexName); err != nil {
		return nil, "", err
	}

	return client, indexName, nil
}

// TestMeilisearchConnection verifies connectivity to the Meilisearch server.
func TestMeilisearchConnection(client meilisearch.ServiceManager) error {
	log.Println("Testing Meilisearch connection...")
	health, err := client.Health()
	if err != nil {
		log.Printf("Failed to connect to Meilisearch: %v", err)
		return err
	}
	if health.Status != "available" {
		return fmt.Errorf("meilisearch status unavailable: %s", health.Status)
	}
	log.Printf("Meilisearch connection successful: %s", health.Status)
	return nil
}

// EnsureMeilisearchIndex creates the index if it does not already exist.
func EnsureMeilisearchIndex(client meilisearch.ServiceManager, indexName string) error {
	if indexName == "" {
		return errors.New("meilisearch index name is empty")
	}
	_, err := client.GetIndex(indexName)
	if err == nil {
		return nil
	}
	log.Printf("Meilisearch index %q not found, creating...", indexName)
	_, err = client.CreateIndex(&meilisearch.IndexConfig{
		Uid:        indexName,
		PrimaryKey: "id",
	})
	if err != nil {
		log.Printf("Failed to create Meilisearch index %q: %v", indexName, err)
		return err
	}
	return nil
}
