package ledger

import "testing"

func TestCreditAndCharge(t *testing.T) {
	l := NewMemoryLedger()
	if err := l.Credit("a1", 100, "tx1", "dep"); err != nil {
		t.Fatal(err)
	}
	if err := l.Credit("a1", 100, "tx1", "dep"); err != nil {
		t.Fatal(err)
	}
	b, _ := l.GetBalance("a1")
	if b != 100 {
		t.Fatalf("balance after duplicate credit: %d", b)
	}
	rem, err := l.ChargeUsage("a1", "p1", 40, 1)
	if err != nil || rem != 60 {
		t.Fatalf("charge: rem=%d err=%v", rem, err)
	}
	_, err = l.ChargeUsage("a1", "p1", 40, 1)
	if err != nil {
		t.Fatalf("idempotent charge: %v", err)
	}
	b, _ = l.GetBalance("a1")
	if b != 60 {
		t.Fatalf("balance: %d", b)
	}
}

func TestInsufficient(t *testing.T) {
	l := NewMemoryLedger()
	_ = l.Credit("a1", 10, "", "x")
	_, err := l.ChargeUsage("a1", "p1", 20, 5)
	if err != ErrInsufficientFunds {
		t.Fatalf("want ErrInsufficientFunds got %v", err)
	}
}
