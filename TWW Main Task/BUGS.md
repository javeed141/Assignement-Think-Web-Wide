# BUGS.md — Debugging findings

---

## Bug 1 — Duplicate detection only works within a batch, not across batches

**What the bug is:**
The `seen` map was declared *inside* `processBatch()`, so it was created fresh for every batch of 200 events. If the same `event_id` appeared in two different batches (e.g. batch 1 and batch 3), it was counted twice.

**Why it happens:**
```go
// BEFORE (wrong) — new map every batch call
seen := make(map[string]bool)
```
`seen` lives and dies with each call to `processBatch`. The requirement says each `event_id` must be counted at most once *across the whole file*, not just within a batch.

**Fix:**
Moved `seen` to a global variable (like `stats` and `openedBy`) so it persists across all batch calls:
```go
// global
var seen = map[string]bool{}

// inside processBatch — just use the global `seen` directly, removed the local declaration
```

**How I verified:**
The `seen` map now spans the entire file. A duplicate `event_id` in batch 5 that first appeared in batch 1 is correctly skipped.

---

## Bug 2 — `unique_opens` not tracked per campaign — same contact shared across all campaigns

**What the bug is:**
`openedBy` used only `ev.ContactID` as the key. So if contact `ct_001` opened in `cmp_A` and also in `cmp_B`, the second open was silently ignored — `cmp_B` got zero unique opens for that contact.

**Why it happens:**
```go
// BEFORE (wrong) — key is just contact ID, shared globally
if !openedBy[ev.ContactID] {
```
The requirement says: *"The same contact opening in two campaigns counts once in each."* The key must include the campaign.

**Fix:**
```go
// AFTER — key includes campaign ID
key := ev.CampaignID + ":" + ev.ContactID
if !openedBy[key] {
    openedBy[key] = true
    cs.UniqueOpens++
}
```

**How I verified:**
A contact who opens in two campaigns now increments `UniqueOpens` in both campaigns separately.

---

## Bug 3 — Daily delivered buckets use local timezone instead of UTC

**What the bug is:**
Events near midnight were bucketed into the wrong date depending on the machine's local timezone.

**Why it happens:**
```go
// BEFORE (wrong)
day := ev.Timestamp.Local().Format("2006-01-02")
```
The requirement explicitly states: *"Daily delivered buckets use the event's UTC date."* `.Local()` converts to the system's timezone, which can shift the date.

**Fix:**
```go
// AFTER
day := ev.Timestamp.UTC().Format("2006-01-02")
```

**How I verified:**
The daily bucket dates now match the UTC date in the event's timestamp regardless of the machine's timezone.

---

## Bug 4 — Race condition: `apply()` increments counters from 8 goroutines without a lock

**What the bug is:**
`apply()` is called concurrently by 8 worker goroutines. It increments plain `int` fields (`cs.Sent++`, `cs.Delivered++`, etc.) on a shared struct without any synchronisation. This is a data race — two goroutines can read the same value, both increment it, and one write is lost.

**Why it happens:**
The comment in the code says *"counters are plain ints, so the increments themselves are cheap"* — but cheap doesn't mean safe. `cs.Sent++` is not atomic in Go. `go run -race . events.jsonl` reports this as a data race.

**Fix:**
Added a mutex `applyMu` that wraps the counter increment inside `apply()`:
```go
var applyMu sync.Mutex

func apply(ev Event) {
    cs := stats[ev.CampaignID]
    applyMu.Lock()
    defer applyMu.Unlock()
    switch ev.Type { ... }
}
```

**How I verified:**
`go run -race . events.jsonl` no longer reports any data race warnings, and output is consistent across multiple runs.

