package migrate

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

// Migration represents a single migration file pair.
type Migration struct {
	Version string // timestamp prefix, e.g. "20260308120000"
	Name    string // human-readable name
}

// MigrationStatus describes whether a migration has been applied.
type MigrationStatus struct {
	Migration
	Applied   bool
	AppliedAt *time.Time
}

// schemaMigration represents a row in the schema_migrations table.
type schemaMigration struct {
	Version   string    `gorm:"primaryKey"`
	AppliedAt time.Time `gorm:"autoCreateTime"`
}

// Create generates a new migration file pair in the given directory.
func Create(migrationsDir, name string) (string, string, error) {
	if err := os.MkdirAll(migrationsDir, 0755); err != nil {
		return "", "", fmt.Errorf("create migrations dir: %w", err)
	}

	timestamp := time.Now().UTC().Format("20060102150405")
	base := fmt.Sprintf("%s_%s", timestamp, name)
	upFile := filepath.Join(migrationsDir, base+".up.sql")
	downFile := filepath.Join(migrationsDir, base+".down.sql")

	upContent := "-- Write your migration SQL here\n"
	downContent := "-- Write your rollback SQL here\n"

	if err := os.WriteFile(upFile, []byte(upContent), 0644); err != nil {
		return "", "", fmt.Errorf("write up file: %w", err)
	}
	if err := os.WriteFile(downFile, []byte(downContent), 0644); err != nil {
		return "", "", fmt.Errorf("write down file: %w", err)
	}

	return upFile, downFile, nil
}

// Up applies all pending migrations in order.
func Up(db *gorm.DB, migrationsDir string) ([]string, error) {
	if err := ensureMigrationsTable(db); err != nil {
		return nil, err
	}

	applied, err := getApplied(db)
	if err != nil {
		return nil, err
	}

	files, err := findMigrations(migrationsDir)
	if err != nil {
		return nil, err
	}

	var ran []string
	for _, m := range files {
		if applied[m.Version] {
			continue
		}

		upFile := filepath.Join(migrationsDir, fmt.Sprintf("%s_%s.up.sql", m.Version, m.Name))
		sql, err := os.ReadFile(upFile)
		if err != nil {
			return ran, fmt.Errorf("read %s: %w", upFile, err)
		}

		if err := db.Exec(string(sql)).Error; err != nil {
			return ran, fmt.Errorf("apply %s_%s: %w", m.Version, m.Name, err)
		}

		if err := db.Create(&schemaMigration{Version: m.Version}).Error; err != nil {
			return ran, fmt.Errorf("record migration %s: %w", m.Version, err)
		}

		ran = append(ran, fmt.Sprintf("%s_%s", m.Version, m.Name))
	}

	return ran, nil
}

// Down rolls back the last applied migration.
func Down(db *gorm.DB, migrationsDir string) (string, error) {
	if err := ensureMigrationsTable(db); err != nil {
		return "", err
	}

	// Find the last applied migration
	var last schemaMigration
	if err := db.Order("version DESC").First(&last).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", fmt.Errorf("no migrations to roll back")
		}
		return "", fmt.Errorf("query last migration: %w", err)
	}

	files, err := findMigrations(migrationsDir)
	if err != nil {
		return "", err
	}

	// Find the matching migration
	var target *Migration
	for _, m := range files {
		if m.Version == last.Version {
			target = &m
			break
		}
	}
	if target == nil {
		return "", fmt.Errorf("migration file for version %s not found", last.Version)
	}

	downFile := filepath.Join(migrationsDir, fmt.Sprintf("%s_%s.down.sql", target.Version, target.Name))
	sql, err := os.ReadFile(downFile)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", downFile, err)
	}

	if err := db.Exec(string(sql)).Error; err != nil {
		return "", fmt.Errorf("rollback %s_%s: %w", target.Version, target.Name, err)
	}

	if err := db.Where("version = ?", target.Version).Delete(&schemaMigration{}).Error; err != nil {
		return "", fmt.Errorf("remove migration record %s: %w", target.Version, err)
	}

	return fmt.Sprintf("%s_%s", target.Version, target.Name), nil
}

// Status returns the status of all migrations.
func Status(db *gorm.DB, migrationsDir string) ([]MigrationStatus, error) {
	if err := ensureMigrationsTable(db); err != nil {
		return nil, err
	}

	applied, err := getAppliedWithTimes(db)
	if err != nil {
		return nil, err
	}

	files, err := findMigrations(migrationsDir)
	if err != nil {
		return nil, err
	}

	var statuses []MigrationStatus
	for _, m := range files {
		ms := MigrationStatus{Migration: m}
		if t, ok := applied[m.Version]; ok {
			ms.Applied = true
			ms.AppliedAt = &t
		}
		statuses = append(statuses, ms)
	}

	return statuses, nil
}

func ensureMigrationsTable(db *gorm.DB) error {
	return db.AutoMigrate(&schemaMigration{})
}

func getApplied(db *gorm.DB) (map[string]bool, error) {
	var records []schemaMigration
	if err := db.Find(&records).Error; err != nil {
		return nil, fmt.Errorf("query applied migrations: %w", err)
	}
	m := make(map[string]bool, len(records))
	for _, r := range records {
		m[r.Version] = true
	}
	return m, nil
}

func getAppliedWithTimes(db *gorm.DB) (map[string]time.Time, error) {
	var records []schemaMigration
	if err := db.Find(&records).Error; err != nil {
		return nil, fmt.Errorf("query applied migrations: %w", err)
	}
	m := make(map[string]time.Time, len(records))
	for _, r := range records {
		m[r.Version] = r.AppliedAt
	}
	return m, nil
}

// findMigrations scans the directory for .up.sql files and returns sorted migrations.
func findMigrations(dir string) ([]Migration, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}

	var migrations []Migration
	seen := make(map[string]bool)

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".up.sql") {
			continue
		}

		// Parse: YYYYMMDDHHMMSS_name.up.sql
		base := strings.TrimSuffix(name, ".up.sql")
		parts := strings.SplitN(base, "_", 2)
		if len(parts) != 2 {
			continue
		}

		version := parts[0]
		migName := parts[1]

		if seen[version] {
			continue
		}
		seen[version] = true

		migrations = append(migrations, Migration{
			Version: version,
			Name:    migName,
		})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}
