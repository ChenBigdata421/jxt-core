package antd_apis

import "testing"

// TestResolve_Dive_RecurseIntoNestedStruct mirrors sdk/api's dive regression test for this
// duplicate binding.go copy: a field tagged binding:"dive" must make resolve() recurse into
// the field's struct type and surface its bindings. Guards the same reflect.ValueOf(ptr).Field
// panic that previously made the dive branch unreachable.
func TestResolve_Dive_RecurseIntoNestedStruct(t *testing.T) {
	type diveInner struct {
		JSONField  string `json:"jsonField"`
		FormField  string `form:"formField"`
		QueryField string `query:"queryField"`
	}
	type diveOuter struct {
		Inner diveInner `binding:"dive"`
	}

	list := constructor.GetBindingForGin(&diveOuter{})

	got := map[string]bool{}
	for _, b := range list {
		if b != nil {
			got[b.Name()] = true
		}
	}
	for _, want := range []string{"json", "form", "query"} {
		if !got[want] {
			names := make([]string, 0, len(list))
			for _, b := range list {
				if b == nil {
					names = append(names, "<nil>")
				} else {
					names = append(names, b.Name())
				}
			}
			t.Errorf("binding:dive must surface inner %q binding; got bindings %v", want, names)
		}
	}
}
