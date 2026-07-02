package gorm

import (
	"context"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	_ "modernc.org/sqlite"
)

// setupTestDB creates an in-memory sqlite DB with the outbox_events table migrated.
func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Dialector{DriverName: "sqlite", DSN: "file::memory:?cache=shared"}, &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&OutboxEventModel{}))
	return db
}

func TestGormOutboxRepository_FindPublishedByIdempotencyKeys(t *testing.T) {
	db := setupTestDB(t)
	repo := NewGormOutboxRepository(db)
	ctx := context.Background()
	now := time.Now()

	// Seed: k1, k2, k3 = Published; k4 = Pending; k5 = does not exist
	for _, id := range []struct {
		id, key, status string
	}{
		{"e1", "k1", string(outbox.EventStatusPublished)},
		{"e2", "k2", string(outbox.EventStatusPublished)},
		{"e3", "k3", string(outbox.EventStatusPublished)},
		{"e4", "k4", string(outbox.EventStatusPending)},
	} {
		require.NoError(t, db.Create(&OutboxEventModel{
			ID: id.id, IdempotencyKey: id.key, Status: id.status,
			CreatedAt: now, UpdatedAt: now,
		}).Error)
	}

	found, err := repo.FindPublishedByIdempotencyKeys(ctx, []string{"k1", "k2", "k3", "k4", "k5"})
	require.NoError(t, err)
	require.Len(t, found, 3)
	require.Contains(t, found, "k1")
	require.Contains(t, found, "k2")
	require.Contains(t, found, "k3")
	require.NotContains(t, found, "k4")
	require.NotContains(t, found, "k5")
}

func TestGormOutboxRepository_FindPublishedByIdempotencyKeys_EmptyInput(t *testing.T) {
	db := setupTestDB(t)
	repo := NewGormOutboxRepository(db)

	found, err := repo.FindPublishedByIdempotencyKeys(context.Background(), []string{})
	require.NoError(t, err)
	require.Empty(t, found)
}
