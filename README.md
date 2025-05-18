# Video Stream File Processor

A robust application for monitoring a directory for MP4 video stream files, processing them in chunks, and uploading to S3-compatible object storage with checkpoint management and recovery.

## Features
- Watches a specified directory for new or updated `.mp4` files
- Processes files in configurable-size chunks (default: 10MB)
- Uploads chunks sequentially to S3-compatible storage (e.g., MinIO)
- Maintains upload order and checkpointing for reliability
- Detects stream completion via file modification timeout
- Generates metadata JSON for each processed video
- Resumes interrupted uploads and handles network failures
- Exposes HTTP API for managing watched paths
- Prometheus metrics and structured logging
- Containerized with Docker and Docker Compose

## Architecture
- **Language:** Go
- **Checkpoint Storage:** Redis
- **Object Storage:** MinIO (S3-compatible)
- **Containerization:** Docker, Docker Compose
- **Metrics:** Prometheus

## Prerequisites
- [Docker](https://www.docker.com/get-started)
- [Docker Compose](https://docs.docker.com/compose/)

## Setup Instructions

1. **Clone the repository:**
   ```sh
   git clone <your-repo-url>
   cd <repo-directory>
   ```

2. **Configure environment variables:**
   - Copy `.env.example` to `.env` and edit as needed:
     ```sh
     cp .env.example .env
     ```
   - Set values for:
     - `S3_BUCKET`
     - `DEFAULT_INPUT_PATH`
     - `MINIO_ACCESS_KEY`, `MINIO_SECRET_KEY`, `MINIO_ENDPOINT`
     - `REDIS_ADDR`, etc.

3. **Start the stack:**
   ```sh
   docker-compose up --build
   ```
   This will start the app, MinIO, and Redis. The app will watch the directory specified by `DEFAULT_INPUT_PATH` (mounted as a volume).

## Usage

- **Add video files:**
  - Place `.mp4` files in the watched directory (default: `./input-videos`).
  - The app will detect new/updated files and begin processing.

- **API Endpoints:**
  - Add a path to watch:
    ```sh
    curl -X POST http://localhost:8080/v1/path/add -H 'Content-Type: application/json' -d '{"path": "/input-videos"}'
    ```
  - Remove a path from watch:
    ```sh
    curl -X DELETE http://localhost:8080/v1/path/remove -H 'Content-Type: application/json' -d '{"path": "/input-videos"}'
    ```
  - Health check:
    ```sh
    curl http://localhost:8080/health
    ```

- **Access MinIO UI:**
  - Visit [http://localhost:9001](http://localhost:9001)
  - Login with credentials from your `.env` file (default: `minioadmin`/`minioadmin`)
  - Uploaded chunks and metadata will appear in the configured bucket.

- **Prometheus Metrics:**
  - Exposed at `/metrics` (if enabled in the app)

## Running Tests

1. **Install Go (if running tests locally):**
   - [Go installation guide](https://golang.org/doc/install)

2. **Run tests:**
   ```sh
   cd internal/service
   go test -v
   ```
   Or run all tests:
   ```sh
   go test ./...
   ```

## Troubleshooting
- Ensure all environment variables are set correctly in `.env`.
- Check Docker Compose logs for errors:
  ```sh
  docker-compose logs app
  ```
- Make sure the input directory is mounted and accessible by the container.
- If MinIO or Redis are not reachable, check their logs and network settings.
