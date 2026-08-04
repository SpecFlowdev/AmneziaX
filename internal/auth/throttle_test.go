package auth

import (
	"testing"
	"time"
)

func newTestThrottle(clock *time.Time) *Throttle {
	t := NewThrottle()
	t.now = func() time.Time { return *clock }
	return t
}

func TestThrottleLocksAfterThreshold(t *testing.T) {
	now := time.Now()
	th := newTestThrottle(&now)

	for i := 1; i < th.Threshold; i++ {
		if d := th.Fail("user:admin", "ip:198.51.100.7"); d != 0 {
			t.Fatalf("failure %d locked early, for %v", i, d)
		}
		if locked, _ := th.Locked("user:admin", "ip:198.51.100.7"); locked {
			t.Fatalf("locked after only %d failures", i)
		}
	}

	if d := th.Fail("user:admin", "ip:198.51.100.7"); d != th.Base {
		t.Fatalf("threshold failure gave %v, want %v", d, th.Base)
	}
	locked, left := th.Locked("user:admin", "ip:198.51.100.7")
	if !locked || left <= 0 {
		t.Fatalf("not locked after the threshold: locked=%v left=%v", locked, left)
	}
}

func TestThrottleLockExpires(t *testing.T) {
	now := time.Now()
	th := newTestThrottle(&now)
	for i := 0; i < th.Threshold; i++ {
		th.Fail("user:admin")
	}
	if locked, _ := th.Locked("user:admin"); !locked {
		t.Fatal("expected a lock")
	}

	now = now.Add(th.Base + time.Second)
	if locked, _ := th.Locked("user:admin"); locked {
		t.Fatal("still locked after the lockout elapsed")
	}
}

func TestThrottleBackoffDoublesAndCaps(t *testing.T) {
	now := time.Now()
	th := newTestThrottle(&now)
	for i := 0; i < th.Threshold; i++ {
		th.Fail("user:admin")
	}

	want := th.Base
	for i := 0; i < 12; i++ {
		now = now.Add(time.Second)
		got := th.Fail("user:admin")
		if want < th.Max {
			want *= 2
		}
		if want > th.Max {
			want = th.Max
		}
		if got != want {
			t.Fatalf("failure %d gave %v, want %v", i, got, want)
		}
	}
	if want != th.Max {
		t.Fatalf("backoff settled at %v, want the cap %v", want, th.Max)
	}
}

// Either key crossing the threshold must be enough, otherwise an attacker
// spreading one guess across many addresses, or many guesses from one address,
// slips through.
func TestThrottleEitherKeyLocks(t *testing.T) {
	now := time.Now()
	th := newTestThrottle(&now)

	// One address, a different username every time: the username counters stay
	// low, the address counter does not.
	for i := 0; i < th.Threshold; i++ {
		th.Fail(string(rune('a'+i))+":user", "ip:203.0.113.9")
	}
	if locked, _ := th.Locked("z:user", "ip:203.0.113.9"); !locked {
		t.Fatal("the address should be locked after spreading guesses across usernames")
	}
	// A different address with an untouched username is unaffected.
	if locked, _ := th.Locked("z:user", "ip:203.0.113.10"); locked {
		t.Fatal("an unrelated address was locked")
	}
}

func TestThrottleForgetsOldFailures(t *testing.T) {
	now := time.Now()
	th := newTestThrottle(&now)

	for i := 0; i < th.Threshold-1; i++ {
		th.Fail("user:admin")
		now = now.Add(time.Second)
	}
	// A typo an hour ago should not combine with a typo now.
	now = now.Add(th.Window + time.Minute)
	if d := th.Fail("user:admin"); d != 0 {
		t.Fatalf("stale failures still counted: locked for %v", d)
	}
}

func TestThrottleSucceedClears(t *testing.T) {
	now := time.Now()
	th := newTestThrottle(&now)
	for i := 0; i < th.Threshold; i++ {
		th.Fail("user:admin", "ip:198.51.100.7")
	}
	th.Succeed("user:admin", "ip:198.51.100.7")
	if locked, _ := th.Locked("user:admin", "ip:198.51.100.7"); locked {
		t.Fatal("still locked after a successful sign-in")
	}
}
