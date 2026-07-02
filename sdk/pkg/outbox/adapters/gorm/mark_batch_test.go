package gorm

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	_ "modernc.org/sqlite"
)

var testDBCounter atomic.Int64

// setupTestDB creates an isolated in-memory sqlite DB for a single test.
// Each call gets a unique DSN so tests never share state (cache=shared shares
// across connections on the SAME dsn; a unique dsn per test keeps pools private).
func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	id := testDBCounter.Add(1)
	dsn := fmt.Sprintf("file:memdb_%d?mode=memory&cache=shared", id)
	db, err := gorm.Open(sqlite.Dialector{DriverName: "sqlite", DSN: dsn}, &gorm.Config{})
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

	// Seed: 3 pending events (distinct idempotency keys required by the model's
	// unique index — empty keys would collide)
	for _, e := range []struct{ id, key string }{
		{"e1", "mk1"}, {"e2", "mk2"}, {"e3", "mk3"},
	} {
		require.NoError(t, db.Create(&OutboxEventModel{
			ID: e.id, Status: string(outbox.EventStatusPending), IdempotencyKey: e.key,
			CreatedAt: now, UpdatedAt: now,
		}).Error)
	}

	err := repo.MarkBatchAsPublished(ctx, []*outbox.OutboxEvent{
		{ID: "e1"}, {ID: "e2"}, {ID: "e3"},
	})
	require.NoError(t, err)

	for _, id := range []string{"e1", "e2", "e3"} {
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
	// (distinct idempotency keys required by the model's unique index)
	require.NoError(t, db.Create(&OutboxEventModel{
		ID: "e1", Status: string(outbox.EventStatusPending), IdempotencyKey: "mk1",
		CreatedAt: now, UpdatedAt: now,
	}).Error)
	require.NoError(t, db.Create(&OutboxEventModel{
		ID: "e2", Status: string(outbox.EventStatusFailed), IdempotencyKey: "mk2",
		CreatedAt: now, UpdatedAt: now,
	}).Error)
	require.NoError(t, db.Create(&OutboxEventModel{
		ID: "e3", Status: string(outbox.EventStatusPublished), PublishedAt: &originalPublishedAt, IdempotencyKey: "mk3",
		CreatedAt: now, UpdatedAt: now,
	}).Error)

	err := repo.MarkBatchAsPublished(ctx, []*outbox.OutboxEvent{
		{ID: "e1"}, {ID: "e2"}, {ID: "e3"},
	})
	require.NoError(t, err)

	// e1 should be marked
	var m1 OutboxEventModel
	require.NoError(t, db.First(&m1, "id = ?", "e1").Error)
	require.Equal(t, string(outbox.EventStatusPublished), m1.Status)

	// e2 should be UNCHANGED (Failed, not Pending — idempotency guard)
	var m2 OutboxEventModel
	require.NoError(t, db.First(&m2, "id = ?", "e2").Error)
	require.Equal(t, string(outbox.EventStatusFailed), m2.Status)

	// e3 should be UNCHANGED (already Published — idempotency guard via WHERE status='pending')
	var m3 OutboxEventModel
	require.NoError(t, db.First(&m3, "id = ?", "e3").Error)
	require.Equal(t, originalPublishedAt.Unix(), m3.PublishedAt.Unix(),
		"already-published event's published_at must not be overwritten")
}

func TestGormOutboxRepository_MarkBatchAsPublished_EmptyInput(t *testing.T) {
	db := setupTestDB(t)
	repo := NewGormOutboxRepository(db)

	err := repo.MarkBatchAsPublished(context.Background(), []*outbox.OutboxEvent{})
	require.NoError(t, err)
}
