package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/joho/godotenv"
)

const (
	BackupDirName  = "backup"
	SystemPrompt   = "You are an API for formatting and fixing spelling mistakes in a markdown file passed to you. Your two main focuses are DO NOT CHANGE the actual content or meaning of the file whatsoever, only rectify the grammer and make it beautifully well formatted in markdown, utilising all markdown tools. Nothing more. Ensure your response is PURELY the file, as its being used directly in the program. Dont say here you go: or anything, and dont embed in code blocks."
	MaxRetries     = 5
	InitialBackoff = 1 * time.Second
	MaxBackoff     = 30 * time.Second
	MaxConcurrent  = 3

	// Default values for environment variables
	DefaultGeminiAPIURL = "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent"
	EnvFile             = ".env"
)

var (
	// Environment variables
	GeminiAPIKey string
	GeminiAPIURL string
)

type GeminiRequest struct {
	Contents []Content `json:"contents"`
}

type Content struct {
	Role  string `json:"role,omitempty"`
	Parts []Part `json:"parts"`
}

type Part struct {
	Text string `json:"text"`
}

type GeminiResponse struct {
	Candidates []Candidate `json:"candidates"`
}

type Candidate struct {
	Content Content `json:"content"`
}

// FileToProcess contains the info of a file to be processed
type FileToProcess struct {
	Path       string
	Content    []byte
	BackupPath string
}

// StatusBar represents a console status bar for processing
type StatusBar struct {
	mu          sync.Mutex
	total       int
	processed   int
	success     int
	failed      int
	lastUpdated time.Time
	startTime   time.Time
}

func NewStatusBar(total int) *StatusBar {
	return &StatusBar{
		total:     total,
		startTime: time.Now(),
	}
}

func (s *StatusBar) IncrementSuccess() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.processed++
	s.success++
	s.lastUpdated = time.Now()
	s.update()
}

func (s *StatusBar) IncrementFailed() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.processed++
	s.failed++
	s.lastUpdated = time.Now()
	s.update()
}

func (s *StatusBar) update() {
	elapsed := time.Since(s.startTime).Seconds()
	percentage := float64(s.processed) / float64(s.total) * 100

	fmt.Printf("\r[%s] Processing: %d/%d (%.1f%%) | Success: %d | Failed: %d | Elapsed: %.1fs",
		getProgressBar(percentage),
		s.processed, s.total,
		percentage,
		s.success, s.failed,
		elapsed)
}

func (s *StatusBar) Finish() {
	s.mu.Lock()
	defer s.mu.Unlock()
	elapsed := time.Since(s.startTime).Seconds()
	fmt.Printf("\r[%s] Completed: %d/%d (100%%) | Success: %d | Failed: %d | Elapsed: %.1fs\n\n",
		getProgressBar(100),
		s.total, s.total,
		s.success, s.failed,
		elapsed)
}

func getProgressBar(percentage float64) string {
	width := 20
	completed := int(percentage / 100 * float64(width))

	bar := ""
	for i := 0; i < width; i++ {
		if i < completed {
			bar += "="
		} else {
			bar += " "
		}
	}

	return bar
}

// ApiSemaphore controls concurrent access to the API
type ApiSemaphore struct {
	ch chan struct{}
}

func NewApiSemaphore(max int) *ApiSemaphore {
	return &ApiSemaphore{
		ch: make(chan struct{}, max),
	}
}

func (s *ApiSemaphore) Acquire() {
	s.ch <- struct{}{}
}

func (s *ApiSemaphore) Release() {
	<-s.ch
}

func init() {
	// Load environment variables from .env file
	err := godotenv.Load(EnvFile)
	if err != nil {
		fmt.Printf("Warning: .env file not found or couldn't be loaded: %v\n", err)
		fmt.Println("Will check for environment variables directly or use defaults.")
	}

	// Get API key from environment variable
	GeminiAPIKey = os.Getenv("GEMINI_API_KEY")
	if GeminiAPIKey == "" {
		fmt.Println("GEMINI_API_KEY not found in environment variables or .env file.")
		fmt.Println("Please set it in .env file or as an environment variable.")
		os.Exit(1)
	}

	// Get API URL from environment variable or use default
	GeminiAPIURL = os.Getenv("GEMINI_API_URL")
	if GeminiAPIURL == "" {
		GeminiAPIURL = DefaultGeminiAPIURL
		fmt.Println("GEMINI_API_URL not found, using default URL.")
	}
}

func main() {
	// Parse command line flags
	recursive := flag.Bool("r", false, "Process directories recursively")
	flag.Parse()

	// Check if we have enough arguments
	args := flag.Args()
	if len(args) < 1 {
		fmt.Println("Usage: ./fixmd [-r] <filename.md or directory>")
		os.Exit(1)
	}

	path := args[0]

	// Get the current working directory
	workDir, err := os.Getwd()
	if err != nil {
		printError("Error getting current directory: %v", err)
		os.Exit(1)
	}

	// Create the backup directory in the current working directory
	backupDir := filepath.Join(workDir, BackupDirName)

	// Initialize random seed for jitter in backoff
	rand.Seed(time.Now().UnixNano())

	// Check if path is a file or directory
	fileInfo, err := os.Stat(path)
	if err != nil {
		printError("Error accessing path: %v", err)
		os.Exit(1)
	}

	// Collect files to process
	var filesToProcess []FileToProcess

	if fileInfo.IsDir() {
		// Process directory
		fmt.Printf("Collecting markdown files from directory: %s\n", path)
		filesToProcess, err = collectFilesFromDir(path, backupDir, *recursive)
		if err != nil {
			printError("Error collecting files: %v", err)
			os.Exit(1)
		}
		fmt.Printf("Found %d markdown files to process\n", len(filesToProcess))
	} else {
		// Process single file (only if it's a markdown file)
		if !strings.HasSuffix(strings.ToLower(path), ".md") {
			fmt.Printf("Skipping non-markdown file: %s\n", path)
			os.Exit(0)
		}

		fileToProcess, err := prepareFileForProcessing(path, backupDir)
		if err != nil {
			printError("Error preparing file: %v", err)
			os.Exit(1)
		}
		filesToProcess = append(filesToProcess, fileToProcess)
	}

	if len(filesToProcess) == 0 {
		fmt.Println("No markdown files found to process.")
		os.Exit(0)
	}

	// Backup all files first
	fmt.Println("\n=== Creating Backups ===")
	if err := backupAllFiles(filesToProcess); err != nil {
		printError("Error during backup phase: %v", err)
		os.Exit(1)
	}

	// Process all files only after successful backup
	fmt.Println("\n=== Processing Files ===")

	// Setup semaphore for API rate limiting
	apiSemaphore := NewApiSemaphore(MaxConcurrent)

	// Setup status bar
	statusBar := NewStatusBar(len(filesToProcess))

	// Setup wait group for all processing
	var wg sync.WaitGroup

	for _, fileInfo := range filesToProcess {
		wg.Add(1)
		go func(fi FileToProcess) {
			defer wg.Done()
			// Acquire semaphore to limit concurrent API calls
			apiSemaphore.Acquire()
			defer apiSemaphore.Release()

			success := processFileContent(fi)
			if success {
				statusBar.IncrementSuccess()
			} else {
				statusBar.IncrementFailed()
			}
		}(fileInfo)
	}

	wg.Wait()
	statusBar.Finish()

	// Print summary
	fmt.Println("Processing complete!")
}

func collectFilesFromDir(dirPath string, backupBaseDir string, recursive bool) ([]FileToProcess, error) {
	var filesToProcess []FileToProcess

	// Function to handle each file/directory
	walkFunc := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories unless we're at the root directory
		if info.IsDir() {
			// Skip subdirectories if not recursive
			if !recursive && path != dirPath {
				return filepath.SkipDir
			}
			return nil
		}

		// Only process markdown files
		if !strings.HasSuffix(strings.ToLower(info.Name()), ".md") {
			return nil
		}

		fileToProcess, err := prepareFileForProcessing(path, backupBaseDir)
		if err != nil {
			return err
		}

		filesToProcess = append(filesToProcess, fileToProcess)
		return nil
	}

	// Walk through the directory
	if err := filepath.Walk(dirPath, walkFunc); err != nil {
		return nil, fmt.Errorf("error walking directory: %v", err)
	}

	return filesToProcess, nil
}

func prepareFileForProcessing(filePath string, backupBaseDir string) (FileToProcess, error) {
	// Read the file content
	content, err := os.ReadFile(filePath)
	if err != nil {
		return FileToProcess{}, fmt.Errorf("error reading file: %v", err)
	}

	// Get the relative path to maintain directory structure in backup
	absFilePath, err := filepath.Abs(filePath)
	if err != nil {
		return FileToProcess{}, fmt.Errorf("error getting absolute path: %v", err)
	}

	// Extract the directory structure to recreate it in the backup
	fileDir := filepath.Dir(absFilePath)
	workDir, _ := os.Getwd()
	absWorkDir, _ := filepath.Abs(workDir)

	// Get the relative path from the working directory
	var relPath string
	if fileDir == absWorkDir {
		// File is in the current directory
		relPath = filepath.Base(filePath)
	} else if strings.HasPrefix(fileDir, absWorkDir) {
		// File is in a subdirectory of the current directory
		relPath = fileDir[len(absWorkDir)+1:]
		relPath = filepath.Join(relPath, filepath.Base(filePath))
	} else {
		// File is outside the current directory - use its full path structure
		relPath = absFilePath
		// Replace root directory separator with an underscore to avoid absolute paths
		relPath = strings.ReplaceAll(relPath, string(filepath.Separator), "_")
		// Remove drive letter or leading slash
		if len(relPath) > 0 && (relPath[0] == '/' || relPath[0] == '\\') {
			relPath = relPath[1:]
		}
	}

	// Create backup file path
	backupFilePath := filepath.Join(backupBaseDir, relPath+".bak")

	return FileToProcess{
		Path:       filePath,
		Content:    content,
		BackupPath: backupFilePath,
	}, nil
}

func backupAllFiles(filesToProcess []FileToProcess) error {
	for i, fileInfo := range filesToProcess {
		// Create the backup directory structure
		backupDirPath := filepath.Dir(fileInfo.BackupPath)
		if err := os.MkdirAll(backupDirPath, 0755); err != nil {
			return fmt.Errorf("error creating backup directory structure for %s: %v", fileInfo.Path, err)
		}

		// Write the backup file
		if err := os.WriteFile(fileInfo.BackupPath, fileInfo.Content, 0644); err != nil {
			return fmt.Errorf("error creating backup file for %s: %v", fileInfo.Path, err)
		}

		fmt.Printf("\r[%d/%d] Backup created: %s", i+1, len(filesToProcess), fileInfo.Path)
	}

	fmt.Println("\nAll files successfully backed up.")
	return nil
}

func processFileContent(fileInfo FileToProcess) bool {
	// Process the content with Gemini API
	processedContent, err := processWithGeminiRetry(string(fileInfo.Content))
	if err != nil {
		printError("Error processing file %s: %v", fileInfo.Path, err)
		return false
	}

	// Write the processed content back to the original file
	if err := os.WriteFile(fileInfo.Path, []byte(processedContent), 0644); err != nil {
		printError("Error writing to file %s: %v", fileInfo.Path, err)
		return false
	}

	return true
}

func processWithGeminiRetry(content string) (string, error) {
	var lastErr error

	// Implement exponential backoff with jitter
	for attempt := 0; attempt < MaxRetries; attempt++ {
		result, err := processWithGemini(content)
		if err == nil {
			return result, nil
		}

		lastErr = err

		// Calculate backoff duration with jitter
		backoffSeconds := float64(InitialBackoff.Seconds()) * math.Pow(2, float64(attempt))
		if backoffSeconds > MaxBackoff.Seconds() {
			backoffSeconds = MaxBackoff.Seconds()
		}

		// Add jitter (±20%)
		jitter := rand.Float64()*0.4 - 0.2 // -20% to +20%
		backoffWithJitter := time.Duration((backoffSeconds * (1 + jitter)) * float64(time.Second))

		// Sleep before retry
		time.Sleep(backoffWithJitter)
	}

	return "", fmt.Errorf("max retries exceeded: %v", lastErr)
}

func processWithGemini(content string) (string, error) {
	// Prepare the request
	requestBody := GeminiRequest{
		Contents: []Content{
			{
				Role: "user",
				Parts: []Part{
					{Text: SystemPrompt},
					{Text: content},
				},
			},
		},
	}

	requestJSON, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("error marshaling request: %v", err)
	}

	// Create the request
	apiURL := fmt.Sprintf("%s?key=%s", GeminiAPIURL, GeminiAPIKey)
	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(requestJSON))
	if err != nil {
		return "", fmt.Errorf("error creating request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Send the request
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error sending request: %v", err)
	}
	defer resp.Body.Close()

	// Read the response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API returned error status: %d, body: %s", resp.StatusCode, string(respBody))
	}

	// Parse the response
	var geminiResp GeminiResponse
	if err := json.Unmarshal(respBody, &geminiResp); err != nil {
		return "", fmt.Errorf("error unmarshaling response: %v", err)
	}

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("no valid response from API")
	}

	// Return the processed content
	return geminiResp.Candidates[0].Content.Parts[0].Text, nil
}

func printError(format string, args ...interface{}) {
	errorMsg := fmt.Sprintf(format, args...)
	fmt.Printf("\n\033[31mERROR: %s\033[0m\n", errorMsg)
}
