# Docker Setup Guide

This guide explains how to build and run the deploycrane application using Docker.

## Prerequisites

- Docker Engine 20.10+
- Docker Compose 2.0+ (for docker-compose.yml)
- At least 2GB of free disk space

## Using Pre-built Image from DockerHub

The easiest way to get started is to use the pre-built image:

```bash
# Pull the latest image from DockerHub
docker pull parsasafavi/deploycrane:latest

# Run it with Docker socket
docker run -d \
  --name deploycrane \
  -p 8080:8080 \
  -v /var/run/docker.sock:/var/run/docker.sock:rw \
  -v ./data:/app/data \
  -v ./clones:/app/clones \
  parsasafavi/deploycrane:latest
```

## Building the Docker Image Locally

### Standard Build

```bash
docker build -t deploycrane:latest .
# Tag for local use
docker tag deploycrane:latest parsasafavi/deploycrane:latest
```

### Build and Tag for DockerHub

```bash
# Build with DockerHub tag
docker build -t parsasafavi/deploycrane:latest .

# Build with version tag
docker build -t parsasafavi/deploycrane:v1.0.0 .
```

## Building and Pushing to DockerHub

### Prerequisites

1. Create a DockerHub account at https://hub.docker.com
2. Log in locally: `docker login`

### Build and Push

```bash
# Build with DockerHub tag
docker build -t parsasafavi/deploycrane:latest .

# Push to DockerHub
docker push parsasafavi/deploycrane:latest

# Tag and push specific version
docker build -t parsasafavi/deploycrane:v1.0.0 .
docker push parsasafavi/deploycrane:v1.0.0

# With BuildKit for better caching
DOCKER_BUILDKIT=1 docker build -t parsasafavi/deploycrane:latest .
docker push parsasafavi/deploycrane:latest

# Multi-platform builds (requires buildx)
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  -t parsasafavi/deploycrane:latest \
  --push .
```

## Running the Container

### Using Pre-built Image from DockerHub

```bash
docker run -d \
  --name deploycrane \
  -p 8080:8080 \
  -v /var/run/docker.sock:/var/run/docker.sock:rw \
  -v ./data:/app/data \
  -v ./clones:/app/clones \
  parsasafavi/deploycrane:latest
```

### Using Locally Built Image

```bash
docker run -d \
  --name deploycrane \
  -p 8080:8080 \
  -v /var/run/docker.sock:/var/run/docker.sock:rw \
  -v ./data:/app/data \
  -v ./clones:/app/clones \
  deploycrane:latest
```

### Using Docker Compose (Recommended for development)

```bash
# Start the service
docker-compose up -d

# View logs
docker-compose logs -f deploycrane

# Stop the service
docker-compose down

# Stop and remove volumes
docker-compose down -v
```

## Environment Variables

Configure via `-e` flag or in `.env` file:

| Variable | Default | Description |
|----------|---------|-------------|
| `LISTEN_PORT` | 8080 | HTTP server port |
| `SERVER_ADDR` | 0.0.0.0 | Server bind address |
| `DB_PATH` | /app/data/deploycrane.db | SQLite database path |
| `CLONE_BASE_DIR` | /app/clones | Directory for cloned repositories |
| `IMAGE_PREFIX` | deploycrane | Docker image prefix for built containers |
| `PORT_ALLOCATION_MIN` | 8000 | Minimum port for container allocation |
| `PORT_ALLOCATION_MAX` | 9000 | Maximum port for container allocation |
| `CORS_ORIGINS` | * | Allowed CORS origins |
| `READ_TIMEOUT` | 15s | HTTP read timeout |
| `WRITE_TIMEOUT` | 15s | HTTP write timeout |
| `IDLE_TIMEOUT` | 60s | HTTP idle timeout |
| `SHUTDOWN_TIMEOUT` | 30s | Graceful shutdown timeout |

## Health Check

The container includes a health check endpoint:

```bash
curl http://your-server-address:8080/health
```

## Volume Mounts

The container uses several volumes:

- `/var/run/docker.sock` — Docker daemon socket (required for container management)
- `/app/data` — SQLite database and persistent data
- `/app/clones` — Cloned repository files

## Security Notes

1. **Non-root user**: The application runs as `app` user (UID 1000) for security
2. **Docker socket**: Mount with `:rw` permission to allow container management
3. **Network**: Only expose port 8080 when necessary
4. **Data persistence**: Use named volumes or bind mounts to persist data across container restarts

## Pulling from DockerHub

```bash
# Pull the latest version
docker pull parsasafavi/deploycrane:latest

# Pull specific version
docker pull parsasafavi/deploycrane:v1.0.0

# Check available tags
# Visit: https://hub.docker.com/r/parsasafavi/deploycrane/tags
```

## Troubleshooting

### "Cannot connect to Docker daemon"
- Ensure Docker socket is mounted: `-v /var/run/docker.sock:/var/run/docker.sock:rw`
- Check Docker daemon is running: `docker ps`

### "Database is locked"
- Multiple container instances writing to the same SQLite file
- Use separate database files or implement locking mechanism

### "Port already in use"
- Change port mapping: `-p 8081:8080`
- Or kill existing container: `docker stop deploycrane && docker rm deploycrane`

### "Image not found" when pulling from DockerHub
- Ensure you have internet access
- Verify the image exists: `docker search parsasafavi/deploycrane`
- Check spelling: `parsasafavi/deploycrane:latest`

## Performance Optimization

1. Use Alpine Linux as base (already done) — ~5-10MB smaller than Debian
2. Multi-stage build (already done) — build dependencies not included in runtime image
3. Stripped binary (already done) — reduces binary size by ~60%
4. Layer caching — place frequently changing layers later in Dockerfile

Final image size: ~50-60MB
