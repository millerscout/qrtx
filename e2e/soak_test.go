//go:build e2e && !windows

package e2e

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// soakSample is one `sync status --json` reading during the soak.
type soakSample struct {
	At       time.Time
	Sessions map[string]time.Time // id -> lease_until
	Claims   map[string]string    // key -> session
}

const soakSlack = time.Second

// checkInvariants returns every broken sync invariant: event seqs strictly
// increase, and no claim is held by a session whose lease ended more than
// soakSlack before the sample.
func checkInvariants(events []event, samples []soakSample) []string {
	var bad []string
	var prev int64
	for _, e := range events {
		if e.Seq <= prev {
			bad = append(bad, fmt.Sprintf("event seq %d after %d", e.Seq, prev))
		}
		prev = e.Seq
	}
	for _, s := range samples {
		for key, sess := range s.Claims {
			until, ok := s.Sessions[sess]
			if !ok || s.At.Sub(until) > soakSlack {
				bad = append(bad, fmt.Sprintf("%s: claim %s held by %s past its lease (%v)", s.At.Format("15:04:05.000"), key, sess, until))
			}
		}
	}
	return bad
}

// daemonViolation reports a heartbeat, begin or claim not followed by a
// running daemon within soakSlack.
func daemonViolation(op string, at time.Time, seen bool) string {
	if seen {
		return ""
	}
	return fmt.Sprintf("%s at %s: no daemon within %s", op, at.Format("15:04:05.000"), soakSlack)
}

func TestSoakInvariants(t *testing.T) {
	now := time.Now()
	if bad := checkInvariants([]event{{Seq: 1}, {Seq: 2}, {Seq: 2}}, nil); len(bad) != 1 {
		t.Fatalf("duplicate seq not reported: %v", bad)
	}
	if bad := checkInvariants([]event{{Seq: 3}, {Seq: 1}}, nil); len(bad) != 1 {
		t.Fatalf("decreasing seq not reported: %v", bad)
	}
	late := soakSample{At: now, Sessions: map[string]time.Time{"A": now.Add(-3 * time.Second)}, Claims: map[string]string{"demo/1.1": "A"}}
	if bad := checkInvariants(nil, []soakSample{late}); len(bad) != 1 {
		t.Fatalf("claim past lease not reported: %v", bad)
	}
	orphan := soakSample{At: now, Sessions: map[string]time.Time{}, Claims: map[string]string{"demo/1.1": "gone"}}
	if bad := checkInvariants(nil, []soakSample{orphan}); len(bad) != 1 {
		t.Fatalf("claim of a missing session not reported: %v", bad)
	}
	ok := soakSample{At: now, Sessions: map[string]time.Time{"A": now.Add(time.Minute)}, Claims: map[string]string{"demo/1.1": "A"}}
	if bad := checkInvariants([]event{{Seq: 1}, {Seq: 2}}, []soakSample{ok}); len(bad) != 0 {
		t.Fatalf("healthy run reported: %v", bad)
	}
	if daemonViolation("claim", now, false) == "" || daemonViolation("claim", now, true) != "" {
		t.Fatal("daemonViolation")
	}
}

func (r *repo) sample() soakSample {
	var st struct {
		Sessions []struct {
			ID         string    `json:"id"`
			LeaseUntil time.Time `json:"lease_until"`
		} `json:"sessions"`
		Claims []struct {
			Session string `json:"session"`
			Key     string `json:"key"`
		} `json:"claims"`
	}
	at := time.Now()
	out := r.mustQ(r.main, []string{"QRTX_NO_DAEMON=1"}, "sync", "status", "--json")
	_ = json.Unmarshal([]byte(out), &st)
	s := soakSample{At: at, Sessions: map[string]time.Time{}, Claims: map[string]string{}}
	for _, x := range st.Sessions {
		s.Sessions[x.ID] = x.LeaseUntil
	}
	for _, c := range st.Claims {
		s.Claims[c.Key] = c.Session
	}
	return s
}

func toggleTask(t *testing.T, dir string) {
	t.Helper()
	p := filepath.Join(dir, "sdd", "changes", "demo", "change.sdd.md")
	raw := mustRead(t, p)
	var next string
	if strings.Contains(raw, "- [ ] 1.1 ") {
		next = strings.Replace(raw, "- [ ] 1.1 ", "- [x] 1.1 ", 1)
	} else {
		next = strings.Replace(raw, "- [x] 1.1 ", "- [ ] 1.1 ", 1)
	}
	if err := os.WriteFile(p, []byte(next), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestSoak runs 5 sessions doing random sync work for $QRTX_SOAK (for
// example 5m) and checks the invariants. Without QRTX_SOAK it skips.
func TestSoak(t *testing.T) {
	d, err := time.ParseDuration(os.Getenv("QRTX_SOAK"))
	if err != nil || d <= 0 {
		t.Skip("set QRTX_SOAK=<duration> to run the soak")
	}
	r := newRepo(t, "2s", "")
	seed := time.Now().UnixNano()
	t.Logf("soak seed %d for %s", seed, d)
	rng := rand.New(rand.NewSource(seed))
	var sessions []*session
	for i := 0; i < 5; i++ {
		dir := r.main
		if i%2 == 1 {
			dir = r.wt
		}
		s := r.session(fmt.Sprintf("S%d", i), dir)
		s.begin()
		sessions = append(sessions, s)
	}
	var samples []soakSample
	var bad []string
	ops := 0
	waitDaemon := func(op string) {
		at := time.Now()
		seen := false
		for time.Since(at) < soakSlack && !seen {
			seen = r.daemonRunning()
			if !seen {
				time.Sleep(50 * time.Millisecond)
			}
		}
		if v := daemonViolation(op, at, seen); v != "" {
			bad = append(bad, v)
		}
	}
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		s := sessions[rng.Intn(len(sessions))]
		key := fmt.Sprintf("demo/%d", rng.Intn(5))
		switch rng.Intn(4) {
		case 0:
			s.hook("UserPromptSubmit", "heartbeat", "--quiet")
			waitDaemon("heartbeat " + s.id)
		case 1:
			if res := s.agent("qrtx sync claim " + key); res.code == 0 {
				waitDaemon("claim " + s.id)
			}
		case 2:
			s.agent("qrtx sync claim --release " + key)
		case 3:
			if rng.Intn(2) == 0 {
				toggleTask(t, r.main)
			} else {
				toggleTask(t, r.wt)
			}
		}
		ops++
		samples = append(samples, r.sample())
		time.Sleep(time.Duration(50+rng.Intn(250)) * time.Millisecond)
	}
	bad = append(bad, checkInvariants(r.events(), samples)...)
	t.Logf("soak: %d operations, %d samples, %d events, %d violations", ops, len(samples), len(r.events()), len(bad))
	for _, b := range bad {
		t.Error(b)
	}
}

func (r *repo) daemonRunning() bool {
	pid := r.daemonPID()
	return pidAlive(pid)
}
