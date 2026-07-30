//go:build system
// +build system

package system_tests

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox"
	gormadapter "github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox/adapters/gorm"
	"github.com/stretchr/testify/require"
)

// ID-S1: the MySQL unique index on idempotency_key rejects a duplicate Save (the hard guarantee
// the unit suite's MockRepository cannot simulate). Two events, same key -> second errors, exactly
// one row persists.
func TestSystem_Idempotency_UniqueIndexRejectsDuplicateSave(t *testing.T) {
	db := setupMySQLDB(t)
	repo := newRepo(db)
	ctx := context.Background()

	require.NoError(t, repo.Save(ctx, newPendingEvent("id-s1-a", "shared-key-s1")))
	err := repo.Save(ctx, newPendingEvent("id-s1-b", "shared-key-s1")) // same key, different event ID
	require.Errorf(t, err, "the MySQL unique index on idempotency_key must reject the duplicate (not silent)")

	var n int64
	require.NoError(t, db.Model(&gormadapter.OutboxEventModel{}).Where("idempotency_key = ?", "shared-key-s1").Count(&n).Error)
	require.Equalf(t, int64(1), n, "exactly one row for the key after a rejected duplicate; got %d", n)
}

// ID-S2: concurrent same-key Save serializes to exactly one row. This is the race the unit suite
// explicitly could not write (TestIdempotency_ConcurrentPublish punts: "1 or more due to race
// conditions"). The MySQL unique index is the real serialization point.
func TestSystem_Idempotency_ConcurrentSameKeySaveSingleRow(t *testing.T) {
	db := setupMySQLDB(t)
	repo := newRepo(db)
	ctx := context.Background()

	const N = 20
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start // release all goroutines simultaneously to maximize contention
			// All N save the SAME key. The index lets exactly one win; the rest get duplicate-key errors
			// (ignored here — the guarantee under test is the end-state row count).
			_ = repo.Save(ctx, newPendingEvent(fmt.Sprintf("c-%d", i), "concurrent-key-s2"))
		}(i)
	}
	close(start)
	wg.Wait()

	var n int64
	require.NoError(t, db.Model(&gormadapter.OutboxEventModel{}).Where("idempotency_key = ?", "concurrent-key-s2").Count(&n).Error)
	require.Equalf(t, int64(1), n, "concurrent same-key Save must serialize to exactly one row; got %d", n)
}

// ID-S3: SaveBatch with an intra-batch key collision is rejected by the unique index and leaves no
// partial persistence (at most one row for the colliding key). Verifies batch idempotency on MySQL
// regardless of whether SaveBatch is transactional.
func TestSystem_Idempotency_SaveBatchRejectsIntraBatchCollision(t *testing.T) {
	db := setupMySQLDB(t)
	repo := newRepo(db)
	ctx := context.Background()

	// Two events in ONE batch sharing a key — the index cannot hold both.
	batch := []*outbox.OutboxEvent{
		newPendingEvent("batch-a", "batch-key-s3"),
		newPendingEvent("batch-b", "batch-key-s3"),
	}
	err := repo.SaveBatch(ctx, batch)
	require.Errorf(t, err, "SaveBatch must reject an intra-batch idempotency-key collision")

	var n int64
	require.NoError(t, db.Model(&gormadapter.OutboxEventModel{}).Where("idempotency_key = ?", "batch-key-s3").Count(&n).Error)
	require.LessOrEqualf(t, n, int64(1), "at most one row for the colliding key (index prevents partial persistence); got %d", n)
}
