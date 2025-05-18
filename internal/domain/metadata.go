package domain

import "time"

type MetadataFilePart struct {
	ChunkIndex uint `json:"chunk_index"`
	ChunkSize int `json:"chunk_size"`
	ChunkName string `json:"chunk_name"`
	Hash string `json:"hash"`
}

type MetadataFile struct {
	ChunkProgressIndex uint `json:"chunk_progress_index"`
	TotalChunks uint `json:"total_chunks"`
	TotalBytes uint64 `json:"total_bytes"`
	Parts []MetadataFilePart `json:"parts"`
	Path string `json:"path"`
	UploadTime time.Time `json:"upload_time"`
	StreamDuration time.Duration `json:"stream_duration"`
	S3Bucket string `json:"s3_bucket"`
	Status string `json:"status"`
}