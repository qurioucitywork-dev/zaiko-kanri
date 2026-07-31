package database

import (
	"context"
	"errors"
	"strings"
	"time"
)

var (
	ErrManagedUserNotFound = errors.New("対象の利用者が見つかりません")
	ErrManagedUserExists   = errors.New("同じログインIDが既に登録されています")
)

type ManagedUserInput struct {
	OrganizationID string
	Username       string
	DisplayName    string
	Role           string
	Password       string
}

func (s *Store) CreateManagedUser(ctx context.Context, input ManagedUserInput) (User, error) {
	input.Username = strings.ToLower(strings.TrimSpace(input.Username))
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	if input.Username == "" || input.DisplayName == "" {
		return User{}, errors.New("名前とログインIDを入力してください")
	}
	if input.Role != RoleAdmin && input.Role != RoleWorker {
		return User{}, errors.New("利用者種別が正しくありません")
	}
	hash, err := HashPassword(input.Password)
	if err != nil {
		return User{}, errors.New("パスワードは12文字以上で入力してください")
	}
	id, err := NewID("usr")
	if err != nil {
		return User{}, err
	}
	now := s.now().Format(time.RFC3339Nano)
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO users(id,organization_id,username,password_hash,display_name,role_key,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?)`,
		id, input.OrganizationID, input.Username, hash, input.DisplayName, input.Role, now, now)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return User{}, ErrManagedUserExists
		}
		return User{}, err
	}
	return User{ID: id, OrganizationID: input.OrganizationID, Username: input.Username,
		DisplayName: input.DisplayName, Role: input.Role, Active: true}, nil
}

func (s *Store) ChangeManagedUserPassword(ctx context.Context, organizationID, userID, password string) error {
	hash, err := HashPassword(password)
	if err != nil {
		return errors.New("パスワードは12文字以上で入力してください")
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE users SET password_hash=?,updated_at=?
		WHERE organization_id=? AND id=? AND deleted_at IS NULL`,
		hash, s.now().Format(time.RFC3339Nano), organizationID, userID)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrManagedUserNotFound
	}
	// Existing sessions are revoked so the new credential takes effect safely.
	_, err = s.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id=?`, userID)
	return err
}

func (s *Store) DeleteManagedUser(ctx context.Context, organizationID, userID string) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE users SET is_active=0,deleted_at=?,updated_at=?
		WHERE organization_id=? AND id=? AND deleted_at IS NULL AND role_key<>?`,
		s.now().Format(time.RFC3339Nano), s.now().Format(time.RFC3339Nano), organizationID, userID, RoleAdmin)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrManagedUserNotFound
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id=?`, userID)
	return err
}

type GuestCredential struct {
	CompanyID   string
	CompanyName string
	GuestID     string
	Email       string
}

func (s *Store) GuestCredentials(ctx context.Context, organizationID string) ([]GuestCredential, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.id,c.name,g.guest_id,g.email
		FROM guest_companies c
		JOIN guest_credentials g
		  ON g.organization_id=c.organization_id AND g.company_id=c.id
		WHERE c.organization_id=? AND c.is_active=1
		ORDER BY g.guest_id`, organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var credentials []GuestCredential
	for rows.Next() {
		var credential GuestCredential
		if err := rows.Scan(&credential.CompanyID, &credential.CompanyName,
			&credential.GuestID, &credential.Email); err != nil {
			return nil, err
		}
		credentials = append(credentials, credential)
	}
	return credentials, rows.Err()
}

func (s *Store) RotateGuestPasswords(ctx context.Context, organizationID, actorID, password string) (int64, error) {
	hash, err := HashPassword(password)
	if err != nil {
		return 0, errors.New("パスワードは12文字以上で入力してください")
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE guest_credentials SET password_hash=?,updated_by=?,updated_at=?
		WHERE organization_id=?`,
		hash, actorID, s.now().Format(time.RFC3339Nano), organizationID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (s *Store) ChangeGuestPassword(ctx context.Context, organizationID, companyID, actorID, password string) error {
	hash, err := HashPassword(password)
	if err != nil {
		return errors.New("パスワードは12文字以上で入力してください")
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE guest_credentials SET password_hash=?,updated_by=?,updated_at=?
		WHERE organization_id=? AND company_id=?`,
		hash, actorID, s.now().Format(time.RFC3339Nano), organizationID, companyID)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrGuestCompanyNotFound
	}
	return nil
}

func (s *Store) DeleteGuestCredential(ctx context.Context, organizationID, companyID string) error {
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM guest_credentials WHERE organization_id=? AND company_id=?`,
		organizationID, companyID)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrGuestCompanyNotFound
	}
	return nil
}
