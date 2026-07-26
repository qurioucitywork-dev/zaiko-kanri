package database

import (
	"context"
	"errors"
	"os"
	"path/filepath"
)

func (s *Store) IntegrityCheck(ctx context.Context) error {
	var result string
	if err := s.db.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&result); err != nil {
		return err
	}
	if result != "ok" {
		return errors.New("データベース整合性検査に失敗しました: " + result)
	}
	return nil
}

func (s *Store) MigrationVersions(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var versions []string
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}
	return versions, rows.Err()
}

func (s *Store) Backup(ctx context.Context, targetPath string) error {
	if targetPath == "" {
		return errors.New("バックアップ先が指定されていません")
	}
	absolute, err := filepath.Abs(targetPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(absolute), 0o750); err != nil {
		return err
	}
	if _, err := os.Stat(absolute); err == nil {
		return errors.New("バックアップ先ファイルが既に存在します")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := s.IntegrityCheck(ctx); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `VACUUM INTO ?`, absolute); err != nil {
		return err
	}
	backup, err := Open(absolute)
	if err != nil {
		return err
	}
	defer backup.Close()
	return backup.IntegrityCheck(ctx)
}
