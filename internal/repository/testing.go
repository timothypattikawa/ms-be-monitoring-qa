package repository

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// NewSQLiteForTest returns a Monitoring repository backed by an in-memory
// SQLite database with the full schema migrated, for handler/repository
// tests that need real query behavior instead of just schema-shape checks.
func NewSQLiteForTest(t *testing.T) *Monitoring {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(Models()...); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return &Monitoring{db: db}
}
