// Package awssigner bridges the ECS task-role credential chain to the narrow
// s3blob.RequestSigner port. Credentials are retrieved for every request
// through the SDK cache, so rotated task credentials are not frozen at startup.
package awssigner

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
)

type httpSigner interface {
	SignHTTP(
		ctx context.Context,
		credentials aws.Credentials,
		request *http.Request,
		payloadHash, service, region string,
		signingTime time.Time,
		optFns ...func(*v4.SignerOptions),
	) error
}

// Signer implements s3blob.RequestSigner without exposing AWS credentials to
// the storage adapter, logs, errors, or returned DTOs.
type Signer struct {
	region      string
	credentials aws.CredentialsProvider
	signer      httpSigner
}

// New loads the standard AWS credential chain. On ECS this resolves the task
// role; static application credentials are neither required nor recommended.
func New(ctx context.Context, region string) (*Signer, error) {
	region = strings.TrimSpace(region)
	if region == "" {
		return nil, errors.New("awssigner: region is required")
	}
	configuration, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return nil, errors.New("awssigner: load AWS configuration failed")
	}
	return NewWithProvider(region, configuration.Credentials)
}

// NewWithProvider is useful for tests and for an explicitly injected,
// auto-refreshing credentials provider.
func NewWithProvider(region string, provider aws.CredentialsProvider) (*Signer, error) {
	region = strings.TrimSpace(region)
	if region == "" || provider == nil {
		return nil, errors.New("awssigner: invalid configuration")
	}
	return &Signer{
		region:      region,
		credentials: provider,
		signer:      v4.NewSigner(),
	}, nil
}

// Sign retrieves current credentials and signs an S3 request in place.
func (s *Signer) Sign(
	ctx context.Context,
	request *http.Request,
	payloadSHA256 string,
	at time.Time,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s == nil || s.credentials == nil || s.signer == nil ||
		request == nil || request.URL == nil ||
		strings.TrimSpace(payloadSHA256) == "" || at.IsZero() {
		return errors.New("awssigner: invalid argument")
	}
	credentials, err := s.credentials.Retrieve(ctx)
	if err != nil {
		if contextError := ctx.Err(); contextError != nil {
			return contextError
		}
		return errors.New("awssigner: retrieve credentials failed")
	}
	if !credentials.HasKeys() {
		return errors.New("awssigner: credentials are unavailable")
	}
	if err := s.signer.SignHTTP(
		ctx,
		credentials,
		request,
		payloadSHA256,
		"s3",
		s.region,
		at.UTC(),
	); err != nil {
		if contextError := ctx.Err(); contextError != nil {
			return contextError
		}
		return errors.New("awssigner: sign request failed")
	}
	return nil
}
