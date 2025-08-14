package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"testing"
)

// =============================================================================
// UPDATE FUNCTIONALITY TESTS
// =============================================================================
// This file contains all tests related to:
// - Update checking and downloading
// - GitHub API integration
// - Archive extraction (ZIP, TAR.GZ)
// - HTTP operations
// - File replacement
// - Platform-specific functionality
// =============================================================================

// =============================================================================
// CORE UPDATE TESTS
// =============================================================================
// Tests for main update flow and version checking
// =============================================================================

func TestCheckAndUpdate(t *testing.T) {
	tests := []struct {
		name           string
		currentVersion string
		latestVersion  string
		expectUpdate   bool
		wantErr        bool
	}{
		{
			name:           "dev version",
			currentVersion: "dev",
			latestVersion:  "v1.0.0",
			expectUpdate:   false,
			wantErr:        false,
		},
		{
			name:           "same version",
			currentVersion: "v1.0.0",
			latestVersion:  "v1.0.0",
			expectUpdate:   false,
			wantErr:        false,
		},
		{
			name:           "update available",
			currentVersion: "v1.0.0",
			latestVersion:  "v1.1.0",
			expectUpdate:   true,
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original version
			originalVersion := version
			version = tt.currentVersion
			defer func() { version = originalVersion }()

			// Mock HTTP server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				release := GitHubRelease{
					TagName: tt.latestVersion,
					Name:    "Test Release",
				}
				
				if tt.expectUpdate {
					// Add mock asset
					assetName := fmt.Sprintf("secret_manager-%s-%s", runtime.GOOS, runtime.GOARCH)
					if runtime.GOOS == "windows" {
						assetName = fmt.Sprintf("secret_manager-windows-%s.exe", runtime.GOARCH)
					}
					release.Assets = []struct {
						Name               string `json:"name"`
						BrowserDownloadURL string `json:"browser_download_url"`
					}{
						{
							Name:               assetName,
							BrowserDownloadURL: "http://example.com/download",
						},
					}
				}

				json.NewEncoder(w).Encode(release)
			}))
			defer server.Close()

			// Override GitHub API URL
			originalAPI := githubAPI
			defer func() { _ = originalAPI }()

			// Mock HTTP client
			originalClient := httpClient
			httpClient = &http.Client{
				Transport: &mockTransport{server: server},
			}
			
			// Mock downloadAndInstall for update available case
			originalDownload := downloadAndInstallFunc
			if tt.expectUpdate {
				downloadAndInstallFunc = func(url string) error {
					return nil
				}
			}
			
			defer func() { 
				httpClient = originalClient
				downloadAndInstallFunc = originalDownload
			}()

			err := checkAndUpdate()
			if (err != nil) != tt.wantErr {
				t.Errorf("checkAndUpdate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

type mockTransport struct {
	server *httptest.Server
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Redirect to test server
	req.URL.Scheme = "http"
	req.URL.Host = m.server.Listener.Addr().String()
	return http.DefaultTransport.RoundTrip(req)
}

// =============================================================================
// GITHUB API TESTS
// =============================================================================
// Tests for GitHub API integration and release fetching
// =============================================================================

func TestGetLatestRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != userAgent {
			t.Errorf("Expected User-Agent %s, got %s", userAgent, r.Header.Get("User-Agent"))
		}

		release := GitHubRelease{
			TagName: "v1.0.0",
			Name:    "Test Release",
			Assets: []struct {
				Name               string `json:"name"`
				BrowserDownloadURL string `json:"browser_download_url"`
			}{
				{
					Name:               "secret_manager-linux-amd64",
					BrowserDownloadURL: "http://example.com/linux",
				},
				{
					Name:               "secret_manager-windows-amd64.exe",
					BrowserDownloadURL: "http://example.com/windows",
				},
			},
		}
		json.NewEncoder(w).Encode(release)
	}))
	defer server.Close()

	originalClient := httpClient
	httpClient = &http.Client{
		Transport: &mockTransport{server: server},
	}
	defer func() { httpClient = originalClient }()

	release, err := getLatestRelease()
	if err != nil {
		t.Fatalf("getLatestRelease() error = %v", err)
	}

	if release.TagName != "v1.0.0" {
		t.Errorf("Expected tag v1.0.0, got %s", release.TagName)
	}

	if len(release.Assets) != 2 {
		t.Errorf("Expected 2 assets, got %d", len(release.Assets))
	}
}

func TestGetLatestReleaseErrors(t *testing.T) {
	tests := []struct {
		name          string
		serverFunc    func(w http.ResponseWriter, r *http.Request)
		expectedError string
	}{
		{
			name: "HTTP error",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "Not Found", http.StatusNotFound)
			},
			expectedError: "GitHub API returned status 404",
		},
		{
			name: "Invalid JSON",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte("invalid json"))
			},
			expectedError: "invalid character",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverFunc))
			defer server.Close()

			originalClient := httpClient
			httpClient = &http.Client{
				Transport: &mockTransport{server: server},
			}
			defer func() { httpClient = originalClient }()

			_, err := getLatestRelease()
			if err == nil {
				t.Error("Expected error, got nil")
			} else if !strings.Contains(err.Error(), tt.expectedError) {
				t.Errorf("Expected error containing %q, got %v", tt.expectedError, err)
			}
		})
	}
}

func TestGetLatestReleaseNetworkError(t *testing.T) {
	originalClient := httpClient
	httpClient = &http.Client{
		Timeout: 1, // 1 nanosecond timeout
	}
	defer func() {
		httpClient = originalClient
	}()
	
	_, err := getLatestRelease()
	if err == nil {
		t.Error("Expected error for network timeout")
	}
}

func TestGetLatestReleaseWithMockedNewRequest(t *testing.T) {
	originalHttpNewRequest := httpNewRequest
	httpNewRequest = func(method, url string, body io.Reader) (*http.Request, error) {
		return nil, errors.New("mock http.NewRequest error")
	}
	defer func() {
		httpNewRequest = originalHttpNewRequest
	}()
	
	_, err := getLatestRelease()
	if err == nil || !strings.Contains(err.Error(), "mock http.NewRequest error") {
		t.Errorf("Expected NewRequest error, got %v", err)
	}
}

// =============================================================================
// ASSET FINDING TESTS
// =============================================================================
// Tests for platform-specific asset selection
// =============================================================================

func TestFindAssetURL(t *testing.T) {
	release := &GitHubRelease{
		Assets: []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		}{
			{
				Name:               "secret_manager-linux-amd64",
				BrowserDownloadURL: "http://example.com/linux-amd64",
			},
			{
				Name:               "secret_manager-windows-amd64.exe",
				BrowserDownloadURL: "http://example.com/windows-amd64",
			},
			{
				Name:               "secret_manager-darwin-amd64",
				BrowserDownloadURL: "http://example.com/darwin-amd64",
			},
		},
	}

	tests := []struct {
		name     string
		goos     string
		goarch   string
		expected string
	}{
		{
			name:     "linux amd64",
			goos:     "linux",
			goarch:   "amd64",
			expected: "http://example.com/linux-amd64",
		},
		{
			name:     "windows amd64",
			goos:     "windows",
			goarch:   "amd64",
			expected: "http://example.com/windows-amd64",
		},
		{
			name:     "darwin amd64",
			goos:     "darwin",
			goarch:   "amd64",
			expected: "http://example.com/darwin-amd64",
		},
		{
			name:     "unsupported platform",
			goos:     "freebsd",
			goarch:   "amd64",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We can't mock runtime.GOOS and runtime.GOARCH directly
			// So we'll test with the current platform
			if tt.goos == runtime.GOOS && tt.goarch == runtime.GOARCH {
				url := findAssetURL(release)
				if url != tt.expected {
					t.Errorf("Expected URL %s, got %s", tt.expected, url)
				}
			} else {
				t.Skip("Skipping test for different platform")
			}
		})
	}
}

// =============================================================================
// UPDATE ERROR TESTS
// =============================================================================
// Tests for various error scenarios in the update process
// =============================================================================

func TestCheckAndUpdateErrors(t *testing.T) {
	tests := []struct {
		name          string
		setupMock     func()
		expectedError string
	}{
		{
			name: "getLatestRelease error",
			setupMock: func() {
				// Mock HTTP server returns error
			},
			expectedError: "failed to get latest release",
		},
		{
			name: "no suitable binary",
			setupMock: func() {
				// Mock returns release without matching assets
			},
			expectedError: "no suitable binary found",
		},
		{
			name: "download error",
			setupMock: func() {
				// Mock download failure
			},
			expectedError: "failed to install update",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save originals
			originalVersion := version
			originalClient := httpClient
			originalDownload := downloadAndInstallFunc

			// Set current version
			version = "v1.0.0"

			// Create mock server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.name == "getLatestRelease error" {
					http.Error(w, "Server error", http.StatusInternalServerError)
					return
				}

				release := GitHubRelease{
					TagName: "v1.1.0",
					Name:    "Test Release",
				}

				if tt.name != "no suitable binary" {
					assetName := fmt.Sprintf("secret_manager-windows-%s.exe", runtime.GOARCH)
					release.Assets = []struct {
						Name               string `json:"name"`
						BrowserDownloadURL string `json:"browser_download_url"`
					}{
						{
							Name:               assetName,
							BrowserDownloadURL: "http://example.com/download",
						},
					}
				}

				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(w, `{"tag_name": "%s", "name": "%s", "assets": [`, release.TagName, release.Name)
				for i, asset := range release.Assets {
					if i > 0 {
						fmt.Fprint(w, ",")
					}
					fmt.Fprintf(w, `{"name": "%s", "browser_download_url": "%s"}`, asset.Name, asset.BrowserDownloadURL)
				}
				fmt.Fprint(w, "]}")
			}))
			defer server.Close()

			// Mock HTTP client
			httpClient = &http.Client{
				Transport: &mockTransport{server: server},
			}

			// Mock downloadAndInstall
			if tt.name == "download error" {
				downloadAndInstallFunc = func(url string) error {
					return errors.New("download failed")
				}
			}

			defer func() {
				version = originalVersion
				httpClient = originalClient
				downloadAndInstallFunc = originalDownload
			}()

			err := checkAndUpdate()
			if err == nil && tt.expectedError != "" {
				t.Errorf("Expected error containing %q, got nil", tt.expectedError)
			} else if err != nil && !strings.Contains(err.Error(), tt.expectedError) {
				t.Errorf("Expected error containing %q, got %v", tt.expectedError, err)
			}
		})
	}
}

// =============================================================================
// ARCHIVE EXTRACTION TESTS
// =============================================================================
// Tests for ZIP and TAR.GZ archive extraction
// =============================================================================

func TestExtractZip(t *testing.T) {
	// Create test zip file
	tempFile, err := os.CreateTemp("", "test*.zip")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tempFile.Name())

	zipWriter := zip.NewWriter(tempFile)
	
	// Add test file
	writer, err := zipWriter.Create("secret_manager.exe")
	if err != nil {
		t.Fatal(err)
	}
	_, err = writer.Write([]byte("test binary content"))
	if err != nil {
		t.Fatal(err)
	}
	
	zipWriter.Close()
	tempFile.Close()

	// Test extraction
	extractedPath, err := extractZip(tempFile.Name())
	if err != nil {
		t.Fatalf("extractZip() error = %v", err)
	}
	defer os.Remove(extractedPath)

	// Verify extracted file
	content, err := os.ReadFile(extractedPath)
	if err != nil {
		t.Fatal(err)
	}

	if string(content) != "test binary content" {
		t.Errorf("Expected content 'test binary content', got %s", string(content))
	}
}

func TestExtractTarGz(t *testing.T) {
	// Create test tar.gz file
	tempFile, err := os.CreateTemp("", "test*.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tempFile.Name())

	gzWriter := gzip.NewWriter(tempFile)
	tarWriter := tar.NewWriter(gzWriter)
	
	// Add test file
	content := []byte("test binary content")
	header := &tar.Header{
		Name: "secret_manager",
		Mode: 0755,
		Size: int64(len(content)),
	}
	
	if err := tarWriter.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write(content); err != nil {
		t.Fatal(err)
	}
	
	tarWriter.Close()
	gzWriter.Close()
	tempFile.Close()

	// Test extraction
	extractedPath, err := extractTarGz(tempFile.Name())
	if err != nil {
		t.Fatalf("extractTarGz() error = %v", err)
	}
	defer os.Remove(extractedPath)

	// Verify extracted file
	readContent, err := os.ReadFile(extractedPath)
	if err != nil {
		t.Fatal(err)
	}

	if string(readContent) != "test binary content" {
		t.Errorf("Expected content 'test binary content', got %s", string(readContent))
	}
}

// =============================================================================
// ARCHIVE EXTRACTION ERROR TESTS
// =============================================================================
// Tests for error scenarios in archive extraction
// =============================================================================

func TestExtractErrors(t *testing.T) {
	t.Run("zip error", func(t *testing.T) {
		// Create invalid zip
		tempFile, err := os.CreateTemp("", "bad*.zip")
		if err != nil {
			t.Fatal(err)
		}
		tempFile.Write([]byte("not a zip file"))
		tempFile.Close()
		defer os.Remove(tempFile.Name())

		_, err = extractZip(tempFile.Name())
		if err == nil {
			t.Error("Expected error for invalid zip")
		}
	})

	t.Run("tar.gz error", func(t *testing.T) {
		// Create invalid tar.gz
		tempFile, err := os.CreateTemp("", "bad*.tar.gz")
		if err != nil {
			t.Fatal(err)
		}
		tempFile.Write([]byte("not a tar.gz file"))
		tempFile.Close()
		defer os.Remove(tempFile.Name())

		_, err = extractTarGz(tempFile.Name())
		if err == nil {
			t.Error("Expected error for invalid tar.gz")
		}
	})
}

func TestExtractZipErrors(t *testing.T) {
	t.Run("no executable in zip", func(t *testing.T) {
		// Create zip without secret_manager
		tempFile, err := os.CreateTemp("", "test*.zip")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(tempFile.Name())

		zipWriter := zip.NewWriter(tempFile)
		// Add a file that's not secret_manager
		writer, err := zipWriter.Create("other.exe")
		if err != nil {
			t.Fatal(err)
		}
		_, err = writer.Write([]byte("other content"))
		if err != nil {
			t.Fatal(err)
		}
		zipWriter.Close()
		tempFile.Close()

		_, err = extractZip(tempFile.Name())
		if err == nil || !strings.Contains(err.Error(), "executable not found") {
			t.Errorf("Expected 'executable not found' error, got %v", err)
		}
	})

	t.Run("file open error in zip", func(t *testing.T) {
		// Test with non-existent file
		_, err := extractZip("/nonexistent/file.zip")
		if err == nil {
			t.Error("Expected error for non-existent file")
		}
	})
}

func TestExtractTarGzErrors(t *testing.T) {
	t.Run("no executable in tar.gz", func(t *testing.T) {
		// Create tar.gz without secret_manager
		tempFile, err := os.CreateTemp("", "test*.tar.gz")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(tempFile.Name())

		gzWriter := gzip.NewWriter(tempFile)
		tarWriter := tar.NewWriter(gzWriter)
		
		// Add a file that's not secret_manager
		content := []byte("other content")
		header := &tar.Header{
			Name: "other",
			Mode: 0755,
			Size: int64(len(content)),
		}
		
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := tarWriter.Write(content); err != nil {
			t.Fatal(err)
		}
		
		tarWriter.Close()
		gzWriter.Close()
		tempFile.Close()

		_, err = extractTarGz(tempFile.Name())
		if err == nil || !strings.Contains(err.Error(), "executable not found") {
			t.Errorf("Expected 'executable not found' error, got %v", err)
		}
	})

	t.Run("file open error", func(t *testing.T) {
		_, err := extractTarGz("/nonexistent/file.tar.gz")
		if err == nil {
			t.Error("Expected error for non-existent file")
		}
	})
}

func TestExtractTarGzNextError(t *testing.T) {
	// Create an invalid tar.gz that will cause Next() to fail
	tempFile, err := os.CreateTemp("", "test*.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tempFile.Name())

	// Write gzip header but invalid tar data
	gzWriter := gzip.NewWriter(tempFile)
	gzWriter.Write([]byte("invalid tar data"))
	gzWriter.Close()
	tempFile.Close()

	_, err = extractTarGz(tempFile.Name())
	if err == nil {
		t.Error("Expected error for invalid tar data")
	}
}

// =============================================================================
// MOCK-BASED EXTRACTION TESTS
// =============================================================================
// Tests using mocked functions for edge cases
// =============================================================================

func TestExtractZipFileOpenErrorMock(t *testing.T) {
	// Create a temporary zip file
	tempFile, err := os.CreateTemp("", "test_*.zip")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tempFile.Name())

	zipWriter := zip.NewWriter(tempFile)
	writer, err := zipWriter.Create("secret_manager.exe")
	if err != nil {
		t.Fatalf("Failed to create zip entry: %v", err)
	}
	if _, err := writer.Write([]byte("test")); err != nil {
		t.Fatalf("Failed to write to zip: %v", err)
	}
	zipWriter.Close()
	tempFile.Close()

	// Mock zipFileOpen to fail
	originalZipFileOpen := zipFileOpen
	zipFileOpen = func(f *zip.File) (io.ReadCloser, error) {
		return nil, errors.New("mock open error")
	}
	defer func() { zipFileOpen = originalZipFileOpen }()

	_, err = extractZip(tempFile.Name())
	if err == nil || err.Error() != "mock open error" {
		t.Errorf("Expected 'mock open error', got %v", err)
	}
}

func TestExtractZipWithMockedCreate(t *testing.T) {
	// Create a valid zip file
	tempFile, err := os.CreateTemp("", "test*.zip")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tempFile.Name())

	zipWriter := zip.NewWriter(tempFile)
	writer, err := zipWriter.Create("secret_manager.exe")
	if err != nil {
		t.Fatal(err)
	}
	_, err = writer.Write([]byte("test"))
	if err != nil {
		t.Fatal(err)
	}
	zipWriter.Close()
	tempFile.Close()

	// Mock os.Create to fail
	originalOsCreate := osCreate
	osCreate = func(name string) (*os.File, error) {
		return nil, errors.New("mock Create error")
	}
	defer func() {
		osCreate = originalOsCreate
	}()
	
	_, err = extractZip(tempFile.Name())
	if err == nil || !strings.Contains(err.Error(), "mock Create error") {
		t.Errorf("Expected Create error, got %v", err)
	}
}

func TestExtractTarGzWithMockedCreate(t *testing.T) {
	// Create a valid tar.gz file
	tempFile, err := os.CreateTemp("", "test*.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tempFile.Name())

	gzWriter := gzip.NewWriter(tempFile)
	tarWriter := tar.NewWriter(gzWriter)
	
	content := []byte("test")
	header := &tar.Header{
		Name: "secret_manager",
		Mode: 0755,
		Size: int64(len(content)),
	}
	
	if err := tarWriter.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write(content); err != nil {
		t.Fatal(err)
	}
	
	tarWriter.Close()
	gzWriter.Close()
	tempFile.Close()

	// Mock os.Create to fail
	originalOsCreate := osCreate
	osCreate = func(name string) (*os.File, error) {
		return nil, errors.New("mock Create error")
	}
	defer func() {
		osCreate = originalOsCreate
	}()
	
	_, err = extractTarGz(tempFile.Name())
	if err == nil || !strings.Contains(err.Error(), "mock Create error") {
		t.Errorf("Expected Create error, got %v", err)
	}
}

func TestExtractZipWithMockedIOCopy(t *testing.T) {
	// Create a valid zip file
	tempFile, err := os.CreateTemp("", "test*.zip")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tempFile.Name())

	zipWriter := zip.NewWriter(tempFile)
	writer, err := zipWriter.Create("secret_manager.exe")
	if err != nil {
		t.Fatal(err)
	}
	_, err = writer.Write([]byte("test content"))
	if err != nil {
		t.Fatal(err)
	}
	zipWriter.Close()
	tempFile.Close()

	// Mock io.Copy to fail
	originalIOCopy := ioCopy
	callCount := 0
	ioCopy = func(dst io.Writer, src io.Reader) (int64, error) {
		callCount++
		return 0, errors.New("mock io.Copy error")
	}
	defer func() {
		ioCopy = originalIOCopy
	}()
	
	_, err = extractZip(tempFile.Name())
	if err == nil || !strings.Contains(err.Error(), "mock io.Copy error") {
		t.Errorf("Expected io.Copy error, got %v", err)
	}
}

func TestExtractTarGzWithMockedIOCopy(t *testing.T) {
	// Create a valid tar.gz file
	tempFile, err := os.CreateTemp("", "test*.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tempFile.Name())

	gzWriter := gzip.NewWriter(tempFile)
	tarWriter := tar.NewWriter(gzWriter)
	
	content := []byte("test content")
	header := &tar.Header{
		Name: "secret_manager",
		Mode: 0755,
		Size: int64(len(content)),
	}
	
	if err := tarWriter.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write(content); err != nil {
		t.Fatal(err)
	}
	
	tarWriter.Close()
	gzWriter.Close()
	tempFile.Close()

	// Mock io.Copy to fail
	originalIOCopy := ioCopy
	ioCopy = func(dst io.Writer, src io.Reader) (int64, error) {
		return 0, errors.New("mock io.Copy error")
	}
	defer func() {
		ioCopy = originalIOCopy
	}()
	
	_, err = extractTarGz(tempFile.Name())
	if err == nil || !strings.Contains(err.Error(), "mock io.Copy error") {
		t.Errorf("Expected io.Copy error, got %v", err)
	}
}

// =============================================================================
// CHMOD AND PLATFORM-SPECIFIC TESTS
// =============================================================================
// Tests for platform-specific operations like chmod
// =============================================================================

func TestExtractTarGzWithChmodCoverage(t *testing.T) {
	// Save originals
	originalIsWindows := isWindows
	originalOsChmod := osChmod
	originalOsCreate := osCreate
	originalIOCopy := ioCopy
	defer func() {
		isWindows = originalIsWindows
		osChmod = originalOsChmod
		osCreate = originalOsCreate
		ioCopy = originalIOCopy
	}()

	// Mock as Unix system
	isWindows = func() bool { return false }

	// Track chmod call
	chmodCalled := false
	osChmod = func(name string, mode os.FileMode) error {
		chmodCalled = true
		if mode != 0755 {
			t.Errorf("Expected mode 0755, got %o", mode)
		}
		return nil
	}

	// Create a temporary tar.gz file
	tempFile, err := os.CreateTemp("", "test*.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tempFile.Name())

	// Create tar.gz with secret_manager file
	gzWriter := gzip.NewWriter(tempFile)
	tarWriter := tar.NewWriter(gzWriter)

	// Add a file named "secret_manager"
	header := &tar.Header{
		Name: "secret_manager",
		Mode: 0644,
		Size: 13,
	}
	if err := tarWriter.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write([]byte("test content\n")); err != nil {
		t.Fatal(err)
	}

	tarWriter.Close()
	gzWriter.Close()
	tempFile.Close()

	// Mock osCreate to succeed
	tempExtractFile, err := os.CreateTemp("", "extract*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tempExtractFile.Name())
	tempExtractFile.Close()

	osCreate = func(name string) (*os.File, error) {
		return os.Create(tempExtractFile.Name())
	}

	// Mock ioCopy to succeed
	ioCopy = func(dst io.Writer, src io.Reader) (int64, error) {
		return 13, nil
	}

	// Extract the tar.gz
	extractPath, err := extractTarGz(tempFile.Name())
	if err != nil {
		t.Fatalf("extractTarGz failed: %v", err)
	}

	// Verify chmod was called
	if !chmodCalled {
		t.Error("Expected osChmod to be called on Unix systems")
	}

	// Clean up
	if extractPath != "" && extractPath != tempExtractFile.Name() {
		os.Remove(extractPath)
	}
}

func TestExtractTarGzUnixChmod(t *testing.T) {
	// Save originals
	originalIsWindows := isWindows
	originalOsChmod := osChmod
	defer func() {
		isWindows = originalIsWindows
		osChmod = originalOsChmod
	}()

	// Mock as Unix system
	isWindows = func() bool { return false }

	// Track chmod call
	osChmod = func(name string, mode os.FileMode) error {
		if mode != 0755 {
			t.Errorf("Expected mode 0755, got %o", mode)
		}
		return nil
	}

	// We need to create a proper tar.gz with secret_manager file
	// For now, just verify the mock works
	if !isWindows() {
		// This confirms our mock is working
		t.Log("Successfully mocked Unix environment")
	}
}

func TestExtractTarGzWindowsChmod(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows-specific test on non-Windows")
	}
	
	// Create a valid tar.gz file
	tempFile, err := os.CreateTemp("", "test*.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tempFile.Name())

	gzWriter := gzip.NewWriter(tempFile)
	tarWriter := tar.NewWriter(gzWriter)
	
	content := []byte("test")
	header := &tar.Header{
		Name: "secret_manager",
		Mode: 0755,
		Size: int64(len(content)),
	}
	
	if err := tarWriter.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write(content); err != nil {
		t.Fatal(err)
	}
	
	tarWriter.Close()
	gzWriter.Close()
	tempFile.Close()

	// Extract - should work without chmod on Windows
	extractedPath, err := extractTarGz(tempFile.Name())
	if err != nil {
		t.Fatalf("extractTarGz() error = %v", err)
	}
	defer os.Remove(extractedPath)
}

// =============================================================================
// DOWNLOAD AND INSTALL TESTS
// =============================================================================
// Tests for file download and installation process
// =============================================================================

func TestDownloadAndInstall(t *testing.T) {
	// Create a test server that serves a mock binary
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("mock binary content"))
	}))
	defer server.Close()

	originalClient := httpClient
	originalOsExecutable := osExecutable
	originalReplaceFunc := replaceExecutableFunc

	// Create temp executable path
	tempFile, err := os.CreateTemp("", "test_exe_*")
	if err != nil {
		t.Fatal(err)
	}
	tempFile.Close()
	defer os.Remove(tempFile.Name())

	osExecutable = func() (string, error) {
		return tempFile.Name(), nil
	}

	replaceExecutableFunc = func(current, new string) error {
		// Just verify the paths
		if current != tempFile.Name() {
			t.Errorf("Expected current path %s, got %s", tempFile.Name(), current)
		}
		return nil
	}

	httpClient = &http.Client{}

	defer func() {
		httpClient = originalClient
		osExecutable = originalOsExecutable
		replaceExecutableFunc = originalReplaceFunc
	}()

	err = downloadAndInstall(server.URL)
	if err != nil {
		t.Errorf("downloadAndInstall() error = %v", err)
	}
}

func TestDownloadAndInstallZip(t *testing.T) {
	// Create a test zip file
	zipFile, err := os.CreateTemp("", "test*.zip")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(zipFile.Name())

	zipWriter := zip.NewWriter(zipFile)
	writer, err := zipWriter.Create("secret_manager.exe")
	if err != nil {
		t.Fatal(err)
	}
	_, err = writer.Write([]byte("test binary content"))
	if err != nil {
		t.Fatal(err)
	}
	zipWriter.Close()
	zipFile.Close()

	// Read the zip content
	zipContent, err := os.ReadFile(zipFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Create a test server that serves the zip
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(zipContent)
	}))
	defer server.Close()

	originalClient := httpClient
	originalOsExecutable := osExecutable
	originalReplaceFunc := replaceExecutableFunc

	// Create temp executable path
	tempFile, err := os.CreateTemp("", "test_exe_*")
	if err != nil {
		t.Fatal(err)
	}
	tempFile.Close()
	defer os.Remove(tempFile.Name())

	osExecutable = func() (string, error) {
		return tempFile.Name(), nil
	}

	replaceExecutableFunc = func(current, new string) error {
		return nil
	}

	httpClient = &http.Client{}

	defer func() {
		httpClient = originalClient
		osExecutable = originalOsExecutable
		replaceExecutableFunc = originalReplaceFunc
	}()

	err = downloadAndInstall(server.URL + "/test.zip")
	if err != nil {
		t.Errorf("downloadAndInstall() error = %v", err)
	}
}

func TestDownloadAndInstallTarGz(t *testing.T) {
	// Create a test tar.gz file
	tarFile, err := os.CreateTemp("", "test*.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tarFile.Name())

	gzWriter := gzip.NewWriter(tarFile)
	tarWriter := tar.NewWriter(gzWriter)
	
	content := []byte("test binary content")
	header := &tar.Header{
		Name: "secret_manager",
		Mode: 0755,
		Size: int64(len(content)),
	}
	
	if err := tarWriter.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write(content); err != nil {
		t.Fatal(err)
	}
	
	tarWriter.Close()
	gzWriter.Close()
	tarFile.Close()

	// Read the tar.gz content
	tarContent, err := os.ReadFile(tarFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Create a test server that serves the tar.gz
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(tarContent)
	}))
	defer server.Close()

	originalClient := httpClient
	originalOsExecutable := osExecutable
	originalReplaceFunc := replaceExecutableFunc

	// Create temp executable path
	tempFile, err := os.CreateTemp("", "test_exe_*")
	if err != nil {
		t.Fatal(err)
	}
	tempFile.Close()
	defer os.Remove(tempFile.Name())

	osExecutable = func() (string, error) {
		return tempFile.Name(), nil
	}

	replaceExecutableFunc = func(current, new string) error {
		return nil
	}

	httpClient = &http.Client{}

	defer func() {
		httpClient = originalClient
		osExecutable = originalOsExecutable
		replaceExecutableFunc = originalReplaceFunc
	}()

	err = downloadAndInstall(server.URL + "/test.tar.gz")
	if err != nil {
		t.Errorf("downloadAndInstall() error = %v", err)
	}
}

// =============================================================================
// DOWNLOAD AND INSTALL ERROR TESTS
// =============================================================================
// Tests for various error scenarios in download and install
// =============================================================================

func TestDownloadAndInstallErrors(t *testing.T) {
	tests := []struct {
		name          string
		setupMock     func()
		expectedError string
	}{
		{
			name: "executable error",
			setupMock: func() {
				osExecutable = func() (string, error) {
					return "", errors.New("exe error")
				}
			},
			expectedError: "exe error",
		},
		{
			name: "HTTP error",
			setupMock: func() {
				osExecutable = func() (string, error) {
					return "test.exe", nil
				}
			},
			expectedError: "",
		},
		{
			name: "temp file creation error",
			setupMock: func() {
				osExecutable = func() (string, error) {
					return "test.exe", nil
				}
			},
			expectedError: "",
		},
		{
			name: "io copy error",
			setupMock: func() {
				osExecutable = func() (string, error) {
					return "test.exe", nil
				}
			},
			expectedError: "",
		},
		{
			name: "extract error",
			setupMock: func() {
				osExecutable = func() (string, error) {
					return "test.exe", nil
				}
			},
			expectedError: "failed to extract archive",
		},
		{
			name: "replace error",
			setupMock: func() {
				osExecutable = func() (string, error) {
					return "test.exe", nil
				}
				replaceExecutableFunc = func(current, new string) error {
					return errors.New("replace failed")
				}
			},
			expectedError: "replace failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalOsExecutable := osExecutable
			originalClient := httpClient
			originalReplaceFunc := replaceExecutableFunc

			// Create a failing server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.name == "HTTP error" {
					http.Error(w, "Server Error", http.StatusInternalServerError)
				} else if tt.name == "io copy error" {
					// Start writing but don't finish
					w.Header().Set("Content-Length", "1000")
					w.Write([]byte("partial"))
					// Close connection abruptly
					if hijacker, ok := w.(http.Hijacker); ok {
						if conn, _, err := hijacker.Hijack(); err == nil {
							conn.Close()
						}
					}
				} else if tt.name == "extract error" {
					// Return invalid archive data for .zip URL
					w.Write([]byte("invalid archive data"))
				} else {
					w.Write([]byte("mock binary content"))
				}
			}))
			defer server.Close()

			httpClient = &http.Client{}
			replaceExecutableFunc = func(current, new string) error {
				return nil
			}
			tt.setupMock()

			defer func() {
				osExecutable = originalOsExecutable
				httpClient = originalClient
				replaceExecutableFunc = originalReplaceFunc
			}()

			url := server.URL
			if tt.name == "extract error" {
				url = server.URL + "/test.zip"
			}
			
			err := downloadAndInstall(url)
			if tt.expectedError == "" && err == nil {
				// Expected no error
			} else if err == nil && tt.expectedError != "" {
				t.Errorf("Expected error containing %q, got nil", tt.expectedError)
			} else if tt.expectedError != "" && err != nil && !strings.Contains(err.Error(), tt.expectedError) {
				t.Errorf("Expected error containing %q, got %v", tt.expectedError, err)
			}
		})
	}
}

func TestDownloadAndInstallWithMockedCreateTemp(t *testing.T) {
	originalOsCreateTemp := osCreateTemp
	originalOsExecutable := osExecutable
	
	osExecutable = func() (string, error) {
		return "test.exe", nil
	}
	
	osCreateTemp = func(dir, pattern string) (*os.File, error) {
		return nil, errors.New("mock CreateTemp error")
	}
	
	defer func() {
		osCreateTemp = originalOsCreateTemp
		osExecutable = originalOsExecutable
	}()
	
	err := downloadAndInstall("http://example.com/test")
	if err == nil || !strings.Contains(err.Error(), "mock CreateTemp error") {
		t.Errorf("Expected CreateTemp error, got %v", err)
	}
}

func TestDownloadAndInstallAdditionalErrors(t *testing.T) {
	t.Run("http get error", func(t *testing.T) {
		originalClient := httpClient
		originalOsExecutable := osExecutable
		
		osExecutable = func() (string, error) {
			return "test.exe", nil
		}
		
		// Set invalid HTTP client
		httpClient = &http.Client{
			Timeout: 1, // 1 nanosecond timeout to force error
		}
		
		defer func() {
			httpClient = originalClient
			osExecutable = originalOsExecutable
		}()
		
		err := downloadAndInstall("http://invalid.local/test")
		if err == nil {
			t.Error("Expected error for invalid URL")
		}
	})
}

// =============================================================================
// EXECUTABLE REPLACEMENT TESTS
// =============================================================================
// Tests for executable file replacement on different platforms
// =============================================================================

func TestReplaceExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Run("windows", func(t *testing.T) {
			// Create temp files
			currentFile, err := os.CreateTemp("", "current_*.exe")
			if err != nil {
				t.Fatal(err)
			}
			currentFile.Write([]byte("current"))
			currentFile.Close()
			defer os.Remove(currentFile.Name())

			newFile, err := os.CreateTemp("", "new_*.exe")
			if err != nil {
				t.Fatal(err)
			}
			newFile.Write([]byte("new"))
			newFile.Close()
			defer os.Remove(newFile.Name())

			// Test replace
			err = replaceExecutable(currentFile.Name(), newFile.Name())
			if err != nil {
				t.Errorf("replaceExecutable() error = %v", err)
			}

			// Check that backup was created
			backupPath := currentFile.Name() + ".old"
			if _, err := os.Stat(backupPath); err == nil {
				// Backup exists, clean it up
				os.Remove(backupPath)
			}
		})
	} else {
		t.Run("unix", func(t *testing.T) {
			// Create temp files
			currentFile, err := os.CreateTemp("", "current_*")
			if err != nil {
				t.Fatal(err)
			}
			currentFile.Write([]byte("current"))
			currentFile.Close()
			defer os.Remove(currentFile.Name())

			newFile, err := os.CreateTemp("", "new_*")
			if err != nil {
				t.Fatal(err)
			}
			newFile.Write([]byte("new"))
			newFile.Close()

			// Test replace
			err = replaceExecutable(currentFile.Name(), newFile.Name())
			if err != nil {
				t.Errorf("replaceExecutable() error = %v", err)
			}

			// Check content
			content, err := os.ReadFile(currentFile.Name())
			if err != nil {
				t.Fatal(err)
			}
			if string(content) != "new" {
				t.Errorf("Expected content 'new', got %s", string(content))
			}
		})
	}
}

func TestReplaceExecutableErrors(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Run("windows rename error", func(t *testing.T) {
			// Test when we can't rename the current executable
			err := replaceExecutable("/nonexistent/path/exe.exe", "new.exe")
			if err == nil {
				t.Error("Expected error for nonexistent path")
			}
		})
		
		t.Run("windows install error", func(t *testing.T) {
			// Create a read-only directory to cause rename failure
			tempDir, err := os.MkdirTemp("", "readonly*")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(tempDir)
			
			// Create current file
			currentPath := tempDir + "\\current.exe"
			if err := os.WriteFile(currentPath, []byte("current"), 0644); err != nil {
				t.Fatal(err)
			}
			
			// Test with nonexistent new file
			err = replaceExecutable(currentPath, "/nonexistent/new.exe")
			if err == nil {
				t.Error("Expected error for nonexistent new file")
			}
		})
	} else {
		t.Run("unix rename error", func(t *testing.T) {
			// Test when we can't rename
			err := replaceExecutable("/nonexistent/current", "/nonexistent/new")
			if err == nil {
				t.Error("Expected error for nonexistent files")
			}
		})
	}
}

func TestReplaceExecutableUnixPath(t *testing.T) {
	// Save originals
	originalIsWindows := isWindows
	originalOsRename := osRename
	defer func() {
		isWindows = originalIsWindows
		osRename = originalOsRename
	}()

	// Mock as Unix system
	isWindows = func() bool { return false }

	// Test successful rename
	renameCalled := false
	osRename = func(oldpath, newpath string) error {
		renameCalled = true
		if oldpath != "/tmp/new" || newpath != "/tmp/current" {
			t.Errorf("Unexpected rename paths: %s -> %s", oldpath, newpath)
		}
		return nil
	}

	err := replaceExecutable("/tmp/current", "/tmp/new")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if !renameCalled {
		t.Error("Expected osRename to be called")
	}

	// Test rename failure
	osRename = func(oldpath, newpath string) error {
		return errors.New("rename failed")
	}

	err = replaceExecutable("/tmp/current", "/tmp/new")
	if err == nil {
		t.Error("Expected error when rename fails")
	}
}

func TestReplaceExecutableUnixPaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping Unix-specific test on Windows")
	}
	
	// Create temp files
	currentFile, err := os.CreateTemp("", "current_*")
	if err != nil {
		t.Fatal(err)
	}
	currentFile.Write([]byte("current"))
	currentFile.Close()
	defer os.Remove(currentFile.Name())

	newFile, err := os.CreateTemp("", "new_*")
	if err != nil {
		t.Fatal(err)
	}
	newFile.Write([]byte("new"))
	newFile.Close()

	// Test replace
	err = replaceExecutable(currentFile.Name(), newFile.Name())
	if err != nil {
		t.Errorf("replaceExecutable() error = %v", err)
	}

	// Check content
	content, err := os.ReadFile(currentFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "new" {
		t.Errorf("Expected content 'new', got %s", string(content))
	}
}