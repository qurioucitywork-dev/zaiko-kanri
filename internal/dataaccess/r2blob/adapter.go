// Package r2blob implements dataaccess.ObjectBlobStore for Cloudflare R2
// through its S3-compatible HTTP API.
package r2blob

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/dataaccess"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/dataaccess/s3blob"
)

// RequestSigner is the narrow signing boundary shared with the S3-compatible
// protocol implementation.
type RequestSigner = s3blob.RequestSigner

// Config contains R2's private endpoint and credential values. Region is fixed
// to "auto", as required by R2's SigV4 compatibility endpoint.
type Config struct {
	Endpoint        string
	Bucket          string
	Prefix          string
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string

	HTTPClient    *http.Client
	Signer        RequestSigner
	Clock         func() time.Time
	AllowInsecure bool
}

type Adapter struct {
	delegate *s3blob.Adapter
}

// New validates configuration without contacting Cloudflare.
func New(config Config) (*Adapter, error) {
	delegate, err := s3blob.New(s3blob.Config{
		Endpoint:        config.Endpoint,
		Bucket:          config.Bucket,
		Region:          "auto",
		Prefix:          config.Prefix,
		AccessKeyID:     config.AccessKeyID,
		SecretAccessKey: config.SecretAccessKey,
		SessionToken:    config.SessionToken,
		HTTPClient:      config.HTTPClient,
		Signer:          config.Signer,
		Clock:           config.Clock,
		AllowInsecure:   config.AllowInsecure,
	})
	if err != nil {
		return nil, err
	}
	return &Adapter{delegate: delegate}, nil
}

func (a *Adapter) Put(
	ctx context.Context,
	tenantID, objectID, contentType string,
	maxBytes int64,
	body io.Reader,
) (dataaccess.BlobReceipt, error) {
	if a == nil || a.delegate == nil {
		return dataaccess.BlobReceipt{}, dataaccess.ErrInvalidArgument
	}
	return a.delegate.Put(ctx, tenantID, objectID, contentType, maxBytes, body)
}

func (a *Adapter) Head(
	ctx context.Context,
	tenantID, objectID string,
) (dataaccess.BlobHead, error) {
	if a == nil || a.delegate == nil {
		return dataaccess.BlobHead{}, dataaccess.ErrInvalidArgument
	}
	return a.delegate.Head(ctx, tenantID, objectID)
}

func (a *Adapter) Open(
	ctx context.Context,
	tenantID, objectID string,
) (io.ReadCloser, error) {
	if a == nil || a.delegate == nil {
		return nil, dataaccess.ErrInvalidArgument
	}
	return a.delegate.Open(ctx, tenantID, objectID)
}

func (a *Adapter) Delete(
	ctx context.Context,
	tenantID, objectID string,
) error {
	if a == nil || a.delegate == nil {
		return dataaccess.ErrInvalidArgument
	}
	return a.delegate.Delete(ctx, tenantID, objectID)
}

// IsRetryable allows a service-level bounded backoff policy to classify R2
// responses without exposing endpoint, bucket, key, or provider response body.
func IsRetryable(err error) bool {
	return s3blob.IsRetryable(err)
}

var _ dataaccess.ObjectBlobStore = (*Adapter)(nil)
