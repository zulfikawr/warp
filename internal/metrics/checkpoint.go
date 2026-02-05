package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Checkpoint metrics track resume/checkpoint operations

var (
	// CheckpointSavesTotal counts successful checkpoint saves
	CheckpointSavesTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "warp_checkpoint_saves_total",
		Help: "Total number of successful checkpoint saves",
	})

	// CheckpointSaveErrors counts failed checkpoint save attempts
	CheckpointSaveErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "warp_checkpoint_save_errors_total",
		Help: "Total number of failed checkpoint save attempts",
	})

	// CheckpointSaveDuration tracks time spent saving checkpoints
	CheckpointSaveDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "warp_checkpoint_save_duration_seconds",
		Help:    "Time spent saving checkpoints to disk",
		Buckets: prometheus.ExponentialBuckets(0.001, 2, 10), // 1ms to ~1s
	})

	// CheckpointLoadsTotal counts successful checkpoint loads
	CheckpointLoadsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "warp_checkpoint_loads_total",
		Help: "Total number of successful checkpoint loads",
	})

	// CheckpointLoadErrors counts failed checkpoint load attempts
	CheckpointLoadErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "warp_checkpoint_load_errors_total",
		Help: "Total number of failed checkpoint load attempts",
	})

	// CheckpointLoadDuration tracks time spent loading checkpoints
	CheckpointLoadDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "warp_checkpoint_load_duration_seconds",
		Help:    "Time spent loading checkpoints from disk",
		Buckets: prometheus.ExponentialBuckets(0.001, 2, 10), // 1ms to ~1s
	})

	// CheckpointFileSize tracks the size of checkpoint files
	CheckpointFileSize = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "warp_checkpoint_file_size_bytes",
		Help:    "Size of checkpoint files in bytes",
		Buckets: prometheus.ExponentialBuckets(100, 2, 12), // 100B to ~400KB
	})

	// ActiveCheckpoints tracks the number of active checkpoint files
	ActiveCheckpoints = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "warp_active_checkpoints",
		Help: "Current number of active checkpoint files",
	})

	// CheckpointCleanups counts checkpoint cleanup operations
	CheckpointCleanups = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "warp_checkpoint_cleanups_total",
		Help: "Total number of checkpoint cleanup operations",
	}, []string{"reason"}) // reason: expired, completed, failed

	// ResumedTransfers counts transfers resumed from checkpoints
	ResumedTransfers = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "warp_resumed_transfers_total",
		Help: "Total number of transfers resumed from checkpoints",
	}, []string{"direction"}) // direction: upload, download
)
