# Continuous integration

The repository includes the following GitHub Actions workflows:

- `.github/workflows/ci.yml`: runs `go mod tidy` tidiness checks, `gofmt`, `go vet`, `staticcheck`, builds, and race tests for the backend; runs type checking and builds for the frontend; and performs an end-to-end build of both.
- `.github/workflows/docker.yml`: builds multi-architecture container images and pushes them to GHCR (pull requests only build, they do not push).
- `.github/workflows/release.yml`: when a `v*` tag is pushed (or a tag is specified manually), cross-compiles binaries for multiple platforms and creates a GitHub Release; release notes are generated from the commit messages between that tag and the previous tag (for the first release, from the commit messages of that tag).
