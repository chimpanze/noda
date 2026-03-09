package db

import (
	"fmt"
	"time"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Plugin implements the PostgreSQL database plugin.
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
	}
}

func (p *Plugin) CreateService(config map[string]any) (any, error) {
	url, _ := config["url"].(string)
	if url == "" {
		return nil, fmt.Errorf("postgres: missing connection 'url'")
	}

	gormConfig := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	}

	db, err := gorm.Open(postgres.Open(url), gormConfig)
	if err != nil {
		return nil, fmt.Errorf("postgres: connect: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("postgres: get sql.DB: %w", err)
	}

	// Pool settings
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
