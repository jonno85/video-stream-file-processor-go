## **Objective**

Develop a robust application that monitors a specified directory for MP4 video stream files, processes them in chunks, and uploads to S3-compatible object storage with checkpoint management.

## **Technical Requirements**

### **Core Functionality**

1. **File Monitoring**  
   * Watch a specified directory for new `.mp4` files  
   * Detect file modifications to determine active video streams  
   * Support continuous file updates during streaming  
2. **Chunk Processing**  
   * Read input files in 10MB chunks  
   * Upload chunks sequentially to S3 object storage  
   * Maintain chunk upload order and tracking  
3. **Stream Detection**  
   * Implement a mechanism to detect when a video stream is complete  
   * Use file modification timeout (configurable, default 30 seconds)  
   * Handle partial and complete video streams gracefully  
4. **Checkpoint and Reliability**  
   * Implement checkpoint mechanism to track:  
     * Uploaded chunks  
     * Current file processing state  
     * Upload progress  
   * Support resuming interrupted uploads  
   * Handle network failures and application restarts  
5. **Metadata Management**  
   * Generate a metadata JSON file for each processed video stream  
   * Include information:  
     * Chunk list  
     * Upload timestamps  
     * Stream duration  
     * Total file size

### **Technical Stack**

* **Language**: any of JavaScript/TypeScript, Go, Java/Kotlin, Rust, Python  
* **Checkpoint Storage**: any of your preference (database/persistent queue/key-value)  
* **Containerization**: Docker

**Implementation Constraints**

* Implement robust error handling  
* Provide configurable parameters  
* Ensure idempotent chunk uploads  
* Support cancellation and graceful shutdown

## **Acceptance Criteria**

1. Docker Compose setup with:  
   * Application service  
   * Checkpoints storage  
   * MinIO (or any other S3-compatible storage)  
2. Mountable local volume for input files  
3. Successful processing of multiple MP4 files  
4. Chunks and metadata visible in MinIO  
5. Resilience to network interruptions  
6. Checkpoint recovery mechanism

## **Detailed Workflow**

1. Application starts and configures watchers  
2. New MP4 file detected in watched directory  
3. File chunks read and uploaded to S3  
4. Metadata tracked in Redis  
5. Stream completion detected  
6. Final metadata file written  
7. Ability to resume from last checkpoint on restart

## **Non-Functional Requirements**

* Logging of all significant events  
* Prometheus metrics for monitoring  
* Configurable timeout and chunk size  
* Minimal resource consumption

## **Deliverables**

* Complete source code in public Github repository  
* Dockerfile  
* Docker Compose configuration  
* README with setup and usage instructions  
* Example configuration files

## **Evaluation Criteria**

* Code quality  
* Error handling  
* Performance  
* Scalability  
* Documentation

## **Samples**

Here are some code examples that can give you more context. You don’t have to reuse it.

**Sample Implementation**

```javascript
import fs from 'fs';
import path from 'path';
import chokidar from 'chokidar';
import { S3Client, PutObjectCommand } from '@aws-sdk/client-s3';
import Redis from 'ioredis';
import { v4 as uuidv4 } from 'uuid';

interface StreamProcessorConfig {
  watchDir: string;
  s3Bucket: string;
  chunkSize: number;
  streamTimeout: number;
}

class VideoStreamProcessor {
  private s3Client: S3Client;
  private redisClient: Redis;
  private config: StreamProcessorConfig;

  constructor(config: StreamProcessorConfig) {
    this.config = config;
    this.s3Client = new S3Client({ /* S3 configuration */ });
    this.redisClient = new Redis(/* Redis connection */);
  }

  async startWatching() {
    const watcher = chokidar.watch(this.config.watchDir, {
      ignored: /(^|[\/\\])\../, // ignore dotfiles
      persistent: true,
      awaitWriteFinish: {
        stabilityThreshold: this.config.streamTimeout,
        pollInterval: 100
      }
    });

    watcher
      .on('add', path => this.processNewFile(path))
      .on('change', path => this.processUpdatedFile(path));
  }

  private async processNewFile(filePath: string) {
    if (!filePath.endsWith('.mp4')) return;

    const streamId = uuidv4();
    await this.initializeStreamMetadata(streamId, filePath);
  }

  private async processUpdatedFile(filePath: string) {
    // Implement chunk reading and S3 upload logic
    // Track progress in Redis
  }

  private async initializeStreamMetadata(streamId: string, filePath: string) {
    // Store initial stream metadata
  }

  private async uploadChunkToS3(streamId: string, chunk: Buffer) {
    // Implement idempotent chunk upload
  }

  private async finalizeStream(streamId: string) {
    // Write metadata file to S3
    // Clean up Redis entries
  }
}

// Configuration and startup
const processor = new VideoStreamProcessor({
  watchDir: '/input-videos',
  s3Bucket: 'video-streams',
  chunkSize: 10 * 1024 * 1024, // 10MB
  streamTimeout: 30000 // 30 seconds
});

processor.startWatching();
```

**Dockerfile**

```
FROM node:18-alpine
WORKDIR /usr/src/app
RUN apk add --no-cache dumb-init
COPY package*.json ./
RUN npm ci --only=production
COPY . .
RUN npm run build
ENTRYPOINT ["dumb-init", "--"]
CMD ["node", "dist/index.js"]

```**Docker Compose**

```
version: '3.8'

services:
  app:
    build: .
    volumes:
      - ./input-videos:/input-videos
    depends_on:
      - minio
      - redis
    environment:
      - MINIO_ENDPOINT=minio
      - REDIS_URL=redis://redis:6379

  minio:
    image: minio/minio
    ports:
      - "9000:9000"
      - "9001:9001"
    environment:
      MINIO_ROOT_USER: minioadmin
      MINIO_ROOT_PASSWORD: minioadmin
    command: server /data --console-address ":9001"
    volumes:
      - minio-data:/data

  redis:
    image: redis:alpine
    volumes:
      - redis-data:/data

volumes:
  minio-data:
  redis-data:

```



