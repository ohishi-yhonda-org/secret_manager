package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type SymlinkConfig struct {
	Targets []Target `json:"targets"`
}

type Target struct {
	Path        string `json:"path"`
	Description string `json:"description"`
}

// exitFunc is a variable to allow mocking in tests
var exitFunc = os.Exit

// executableDir is a variable to allow mocking in tests
var executableDir = getExecutableDir

// Version information (set at build time)
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// osExecutable is a variable to allow mocking in tests
var osExecutable = os.Executable

// filepathWalk is a variable to allow mocking in tests
var filepathWalk = filepath.Walk

// findSecretDirs is a variable to allow mocking in tests
var findSecretDirs = findSecretDirectories

// checkAndUpdateFunc is a variable to allow mocking in tests
var checkAndUpdateFunc = checkAndUpdate

func getExecutableDir() (string, error) {
	exe, err := osExecutable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(exe), nil
}

// findSecretDirectories recursively finds all directories containing "secret" in their name
func findSecretDirectories(root string) ([]string, error) {
	var secretDirs []string
	
	err := filepathWalk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip directories that can't be accessed
		}
		
		if info.IsDir() && strings.Contains(strings.ToLower(info.Name()), "secret") {
			secretDirs = append(secretDirs, path)
		}
		
		return nil
	})
	
	if err != nil {
		return nil, err
	}
	
	return secretDirs, nil
}

// parseFlags is a variable to allow mocking in tests
var parseFlags func() (*bool, *bool)

// defaultParseFlags is the default implementation of parseFlags
func defaultParseFlags() (*bool, *bool) {
	versionFlag := flag.Bool("version", false, "Show version information")
	updateFlag := flag.Bool("update", false, "Check for updates and install if available")
	flag.Parse()
	return versionFlag, updateFlag
}

func init() {
	parseFlags = defaultParseFlags
}

func main() {
	// Parse command line flags
	versionFlag, updateFlag := parseFlags()

	// Handle version flag
	if *versionFlag {
		fmt.Printf("secret_manager version %s (commit: %s, built: %s)\n", version, commit, date)
		exitFunc(0)
	}

	// Handle update flag
	if *updateFlag {
		if err := checkAndUpdateFunc(); err != nil {
			fmt.Fprintf(os.Stderr, "Error checking for updates: %v\n", err)
			exitFunc(1)
		}
		exitFunc(0)
	}

	// Get the directory where the executable is located
	exeDir, err := executableDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting executable directory: %v\n", err)
		exitFunc(1)
	}
	
	// Change to executable directory
	err = os.Chdir(exeDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error changing directory: %v\n", err)
		exitFunc(1)
	}
	
	// Find all directories containing "secret" in their name
	secretDirs, err := findSecretDirs(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error finding secret directories: %v\n", err)
		exitFunc(1)
	}
	
	if len(secretDirs) == 0 {
		fmt.Println("No directories containing 'secret' found")
		exitFunc(0)
	}
	
	fmt.Printf("Found %d secret directories\n", len(secretDirs))
	
	// Process each secret directory
	for _, secretDir := range secretDirs {
		fmt.Printf("\nProcessing: %s\n", secretDir)
		err = processSecretDirectory(secretDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error processing %s: %v\n", secretDir, err)
			// Continue with other directories
		}
	}
	
	fmt.Println("Symlink creation completed successfully!")
}

func processSecretDirectory(secretDir string) error {
	files, err := readDirFunc(secretDir)
	if err != nil {
		return fmt.Errorf("failed to read secret directory: %w", err)
	}
	
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		
		if strings.HasSuffix(file.Name(), ".symlink.json") {
			sourceFile := strings.TrimSuffix(file.Name(), ".symlink.json")
			sourcePath := filepath.Join(secretDir, sourceFile)
			configPath := filepath.Join(secretDir, file.Name())
			
			if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
				fmt.Printf("Warning: Source file %s does not exist, skipping\n", sourcePath)
				continue
			}
			
			err := processSymlinkConfig(sourcePath, configPath)
			if err != nil {
				fmt.Printf("Error processing %s: %v\n", configPath, err)
			}
		}
	}
	
	return nil
}

func processSymlinkConfig(sourcePath, configPath string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}
	
	var config SymlinkConfig
	err = json.Unmarshal(data, &config)
	if err != nil {
		return fmt.Errorf("failed to parse JSON: %w", err)
	}
	
	for _, target := range config.Targets {
		err := createSymlink(sourcePath, target)
		if err != nil {
			fmt.Printf("Failed to create symlink for %s: %v\n", target.Path, err)
		}
	}
	
	return nil
}

// Functions that can be mocked in tests
var (
	symlinkFunc = os.Symlink
	removeFunc  = os.Remove
	lstatFunc   = os.Lstat
	readDirFunc = os.ReadDir
)

func createSymlink(sourcePath string, target Target) error {
	targetPath := target.Path
	
	// Check if target directory exists
	targetDir := filepath.Dir(targetPath)
	if _, err := os.Stat(targetDir); os.IsNotExist(err) {
		fmt.Printf("Error: Target directory does not exist: %s\n", targetDir)
		return nil // Continue with next target
	}
	
	if _, err := lstatFunc(targetPath); err == nil {
		err = removeFunc(targetPath)
		if err != nil {
			return fmt.Errorf("failed to remove existing symlink: %w", err)
		}
	}
	
	err := symlinkFunc(sourcePath, targetPath)
	if err != nil {
		return fmt.Errorf("failed to create symlink: %w", err)
	}
	
	fmt.Printf("Created symlink: %s -> %s (%s)\n", targetPath, sourcePath, target.Description)
	
	return nil
}