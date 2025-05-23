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


