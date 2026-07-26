package main

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
)

const latestMigration = "000006_approvals"

type backupManifest struct {
	CreatedAt         string   `json:"created_at"`
	DatabaseFile      string   `json:"database_file"`
	DatabaseSHA256    string   `json:"database_sha256"`
	MigrationVersions []string `json:"migration_versions"`
}

func main() {
	if len(os.Args) < 2 {
		fatal(errors.New("usage: zaiko-maintenance <verify|backup> [options]"))
	}
	var err error
	switch os.Args[1] {
	case "verify":
		err = verifyCommand(os.Args[2:])
	case "backup":
		err = backupCommand(os.Args[2:])
	case "bootstrap-admin":
		err = bootstrapAdminCommand(os.Args[2:])
	default:
		err = fmt.Errorf("unknown command: %s", os.Args[1])
	}
	if err != nil {
		fatal(err)
	}
}

func bootstrapAdminCommand(args []string) error {
	flags := flag.NewFlagSet("bootstrap-admin", flag.ContinueOnError)
	databasePath := flags.String("database", "", "SQLite database path")
	organizationCode := flags.String("organization-code", "", "organization code")
	organizationName := flags.String("organization-name", "", "organization name")
	username := flags.String("username", "admin", "admin username")
	displayName := flags.String("display-name", "管理者", "admin display name")
	passwordFile := flags.String("password-file", "", "UTF-8 file containing the initial password")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *databasePath == "" || *organizationCode == "" || *organizationName == "" || *passwordFile == "" {
		return errors.New("-database, -organization-code, -organization-name and -password-file are required")
	}
	passwordBytes, err := os.ReadFile(*passwordFile)
	if err != nil {
		return err
	}
	password := strings.TrimSpace(string(passwordBytes))
	store, err := database.Open(*databasePath)
	if err != nil {
		return err
	}
	defer store.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := store.Migrate(ctx); err != nil {
		return err
	}
	user, err := store.BootstrapOrganizationAdmin(
		ctx, *organizationCode, *organizationName, *username, *displayName, password,
	)
	if err != nil {
		return err
	}
	fmt.Printf("initial admin created: organization=%s username=%s user_id=%s\n",
		strings.ToUpper(strings.TrimSpace(*organizationCode)), user.Username, user.ID)
	return nil
}

func verifyCommand(args []string) error {
	flags := flag.NewFlagSet("verify", flag.ContinueOnError)
	databasePath := flags.String("database", "", "SQLite database path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*databasePath) == "" {
		return errors.New("-database is required")
	}
	store, err := database.Open(*databasePath)
	if err != nil {
		return err
	}
	defer store.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := store.IntegrityCheck(ctx); err != nil {
		return err
	}
	versions, err := store.MigrationVersions(ctx)
	if err != nil {
		return err
	}
	if len(versions) == 0 || versions[len(versions)-1] != latestMigration {
		return fmt.Errorf("latest migration is missing: got %v", versions)
	}
	fmt.Printf("database ok: integrity=ok migrations=%d latest=%s\n", len(versions), versions[len(versions)-1])
	return nil
}

func backupCommand(args []string) error {
	flags := flag.NewFlagSet("backup", flag.ContinueOnError)
	databasePath := flags.String("database", "", "SQLite database path")
	uploadsPath := flags.String("uploads", "", "uploads directory")
	outputPath := flags.String("output", "", "output zip path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*databasePath) == "" || strings.TrimSpace(*outputPath) == "" {
		return errors.New("-database and -output are required")
	}
	outputAbsolute, err := filepath.Abs(*outputPath)
	if err != nil {
		return err
	}
	if filepath.Ext(outputAbsolute) != ".zip" {
		return errors.New("backup output must use the .zip extension")
	}
	if _, err := os.Stat(outputAbsolute); err == nil {
		return errors.New("backup output already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	temp, err := os.MkdirTemp("", "zaiko-backup-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)
	payload := filepath.Join(temp, "payload")
	if err := os.MkdirAll(filepath.Join(payload, "database"), 0o750); err != nil {
		return err
	}
	store, err := database.Open(*databasePath)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	databaseBackup := filepath.Join(payload, "database", "zaiko.db")
	if err := store.Backup(ctx, databaseBackup); err != nil {
		store.Close()
		return err
	}
	versions, err := store.MigrationVersions(ctx)
	closeErr := store.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if len(versions) == 0 || versions[len(versions)-1] != latestMigration {
		return fmt.Errorf("refusing backup with incomplete migrations: %v", versions)
	}
	if strings.TrimSpace(*uploadsPath) != "" {
		if info, statErr := os.Stat(*uploadsPath); statErr == nil && info.IsDir() {
			if err := copyDirectory(*uploadsPath, filepath.Join(payload, "uploads")); err != nil {
				return err
			}
		} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			return statErr
		}
	}
	hash, err := fileSHA256(databaseBackup)
	if err != nil {
		return err
	}
	manifest := backupManifest{
		CreatedAt: time.Now().UTC().Format(time.RFC3339), DatabaseFile: "database/zaiko.db",
		DatabaseSHA256: hash, MigrationVersions: versions,
	}
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(payload, "manifest.json"), encoded, 0o640); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outputAbsolute), 0o750); err != nil {
		return err
	}
	if err := zipDirectory(payload, outputAbsolute); err != nil {
		return err
	}
	fmt.Printf("backup created: %s\n", outputAbsolute)
	return nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func copyDirectory(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("uploads contain unsupported symbolic link: %s", path)
		}
		if entry.IsDir() {
			return os.MkdirAll(target, 0o750)
		}
		return copyFile(path, target, info.Mode().Perm())
	})
}

func copyFile(source, destination string, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0o750); err != nil {
		return err
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(output, input); err != nil {
		output.Close()
		return err
	}
	return output.Close()
}

func zipDirectory(source, destination string) error {
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	archive := zip.NewWriter(output)
	walkErr := filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		writer, err := archive.Create(filepath.ToSlash(relative))
		if err != nil {
			return err
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(writer, input)
		closeErr := input.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	closeArchiveErr := archive.Close()
	closeOutputErr := output.Close()
	if walkErr != nil {
		return walkErr
	}
	if closeArchiveErr != nil {
		return closeArchiveErr
	}
	return closeOutputErr
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
