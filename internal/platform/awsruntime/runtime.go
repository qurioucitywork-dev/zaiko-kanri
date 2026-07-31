// Package awsruntime composes the production AWS persistence dependencies.
// It does not run migrations, seed data, or switch the legacy SQLite server.
package awsruntime

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/dataaccess"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/dataaccess/postgresadapter"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/dataaccess/s3blob"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/objectservice"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/platform/awssigner"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/platform/postgresdb"
)

const defaultObjectPrefix = "objects"

// Config contains runtime values supplied to an ECS task through Secrets
// Manager or environment configuration. It intentionally has no static AWS
// access-key fields: S3 signing uses the standard AWS credential chain.
type Config struct {
	PostgresDSN string
	Production  bool

	AWSRegion      string
	S3Endpoint     string
	S3Bucket       string
	S3ObjectPrefix string

	ApplicationName   string
	MaxOpenConns      int
	MaxIdleConns      int
	ConnMaxLifetime   time.Duration
	ConnMaxIdleTime   time.Duration
	ConnectivityProbe time.Duration
	HTTPClient        *http.Client
}

// Runtime is the provider composition boundary used by a future gradual
// handler cutover. The caller must close it.
type Runtime struct {
	DB      *sql.DB
	Data    *postgresadapter.Adapter
	Blobs   dataaccess.ObjectBlobStore
	Objects *objectservice.Service
}

// Open validates configuration, opens and probes PostgreSQL, then composes
// PostgreSQL metadata and private S3 object storage. It performs no writes.
func Open(ctx context.Context, config Config) (*Runtime, error) {
	normalized, err := normalizeConfig(config)
	if err != nil {
		return nil, err
	}
	database, err := postgresdb.Open(ctx, postgresdb.Config{
		DSN:               normalized.PostgresDSN,
		ApplicationName:   normalized.ApplicationName,
		Production:        normalized.Production,
		MaxOpenConns:      normalized.MaxOpenConns,
		MaxIdleConns:      normalized.MaxIdleConns,
		ConnMaxLifetime:   normalized.ConnMaxLifetime,
		ConnMaxIdleTime:   normalized.ConnMaxIdleTime,
		ConnectivityProbe: normalized.ConnectivityProbe,
	})
	if err != nil {
		return nil, err
	}
	closeOnError := func(openErr error) (*Runtime, error) {
		return nil, errors.Join(openErr, database.Close())
	}

	dataAdapter, err := postgresadapter.New(database, postgresadapter.Config{
		StorageProvider: "s3",
		StorageBucket:   normalized.S3Bucket,
		ObjectKeyPrefix: normalized.S3ObjectPrefix,
	})
	if err != nil {
		return closeOnError(err)
	}
	signer, err := awssigner.New(ctx, normalized.AWSRegion)
	if err != nil {
		return closeOnError(err)
	}
	blobAdapter, err := s3blob.New(s3blob.Config{
		Endpoint:   normalized.S3Endpoint,
		Bucket:     normalized.S3Bucket,
		Region:     normalized.AWSRegion,
		Prefix:     normalized.S3ObjectPrefix,
		HTTPClient: normalized.HTTPClient,
		Signer:     signer,
	})
	if err != nil {
		return closeOnError(err)
	}
	objectService, err := objectservice.New(dataAdapter, blobAdapter)
	if err != nil {
		return closeOnError(err)
	}
	return &Runtime{
		DB: database, Data: dataAdapter, Blobs: blobAdapter, Objects: objectService,
	}, nil
}

// Close releases only resources owned by this composition root. The adapters
// themselves do not own additional background resources.
func (r *Runtime) Close() error {
	if r == nil || r.DB == nil {
		return nil
	}
	return r.DB.Close()
}

func normalizeConfig(config Config) (Config, error) {
	config.PostgresDSN = strings.TrimSpace(config.PostgresDSN)
	config.AWSRegion = strings.TrimSpace(config.AWSRegion)
	config.S3Bucket = strings.TrimSpace(config.S3Bucket)
	config.S3ObjectPrefix = strings.Trim(strings.TrimSpace(config.S3ObjectPrefix), "/")
	config.ApplicationName = strings.TrimSpace(config.ApplicationName)
	if config.S3ObjectPrefix == "" {
		config.S3ObjectPrefix = defaultObjectPrefix
	}
	if config.PostgresDSN == "" || config.AWSRegion == "" || config.S3Bucket == "" {
		return Config{}, errors.New("awsruntime: PostgreSQL DSN, AWS region, and S3 bucket are required")
	}
	if strings.TrimSpace(config.S3Endpoint) == "" {
		config.S3Endpoint = "https://s3." + config.AWSRegion + ".amazonaws.com"
	}
	endpoint, err := url.Parse(strings.TrimSpace(config.S3Endpoint))
	if err != nil || endpoint.Scheme != "https" || endpoint.Host == "" ||
		endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return Config{}, errors.New("awsruntime: S3 endpoint must be an HTTPS origin")
	}
	config.S3Endpoint = strings.TrimRight(endpoint.String(), "/")
	return config, nil
}
