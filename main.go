package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
)

// MinimalAPIResponse retrieves thereleaseNumber API response
type MinimalAPIResponse struct {
	ReleaseNumber string `json:"releaseNumber"`
}

// FetchStatus retrieves the release number for a given instance from the Salesforce API
func fetchStatus(instance string) (string, error) {
	url := fmt.Sprintf("https://status.salesforce.com/api/instances/%s/status/preview?locale=en", instance)
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to fetch status: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API returned non-OK status: %s", resp.Status)
	}

	var apiResponse MinimalAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResponse); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	return apiResponse.ReleaseNumber, nil
}

// compareReleaseNumbers compares the fetched release number with the expected version
func compareReleaseNumbers(instance, expectedVersion string, wg *sync.WaitGroup, results chan<- string) {
	defer wg.Done()

	releaseNumber, err := fetchStatus(instance)
	if err != nil {
		results <- fmt.Sprintf("Error fetching status for instance %s: %s", instance, err)
		return
	}

	if releaseNumber != expectedVersion {
		results <- fmt.Sprintf("Release number mismatch for instance %s: expected %s, got %s", instance, expectedVersion, releaseNumber)
	} else {
		results <- fmt.Sprintf("Release number matches for instance %s: %s", instance, releaseNumber)
	}
}

// main handles command-line arguments, starts parallel processing, and collects results
func main() {
	instanceFlag := flag.String("instance", "", "Specify the instance name(s), separated by commas")
	versionFlag := flag.String("version", "", "Specify the version")
	flag.Parse()

	if *instanceFlag == "" || *versionFlag == "" {
		slog.Error("Instance and version flags are required")
		flag.Usage()
		os.Exit(1)
	}

	instances := parseInstances(*instanceFlag)
	if len(instances) == 0 {
		slog.Error("No valid instances provided")
		os.Exit(1)
	}

	var wg sync.WaitGroup
	results := make(chan string, len(instances))

	for _, instance := range instances {
		wg.Add(1)
		go compareReleaseNumbers(instance, *versionFlag, &wg, results)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	if !processResults(results) {
		os.Exit(1)
	}
}

// parseInstances splits and trims the instance names from the command-line flag
func parseInstances(instanceFlag string) []string {
	instances := strings.Split(instanceFlag, ",")
	return filterEmptyStrings(instances)
}

// filterEmptyStrings removes empty or whitespace-only strings from a slice
func filterEmptyStrings(slice []string) []string {
	var filtered []string
	for _, s := range slice {
		if trimmed := strings.TrimSpace(s); trimmed != "" {
			filtered = append(filtered, trimmed)
		}
	}
	return filtered
}

// processResults reads results from the channel and logs them
func processResults(results <-chan string) bool {
	exitHealthy := true
	for result := range results {
		slog.Debug(result)
		if strings.Contains(result, "mismatch") {
			exitHealthy = false
		}
	}
	return exitHealthy
}
