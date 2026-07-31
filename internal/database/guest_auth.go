package database

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

type GuestPrincipal struct {
	OrganizationID string
	CompanyID      string
	CompanyCode    string
	CompanyName    string
	GuestID        string
	Email          string
}

func (s *Store) AuthenticateGuest(ctx context.Context, organizationCode, login, password string) (GuestPrincipal, error) {
	var principal GuestPrincipal
	var passwordHash string
	err := s.db.QueryRowContext(ctx, `
		SELECT c.organization_id,c.id,c.company_code,c.name,g.guest_id,g.email,g.password_hash
		FROM guest_credentials g
		JOIN guest_companies c ON c.organization_id=g.organization_id AND c.id=g.company_id
		JOIN organizations o ON o.id=c.organization_id
		WHERE o.code=? AND o.is_active=1 AND c.is_active=1
		  AND (g.guest_id=? OR lower(g.email)=lower(?))`,
		strings.ToUpper(strings.TrimSpace(organizationCode)), strings.TrimSpace(login), strings.TrimSpace(login)).
		Scan(&principal.OrganizationID, &principal.CompanyID, &principal.CompanyCode,
			&principal.CompanyName, &principal.GuestID, &principal.Email, &passwordHash)
	if err != nil || !VerifyPassword(passwordHash, password) {
		return GuestPrincipal{}, errors.New("invalid guest credentials")
	}
	return principal, nil
}

// CreateGuestSession stores the hash of a company-bound opaque token. The
// company ID is part of the hashed value, so changing it in the cookie does not
// turn a valid session into a session for another company.
func (s *Store) CreateGuestSession(ctx context.Context, principal GuestPrincipal, token string, ttl time.Duration) error {
	if !strings.HasPrefix(token, principal.CompanyID+".") {
		return errors.New("invalid guest session token")
	}
	var valid int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM guest_credentials g
		JOIN guest_companies c ON c.organization_id=g.organization_id AND c.id=g.company_id
		WHERE g.organization_id=? AND g.company_id=? AND c.is_active=1`,
		principal.OrganizationID, principal.CompanyID).Scan(&valid); err != nil {
		return err
	}
	if valid != 1 {
		return ErrGuestCompanyNotFound
	}
	now := s.now()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO login_csrf_tokens(token_hash,expires_at,created_at) VALUES(?,?,?)`,
		TokenHash(token), now.Add(ttl).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	return err
}

func (s *Store) GuestSession(ctx context.Context, organizationCode, token string) (GuestPrincipal, error) {
	companyID, _, ok := strings.Cut(strings.TrimSpace(token), ".")
	if !ok || companyID == "" {
		return GuestPrincipal{}, errors.New("guest session not found")
	}
	var principal GuestPrincipal
	var expires string
	err := s.db.QueryRowContext(ctx, `
		SELECT c.organization_id,c.id,c.company_code,c.name,g.guest_id,g.email,t.expires_at
		FROM login_csrf_tokens t
		JOIN guest_companies c ON c.id=? AND c.is_active=1
		JOIN guest_credentials g ON g.organization_id=c.organization_id AND g.company_id=c.id
		JOIN organizations o ON o.id=c.organization_id AND o.is_active=1
		WHERE t.token_hash=? AND o.code=?`,
		companyID, TokenHash(token), strings.ToUpper(strings.TrimSpace(organizationCode))).
		Scan(&principal.OrganizationID, &principal.CompanyID, &principal.CompanyCode,
			&principal.CompanyName, &principal.GuestID, &principal.Email, &expires)
	if err != nil {
		return GuestPrincipal{}, errors.New("guest session not found")
	}
	expiry, err := time.Parse(time.RFC3339Nano, expires)
	if err != nil || !expiry.After(s.now()) {
		_ = s.DeleteGuestSession(ctx, token)
		return GuestPrincipal{}, errors.New("guest session expired")
	}
	return principal, nil
}

func (s *Store) DeleteGuestSession(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM login_csrf_tokens WHERE token_hash=?`, TokenHash(token))
	return err
}

func (s *Store) GuestPublishedProductImage(ctx context.Context, organizationID, companyID, imageID string) (ProductImage, error) {
	var image ProductImage
	err := s.db.QueryRowContext(ctx, `
		SELECT i.image_id,i.product_id,i.storage_path,i.original_name,i.content_type,i.size_bytes,i.sort_order
		FROM guest_box_published_images i
		JOIN guest_box_publications pub
		  ON pub.organization_id=i.organization_id AND pub.company_id=i.company_id
		  AND pub.box_id=i.box_id AND pub.is_published=1
		JOIN guest_box_published_products bp
		  ON bp.organization_id=i.organization_id AND bp.company_id=i.company_id
		  AND bp.box_id=i.box_id AND bp.product_id=i.product_id
		JOIN guest_companies c
		  ON c.organization_id=i.organization_id AND c.id=i.company_id AND c.is_active=1
		WHERE i.organization_id=? AND i.company_id=? AND i.image_id=?
		ORDER BY i.published_at DESC LIMIT 1`,
		organizationID, companyID, imageID).
		Scan(&image.ID, &image.ProductID, &image.StoragePath, &image.OriginalName,
			&image.ContentType, &image.SizeBytes, &image.SortOrder)
	if errors.Is(err, sql.ErrNoRows) {
		return ProductImage{}, sql.ErrNoRows
	}
	return image, err
}
