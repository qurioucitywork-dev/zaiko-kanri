// Package postgresadapter implements the provider-neutral dataaccess ports
// against the zaiko PostgreSQL schema. It deliberately depends only on
// database/sql; the application composition root owns the concrete driver,
// pool configuration, credentials, and database lifetime.
package postgresadapter

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/dataaccess"
)

const (
	defaultStorageProvider = "s3"
	maxObjectBytes         = int64(15 * 1024 * 1024)

	permissionInventoryWrite  = "inventory.write"
	permissionPurchaseConfirm = "purchase.confirm"
	permissionSalesConfirm    = "sales.confirm"
	permissionShipmentConfirm = "shipment.confirm"
)

var (
	safeObjectID = regexp.MustCompile(`\A[a-zA-Z0-9_-]{1,128}\z`)
	safeBucket   = regexp.MustCompile(`\A[a-zA-Z0-9][a-zA-Z0-9._-]{1,61}[a-zA-Z0-9]\z`)
	safePrefix   = regexp.MustCompile(`\A[a-zA-Z0-9._/-]{1,256}\z`)

	_ dataaccess.DiagnosticReader        = (*Adapter)(nil)
	_ dataaccess.ProductReader           = (*Adapter)(nil)
	_ dataaccess.ProductWriter           = (*Adapter)(nil)
	_ dataaccess.InventoryWorkflowWriter = (*Adapter)(nil)
	_ dataaccess.ObjectMetadataReader    = (*Adapter)(nil)
	_ dataaccess.ObjectMetadataWriter    = (*Adapter)(nil)
)

// Config contains only metadata-location values required when creating a
// pending object row. Credentials and endpoints belong to the S3 adapter.
type Config struct {
	StorageProvider string
	StorageBucket   string
	ObjectKeyPrefix string

	// Clock and NewID exist for deterministic tests. Production callers should
	// leave them nil.
	Clock func() time.Time
	NewID func() (string, error)
}

// Adapter is safe for concurrent use when its *sql.DB is safe for concurrent
// use. The caller retains ownership of DB.
type Adapter struct {
	db              *sql.DB
	storageProvider string
	storageBucket   string
	objectKeyPrefix string
	now             func() time.Time
	newID           func() (string, error)
}

// New validates configuration without opening a connection.
func New(db *sql.DB, config Config) (*Adapter, error) {
	if db == nil {
		return nil, dataaccess.ErrInvalidArgument
	}
	provider := strings.TrimSpace(config.StorageProvider)
	if provider == "" {
		provider = defaultStorageProvider
	}
	bucket := strings.TrimSpace(config.StorageBucket)
	prefix := strings.Trim(strings.TrimSpace(config.ObjectKeyPrefix), "/")
	if provider != defaultStorageProvider || !safeBucket.MatchString(bucket) || !validPrefix(prefix) {
		return nil, dataaccess.ErrInvalidArgument
	}
	clock := config.Clock
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	newID := config.NewID
	if newID == nil {
		newID = randomID
	}
	return &Adapter{
		db:              db,
		storageProvider: provider,
		storageBucket:   bucket,
		objectKeyPrefix: prefix,
		now:             clock,
		newID:           newID,
	}, nil
}

func validPrefix(prefix string) bool {
	if prefix == "" || !safePrefix.MatchString(prefix) {
		return false
	}
	for _, segment := range strings.Split(prefix, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	return true
}

func randomID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("postgresadapter: generate ID: %w", err)
	}
	return "id_" + hex.EncodeToString(value[:]), nil
}

func (a *Adapter) objectKey(tenantID, objectID string) string {
	tenantHash := sha256.Sum256([]byte(strings.TrimSpace(tenantID)))
	return a.objectKeyPrefix + "/" + hex.EncodeToString(tenantHash[:]) + "/" + objectID
}

type scanner interface {
	Scan(dest ...any) error
}

func validateReader(a *Adapter, ctx context.Context, values ...string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if a == nil || a.db == nil {
		return dataaccess.ErrInvalidArgument
	}
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return dataaccess.ErrInvalidArgument
		}
	}
	return nil
}

func normalizeDBError(ctx context.Context, operation string, err error) error {
	if err == nil {
		return nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	var state interface{ SQLState() string }
	if errors.As(err, &state) {
		switch state.SQLState() {
		case "23505", "40001", "40P01":
			return fmt.Errorf("%w: %s", dataaccess.ErrConflict, operation)
		}
	}
	return fmt.Errorf("postgresadapter: %s: %w", operation, err)
}

func beginTx(ctx context.Context, db *sql.DB) (*sql.Tx, error) {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, normalizeDBError(ctx, "begin transaction", err)
	}
	return tx, nil
}

func commitTx(ctx context.Context, tx *sql.Tx) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return normalizeDBError(ctx, "commit transaction", err)
	}
	return nil
}

func ensureUserActive(ctx context.Context, tx *sql.Tx, tenantID, userID string) error {
	var one int
	err := tx.QueryRowContext(ctx, `
		SELECT 1
		FROM zaiko.users u
		JOIN zaiko.organizations o ON o.id = u.organization_id
		WHERE u.organization_id = $1 AND u.id = $2
		  AND u.is_active = TRUE AND u.deleted_at IS NULL
		  AND o.is_active = TRUE`,
		tenantID, userID,
	).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return dataaccess.ErrNotFound
	}
	if err != nil {
		return normalizeDBError(ctx, "verify actor ownership", err)
	}
	return nil
}

func ensureActor(ctx context.Context, tx *sql.Tx, tenantID, actorID, permission string) error {
	var one int
	err := tx.QueryRowContext(ctx, `
		SELECT 1
		FROM zaiko.users u
		JOIN zaiko.organizations o ON o.id = u.organization_id
		WHERE u.organization_id = $1 AND u.id = $2
		  AND u.is_active = TRUE AND u.deleted_at IS NULL
		  AND o.is_active = TRUE
		  AND (
			  EXISTS (
				  SELECT 1
				  FROM zaiko.user_permissions allowed
				  WHERE allowed.organization_id = u.organization_id
				    AND allowed.user_id = u.id
				    AND allowed.permission_key = $3
				    AND allowed.effect = 'allow'
			  )
			  OR (
				  NOT EXISTS (
					  SELECT 1
					  FROM zaiko.user_permissions individual
					  WHERE individual.organization_id = u.organization_id
					    AND individual.user_id = u.id
					    AND individual.permission_key = $3
				  )
				  AND EXISTS (
					  SELECT 1
					  FROM zaiko.role_permissions rp
					  WHERE rp.role_key = u.role_key
					    AND rp.permission_key = $3
				  )
			  )
		  )`,
		tenantID, actorID, permission,
	).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		// Missing, inactive, cross-tenant and unauthorized actors intentionally
		// share one result so the provider never discloses account existence.
		return dataaccess.ErrNotFound
	}
	if err != nil {
		return normalizeDBError(ctx, "verify actor authorization", err)
	}
	return nil
}

func (a *Adapter) Diagnose(ctx context.Context) (dataaccess.DiagnosticReport, error) {
	if err := validateReader(a, ctx); err != nil {
		return dataaccess.DiagnosticReport{}, err
	}
	checkedAt := a.now().UTC()
	report := dataaccess.DiagnosticReport{
		Provider:  "postgresql",
		CheckedAt: checkedAt,
	}

	report.Components = append(report.Components, a.diagnosticProbe(
		ctx, checkedAt, "connectivity", "read probe succeeded",
		`SELECT 1`,
	))
	if err := ctx.Err(); err != nil {
		return dataaccess.DiagnosticReport{}, err
	}
	report.Components = append(report.Components, a.diagnosticProbe(
		ctx, checkedAt, "schema", "zaiko schema is available",
		`SELECT 1 FROM zaiko.schema_migrations LIMIT 1`,
	))
	if err := ctx.Err(); err != nil {
		return dataaccess.DiagnosticReport{}, err
	}
	return report, nil
}

func (a *Adapter) diagnosticProbe(
	ctx context.Context,
	checkedAt time.Time,
	name, okMessage, query string,
) dataaccess.ComponentDiagnostic {
	started := time.Now()
	var one int
	err := a.db.QueryRowContext(ctx, query).Scan(&one)
	component := dataaccess.ComponentDiagnostic{
		Name:      name,
		Status:    dataaccess.DiagnosticOK,
		Message:   okMessage,
		Latency:   time.Since(started),
		CheckedAt: checkedAt,
	}
	if err != nil {
		component.Status = dataaccess.DiagnosticFailed
		component.Message = name + " read probe failed"
	}
	return component
}
