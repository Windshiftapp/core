//go:build ignore
// +build ignore

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type StressTestResults struct {
	TotalRequests      int64
	SuccessfulRequests int64
	FailedRequests     int64
	DatabaseErrors     int64
	TimeoutErrors      int64
	AverageLatency     time.Duration
	MaxLatency         time.Duration
	MinLatency         time.Duration
	RequestsPerSecond  float64
}

type RequestMetrics struct {
	latency time.Duration
	success bool
	error   string
}

func main() {
	baseURL := "http://localhost:8080"
	if len(os.Args) > 1 {
		baseURL = os.Args[1]
	}

	fmt.Println("Starting Windshift Dual-Pool Stress Test")
	fmt.Println("========================================")

	// Test scenarios focusing on database connection pooling
	scenarios := []struct {
		name        string
		concurrency int
		duration    time.Duration
		endpoint    string
		method      string
		body        interface{}
	}{
		{
			name:        "Permission Read Load (Read Pool Test)",
			concurrency: 50,
			duration:    30 * time.Second,
			endpoint:    "/api/permissions",
			method:      "GET",
		},
		{
			name:        "Mixed Read/Write (Pool Separation Test)",
			concurrency: 100,
			duration:    60 * time.Second,
			endpoint:    "/api/users/1/permissions", // Mix of reads
			method:      "GET",
		},
		{
			name:        "High Concurrency Reads (120+ connections)",
			concurrency: 150,
			duration:    45 * time.Second,
			endpoint:    "/api/permissions",
			method:      "GET",
		},
	}

	for _, scenario := range scenarios {
		fmt.Printf("\n📊 Running: %s\n", scenario.name)
		fmt.Printf("   Concurrency: %d, Duration: %v\n", scenario.concurrency, scenario.duration)

		results := runStressTest(baseURL, scenario.endpoint, scenario.method, scenario.body,
			scenario.concurrency, scenario.duration)

		printResults(results)

		// Cool down between tests
		fmt.Println("   ⏳ Cooling down...")
		time.Sleep(5 * time.Second)
	}

	fmt.Println("\nTesting Complete!")
	fmt.Println("\nKey Metrics to Watch:")
	fmt.Println("• Database Errors: Should be 0 with dual pools")
	fmt.Println("• Timeout Errors: Should be minimal with 5s timeout")
	fmt.Println("• Request/sec: Should be higher with read pool scaling")
	fmt.Println("• Latency: Should be more consistent")
}

func runStressTest(baseURL, endpoint, method string, body interface{}, concurrency int, duration time.Duration) StressTestResults {
	var (
		totalRequests      int64
		successfulRequests int64
		failedRequests     int64
		databaseErrors     int64
		timeoutErrors      int64
		totalLatency       int64
		maxLatency         time.Duration
		minLatency         = time.Duration(1<<63 - 1) // Max duration
	)

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Channel to collect metrics
	metricsChan := make(chan RequestMetrics, concurrency*2)

	// Worker pool
	var wg sync.WaitGroup

	// Start time
	startTime := time.Now()
	endTime := startTime.Add(duration)

	// Launch workers
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for time.Now().Before(endTime) {
				start := time.Now()

				// Create request
				var req *http.Request
				var err error

				if body != nil {
					bodyBytes, _ := json.Marshal(body)
					req, err = http.NewRequest(method, baseURL+endpoint, bytes.NewBuffer(bodyBytes))
					req.Header.Set("Content-Type", "application/json")
				} else {
					req, err = http.NewRequest(method, baseURL+endpoint, nil)
				}

				if err != nil {
					metricsChan <- RequestMetrics{
						latency: time.Since(start),
						success: false,
						error:   "request_creation_failed",
					}
					continue
				}

				// Execute request
				resp, err := client.Do(req)
				latency := time.Since(start)

				atomic.AddInt64(&totalRequests, 1)

				if err != nil {
					metricsChan <- RequestMetrics{
						latency: latency,
						success: false,
						error:   "request_failed",
					}

					if isTimeoutError(err) {
						atomic.AddInt64(&timeoutErrors, 1)
					}
					atomic.AddInt64(&failedRequests, 1)
					continue
				}

				success := resp.StatusCode < 400
				var errorType string

				if !success {
					if resp.StatusCode == 500 {
						errorType = "database_error"
						atomic.AddInt64(&databaseErrors, 1)
					} else {
						errorType = fmt.Sprintf("http_%d", resp.StatusCode)
					}
					atomic.AddInt64(&failedRequests, 1)
				} else {
					atomic.AddInt64(&successfulRequests, 1)
				}

				resp.Body.Close()

				metricsChan <- RequestMetrics{
					latency: latency,
					success: success,
					error:   errorType,
				}

				// Brief pause to prevent overwhelming
				time.Sleep(time.Millisecond)
			}
		}(i)
	}

	// Metrics collector
	go func() {
		wg.Wait()
		close(metricsChan)
	}()

	// Process metrics
	for metric := range metricsChan {
		atomic.AddInt64(&totalLatency, int64(metric.latency))

		if metric.latency > maxLatency {
			maxLatency = metric.latency
		}
		if metric.latency < minLatency {
			minLatency = metric.latency
		}
	}

	totalDuration := time.Since(startTime)

	return StressTestResults{
		TotalRequests:      totalRequests,
		SuccessfulRequests: successfulRequests,
		FailedRequests:     failedRequests,
		DatabaseErrors:     databaseErrors,
		TimeoutErrors:      timeoutErrors,
		AverageLatency:     time.Duration(totalLatency / totalRequests),
		MaxLatency:         maxLatency,
		MinLatency:         minLatency,
		RequestsPerSecond:  float64(totalRequests) / totalDuration.Seconds(),
	}
}

func printResults(results StressTestResults) {
	fmt.Printf("   Results:\n")
	fmt.Printf("     Total Requests: %d\n", results.TotalRequests)
	fmt.Printf("     Successful: %d (%.1f%%)\n", results.SuccessfulRequests,
		float64(results.SuccessfulRequests)/float64(results.TotalRequests)*100)
	fmt.Printf("     Failed: %d (%.1f%%)\n", results.FailedRequests,
		float64(results.FailedRequests)/float64(results.TotalRequests)*100)
	fmt.Printf("     Database Errors: %d\n", results.DatabaseErrors)
	fmt.Printf("     Timeout Errors: %d\n", results.TimeoutErrors)
	fmt.Printf("     Requests/sec: %.1f\n", results.RequestsPerSecond)
	fmt.Printf("     Avg Latency: %v\n", results.AverageLatency)
	fmt.Printf("     Min/Max Latency: %v / %v\n", results.MinLatency, results.MaxLatency)
}

func isTimeoutError(err error) bool {
	return strings.Contains(err.Error(), "timeout") ||
		strings.Contains(err.Error(), "deadline exceeded")
}
