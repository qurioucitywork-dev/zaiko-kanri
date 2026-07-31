package awssigner

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
)

type rotatingProvider struct {
	mu    sync.Mutex
	calls int
}

func (p *rotatingProvider) Retrieve(context.Context) (aws.Credentials, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	return aws.Credentials{
		AccessKeyID:     "AKID" + string(rune('0'+p.calls)),
		SecretAccessKey: "secret-for-test-only",
		SessionToken:    "session-for-test-only",
		Source:          "test",
	}, nil
}

func TestSignerRetrievesCredentialsForEveryRequest(t *testing.T) {
	t.Parallel()
	provider := &rotatingProvider{}
	signer, err := NewWithProvider("ap-northeast-1", provider)
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 2; index++ {
		request, err := http.NewRequest(http.MethodHead, "https://example-bucket.s3.ap-northeast-1.amazonaws.com/object", nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := signer.Sign(
			context.Background(),
			request,
			"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC),
		); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(request.Header.Get("Authorization"), "Credential=AKID") {
			t.Fatalf("missing rotating credential: %s", request.Header.Get("Authorization"))
		}
		if request.Header.Get("X-Amz-Security-Token") == "" {
			t.Fatal("missing session token")
		}
	}
	provider.mu.Lock()
	defer provider.mu.Unlock()
	if provider.calls != 2 {
		t.Fatalf("Retrieve() calls = %d, want 2", provider.calls)
	}
}

func TestSignerRejectsInvalidInput(t *testing.T) {
	t.Parallel()
	if _, err := NewWithProvider("", &rotatingProvider{}); err == nil {
		t.Fatal("expected region validation error")
	}
	signer, err := NewWithProvider("ap-northeast-1", &rotatingProvider{})
	if err != nil {
		t.Fatal(err)
	}
	if err := signer.Sign(context.Background(), nil, "hash", time.Now()); err == nil {
		t.Fatal("expected argument validation error")
	}
}
