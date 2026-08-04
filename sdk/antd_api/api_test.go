package antd_apis

import (
	"errors"
	"strings"
	"testing"

	"go.uber.org/zap"
)

// TestAddError_ChainsBothErrors locks in the AddError chaining contract: a second AddError
// must wrap the previously accumulated error so BOTH messages survive in Errors.Error().
// Regression guard for the e.Error (method value) vs e.Errors (field) mix-up in AddError —
// fmt.Errorf must format the accumulated *error field*, not the Error(...) response method.
func TestAddError_ChainsBothErrors(t *testing.T) {
	e := &Api{Logger: zap.NewNop()}

	e.AddError(errors.New("first failure"))
	e.AddError(errors.New("second failure"))

	if e.Errors == nil {
		t.Fatal("expected a chained error after two AddError calls, got nil")
	}
	got := e.Errors.Error()
	if !strings.Contains(got, "first failure") || !strings.Contains(got, "second failure") {
		t.Fatalf("AddError did not chain both errors; got %q (want both 'first failure' and 'second failure')", got)
	}
}
