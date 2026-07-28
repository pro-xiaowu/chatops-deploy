package postgres

import (
	"context"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"chatops-deploy/migrations"
	"gorm.io/gorm"
)

func Migrate(ctx context.Context, db *gorm.DB) error {
	if err := db.WithContext(ctx).Exec("CREATE TABLE IF NOT EXISTS schema_migrations (version text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())").Error; err != nil {
		return err
	}
	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		return err
	}
	names := make([]string, 0)
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".up.sql") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		var count int64
		if err := db.WithContext(ctx).Table("schema_migrations").Where("version = ?", name).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			continue
		}
		body, err := fs.ReadFile(migrations.FS, name)
		if err != nil {
			return err
		}
		if err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec(string(body)).Error; err != nil {
				return err
			}
			return tx.Exec("INSERT INTO schema_migrations(version) VALUES (?)", name).Error
		}); err != nil {
			return fmt.Errorf("apply %s: %w", name, err)
		}
	}
	return nil
}
