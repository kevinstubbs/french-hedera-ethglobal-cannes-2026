package pipeline

import (
	"testing"
)

func TestAppendNaryoEventDedup(t *testing.T) {
	st := NewMemoryStore()
	sess := &Session{ID: "s1", State: StateCreated}
	st.Put(sess)

	dup, err := st.AppendNaryoEvent("s1", "e1", map[string]any{"k": 1})
	if err != nil || dup {
		t.Fatalf("first append: dup=%v err=%v", dup, err)
	}
	dup2, err := st.AppendNaryoEvent("s1", "e1", map[string]any{"k": 2})
	if err != nil || !dup2 {
		t.Fatalf("second append: dup=%v err=%v", dup2, err)
	}

	evs, err := st.NaryoEvents("s1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 1 || evs[0].EventID != "e1" {
		t.Fatalf("unexpected events: %+v", evs)
	}
	if evs[0].Payload["k"] != 1 {
		t.Fatalf("expected first payload preserved, got %#v", evs[0].Payload["k"])
	}
}

func TestAppendNaryoEventUnknownSession(t *testing.T) {
	st := NewMemoryStore()
	_, err := st.AppendNaryoEvent("nope", "e1", nil)
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestAppendNaryoEventInvalid(t *testing.T) {
	st := NewMemoryStore()
	sess := &Session{ID: "s1", State: StateCreated}
	st.Put(sess)
	_, err := st.AppendNaryoEvent("s1", "", map[string]any{})
	if err != ErrInvalidNaryoEvent {
		t.Fatalf("expected ErrInvalidNaryoEvent, got %v", err)
	}
}
