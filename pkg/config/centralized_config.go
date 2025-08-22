package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/dilipmighty245/telemetry-pipeline/pkg/logging"
)

// CentralizedConfig holds all configuration for the telemetry pipeline
type CentralizedConfig struct {
	// Service identity
	ServiceName string `json:"service_name" env:"SERVICE_NAME" default:"telemetry-pipeline"`
	ServiceID   string `json:"service_id" env:"SERVICE_ID" default:""`
	Environment string `json:"environment" env:"ENVIRONMENT" default:"production"`
	LogLevel    string `json:"log_level" env:"LOG_LEVEL" default:"info"`

	// etcd configuration
	Etcd EtcdConfig `json:"etcd"`

	// Message queue configuration
	MessageQueue MessageQueueConfig `json:"message_queue"`

	// Collector configuration
	Collector CollectorConfig `json:"collector"`

	// Streamer configuration
	Streamer StreamerConfig `json:"streamer"`

	// API Gateway configuration
	Gateway GatewayConfig `json:"gateway"`

	// Database configuration
	Database DatabaseConfig `json:"database"`

	// Observability configuration
	Observability ObservabilityConfig `json:"observability"`

	// Security configuration
	Security SecurityConfig `json:"security"`

	// Performance tuning
	Performance PerformanceConfig `json:"performance"`
}

// EtcdConfig holds etcd-specific configuration
type EtcdConfig struct {
	Endpoints        []string      `json:"endpoints" env:"ETCD_ENDPOINTS" default:"localhost:2379"`
	DialTimeout      time.Duration `json:"dial_timeout" env:"ETCD_DIAL_TIMEOUT" default:"10s"`
	RequestTimeout   time.Duration `json:"request_timeout" env:"ETCD_REQUEST_TIMEOUT" default:"5s"`
	Username         string        `json:"username" env:"ETCD_USERNAME" default:""`
	Password         string        `json:"password" env:"ETCD_PASSWORD" default:""`
	TLSEnabled       bool          `json:"tls_enabled" env:"ETCD_TLS_ENABLED" default:"false"`
	CompactionMode   string        `json:"compaction_mode" env:"ETCD_COMPACTION_MODE" default:"revision"`
	CompactionWindow time.Duration `json:"compaction_window" env:"ETCD_COMPACTION_WINDOW" default:"1h"`
}

// MessageQueueConfig holds message queue configuration
type MessageQueueConfig struct {
	QueuePrefix       string        `json:"queue_prefix" env:"MQ_QUEUE_PREFIX" default:"/queue/telemetry"`
	ProcessingPrefix  string        `json:"processing_prefix" env:"MQ_PROCESSING_PREFIX" default:"/processing"`
	ProcessedPrefix   string        `json:"processed_prefix" env:"MQ_PROCESSED_PREFIX" default:"/processed"`
	DefaultTTL        time.Duration `json:"default_ttl" env:"MQ_DEFAULT_TTL" default:"24h"`
	ProcessingTimeout time.Duration `json:"processing_timeout" env:"MQ_PROCESSING_TIMEOUT" default:"30s"`
	BatchSize         int           `json:"batch_size" env:"MQ_BATCH_SIZE" default:"100"`
	MaxRetries        int           `json:"max_retries" env:"MQ_MAX_RETRIES" default:"3"`
	CompactionWindow  time.Duration `json:"compaction_window" env:"MQ_COMPACTION_WINDOW" default:"1h"`
}

// CollectorConfig holds collector service configuration
type CollectorConfig struct {
	ID            string        `json:"id" env:"COLLECTOR_ID" default:""`
	ConsumerGroup string        `json:"consumer_group" env:"COLLECTOR_CONSUMER_GROUP" default:"telemetry-collectors"`
	BatchSize     int           `json:"batch_size" env:"COLLECTOR_BATCH_SIZE" default:"100"`
	PollInterval  time.Duration `json:"poll_interval" env:"COLLECTOR_POLL_INTERVAL" default:"1s"`
	MaxRetries    int           `json:"max_retries" env:"COLLECTOR_MAX_RETRIES" default:"3"`
	RetryDelay    time.Duration `json:"retry_delay" env:"COLLECTOR_RETRY_DELAY" default:"1s"`
	BufferSize    int           `json:"buffer_size" env:"COLLECTOR_BUFFER_SIZE" default:"1000"`
	FlushInterval time.Duration `json:"flush_interval" env:"COLLECTOR_FLUSH_INTERVAL" default:"5s"`
	WorkerCount   int           `json:"worker_count" env:"COLLECTOR_WORKER_COUNT" default:"4"`
	EnableMetrics bool          `json:"enable_metrics" env:"COLLECTOR_ENABLE_METRICS" default:"true"`
}

// StreamerConfig holds streamer service configuration
type StreamerConfig struct {
	ID             string         `json:"id" env:"STREAMER_ID" default:""`
	CSVFilePath    string         `json:"csv_file_path" env:"STREAMER_CSV_FILE_PATH" default:""`
	BatchSize      int            `json:"batch_size" env:"STREAMER_BATCH_SIZE" default:"100"`
	StreamInterval time.Duration  `json:"stream_interval" env:"STREAMER_STREAM_INTERVAL" default:"1s"`
	LoopMode       bool           `json:"loop_mode" env:"STREAMER_LOOP_MODE" default:"false"`
	MaxRetries     int            `json:"max_retries" env:"STREAMER_MAX_RETRIES" default:"3"`
	RetryDelay     time.Duration  `json:"retry_delay" env:"STREAMER_RETRY_DELAY" default:"1s"`
	BufferSize     int            `json:"buffer_size" env:"STREAMER_BUFFER_SIZE" default:"1000"`
	EnableMetrics  bool           `json:"enable_metrics" env:"STREAMER_ENABLE_METRICS" default:"true"`
	ThrottleConfig ThrottleConfig `json:"throttle"`
}

// ThrottleConfig holds throttling configuration for backpressure
type ThrottleConfig struct {
	Enabled         bool          `json:"enabled" env:"THROTTLE_ENABLED" default:"true"`
	MaxQueueDepth   int64         `json:"max_queue_depth" env:"THROTTLE_MAX_QUEUE_DEPTH" default:"10000"`
	ThrottleRate    float64       `json:"throttle_rate" env:"THROTTLE_RATE" default:"0.5"`
	CheckInterval   time.Duration `json:"check_interval" env:"THROTTLE_CHECK_INTERVAL" default:"5s"`
	TokenBucketSize int           `json:"token_bucket_size" env:"THROTTLE_TOKEN_BUCKET_SIZE" default:"1000"`
}

// GatewayConfig holds API gateway configuration
type GatewayConfig struct {
	Host              string           `json:"host" env:"GATEWAY_HOST" default:"0.0.0.0"`
	Port              int              `json:"port" env:"GATEWAY_PORT" default:"8080"`
	ReadTimeout       time.Duration    `json:"read_timeout" env:"GATEWAY_READ_TIMEOUT" default:"30s"`
	WriteTimeout      time.Duration    `json:"write_timeout" env:"GATEWAY_WRITE_TIMEOUT" default:"30s"`
	MaxHeaderSize     int              `json:"max_header_size" env:"GATEWAY_MAX_HEADER_SIZE" default:"1048576"`
	EnableCORS        bool             `json:"enable_cors" env:"GATEWAY_ENABLE_CORS" default:"true"`
	CORSOrigins       []string         `json:"cors_origins" env:"GATEWAY_CORS_ORIGINS" default:"*"`
	EnableCompression bool             `json:"enable_compression" env:"GATEWAY_ENABLE_COMPRESSION" default:"true"`
	RateLimit         RateLimitConfig  `json:"rate_limit"`
	WebSocket         WebSocketConfig  `json:"websocket"`
	Pagination        PaginationConfig `json:"pagination"`
}

// RateLimitConfig holds rate limiting configuration
type RateLimitConfig struct {
	Enabled        bool          `json:"enabled" env:"RATE_LIMIT_ENABLED" default:"true"`
	RequestsPerSec float64       `json:"requests_per_sec" env:"RATE_LIMIT_REQUESTS_PER_SEC" default:"100"`
	BurstSize      int           `json:"burst_size" env:"RATE_LIMIT_BURST_SIZE" default:"200"`
	WindowSize     time.Duration `json:"window_size" env:"RATE_LIMIT_WINDOW_SIZE" default:"1m"`
}

// WebSocketConfig holds WebSocket configuration
type WebSocketConfig struct {
	Enabled         bool          `json:"enabled" env:"WS_ENABLED" default:"true"`
	PingInterval    time.Duration `json:"ping_interval" env:"WS_PING_INTERVAL" default:"30s"`
	PongTimeout     time.Duration `json:"pong_timeout" env:"WS_PONG_TIMEOUT" default:"60s"`
	WriteTimeout    time.Duration `json:"write_timeout" env:"WS_WRITE_TIMEOUT" default:"10s"`
	ReadBufferSize  int           `json:"read_buffer_size" env:"WS_READ_BUFFER_SIZE" default:"1024"`
	WriteBufferSize int           `json:"write_buffer_size" env:"WS_WRITE_BUFFER_SIZE" default:"1024"`
	MaxConnections  int           `json:"max_connections" env:"WS_MAX_CONNECTIONS" default:"1000"`
}

// PaginationConfig holds pagination configuration
type PaginationConfig struct {
	DefaultLimit int `json:"default_limit" env:"PAGINATION_DEFAULT_LIMIT" default:"100"`
	MaxLimit     int `json:"max_limit" env:"PAGINATION_MAX_LIMIT" default:"1000"`
}

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	// Primary database (Timescale/PostgreSQL)
	Primary DatabaseConnectionConfig `json:"primary"`

	// Connection pool settings
	MaxOpenConns    int           `json:"max_open_conns" env:"DB_MAX_OPEN_CONNS" default:"25"`
	MaxIdleConns    int           `json:"max_idle_conns" env:"DB_MAX_IDLE_CONNS" default:"5"`
	ConnMaxLifetime time.Duration `json:"conn_max_lifetime" env:"DB_CONN_MAX_LIFETIME" default:"1h"`

	// Migration settings
	AutoMigrate bool `json:"auto_migrate" env:"DB_AUTO_MIGRATE" default:"true"`

	// Retention settings
	Retention RetentionConfig `json:"retention"`
}

// DatabaseConnectionConfig holds database connection details
type DatabaseConnectionConfig struct {
	Host     string `json:"host" env:"DB_HOST" default:"localhost"`
	Port     int    `json:"port" env:"DB_PORT" default:"5432"`
	Database string `json:"database" env:"DB_DATABASE" default:"telemetry"`
	Username string `json:"username" env:"DB_USERNAME" default:"postgres"`
	Password string `json:"password" env:"DB_PASSWORD" default:""`
	SSLMode  string `json:"ssl_mode" env:"DB_SSL_MODE" default:"disable"`
	Timezone string `json:"timezone" env:"DB_TIMEZONE" default:"UTC"`
}

// RetentionConfig holds data retention configuration
type RetentionConfig struct {
	Enabled            bool          `json:"enabled" env:"RETENTION_ENABLED" default:"true"`
	EtcdRetention      time.Duration `json:"etcd_retention" env:"RETENTION_ETCD" default:"1h"`
	TimescaleRetention time.Duration `json:"timescale_retention" env:"RETENTION_TIMESCALE" default:"30d"`
	CleanupInterval    time.Duration `json:"cleanup_interval" env:"RETENTION_CLEANUP_INTERVAL" default:"1h"`
	ChunkInterval      time.Duration `json:"chunk_interval" env:"RETENTION_CHUNK_INTERVAL" default:"1d"`
}

// ObservabilityConfig holds observability configuration
type ObservabilityConfig struct {
	Metrics MetricsConfig `json:"metrics"`
	Tracing TracingConfig `json:"tracing"`
	Logging LoggingConfig `json:"logging"`
}

// MetricsConfig holds metrics configuration
type MetricsConfig struct {
	Enabled   bool   `json:"enabled" env:"METRICS_ENABLED" default:"true"`
	Port      int    `json:"port" env:"METRICS_PORT" default:"9090"`
	Path      string `json:"path" env:"METRICS_PATH" default:"/metrics"`
	Namespace string `json:"namespace" env:"METRICS_NAMESPACE" default:"telemetry_pipeline"`
	Subsystem string `json:"subsystem" env:"METRICS_SUBSYSTEM" default:""`
}

// TracingConfig holds tracing configuration
type TracingConfig struct {
	Enabled     bool    `json:"enabled" env:"TRACING_ENABLED" default:"true"`
	Endpoint    string  `json:"endpoint" env:"TRACING_ENDPOINT" default:""`
	ServiceName string  `json:"service_name" env:"TRACING_SERVICE_NAME" default:"telemetry-pipeline"`
	SampleRate  float64 `json:"sample_rate" env:"TRACING_SAMPLE_RATE" default:"0.1"`
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level      string `json:"level" env:"LOG_LEVEL" default:"info"`
	Format     string `json:"format" env:"LOG_FORMAT" default:"json"`
	Output     string `json:"output" env:"LOG_OUTPUT" default:"stdout"`
	Structured bool   `json:"structured" env:"LOG_STRUCTURED" default:"true"`
}

// SecurityConfig holds security configuration
type SecurityConfig struct {
	Auth            AuthConfig            `json:"auth"`
	TLS             TLSConfig             `json:"tls"`
	Secrets         SecretsConfig         `json:"secrets"`
	InputValidation InputValidationConfig `json:"input_validation"`
}

// AuthConfig holds authentication configuration
type AuthConfig struct {
	Enabled     bool          `json:"enabled" env:"AUTH_ENABLED" default:"false"`
	TokenSecret string        `json:"token_secret" env:"AUTH_TOKEN_SECRET" default:""`
	TokenExpiry time.Duration `json:"token_expiry" env:"AUTH_TOKEN_EXPIRY" default:"24h"`
	ReadScopes  []string      `json:"read_scopes" env:"AUTH_READ_SCOPES" default:"read"`
	AdminScopes []string      `json:"admin_scopes" env:"AUTH_ADMIN_SCOPES" default:"admin"`
}

// TLSConfig holds TLS configuration
type TLSConfig struct {
	Enabled  bool   `json:"enabled" env:"TLS_ENABLED" default:"false"`
	CertFile string `json:"cert_file" env:"TLS_CERT_FILE" default:""`
	KeyFile  string `json:"key_file" env:"TLS_KEY_FILE" default:""`
	CAFile   string `json:"ca_file" env:"TLS_CA_FILE" default:""`
}

// SecretsConfig holds secrets management configuration
type SecretsConfig struct {
	Source    string `json:"source" env:"SECRETS_SOURCE" default:"env"` // env, k8s, vault
	K8sPrefix string `json:"k8s_prefix" env:"SECRETS_K8S_PREFIX" default:"telemetry-"`
}

// InputValidationConfig holds input validation configuration
type InputValidationConfig struct {
	Enabled           bool `json:"enabled" env:"INPUT_VALIDATION_ENABLED" default:"true"`
	MaxPayloadSize    int  `json:"max_payload_size" env:"INPUT_VALIDATION_MAX_PAYLOAD_SIZE" default:"1048576"`
	StrictSchemaCheck bool `json:"strict_schema_check" env:"INPUT_VALIDATION_STRICT_SCHEMA" default:"true"`
}

// PerformanceConfig holds performance tuning configuration
type PerformanceConfig struct {
	BatchCommit    BatchCommitConfig    `json:"batch_commit"`
	Prefetch       PrefetchConfig       `json:"prefetch"`
	CircuitBreaker CircuitBreakerConfig `json:"circuit_breaker"`
}

// BatchCommitConfig holds batch commit configuration
type BatchCommitConfig struct {
	Enabled bool          `json:"enabled" env:"BATCH_COMMIT_ENABLED" default:"true"`
	Size    int           `json:"size" env:"BATCH_COMMIT_SIZE" default:"1000"`
	Timeout time.Duration `json:"timeout" env:"BATCH_COMMIT_TIMEOUT" default:"5s"`
	MaxWait time.Duration `json:"max_wait" env:"BATCH_COMMIT_MAX_WAIT" default:"10s"`
}

// PrefetchConfig holds prefetch configuration
type PrefetchConfig struct {
	Enabled bool `json:"enabled" env:"PREFETCH_ENABLED" default:"true"`
	Size    int  `json:"size" env:"PREFETCH_SIZE" default:"1000"`
	Workers int  `json:"workers" env:"PREFETCH_WORKERS" default:"2"`
}

// CircuitBreakerConfig holds circuit breaker configuration
type CircuitBreakerConfig struct {
	Enabled          bool          `json:"enabled" env:"CIRCUIT_BREAKER_ENABLED" default:"true"`
	FailureThreshold int           `json:"failure_threshold" env:"CIRCUIT_BREAKER_FAILURE_THRESHOLD" default:"5"`
	RecoveryTimeout  time.Duration `json:"recovery_timeout" env:"CIRCUIT_BREAKER_RECOVERY_TIMEOUT" default:"30s"`
	RequestThreshold int           `json:"request_threshold" env:"CIRCUIT_BREAKER_REQUEST_THRESHOLD" default:"10"`
}

// LoadFromEnv loads configuration from environment variables and defaults
func LoadFromEnv() (*CentralizedConfig, error) {
	config := &CentralizedConfig{}

	// Load basic service configuration
	config.ServiceName = getEnvOrDefault("SERVICE_NAME", "telemetry-pipeline")
	config.ServiceID = getEnvOrDefault("SERVICE_ID", "")
	config.Environment = getEnvOrDefault("ENVIRONMENT", "production")
	config.LogLevel = getEnvOrDefault("LOG_LEVEL", "info")

	// Load etcd configuration
	config.Etcd = EtcdConfig{
		Endpoints:        parseStringSlice(getEnvOrDefault("ETCD_ENDPOINTS", "localhost:2379")),
		DialTimeout:      parseDurationOrDefault("ETCD_DIAL_TIMEOUT", 10*time.Second),
		RequestTimeout:   parseDurationOrDefault("ETCD_REQUEST_TIMEOUT", 5*time.Second),
		Username:         getEnvOrDefault("ETCD_USERNAME", ""),
		Password:         getEnvOrDefault("ETCD_PASSWORD", ""),
		TLSEnabled:       parseBoolOrDefault("ETCD_TLS_ENABLED", false),
		CompactionMode:   getEnvOrDefault("ETCD_COMPACTION_MODE", "revision"),
		CompactionWindow: parseDurationOrDefault("ETCD_COMPACTION_WINDOW", time.Hour),
	}

	// Load message queue configuration
	config.MessageQueue = MessageQueueConfig{
		QueuePrefix:       getEnvOrDefault("MQ_QUEUE_PREFIX", "/queue/telemetry"),
		ProcessingPrefix:  getEnvOrDefault("MQ_PROCESSING_PREFIX", "/processing"),
		ProcessedPrefix:   getEnvOrDefault("MQ_PROCESSED_PREFIX", "/processed"),
		DefaultTTL:        parseDurationOrDefault("MQ_DEFAULT_TTL", 24*time.Hour),
		ProcessingTimeout: parseDurationOrDefault("MQ_PROCESSING_TIMEOUT", 30*time.Second),
		BatchSize:         parseIntOrDefault("MQ_BATCH_SIZE", 100),
		MaxRetries:        parseIntOrDefault("MQ_MAX_RETRIES", 3),
		CompactionWindow:  parseDurationOrDefault("MQ_COMPACTION_WINDOW", time.Hour),
	}

	// Load collector configuration
	config.Collector = CollectorConfig{
		ID:            getEnvOrDefault("COLLECTOR_ID", ""),
		ConsumerGroup: getEnvOrDefault("COLLECTOR_CONSUMER_GROUP", "telemetry-collectors"),
		BatchSize:     parseIntOrDefault("COLLECTOR_BATCH_SIZE", 100),
		PollInterval:  parseDurationOrDefault("COLLECTOR_POLL_INTERVAL", time.Second),
		MaxRetries:    parseIntOrDefault("COLLECTOR_MAX_RETRIES", 3),
		RetryDelay:    parseDurationOrDefault("COLLECTOR_RETRY_DELAY", time.Second),
		BufferSize:    parseIntOrDefault("COLLECTOR_BUFFER_SIZE", 1000),
		FlushInterval: parseDurationOrDefault("COLLECTOR_FLUSH_INTERVAL", 5*time.Second),
		WorkerCount:   parseIntOrDefault("COLLECTOR_WORKER_COUNT", 4),
		EnableMetrics: parseBoolOrDefault("COLLECTOR_ENABLE_METRICS", true),
	}

	// Load streamer configuration
	config.Streamer = StreamerConfig{
		ID:             getEnvOrDefault("STREAMER_ID", ""),
		CSVFilePath:    getEnvOrDefault("STREAMER_CSV_FILE_PATH", ""),
		BatchSize:      parseIntOrDefault("STREAMER_BATCH_SIZE", 100),
		StreamInterval: parseDurationOrDefault("STREAMER_STREAM_INTERVAL", time.Second),
		LoopMode:       parseBoolOrDefault("STREAMER_LOOP_MODE", false),
		MaxRetries:     parseIntOrDefault("STREAMER_MAX_RETRIES", 3),
		RetryDelay:     parseDurationOrDefault("STREAMER_RETRY_DELAY", time.Second),
		BufferSize:     parseIntOrDefault("STREAMER_BUFFER_SIZE", 1000),
		EnableMetrics:  parseBoolOrDefault("STREAMER_ENABLE_METRICS", true),
		ThrottleConfig: ThrottleConfig{
			Enabled:         parseBoolOrDefault("THROTTLE_ENABLED", true),
			MaxQueueDepth:   parseInt64OrDefault("THROTTLE_MAX_QUEUE_DEPTH", 10000),
			ThrottleRate:    parseFloat64OrDefault("THROTTLE_RATE", 0.5),
			CheckInterval:   parseDurationOrDefault("THROTTLE_CHECK_INTERVAL", 5*time.Second),
			TokenBucketSize: parseIntOrDefault("THROTTLE_TOKEN_BUCKET_SIZE", 1000),
		},
	}

	// Load gateway configuration
	config.Gateway = GatewayConfig{
		Host:              getEnvOrDefault("GATEWAY_HOST", "0.0.0.0"),
		Port:              parseIntOrDefault("GATEWAY_PORT", 8080),
		ReadTimeout:       parseDurationOrDefault("GATEWAY_READ_TIMEOUT", 30*time.Second),
		WriteTimeout:      parseDurationOrDefault("GATEWAY_WRITE_TIMEOUT", 30*time.Second),
		MaxHeaderSize:     parseIntOrDefault("GATEWAY_MAX_HEADER_SIZE", 1048576),
		EnableCORS:        parseBoolOrDefault("GATEWAY_ENABLE_CORS", true),
		CORSOrigins:       parseStringSlice(getEnvOrDefault("GATEWAY_CORS_ORIGINS", "*")),
		EnableCompression: parseBoolOrDefault("GATEWAY_ENABLE_COMPRESSION", true),
		RateLimit: RateLimitConfig{
			Enabled:        parseBoolOrDefault("RATE_LIMIT_ENABLED", true),
			RequestsPerSec: parseFloat64OrDefault("RATE_LIMIT_REQUESTS_PER_SEC", 100),
			BurstSize:      parseIntOrDefault("RATE_LIMIT_BURST_SIZE", 200),
			WindowSize:     parseDurationOrDefault("RATE_LIMIT_WINDOW_SIZE", time.Minute),
		},
		WebSocket: WebSocketConfig{
			Enabled:         parseBoolOrDefault("WS_ENABLED", true),
			PingInterval:    parseDurationOrDefault("WS_PING_INTERVAL", 30*time.Second),
			PongTimeout:     parseDurationOrDefault("WS_PONG_TIMEOUT", 60*time.Second),
			WriteTimeout:    parseDurationOrDefault("WS_WRITE_TIMEOUT", 10*time.Second),
			ReadBufferSize:  parseIntOrDefault("WS_READ_BUFFER_SIZE", 1024),
			WriteBufferSize: parseIntOrDefault("WS_WRITE_BUFFER_SIZE", 1024),
			MaxConnections:  parseIntOrDefault("WS_MAX_CONNECTIONS", 1000),
		},
		Pagination: PaginationConfig{
			DefaultLimit: parseIntOrDefault("PAGINATION_DEFAULT_LIMIT", 100),
			MaxLimit:     parseIntOrDefault("PAGINATION_MAX_LIMIT", 1000),
		},
	}

	// Load database configuration
	config.Database = DatabaseConfig{
		Primary: DatabaseConnectionConfig{
			Host:     getEnvOrDefault("DB_HOST", "localhost"),
			Port:     parseIntOrDefault("DB_PORT", 5432),
			Database: getEnvOrDefault("DB_DATABASE", "telemetry"),
			Username: getEnvOrDefault("DB_USERNAME", "postgres"),
			Password: getEnvOrDefault("DB_PASSWORD", ""),
			SSLMode:  getEnvOrDefault("DB_SSL_MODE", "disable"),
			Timezone: getEnvOrDefault("DB_TIMEZONE", "UTC"),
		},
		MaxOpenConns:    parseIntOrDefault("DB_MAX_OPEN_CONNS", 25),
		MaxIdleConns:    parseIntOrDefault("DB_MAX_IDLE_CONNS", 5),
		ConnMaxLifetime: parseDurationOrDefault("DB_CONN_MAX_LIFETIME", time.Hour),
		AutoMigrate:     parseBoolOrDefault("DB_AUTO_MIGRATE", true),
		Retention: RetentionConfig{
			Enabled:            parseBoolOrDefault("RETENTION_ENABLED", true),
			EtcdRetention:      parseDurationOrDefault("RETENTION_ETCD", time.Hour),
			TimescaleRetention: parseDurationOrDefault("RETENTION_TIMESCALE", 30*24*time.Hour),
			CleanupInterval:    parseDurationOrDefault("RETENTION_CLEANUP_INTERVAL", time.Hour),
			ChunkInterval:      parseDurationOrDefault("RETENTION_CHUNK_INTERVAL", 24*time.Hour),
		},
	}

	// Load observability configuration
	config.Observability = ObservabilityConfig{
		Metrics: MetricsConfig{
			Enabled:   parseBoolOrDefault("METRICS_ENABLED", true),
			Port:      parseIntOrDefault("METRICS_PORT", 9090),
			Path:      getEnvOrDefault("METRICS_PATH", "/metrics"),
			Namespace: getEnvOrDefault("METRICS_NAMESPACE", "telemetry_pipeline"),
			Subsystem: getEnvOrDefault("METRICS_SUBSYSTEM", ""),
		},
		Tracing: TracingConfig{
			Enabled:     parseBoolOrDefault("TRACING_ENABLED", true),
			Endpoint:    getEnvOrDefault("TRACING_ENDPOINT", ""),
			ServiceName: getEnvOrDefault("TRACING_SERVICE_NAME", "telemetry-pipeline"),
			SampleRate:  parseFloat64OrDefault("TRACING_SAMPLE_RATE", 0.1),
		},
		Logging: LoggingConfig{
			Level:      getEnvOrDefault("LOG_LEVEL", "info"),
			Format:     getEnvOrDefault("LOG_FORMAT", "json"),
			Output:     getEnvOrDefault("LOG_OUTPUT", "stdout"),
			Structured: parseBoolOrDefault("LOG_STRUCTURED", true),
		},
	}

	// Load security configuration
	config.Security = SecurityConfig{
		Auth: AuthConfig{
			Enabled:     parseBoolOrDefault("AUTH_ENABLED", false),
			TokenSecret: getEnvOrDefault("AUTH_TOKEN_SECRET", ""),
			TokenExpiry: parseDurationOrDefault("AUTH_TOKEN_EXPIRY", 24*time.Hour),
			ReadScopes:  parseStringSlice(getEnvOrDefault("AUTH_READ_SCOPES", "read")),
			AdminScopes: parseStringSlice(getEnvOrDefault("AUTH_ADMIN_SCOPES", "admin")),
		},
		TLS: TLSConfig{
			Enabled:  parseBoolOrDefault("TLS_ENABLED", false),
			CertFile: getEnvOrDefault("TLS_CERT_FILE", ""),
			KeyFile:  getEnvOrDefault("TLS_KEY_FILE", ""),
			CAFile:   getEnvOrDefault("TLS_CA_FILE", ""),
		},
		Secrets: SecretsConfig{
			Source:    getEnvOrDefault("SECRETS_SOURCE", "env"),
			K8sPrefix: getEnvOrDefault("SECRETS_K8S_PREFIX", "telemetry-"),
		},
		InputValidation: InputValidationConfig{
			Enabled:           parseBoolOrDefault("INPUT_VALIDATION_ENABLED", true),
			MaxPayloadSize:    parseIntOrDefault("INPUT_VALIDATION_MAX_PAYLOAD_SIZE", 1048576),
			StrictSchemaCheck: parseBoolOrDefault("INPUT_VALIDATION_STRICT_SCHEMA", true),
		},
	}

	// Load performance configuration
	config.Performance = PerformanceConfig{
		BatchCommit: BatchCommitConfig{
			Enabled: parseBoolOrDefault("BATCH_COMMIT_ENABLED", true),
			Size:    parseIntOrDefault("BATCH_COMMIT_SIZE", 1000),
			Timeout: parseDurationOrDefault("BATCH_COMMIT_TIMEOUT", 5*time.Second),
			MaxWait: parseDurationOrDefault("BATCH_COMMIT_MAX_WAIT", 10*time.Second),
		},
		Prefetch: PrefetchConfig{
			Enabled: parseBoolOrDefault("PREFETCH_ENABLED", true),
			Size:    parseIntOrDefault("PREFETCH_SIZE", 1000),
			Workers: parseIntOrDefault("PREFETCH_WORKERS", 2),
		},
		CircuitBreaker: CircuitBreakerConfig{
			Enabled:          parseBoolOrDefault("CIRCUIT_BREAKER_ENABLED", true),
			FailureThreshold: parseIntOrDefault("CIRCUIT_BREAKER_FAILURE_THRESHOLD", 5),
			RecoveryTimeout:  parseDurationOrDefault("CIRCUIT_BREAKER_RECOVERY_TIMEOUT", 30*time.Second),
			RequestThreshold: parseIntOrDefault("CIRCUIT_BREAKER_REQUEST_THRESHOLD", 10),
		},
	}

	// Validate and set defaults for empty IDs
	if config.ServiceID == "" {
		config.ServiceID = fmt.Sprintf("%s-%d", config.ServiceName, time.Now().Unix())
	}
	if config.Collector.ID == "" {
		config.Collector.ID = fmt.Sprintf("collector-%d", time.Now().Unix())
	}
	if config.Streamer.ID == "" {
		config.Streamer.ID = fmt.Sprintf("streamer-%d", time.Now().Unix())
	}

	return config, nil
}

// Validate validates the configuration
func (c *CentralizedConfig) Validate() error {
	if len(c.Etcd.Endpoints) == 0 {
		return fmt.Errorf("etcd endpoints cannot be empty")
	}

	if c.MessageQueue.BatchSize <= 0 {
		return fmt.Errorf("message queue batch size must be positive")
	}

	if c.Collector.BatchSize <= 0 {
		return fmt.Errorf("collector batch size must be positive")
	}

	if c.Gateway.Port <= 0 || c.Gateway.Port > 65535 {
		return fmt.Errorf("gateway port must be between 1 and 65535")
	}

	if c.Database.Primary.Host == "" {
		return fmt.Errorf("database host cannot be empty")
	}

	if c.Security.Auth.Enabled && c.Security.Auth.TokenSecret == "" {
		return fmt.Errorf("auth token secret cannot be empty when auth is enabled")
	}

	if c.Security.TLS.Enabled {
		if c.Security.TLS.CertFile == "" || c.Security.TLS.KeyFile == "" {
			return fmt.Errorf("TLS cert file and key file must be specified when TLS is enabled")
		}
	}

	return nil
}

// ToJSON returns the configuration as JSON
func (c *CentralizedConfig) ToJSON() (string, error) {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// LogEffectiveConfig logs the effective configuration (with secrets masked)
func (c *CentralizedConfig) LogEffectiveConfig() {
	// Create a copy for logging with secrets masked
	logConfig := *c
	logConfig.Etcd.Password = maskSecret(c.Etcd.Password)
	logConfig.Database.Primary.Password = maskSecret(c.Database.Primary.Password)
	logConfig.Security.Auth.TokenSecret = maskSecret(c.Security.Auth.TokenSecret)

	configJSON, err := logConfig.ToJSON()
	if err != nil {
		logging.Errorf("Failed to serialize config for logging: %v", err)
		return
	}

	logging.Infof("Effective configuration:\n%s", configJSON)
}

// Helper functions for parsing environment variables

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func parseIntOrDefault(envKey string, defaultValue int) int {
	if value := os.Getenv(envKey); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}

func parseInt64OrDefault(envKey string, defaultValue int64) int64 {
	if value := os.Getenv(envKey); value != "" {
		if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
			return parsed
		}
	}
	return defaultValue
}

func parseFloat64OrDefault(envKey string, defaultValue float64) float64 {
	if value := os.Getenv(envKey); value != "" {
		if parsed, err := strconv.ParseFloat(value, 64); err == nil {
			return parsed
		}
	}
	return defaultValue
}

func parseBoolOrDefault(envKey string, defaultValue bool) bool {
	if value := os.Getenv(envKey); value != "" {
		if parsed, err := strconv.ParseBool(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}

func parseDurationOrDefault(envKey string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(envKey); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}

func parseStringSlice(value string) []string {
	if value == "" {
		return []string{}
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func maskSecret(secret string) string {
	if secret == "" {
		return ""
	}
	if len(secret) <= 4 {
		return "****"
	}
	return secret[:2] + "****" + secret[len(secret)-2:]
}
