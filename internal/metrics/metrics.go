package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	FilesIngested = prometheus.NewCounterVec(
			prometheus.CounterOpts{
					Name: "video_files_ingested_total",
					Help: "Total number of video files ingested",
			},
			[]string{"path"},
	)
	FilesIngestedErrors = prometheus.NewCounterVec(
			prometheus.CounterOpts{
					Name: "video_files_ingested_errors_total",
					Help: "Total number of video files ingested errors",
			},
			[]string{"path"},
	)
	FilesProcessed = prometheus.NewCounterVec(
			prometheus.CounterOpts{
					Name: "video_files_processed_total",
					Help: "Total number of video files processed",
			},
			[]string{"path"},
	)
	ChunksUploaded = prometheus.NewCounterVec(
			prometheus.CounterOpts{
					Name: "video_chunks_uploaded_total",
					Help: "Total number of video chunks uploaded to S3",
			},
			[]string{"path"},
	)
	ChunksUploadedErrors = prometheus.NewCounterVec(
			prometheus.CounterOpts{
					Name: "video_chunks_uploaded_errors_total",
					Help: "Total number of video chunks uploaded to S3 errors",
			},
			[]string{"path"},
	)
	MetadataUploaded = prometheus.NewCounterVec(
			prometheus.CounterOpts{
					Name: "video_metadata_uploaded_total",
					Help: "Total number of video metadata uploaded to S3",
			},
			[]string{"path"},
	)
	MetadataUploadedErrors = prometheus.NewCounterVec(
			prometheus.CounterOpts{
					Name: "video_metadata_uploaded_errors_total",
					Help: "Total number of video metadata uploaded to S3 errors",
			},
			[]string{"path"},
	)
	ProcessingErrors = prometheus.NewCounterVec(
			prometheus.CounterOpts{
					Name: "video_processing_errors_total",
					Help: "Total number of errors during video processing",
			},
			[]string{"path"},
	)
	BytesUploaded = prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
					Name: "video_bytes_uploaded_total",
					Help: "Total number of bytes uploaded to S3",
			},
			[]string{"path"},
	)
)

func init() {
	prometheus.MustRegister(FilesIngested)
	prometheus.MustRegister(FilesIngestedErrors)
	prometheus.MustRegister(FilesProcessed)
	prometheus.MustRegister(ChunksUploaded)
	prometheus.MustRegister(ChunksUploadedErrors)
	prometheus.MustRegister(MetadataUploaded)
	prometheus.MustRegister(MetadataUploadedErrors)
	prometheus.MustRegister(ProcessingErrors)
	prometheus.MustRegister(BytesUploaded)
}