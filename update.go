package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	githubAPI = "https://api.github.com/repos/ohishi-yhonda-org/secret_manager/releases/latest"
	userAgent = "secret_manager-updater"
)

type GitHubRelease struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// httpClient is a variable to allow mocking in tests
var httpClient = &http.Client{Timeout: 30 * time.Second}

// downloadAndInstallFunc is a variable to allow mocking in tests
var downloadAndInstallFunc = downloadAndInstall

// replaceExecutableFunc is a variable to allow mocking in tests
var replaceExecutableFunc = replaceExecutable

// osCreate is a variable to allow mocking in tests
var osCreate = os.Create

// osCreateTemp is a variable to allow mocking in tests
var osCreateTemp = os.CreateTemp

// httpNewRequest is a variable to allow mocking in tests
var httpNewRequest = http.NewRequest

// ioCopy is a variable to allow mocking in tests
var ioCopy = io.Copy

// zipFileOpen is a variable to allow mocking in tests
var zipFileOpen = func(f *zip.File) (io.ReadCloser, error) {
	return f.Open()
}

// osChmod is a variable to allow mocking in tests
var osChmod = os.Chmod

// osRename is a variable to allow mocking in tests
var osRename = os.Rename

// osRemove is a variable to allow mocking in tests
var osRemove = os.Remove

// isWindows is a variable to allow mocking in tests
var isWindows = func() bool {
	return runtime.GOOS == "windows"
}

func checkAndUpdate() error {
	fmt.Println("Checking for updates...")

	// Get latest release info
	release, err := getLatestRelease()
	if err != nil {
		return fmt.Errorf("failed to get latest release: %w", err)
	}

	// Compare versions
	latestVersion := strings.TrimPrefix(release.TagName, "v")
	currentVersion := strings.TrimPrefix(version, "v")

	if currentVersion == "dev" {
		fmt.Println("Running development version, skipping update check")
		return nil
	}

	if latestVersion == currentVersion {
		fmt.Printf("Already running the latest version (%s)\n", version)
		return nil
	}

	fmt.Printf("New version available: %s (current: %s)\n", release.TagName, version)

	// Find appropriate asset for current platform
	assetURL := findAssetURL(release)
	if assetURL == "" {
		return fmt.Errorf("no suitable binary found for %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	// Download and install update
	fmt.Println("Downloading update...")
	if err := downloadAndInstallFunc(assetURL); err != nil {
		return fmt.Errorf("failed to install update: %w", err)
	}

	fmt.Println("Update completed successfully!")
	fmt.Println("Please restart the application to use the new version.")
	return nil
}

func getLatestRelease() (*GitHubRelease, error) {
	req, err := httpNewRequest("GET", githubAPI, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}

	return &release, nil
}

func findAssetURL(release *GitHubRelease) string {
	platform := fmt.Sprintf("%s-%s", runtime.GOOS, runtime.GOARCH)
	
	// Special case for Windows
	if isWindows() {
		platform = fmt.Sprintf("windows-%s.exe", runtime.GOARCH)
	}

	for _, asset := range release.Assets {
		if strings.Contains(asset.Name, platform) {
			return asset.BrowserDownloadURL
		}
	}

	return ""
}

func downloadAndInstall(url string) error {
	// Get current executable path
	exePath, err := osExecutable()
	if err != nil {
		return err
	}

	// Download to temporary file
	tempFile, err := osCreateTemp("", "secret_manager_update_*")
	if err != nil {
		return err
	}
	defer os.Remove(tempFile.Name())

	resp, err := httpClient.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	_, err = ioCopy(tempFile, resp.Body)
	tempFile.Close()
	if err != nil {
		return err
	}

	// Extract if archive, otherwise use directly
	var updatePath string
	if strings.HasSuffix(url, ".zip") {
		updatePath, err = extractZip(tempFile.Name())
	} else if strings.HasSuffix(url, ".tar.gz") {
		updatePath, err = extractTarGz(tempFile.Name())
	} else {
		updatePath = tempFile.Name()
	}
	
	if err != nil {
		return fmt.Errorf("failed to extract archive: %w", err)
	}
	if updatePath != tempFile.Name() {
		defer os.Remove(updatePath)
	}

	// Replace current executable
	return replaceExecutableFunc(exePath, updatePath)
}

func extractZip(archivePath string) (string, error) {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", err
	}
	defer reader.Close()

	for _, file := range reader.File {
		if strings.Contains(file.Name, "secret_manager") {
			extractPath := filepath.Join(os.TempDir(), file.Name)
			
			rc, err := zipFileOpen(file)
			if err != nil {
				return "", err
			}
			defer rc.Close()

			out, err := osCreate(extractPath)
			if err != nil {
				return "", err
			}
			defer out.Close()

			_, err = ioCopy(out, rc)
			if err != nil {
				return "", err
			}

			return extractPath, nil
		}
	}

	return "", fmt.Errorf("executable not found in archive")
}

func extractTarGz(archivePath string) (string, error) {
	file, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return "", err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}

		if strings.Contains(header.Name, "secret_manager") {
			extractPath := filepath.Join(os.TempDir(), filepath.Base(header.Name))
			
			out, err := osCreate(extractPath)
			if err != nil {
				return "", err
			}
			defer out.Close()

			_, err = ioCopy(out, tr)
			if err != nil {
				return "", err
			}

			// Set executable permissions on Unix-like systems
			if !isWindows() {
				osChmod(extractPath, 0755)
			}

			return extractPath, nil
		}
	}

	return "", fmt.Errorf("executable not found in archive")
}

func replaceExecutable(currentPath, newPath string) error {
	// On Windows, we need to rename the current executable first
	if isWindows() {
		backupPath := currentPath + ".old"
		
		// Remove old backup if exists
		osRemove(backupPath)
		
		// Rename current executable
		if err := osRename(currentPath, backupPath); err != nil {
			return fmt.Errorf("failed to backup current executable: %w", err)
		}

		// Move new executable
		if err := osRename(newPath, currentPath); err != nil {
			// Try to restore backup
			osRename(backupPath, currentPath)
			return fmt.Errorf("failed to install new executable: %w", err)
		}

		// Schedule old executable deletion (will happen after process exits)
		go func() {
			time.Sleep(5 * time.Second)
			osRemove(backupPath)
		}()
	} else {
		// On Unix-like systems, we can directly replace
		if err := osRename(newPath, currentPath); err != nil {
			return fmt.Errorf("failed to install new executable: %w", err)
		}
	}

	return nil
}