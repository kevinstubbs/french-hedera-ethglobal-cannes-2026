package naryo

import "testing"

func TestDestinationsIncludePathAndFullURL(t *testing.T) {
	want := "/internal/naryo/v1/events/sess-abc"
	cases := []struct {
		dests []any
		ok    bool
	}{
		{[]any{want}, true},
		{[]any{"http://host.docker.internal:8080" + want}, true},
		{[]any{"https://example.com:8080" + want + "/"}, true},
		{[]any{"/other/prefix" + want}, true},
		{[]any{"/internal/naryo/v1/events/other"}, false},
	}
	for _, tc := range cases {
		if got := destinationsInclude(tc.dests, want); got != tc.ok {
			t.Fatalf("destinationsInclude(%#v, %q) = %v want %v", tc.dests, want, got, tc.ok)
		}
	}
}
