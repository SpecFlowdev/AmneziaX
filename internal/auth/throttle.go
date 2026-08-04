package auth

import (
	"sync"
	"time"
)

// Throttle slows down guessing at the sign-in form.
//
// It counts failures against two keys — the username tried and the address it
// came from — and locks whichever crosses the threshold. Both are needed:
// counting only usernames lets one attacker work through a list unimpeded,
// and counting only addresses lets a botnet spread the same guess across
// thousands of hosts. Either key locking is enough to refuse the attempt.
//
// State is in memory. That is the right trade for a single panel process: it
// costs no write per failed guess, and the only way to clear it is to restart
// the panel, which an attacker on the outside cannot do. An operator running
// several panel replicas behind a balancer gets per-replica counting, which is
// weaker but never wrong.
type Throttle struct {
	mu      sync.Mutex
	entries map[string]*attempt

	// Threshold is the number of failures a key may accumulate before it locks.
	Threshold int
	// Window is how long failures are remembered. Failures further apart than
	// this never add up, so an occasional typo never locks anyone out.
	Window time.Duration
	// Base is the first lockout. Each further failure while locked doubles it,
	// up to Max — a person who mistyped waits seconds, a script waits longer
	// with every attempt.
	Base time.Duration
	Max  time.Duration

	now func() time.Time // injectable for the tests
}

type attempt struct {
	failures int
	last     time.Time
	until    time.Time
	backoff  time.Duration
}

func NewThrottle() *Throttle {
	return &Throttle{
		entries:   make(map[string]*attempt),
		Threshold: 5,
		Window:    15 * time.Minute,
		Base:      30 * time.Second,
		Max:       15 * time.Minute,
		now:       time.Now,
	}
}

// Locked reports whether any of the keys is currently locked, and for how long.
// Keys are checked together so the caller cannot forget one.
func (t *Throttle) Locked(keys ...string) (bool, time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()

	var longest time.Duration
	for _, k := range keys {
		e := t.entries[k]
		if e == nil {
			continue
		}
		if d := e.until.Sub(now); d > longest {
			longest = d
		}
	}
	if longest <= 0 {
		return false, 0
	}
	return true, longest
}

// Fail records a failed attempt against every key and returns the lockout it
// caused, or zero if the threshold has not been reached yet.
func (t *Throttle) Fail(keys ...string) time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	t.sweep(now)

	var longest time.Duration
	for _, k := range keys {
		e := t.entries[k]
		if e == nil {
			e = &attempt{}
			t.entries[k] = e
		}
		// A failure long after the previous one starts a fresh count rather
		// than compounding with something the operator has forgotten about.
		if !e.last.IsZero() && now.Sub(e.last) > t.Window {
			e.failures = 0
			e.backoff = 0
		}
		e.failures++
		e.last = now

		if e.failures >= t.Threshold {
			if e.backoff == 0 {
				e.backoff = t.Base
			} else if e.backoff < t.Max {
				e.backoff *= 2
			}
			if e.backoff > t.Max {
				e.backoff = t.Max
			}
			e.until = now.Add(e.backoff)
			if e.backoff > longest {
				longest = e.backoff
			}
		}
	}
	return longest
}

// Succeed clears the keys. A correct password is the proof that this was not an
// attack, so the count should not follow the operator around afterwards.
func (t *Throttle) Succeed(keys ...string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, k := range keys {
		delete(t.entries, k)
	}
}

// sweep drops entries nobody has touched for a while, so a long run of attempts
// against random usernames cannot grow the map without bound.
func (t *Throttle) sweep(now time.Time) {
	if len(t.entries) < 1024 {
		return
	}
	for k, e := range t.entries {
		if now.Sub(e.last) > t.Window && now.After(e.until) {
			delete(t.entries, k)
		}
	}
}
