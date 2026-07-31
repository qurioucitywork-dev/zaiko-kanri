// Package fsblob provides a local-development ObjectBlobStore. It is not a
// production substitute for R2 or S3.
package fsblob

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/dataaccess"
)

var safeObjectID = regexp.MustCompile(`\A[a-zA-Z0-9_-]{1,128}\z`)

type Adapter struct {
	root string
}

func New(root string) (*Adapter, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, dataaccess.ErrInvalidArgument
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("fsblob: resolve root: %w", err)
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return nil, fmt.Errorf("fsblob: create root: %w", err)
	}
	return &Adapter{root: absolute}, nil
}

func (a *Adapter) Put(
	ctx context.Context,
	tenantID, objectID, contentType string,
	maxBytes int64,
	body io.Reader,
) (dataaccess.BlobReceipt, error) {
	if strings.TrimSpace(tenantID) == "" ||
		!safeObjectID.MatchString(objectID) ||
		maxBytes < 1 ||
		body == nil {
		return dataaccess.BlobReceipt{}, dataaccess.ErrInvalidArgument
	}
	select {
	case <-ctx.Done():
		return dataaccess.BlobReceipt{}, ctx.Err()
	default:
	}

	dir, target, err := a.paths(tenantID, objectID)
	if err != nil {
		return dataaccess.BlobReceipt{}, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return dataaccess.BlobReceipt{}, fmt.Errorf("fsblob: create tenant directory: %w", err)
	}
	if _, err := os.Stat(target); err == nil {
		return dataaccess.BlobReceipt{}, dataaccess.ErrConflict
	} else if !errors.Is(err, os.ErrNotExist) {
		return dataaccess.BlobReceipt{}, fmt.Errorf("fsblob: inspect target: %w", err)
	}

	buffered := bufio.NewReader(io.LimitReader(body, maxBytes+1))
	peek, err := buffered.Peek(512)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, bufio.ErrBufferFull) {
		return dataaccess.BlobReceipt{}, fmt.Errorf("fsblob: inspect content: %w", err)
	}
	detected := http.DetectContentType(peek)
	if detected != strings.ToLower(strings.TrimSpace(contentType)) {
		return dataaccess.BlobReceipt{}, dataaccess.ErrInvalidArgument
	}

	temp, err := os.CreateTemp(dir, ".pending-*")
	if err != nil {
		return dataaccess.BlobReceipt{}, fmt.Errorf("fsblob: create pending object: %w", err)
	}
	tempName := temp.Name()
	defer func() {
		_ = temp.Close()
		_ = os.Remove(tempName)
	}()
	if err := temp.Chmod(0o600); err != nil {
		return dataaccess.BlobReceipt{}, fmt.Errorf("fsblob: protect pending object: %w", err)
	}

	hasher := sha256.New()
	written, err := copyContext(ctx, io.MultiWriter(temp, hasher), buffered)
	if err != nil {
		return dataaccess.BlobReceipt{}, fmt.Errorf("fsblob: write object: %w", err)
	}
	if written > maxBytes {
		return dataaccess.BlobReceipt{}, dataaccess.ErrInvalidArgument
	}
	if err := temp.Sync(); err != nil {
		return dataaccess.BlobReceipt{}, fmt.Errorf("fsblob: sync object: %w", err)
	}
	if err := temp.Close(); err != nil {
		return dataaccess.BlobReceipt{}, fmt.Errorf("fsblob: close object: %w", err)
	}
	// Link is an atomic no-clobber publish on the same volume. Rename may
	// replace an existing target on Unix and would allow a concurrent writer
	// to overwrite immutable object bytes.
	if err := os.Link(tempName, target); err != nil {
		if _, statErr := os.Stat(target); statErr == nil {
			return dataaccess.BlobReceipt{}, dataaccess.ErrConflict
		}
		return dataaccess.BlobReceipt{}, fmt.Errorf("fsblob: publish object: %w", err)
	}
	if err := os.Remove(tempName); err != nil {
		return dataaccess.BlobReceipt{}, fmt.Errorf("fsblob: remove pending link: %w", err)
	}
	return dataaccess.BlobReceipt{
		ChecksumSHA256: hex.EncodeToString(hasher.Sum(nil)),
		SizeBytes:      written,
	}, nil
}

func (a *Adapter) Head(
	ctx context.Context,
	tenantID, objectID string,
) (dataaccess.BlobHead, error) {
	_, target, err := a.paths(tenantID, objectID)
	if err != nil {
		return dataaccess.BlobHead{}, err
	}
	file, err := os.Open(target)
	if errors.Is(err, os.ErrNotExist) {
		return dataaccess.BlobHead{Exists: false}, nil
	}
	if err != nil {
		return dataaccess.BlobHead{}, fmt.Errorf("fsblob: open object: %w", err)
	}
	defer file.Close()
	hasher := sha256.New()
	size, err := copyContext(ctx, hasher, file)
	if err != nil {
		return dataaccess.BlobHead{}, fmt.Errorf("fsblob: hash object: %w", err)
	}
	return dataaccess.BlobHead{
		ChecksumSHA256: hex.EncodeToString(hasher.Sum(nil)),
		SizeBytes:      size,
		Exists:         true,
	}, nil
}

func (a *Adapter) Open(
	ctx context.Context,
	tenantID, objectID string,
) (io.ReadCloser, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	_, target, err := a.paths(tenantID, objectID)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(target)
	if errors.Is(err, os.ErrNotExist) {
		return nil, dataaccess.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("fsblob: open object: %w", err)
	}
	return file, nil
}

func (a *Adapter) Delete(
	ctx context.Context,
	tenantID, objectID string,
) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	_, target, err := a.paths(tenantID, objectID)
	if err != nil {
		return err
	}
	if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("fsblob: delete object: %w", err)
	}
	return nil
}

func (a *Adapter) paths(tenantID, objectID string) (string, string, error) {
	if strings.TrimSpace(tenantID) == "" || !safeObjectID.MatchString(objectID) {
		return "", "", dataaccess.ErrInvalidArgument
	}
	tenantHash := sha256.Sum256([]byte(tenantID))
	dir := filepath.Join(a.root, hex.EncodeToString(tenantHash[:]))
	target := filepath.Join(dir, objectID)
	relative, err := filepath.Rel(a.root, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", "", dataaccess.ErrInvalidArgument
	}
	return dir, target, nil
}

func copyContext(ctx context.Context, destination io.Writer, source io.Reader) (int64, error) {
	buffer := make([]byte, 32<<10)
	var total int64
	for {
		select {
		case <-ctx.Done():
			return total, ctx.Err()
		default:
		}
		count, readErr := source.Read(buffer)
		if count > 0 {
			written, writeErr := destination.Write(buffer[:count])
			total += int64(written)
			if writeErr != nil {
				return total, writeErr
			}
			if written != count {
				return total, io.ErrShortWrite
			}
		}
		if errors.Is(readErr, io.EOF) {
			return total, nil
		}
		if readErr != nil {
			return total, readErr
		}
	}
}

var _ dataaccess.ObjectBlobStore = (*Adapter)(nil)
