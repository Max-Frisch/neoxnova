package handlers

import "testing"

func TestLockoutLifecycle(t *testing.T) {
	l := newLockout()
	if l.locked("alice") {
		t.Fatal("fresh key must not be locked")
	}
	for i := 0; i < lockoutThreshold; i++ {
		l.fail("alice")
	}
	if !l.locked("alice") {
		t.Fatal("key should be locked after threshold failures")
	}
	if l.locked("bob") {
		t.Fatal("other keys must be unaffected")
	}
	l.reset("alice")
	if l.locked("alice") {
		t.Fatal("reset should clear the lockout")
	}
}
