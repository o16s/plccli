# Docker Release Process

This document explains how to release `plccli` Docker images to ensure consistent versions across all architectures.

## Background

The Docker image is built for multiple architectures:
- `linux/amd64` (x86_64 servers)
- `linux/arm64` (ARM servers, Apple Silicon)

**Critical:** The Dockerfile downloads pre-built binaries from GitHub releases. To ensure all architectures get the **same version**, we pin the version explicitly.

## Version Pinning

The Dockerfile uses a build argument to pin the plccli version:

```dockerfile
ARG PLCCLI_VERSION=v0.3.5
```

This ensures that when Docker builds the image for different architectures, **all architectures download the same release**.

## Release Workflow

### Step 1: Prepare the Release

**Update version in `docker/Dockerfile`:**

```dockerfile
ARG PLCCLI_VERSION=v0.3.6  # Update this
```

**Note:** You do NOT need to update `main.go`. The version is automatically set at build time from git tags via the Makefile:
```makefile
VERSION ?= $(shell git describe --tags --always --dirty)
```

### Step 2: Update Documentation

- Update README.md if needed
- Update CHANGELOG if you maintain one

### Step 3: Commit Changes

```bash
git add docker/Dockerfile README.md
git commit -m "Release v0.3.6: <brief description of changes>"
git push origin main
```

This triggers the Docker workflow which builds and pushes `ghcr.io/o16s/plccli:latest`.

### Step 4: Create Git Tag for Binary Release

```bash
git tag v0.3.6
git push origin v0.3.6
```

This triggers the build workflow which:
- Runs tests
- Builds binaries for all platforms
- Creates a GitHub Release with binaries attached

### Step 5: Verify Release

**Check binaries on GitHub:**
```
https://github.com/o16s/plccli/releases/tag/v0.3.6
```

Should have:
- `plccli-darwin-arm64.tar.gz`
- `plccli-linux-amd64.tar.gz`
- `plccli-linux-arm64.tar.gz`

**Verify Docker image:**

Pull and test on different architectures:

```bash
# On Linux amd64 server
docker pull ghcr.io/o16s/plccli:latest
docker run --rm --entrypoint plccli ghcr.io/o16s/plccli:latest --version

# On Mac arm64
docker pull ghcr.io/o16s/plccli:latest
docker run --rm --entrypoint plccli ghcr.io/o16s/plccli:latest --version
```

**Expected output (both should match):**
```
plccli version v0.3.6
Commit: <commit-hash>
Built: <timestamp>
Copyright Octanis Instruments GmbH 2024
```

## GitHub Actions Workflows

### Build Workflow (`.github/workflows/build.yml`)

**Triggers:**
- Push to `main` or `master` branch
- Push tags matching `v*` (e.g., v0.3.6)
- Manual trigger via workflow_dispatch

**Actions:**
- Runs tests
- Builds binaries for macOS (arm64) and Linux (amd64, arm64)
- Creates GitHub Release (only on tag push)
- Uploads binaries as artifacts

### Docker Workflow (`.github/workflows/docker-publish.yml`)

**Triggers:**
- Push to `main` branch
- Pull requests to `main`

**Actions:**
- Builds multi-arch Docker image (linux/amd64, linux/arm64)
- Pushes to GitHub Container Registry
- Tags as `latest` for main branch

## Troubleshooting

### Different Versions on Different Architectures

**Problem:** Running the same image SHA shows different versions on amd64 vs arm64.

**Cause:** The `PLCCLI_VERSION` in Dockerfile doesn't match the actual latest release, or there was a race condition during the multi-arch build.

**Solution:**
1. Check what version is pinned in `docker/Dockerfile`:
   ```bash
   grep "ARG PLCCLI_VERSION" docker/Dockerfile
   ```

2. Check what's the latest release on GitHub:
   ```
   https://github.com/o16s/plccli/releases/latest
   ```

3. If they don't match, update Dockerfile and push to main:
   ```bash
   # Edit docker/Dockerfile - update ARG PLCCLI_VERSION=v0.3.X
   git add docker/Dockerfile
   git commit -m "Fix: Update Docker image to use plccli v0.3.X"
   git push origin main
   ```

### Docker Build Fails: 404 Not Found

**Problem:** Docker build fails with "curl: (22) The requested URL returned error: 404"

**Cause:** The version specified in `PLCCLI_VERSION` doesn't exist as a GitHub release.

**Solution:**
1. Check if the release exists:
   ```
   https://github.com/o16s/plccli/releases/tag/v0.3.X
   ```

2. If the release doesn't exist, you need to create it first:
   ```bash
   git tag v0.3.X
   git push origin v0.3.X
   ```

3. Wait for the build workflow to complete and create the release

4. Then trigger the Docker build again (push to main or manual workflow trigger)

### Binary Release Exists But No Docker Image

**Problem:** GitHub release exists with binaries, but no new Docker image.

**Cause:** Docker workflow only triggers on push to `main` branch, not on tags.

**Solution:**
1. Make a commit to main (e.g., update Dockerfile version)
2. Or manually trigger the Docker workflow:
   - Go to Actions → "Build and Push Docker Image"
   - Click "Run workflow" → Select `main` branch → Run

### Testing Docker Image Before Production

Build locally to test:

```bash
cd docker
docker build -t plccli-test .
docker run --rm --entrypoint plccli plccli-test --version
```

Test with a specific version:

```bash
docker build --build-arg PLCCLI_VERSION=v0.3.5 -t plccli-test .
```

## Version Consistency Checklist

Before releasing, verify:

- [ ] Version in `docker/Dockerfile` ARG matches release version (e.g., v0.3.6)
- [ ] All tests pass (`make test`)
- [ ] Binary builds successfully (`make build`)
- [ ] Commit message describes the release
- [ ] Git tag matches the version (e.g., v0.3.6)
- [ ] After release: Both architectures show same version in Docker image
- [ ] Verify `git describe --tags` output matches expected version

## Release Schedule

**Production deployments:**
- Always use pinned versions in docker-compose.yml:
  ```yaml
  image: ghcr.io/o16s/plccli@sha256:3177d2d1d29ebf9bcc7b61c3ba323ea8f06f3999a4dc63cca235768d40668fd4
  ```

- Or use tagged versions (when version tags are implemented):
  ```yaml
  image: ghcr.io/o16s/plccli:0.3.6
  ```

**Development/testing:**
- Can use `latest` tag:
  ```yaml
  image: ghcr.io/o16s/plccli:latest
  ```

## Future Improvements

### Semantic Version Tags for Docker

Currently Docker images are only tagged as `latest`. To add version tags:

Update `.github/workflows/docker-publish.yml`:

```yaml
on:
  push:
    branches: [ main ]
    tags:
      - 'v*'  # Add this

- name: Extract metadata
  id: meta
  uses: docker/metadata-action@v5
  with:
    images: ghcr.io/${{ github.repository_owner }}/plccli
    tags: |
      type=ref,event=branch
      type=ref,event=pr
      type=semver,pattern={{version}}  # Creates 0.3.6
      type=semver,pattern={{major}}.{{minor}}  # Creates 0.3
      type=raw,value=latest,enable={{is_default_branch}}
```

This would allow:
- `ghcr.io/o16s/plccli:latest`
- `ghcr.io/o16s/plccli:0.3.6`
- `ghcr.io/o16s/plccli:0.3`

### Build Binary in Dockerfile

Instead of downloading from releases, build directly:

```dockerfile
FROM golang:1.24.3-alpine AS builder
ARG VERSION=v0.3.6
ARG COMMIT=unknown
WORKDIR /build
COPY . .
RUN go build -ldflags="-X 'main.buildVersion=${VERSION}' -X 'main.buildCommit=${COMMIT}'" -o plccli .

FROM telegraf:latest
COPY --from=builder /build/plccli /usr/bin/plccli
# ... rest
```

Benefits:
- No dependency on GitHub releases
- Guaranteed same source = same version
- Faster CI/CD pipeline

## Contact

For issues with releases, check:
- GitHub Actions: https://github.com/o16s/plccli/actions
- Container Registry: https://github.com/o16s/plccli/pkgs/container/plccli
- Issues: https://github.com/o16s/plccli/issues
