package api

import (
	"testing"
	"time"
)

func TestIPLimiterBurstAndIsolation(t *testing.T) {
	l := newIPLimiter(0, 2) // no refill, burst 2
	if !l.allow("1.2.3.4") || !l.allow("1.2.3.4") {
		t.Fatal("burst of 2 should be allowed")
	}
	if l.allow("1.2.3.4") {
		t.Fatal("third request should be denied")
	}
	if !l.allow("5.6.7.8") {
		t.Fatal("a different IP must be unaffected")
	}
}

func TestIPLimiterRefills(t *testing.T) {
	l := newIPLimiter(100, 1) // 100 tokens/sec, burst 1
	if !l.allow("x") {
		t.Fatal("first allowed")
	}
	if l.allow("x") {
		t.Fatal("immediate second denied")
	}
	time.Sleep(30 * time.Millisecond)
	if !l.allow("x") {
		t.Fatal("should have refilled")
	}
}
