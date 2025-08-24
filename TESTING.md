# Comprehensive Testing Guide - Telemetry Pipeline

This document provides a complete guide to testing the telemetry-pipeline project with comprehensive coverage analysis.

## 🚀 Quick Start

To run the complete test suite with full coverage analysis, simply run:

```bash
make test
```

This will execute all test types and generate comprehensive coverage reports.

## 📋 Test Types

The telemetry-pipeline project includes multiple types of tests:

### 1. Unit Tests
- **Location**: Throughout the codebase (`*_test.go` files)
- **Purpose**: Test individual functions and components in isolation
- **Command**: `make test-unit`
- **Coverage**: Measures code coverage at the function level

### 2. Integration Tests
- **Location**: `test/integration/`
- **Purpose**: Test component interactions and service integration
- **Command**: `make test-integration`
- **Requirements**: Docker (for etcd and other services)
- **Build Tag**: `integration`

### 3. End-to-End (E2E) Tests
- **Location**: `test/e2e/`
- **Purpose**: Test complete workflows and user scenarios
- **Command**: `make test-e2e`
- **Requirements**: etcd binary

### 4. Benchmark Tests
- **Purpose**: Performance testing and regression detection
- **Command**: `go test -bench=. ./...`
- **Output**: `benchmark-results.txt`

## 🎯 Coverage Goals

The project maintains the following coverage targets:

- **Overall Project Coverage**: ≥80%
- **Critical Packages**: ≥70%
  - `pkg/config`
  - `pkg/messagequeue`
  - `internal/nexus`
- **Standard Packages**: ≥50%
- **Integration Test Contribution**: ≥30%

## 🛠️ Available Commands

### Primary Commands

| Command | Description |
|---------|-------------|
| `make test` | Run comprehensive test suite (default) |
| `make test-comprehensive` | Explicit comprehensive testing |
| `make test-coverage` | Legacy alias for comprehensive testing |

### Individual Test Types

| Command | Description |
|---------|-------------|
| `make test-unit` | Unit tests only |
| `make test-integration` | Integration tests only |
| `make test-e2e` | End-to-end tests only |
| `make test-race` | Tests with race detection |

### Specialized Commands

| Command | Description | Usage |
|---------|-------------|-------|
| `make test-specific` | Run specific test | `make test-specific TEST=TestName` |
| `make test-package` | Test specific package | `make test-package PKG=./pkg/messagequeue` |
| `make test-watch` | Watch mode testing | Requires `entr` tool |
| `make test-clean` | Clean test artifacts | Removes coverage files |

### Script Alternative

You can also use the standalone script:

```bash
# Run comprehensive tests
./scripts/run-comprehensive-tests.sh

# Show help
./scripts/run-comprehensive-tests.sh --help

# Run only unit tests
./scripts/run-comprehensive-tests.sh --unit-only

# Run only integration tests
./scripts/run-comprehensive-tests.sh --integration-only

# Run only E2E tests
./scripts/run-comprehensive-tests.sh --e2e-only
```

## 📊 Coverage Reports

After running tests, the following reports are generated:

### HTML Reports
- **`coverage.html`** - Combined coverage report
- **`coverage-reports/unit-coverage.html`** - Unit test coverage
- **`coverage-reports/integration-coverage.html`** - Integration test coverage
- **`coverage-reports/e2e-coverage.html`** - E2E test coverage

### Text Reports
- **`coverage-summary.txt`** - Detailed coverage by function
- **`benchmark-results.txt`** - Benchmark test results

### Coverage Analysis
The test suite automatically analyzes coverage and reports:
- ✅ **Pass**: Coverage ≥80%
- ❌ **Fail**: Coverage <80%

## 🔧 Prerequisites

### Required
- **Go 1.23+** - Core testing framework
- **Make** - Build automation (optional)

### Optional (for full test suite)
- **Docker** - Required for integration tests
- **etcd** - Required for E2E tests
- **bc** - For precise coverage calculations
- **entr** - For watch mode testing

### Installation

#### macOS (Homebrew)
```bash
# Install etcd
brew install etcd

# Install optional tools
brew install bc entr
```

#### Linux (Ubuntu/Debian)
```bash
# Install etcd
sudo apt-get update
sudo apt-get install etcd

# Install optional tools
sudo apt-get install bc entr
```

## 🏗️ Test Architecture

### Test Execution Flow

1. **Environment Setup**
   - Clean previous artifacts
   - Create coverage directories
   - Clear test cache

2. **Unit Tests**
   - Run with race detection
   - Generate `unit-coverage.out`

3. **Integration Tests**
   - Check Docker availability
   - Run with integration tag
   - Generate `integration-coverage.out`

4. **E2E Tests**
   - Check etcd availability
   - Run end-to-end scenarios
   - Generate `e2e-coverage.out`

5. **Benchmark Tests**
   - Performance testing
   - Generate `benchmark-results.txt`

6. **Coverage Analysis**
   - Merge coverage profiles
   - Generate HTML reports
   - Analyze thresholds

### Coverage Profile Merging

The test suite merges coverage from all test types:

```
unit-coverage.out + integration-coverage.out + e2e-coverage.out = coverage.out
```

This provides a comprehensive view of code coverage across all testing scenarios.

## 🚨 Troubleshooting

### Common Issues

#### Docker Not Available
```
⚠️  Docker not available, skipping integration tests
```
**Solution**: Install Docker or run unit tests only: `make test-unit`

#### etcd Not Available
```
⚠️  etcd not available, skipping E2E tests
```
**Solution**: Install etcd or run without E2E: `make test-unit test-integration`

#### Coverage Below Threshold
```
❌ Coverage below threshold (target: ≥80%, actual: 75.2%)
```
**Solution**: Add more tests to increase coverage

#### Test Timeout
```
panic: test timed out after 10m0s
```
**Solution**: Increase timeout or optimize tests

### Debug Commands

```bash
# Check test status
make test-unit -n  # Dry run

# Verbose test output
go test -v ./...

# Test specific package with verbose output
go test -v ./pkg/messagequeue

# Check coverage for specific package
go test -coverprofile=debug.out ./pkg/messagequeue
go tool cover -html=debug.out -o debug.html
```

## 📈 Continuous Integration

### CI Pipeline Integration

The comprehensive test suite is designed for CI/CD integration:

```yaml
# Example GitHub Actions
- name: Run Comprehensive Tests
  run: make test

- name: Upload Coverage Reports
  uses: actions/upload-artifact@v3
  with:
    name: coverage-reports
    path: |
      coverage.html
      coverage-reports/
      benchmark-results.txt
```

### Coverage Enforcement

The test suite enforces coverage thresholds and will fail if coverage drops below 80%.

## 🔄 Development Workflow

### Pre-commit Testing
```bash
# Quick unit tests
make test-unit

# Watch mode for development
make test-watch
```

### Feature Development
```bash
# Test specific feature
make test-package PKG=./internal/new-feature

# Full integration testing
make test-comprehensive
```

### Release Testing
```bash
# Complete test suite
make test

# Performance validation
go test -bench=. ./...
```

## 📚 Best Practices

### Writing Tests

1. **Unit Tests**: Focus on single functions/methods
2. **Integration Tests**: Test component interactions
3. **E2E Tests**: Test complete user workflows
4. **Benchmarks**: Test performance-critical paths

### Coverage Guidelines

1. **Aim for 80%+ overall coverage**
2. **Critical packages should have 70%+ coverage**
3. **Test both happy path and error conditions**
4. **Include edge cases and boundary conditions**

### Test Organization

1. **Use build tags for integration tests**: `//go:build integration`
2. **Group related tests in test suites**
3. **Use table-driven tests for multiple scenarios**
4. **Mock external dependencies appropriately**

## 🤝 Contributing

When contributing to the project:

1. **Run comprehensive tests**: `make test`
2. **Ensure coverage thresholds are met**
3. **Add tests for new functionality**
4. **Update test documentation as needed**

## 📞 Support

If you encounter issues with testing:

1. Check this documentation
2. Review the troubleshooting section
3. Run tests with verbose output: `go test -v`
4. Check individual test logs in `coverage-reports/`

---

**Happy Testing! 🧪**
