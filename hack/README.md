# Version Synchronization Script

This script automates the synchronization of version numbers between the project's source of truth and its Containerfiles.

### What this does

The script reads the version number from the central `VERSION` file and updates the version and release labels in the specified Containerfiles. This prevents build failures due to version mismatches and ensures consistency across all built artifacts.

### Why here?
This script lives in the hack/ directory because it's a developer tool for local use. It helps developers prepare their commits and pull requests by ensuring all Containerfiles are properly versioned before pushing changes. While Konflux has its own checks, this script helps maintain a clean and consistent repository history.

### The script
`sync-version.sh`

This script finds and updates version labels in the Containerfiles based on the `VERSION` file. It will fail if the `VERSION` file or a Containerfile is not found, or if a version label is not present in the correct format.

### How to use this
Before committing changes that update the VERSION file, run this script to ensure all Containerfiles are in sync.

### Local testing
Run the script from the repository root

`make set-version`

The script will output UPDATE messages to show the status of each Containerfile and will automatically update any files that are out of sync.