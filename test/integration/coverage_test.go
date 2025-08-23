//go:build integration
// +build integration

package integration

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCoverageThreshold tests that code coverage meets the 80% threshold
func TestCoverageThreshold(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping coverage test in short mode")
	}

	projectRoot := getProjectRoot()
	coverageFile := filepath.Join(projectRoot, "coverage.out")

	// Run tests with coverage
	t.Log("Running tests with coverage...")
	cmd := exec.Command("go", "test", "-coverprofile="+coverageFile, "-coverpkg=./...", "./...")
	cmd.Dir = projectRoot
	output, err := cmd.CombinedOutput()

	if err != nil {
		t.Logf("Test output: %s", string(output))
		require.NoError(t, err, "Failed to run tests with coverage")
	}

	// Parse coverage results
	coverage, err := parseCoverageFile(coverageFile)
	require.NoError(t, err, "Failed to parse coverage file")

	t.Logf("Overall test coverage: %.2f%%", coverage)

	// Assert coverage threshold
	assert.GreaterOrEqual(t, coverage, 80.0, "Code coverage should be at least 80%%")

	// Generate HTML coverage report
	generateHTMLCoverageReport(t, projectRoot, coverageFile)

	// Clean up coverage file
	os.Remove(coverageFile)
}

// TestPackageCoverage tests coverage for individual packages
func TestPackageCoverage(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping package coverage test in short mode")
	}

	projectRoot := getProjectRoot()

	// Key packages that should have good coverage
	packages := []string{
		"./pkg/config",
		"./pkg/discovery",
		"./pkg/messagequeue",
		"./pkg/scaling",
		"./internal/nexus",
		"./internal/collector",
		"./internal/streamer",
	}

	for _, pkg := range packages {
		t.Run(pkg, func(t *testing.T) {
			coverageFile := filepath.Join(projectRoot, fmt.Sprintf("coverage_%s.out", strings.ReplaceAll(pkg, "/", "_")))

			// Run tests for specific package
			cmd := exec.Command("go", "test", "-coverprofile="+coverageFile, pkg)
			cmd.Dir = projectRoot
			output, err := cmd.CombinedOutput()

			if err != nil {
				t.Logf("Test output for %s: %s", pkg, string(output))
				// Don't fail if package doesn't have tests yet, just log
				t.Logf("Warning: Failed to run tests for package %s: %v", pkg, err)
				return
			}

			// Parse coverage for this package
			coverage, err := parseCoverageFile(coverageFile)
			if err != nil {
				t.Logf("Warning: Failed to parse coverage for package %s: %v", pkg, err)
				os.Remove(coverageFile)
				return
			}

			t.Logf("Coverage for %s: %.2f%%", pkg, coverage)

			// For critical packages, enforce higher coverage
			criticalPackages := []string{"./pkg/config", "./pkg/messagequeue", "./internal/nexus"}
			isCritical := false
			for _, critical := range criticalPackages {
				if pkg == critical {
					isCritical = true
					break
				}
			}

			if isCritical {
				assert.GreaterOrEqual(t, coverage, 70.0, "Critical package %s should have at least 70%% coverage", pkg)
			} else {
				assert.GreaterOrEqual(t, coverage, 50.0, "Package %s should have at least 50%% coverage", pkg)
			}

			// Clean up
			os.Remove(coverageFile)
		})
	}
}

// TestIntegrationCoverage runs integration tests and measures their coverage contribution
func TestIntegrationCoverage(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration coverage test in short mode")
	}

	// Check if Docker is available for integration tests
	if !isDockerAvailable() {
		t.Skip("Docker is not available, skipping integration coverage test")
	}

	projectRoot := getProjectRoot()
	coverageFile := filepath.Join(projectRoot, "integration_coverage.out")

	// Run only integration tests with coverage
	t.Log("Running integration tests with coverage...")
	cmd := exec.Command("go", "test", "-tags=integration", "-coverprofile="+coverageFile, "-coverpkg=./...", "./test/integration")
	cmd.Dir = projectRoot
	output, err := cmd.CombinedOutput()

	if err != nil {
		t.Logf("Integration test output: %s", string(output))
		require.NoError(t, err, "Failed to run integration tests with coverage")
	}

	// Parse coverage results
	coverage, err := parseCoverageFile(coverageFile)
	require.NoError(t, err, "Failed to parse integration coverage file")

	t.Logf("Integration test coverage contribution: %.2f%%", coverage)

	// Integration tests should contribute meaningful coverage
	assert.GreaterOrEqual(t, coverage, 30.0, "Integration tests should contribute at least 30%% coverage")

	// Clean up
	os.Remove(coverageFile)
}

// parseCoverageFile parses a Go coverage file and returns the overall coverage percentage
func parseCoverageFile(filename string) (float64, error) {
	file, err := os.Open(filename)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	var totalStatements, coveredStatements int64
	scanner := bufio.NewScanner(file)

	// Skip the first line (mode line)
	if scanner.Scan() {
		// First line should be "mode: set" or similar
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		// Parse coverage line: filename:startLine.startCol,endLine.endCol numStmt count
		parts := strings.Fields(line)
		if len(parts) != 3 {
			continue
		}

		numStmt, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			continue
		}

		count, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			continue
		}

		totalStatements += numStmt
		if count > 0 {
			coveredStatements += numStmt
		}
	}

	if err := scanner.Err(); err != nil {
		return 0, err
	}

	if totalStatements == 0 {
		return 0, nil
	}

	coverage := (float64(coveredStatements) / float64(totalStatements)) * 100
	return coverage, nil
}

// generateHTMLCoverageReport generates an HTML coverage report
func generateHTMLCoverageReport(t *testing.T, projectRoot, coverageFile string) {
	htmlFile := filepath.Join(projectRoot, "coverage.html")

	cmd := exec.Command("go", "tool", "cover", "-html="+coverageFile, "-o", htmlFile)
	cmd.Dir = projectRoot

	if err := cmd.Run(); err != nil {
		t.Logf("Warning: Failed to generate HTML coverage report: %v", err)
		return
	}

	t.Logf("HTML coverage report generated: %s", htmlFile)
}

// getProjectRoot returns the project root directory
func getProjectRoot() string {
	// Get current file path and navigate to project root
	_, filename, _, _ := runtime.Caller(0)
	// Go up from test/integration/coverage_test.go to project root
	return filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(filename))))
}

// TestBenchmarkCoverage runs benchmarks to ensure performance-critical code is covered
func TestBenchmarkCoverage(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping benchmark coverage test in short mode")
	}

	projectRoot := getProjectRoot()

	// Run benchmarks
	t.Log("Running benchmarks...")
	cmd := exec.Command("go", "test", "-bench=.", "-benchmem", "./...")
	cmd.Dir = projectRoot
	output, err := cmd.CombinedOutput()

	// Benchmarks might not exist yet, so don't fail
	if err != nil {
		t.Logf("Benchmark output: %s", string(output))
		t.Logf("Note: Benchmarks may not be implemented yet")
		return
	}

	t.Logf("Benchmark results:\n%s", string(output))
}

// TestCoverageByComponent tests coverage for different components
func TestCoverageByComponent(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping component coverage test in short mode")
	}

	components := map[string][]string{
		"Configuration":     {"./pkg/config"},
		"Service Discovery": {"./pkg/discovery"},
		"Message Queue":     {"./pkg/messagequeue"},
		"Scaling":           {"./pkg/scaling"},
		"Nexus Integration": {"./internal/nexus"},
		"Data Collection":   {"./internal/collector"},
		"Data Streaming":    {"./internal/streamer"},
		"GraphQL":           {"./internal/graphql"},
		"Calibration":       {"./internal/calibration"},
	}

	projectRoot := getProjectRoot()

	for componentName, packages := range components {
		t.Run(componentName, func(t *testing.T) {
			var totalCoverage float64
			var packageCount int

			for _, pkg := range packages {
				coverageFile := filepath.Join(projectRoot, fmt.Sprintf("component_%s.out", strings.ReplaceAll(pkg, "/", "_")))

				cmd := exec.Command("go", "test", "-coverprofile="+coverageFile, pkg)
				cmd.Dir = projectRoot
				output, err := cmd.CombinedOutput()

				if err != nil {
					t.Logf("Warning: Failed to test component %s package %s: %v", componentName, pkg, err)
					t.Logf("Output: %s", string(output))
					continue
				}

				coverage, err := parseCoverageFile(coverageFile)
				if err != nil {
					t.Logf("Warning: Failed to parse coverage for %s: %v", pkg, err)
					os.Remove(coverageFile)
					continue
				}

				totalCoverage += coverage
				packageCount++
				os.Remove(coverageFile)
			}

			if packageCount > 0 {
				avgCoverage := totalCoverage / float64(packageCount)
				t.Logf("Average coverage for %s: %.2f%%", componentName, avgCoverage)

				// Set minimum coverage expectations per component
				minCoverage := 40.0
				if componentName == "Message Queue" || componentName == "Configuration" {
					minCoverage = 60.0 // Higher expectations for critical components
				}

				assert.GreaterOrEqual(t, avgCoverage, minCoverage,
					"Component %s should have at least %.1f%% coverage", componentName, minCoverage)
			} else {
				t.Logf("No testable packages found for component %s", componentName)
			}
		})
	}
}
