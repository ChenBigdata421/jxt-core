package reliable

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKeyValidate(t *testing.T) {
	assert.NoError(t, Key{EventID: "e1", Handler: "h1"}.Validate())
	assert.NoError(t, Key{EventID: "e1", Handler: "h1", ItemKey: "item-7"}.Validate())
	assert.Error(t, Key{Handler: "h1"}.Validate(), "missing EventID")
	assert.Error(t, Key{EventID: "e1"}.Validate(), "missing Handler")
	assert.Error(t, Key{EventID: "e1", Handler: "h1", ItemKey: "bad\nkey"}.Validate(),
		"control char in ItemKey rejected")
}

func TestDecisionValues(t *testing.T) {
	assert.Equal(t, Decision(0), Claimed)
	assert.Equal(t, Decision(1), AlreadyProcessing)
	assert.Equal(t, Decision(2), AlreadySettled)
}

func TestClaimTokenString(t *testing.T) {
	assert.Equal(t, "claim-uuid-1234", ClaimToken("claim-uuid-1234").String())
}
