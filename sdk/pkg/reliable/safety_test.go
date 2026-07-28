package reliable

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCanAutoReplay(t *testing.T) {
	assert.True(t, CanAutoReplay(ReplayIdempotent))
	assert.True(t, CanAutoReplay(ReplayNeedsTxClaim))
	assert.False(t, CanAutoReplay(ReplayUnsafe))
}

func TestCanManualReplay(t *testing.T) {
	for _, s := range []ReplaySafety{ReplayUnsafe, ReplayNeedsTxClaim, ReplayIdempotent} {
		assert.True(t, CanManualReplay(s))
	}
}

func TestAggregateGateKeyEmpty(t *testing.T) {
	assert.True(t, AggregateGateKey{TenantID: 1}.Empty())
	assert.False(t, AggregateGateKey{TenantID: 1, AggregateType: "Media", AggregateID: "m1"}.Empty())
}
