package db

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Plugin implements the database plugin (PostgreSQL and SQLite).
type Plugin struct{}

func (p *Plugin) Name() string   { return "postgres" }
func (p *Plugin) Prefix() string { return "db" }

func (p *Plugin) HasServices() bool { return true }

func (p *Plugin) Nodes() []api.NodeRegistration {
	return []api.NodeRegistration{
		{Descriptor: &queryDescriptor{}, Factory: newQueryExecutor},
		{Descriptor: &execDescriptor{}, Factory: newExecExecutor},
		{Descriptor: &createDescriptor{}, Factory: newCreateExecutor},
		{Descriptor: &updateDescriptor{}, Factory: newUpdateExecutor},
		{Descriptor: &deleteDescriptor{}, Factory: newDeleteExecutor},
		{Descriptor: &findDescriptor{}, Factory: newFindExecutor},
		{Descriptor: &findOneDescriptor{}, Factory: newFindOneExecutor},
		{Descriptor: &countDescriptor{}, Factory: newCountExecutor},
		{Descriptor: &upsertDescriptor{}, Factory: newUpsertExecutor},
	}
}

func (p *Plugin) CreateService(config map[string]any) (any, error) {
	driver, _ := config["driver"].(string)
	if driver == "" {
		driver = "postgres"
	}

	gormConfig := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	}

	var db *gorm.DB
	var err error

	switch driver {
	case "postgres":
		db, err = openPostgres(config, gormConfig)
	case "sqlite":
		db, err = openSQLite(config, gormConfig)
	default:
		return nil, fmt.Errorf("db: unsupported driver %q (supported: postgres, sqlite)", driver)
	}
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("db: get sql.DB: %w", err)
	}

	// Pool settings from config (defaults set per-driver above)
	if v, ok := plugin.ToInt(config["max_open"]); ok {
		sqlDB.SetMaxOpenConns(v)
	}
	if v, ok := plugin.ToInt(config["max_idle"]); ok {
		sqlDB.SetMaxIdleConns(v)
	}
	if v, ok := config["conn_lifetime"].(string); ok {
		if d, err := time.ParseDuration(v); err == nil {
			sqlDB.SetConnMaxLifetime(d)
		}
	}

	return db, nil
}

func openPostgres(config map[string]any, gormConfig *gorm.Config) (*gorm.DB, error) {
	url, _ := config["url"].(string)
	if url == "" {
		return nil, fmt.Errorf("db: postgres: missing connection 'url'")
	}

	db, err := gorm.Open(postgres.Open(url), gormConfig)
	if err != nil {
		return nil, fmt.Errorf("db: postgres: connect: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("db: postgres: get sql.DB: %w", err)
	}

	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)

	return db, nil
}

func openSQLite(config map[string]any, gormConfig *gorm.Config) (*gorm.DB, error) {
	path, _ := config["path"].(string)
	if path == "" {
		return nil, fmt.Errorf("db: sqlite: missing 'path'")
	}

	// Ensure parent directory exists.
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("db: sqlite: create directory: %w", err)
		}
	}

	db, err := gorm.Open(sqlite.Open(path), gormConfig)
	if err != nil {
		return nil, fmt.Errorf("db: sqlite: open: %w", err)
	}

	// Enable WAL mode for better concurrent read performance.
	db.Exec("PRAGMA journal_mode=WAL")
	// Enable foreign key enforcement (off by default in SQLite).
	db.Exec("PRAGMA foreign_keys=ON")

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("db: sqlite: get sql.DB: %w", err)
	}

	// SQLite only supports a single writer; keep pool small.
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(0)

	return db, nil
}

func (p *Plugin) HealthCheck(service any) error {
	db, ok := service.(*gorm.DB)
	if !ok {
		return fmt.Errorf("postgres: invalid service type")
	}
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("postgres: get sql.DB: %w", err)
	}
	return sqlDB.Ping()
}

func (p *Plugin) Shutdown(service any) error {
	db, ok := service.(*gorm.DB)
	if !ok {
		return fmt.Errorf("postgres: invalid service type")
	}
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("postgres: get sql.DB: %w", err)
	}
	return sqlDB.Close()
}
