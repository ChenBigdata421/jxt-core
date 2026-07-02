package gorm

import (
	"context"
	"fmt"
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

func TestGormOutboxRepository_MarkBatchAsPublished_TransitionsAllToPublished(t *testing.T) {
	db := setupTestDB(t)
	repo := NewGormOutboxRepository(db)
	ctx := context.Background()
	now := time.Now()

	// Seed: 3 pending events (distinct ids + idempotency keys to avoid collision
	// with rows left by other tests in the shared in-mem DB)
	for _, id := range []string{"t-e1", "t-e2", "t-e3"} {
		require.NoError(t, db.Create(&OutboxEventModel{
			ID: id, Status: string(outbox.EventStatusPending), IdempotencyKey: fmt.Sprintf("mk-%s", id),
			CreatedAt: now, UpdatedAt: now,
		}).Error)
	}

	err := repo.MarkBatchAsPublished(ctx, []*outbox.OutboxEvent{
		{ID: "t-e1"}, {ID: "t-e2"}, {ID: "t-e3"},
	})
	require.NoError(t, err)

	for _, id := range []string{"t-e1", "t-e2", "t-e3"} {
		var model OutboxEventModel
		require.NoError(t, db.First(&model, "id = ?", id).Error)
		require.Equal(t, string(outbox.EventStatusPublished), model.Status)
		require.NotNil(t, model.PublishedAt)
	}
}

func TestGormOutboxRepository_MarkBatchAsPublished_IdempotentGuard(t *testing.T) {
	db := setupTestDB(t)
	repo := NewGormOutboxRepository(db)
	ctx := context.Background()
	now := time.Now()
	originalPublishedAt := now.Add(-1 * time.Hour)

	// Seed: e1 = Pending; e2 = Failed; e3 = already Published (with old timestamp)
	// (distinct ids + idempotency keys to avoid collision with rows left by
	//  TransitionsAllToPublished in the shared in-mem DB)
	require.NoError(t, db.Create(&OutboxEventModel{
		ID: "ig-e1", Status: string(outbox.EventStatusPending), IdempotencyKey: "ig-e1",
		CreatedAt: now, UpdatedAt: now,
	}).Error)
	require.NoError(t, db.Create(&OutboxEventModel{
		ID: "ig-e2", Status: string(outbox.EventStatusFailed), IdempotencyKey: "ig-e2",
		CreatedAt: now, UpdatedAt: now,
	}).Error)
	require.NoError(t, db.Create(&OutboxEventModel{
		ID: "ig-e3", Status: string(outbox.EventStatusPublished), PublishedAt: &originalPublishedAt, IdempotencyKey: "ig-e3",
		CreatedAt: now, UpdatedAt: now,
	}).Error)

	err := repo.MarkBatchAsPublished(ctx, []*outbox.OutboxEvent{
		{ID: "ig-e1"}, {ID: "ig-e2"}, {ID: "ig-e3"},
	})
	require.NoError(t, err)

	// e1 should be marked
	var m1 OutboxEventModel
	require.NoError(t, db.First(&m1, "id = ?", "ig-e1").Error)
	require.Equal(t, string(outbox.EventStatusPublished), m1.Status)

	// e2 should be UNCHANGED (Failed, not Pending — idempotency guard)
	var m2 OutboxEventModel
	require.NoError(t, db.First(&m2, "id = ?", "ig-e2").Error)
	require.Equal(t, string(outbox.EventStatusFailed), m2.Status)

	// e3 should be UNCHANGED (already Published — idempotency guard via WHERE status='pending')
	var m3 OutboxEventModel
	require.NoError(t, db.First(&m3, "id = ?", "ig-e3").Error)
	require.Equal(t, originalPublishedAt.Unix(), m3.PublishedAt.Unix(),
		"already-published event's published_at must not be overwritten")
}

func TestGormOutboxRepository_MarkBatchAsPublished_EmptyInput(t *testing.T) {
	db := setupTestDB(t)
	repo := NewGormOutboxRepository(db)

	err := repo.MarkBatchAsPublished(context.Background(), []*outbox.OutboxEvent{})
	require.NoError(t, err)
}
