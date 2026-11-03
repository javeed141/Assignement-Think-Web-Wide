# NOTES.md

---

## Part 1 — My Understanding of the Problem

### 1. What I think the problem is (in my own words)

We are building a backend service for a company called Relay.

Relay sends marketing messages (like emails, SMS, push notifications) for their clients. But Relay itself doesn't deliver — it gives the job to third-party providers (like SendGrid, Twilio, etc.).

When something happens to a message (sent, delivered, opened, clicked), the provider calls Relay's webhook with an **event**. We need to:

1. **Receive those events** via a `POST /events` endpoint.
2. **Store them** somewhere (SQLite or in-memory).
3. **Serve stats** via `GET /campaigns/{id}/stats` so the dashboard can show counts.

The two main challenges are:
- **Duplicate events** — providers retry. Same event can arrive more than once. We must not count it twice.
- **Out-of-order events** — an `opened` might arrive before `delivered`. We just store them as-is and count correctly.

---

### 2. Assumptions I am making

- `event_id` is unique per event. If the same `event_id` arrives again, it is a duplicate — skip it.
- `campaign_id` and `contact_id` are always strings. No need to validate their format.
- Events with missing required fields (`event_id`, `campaign_id`, `type`) should be rejected with a clear error.
- `metadata` is optional — if missing, that's fine, don't reject the event.
- "How many opened" means: **distinct contacts who opened** — not total open events. A contact opening 5 times still counts as 1. This is the industry standard (Mailchimp, HubSpot, SendGrid all work this way). Marketers care about reach, not raw event counts.
- We are NOT asked to build a login/auth system. The endpoints are open.
- In-memory storage is fine to start with. No need for a real database right now.
- The service runs on a single machine for now (no distributed system).
- A batch `POST /events` should process each event individually — one bad event should not block the others.
- Stats are calculated **real-time** for now (count from database on every request). If traffic grows large, we switch to **pre-aggregated counters** (store running totals, update on every event, dashboard just reads those numbers directly).

---

### 3. Ambiguities I noticed in the brief

- **"How many opened" — the brief doesn't define this precisely.**
  - Decision: `opened` = number of **distinct contacts** who opened. Industry standard. A contact opening 5 times = 1 open.

- **What should the API return for a duplicate event?**
  - Should it return 200 OK (silently ignored) or 409 Conflict?
  - I'll return 200 with a note that it was a duplicate — it's not an error, it's expected behavior.

- **What about a batch with mix of good and bad events?**
  - If one event in the batch is malformed, should we reject the whole batch or just that one?
  - I'll process good ones and report which ones failed — safer for the provider.

- **What counts as an "invalid" event?**
  - Missing `event_id`? Unknown `type`? Future timestamp?
  - The brief says "what to reject and how is one of your decisions." I'll reject events missing required fields or with an unrecognised type.

- **Does `timestamp` matter for stats, or is it just for display?**
  - The brief mentions it, but stats only need counts. I'll store it but not use it for counting.

---

### 4. Questions I would ask the PM

1. **Is there a response time expectation for the dashboard stats endpoint?**
   - e.g., must respond in under 200ms? This affects whether I need an index or cache from the start.

---

### 5. What I will do, and in what order

1. **Fill this NOTES.md** — think before I type code (done).
2. **Fix the 4 bugs in `debugging/main.go`** — short, concrete task, builds confidence.
3. **Write `BUGS.md`** — document each bug clearly.
4. **Build the service** — start from `starter/`, implement `POST /events` with deduplication first, then `GET /campaigns/{id}/stats`.
5. **Write basic tests** — just for the deduplication logic and stats counting (the parts I'd be most embarrassed to break).
6. **Add the scale memo** (Part 4) to this file.
7. **Add the angry marketer section** (Part 5) to this file.
8. **Create `AI_USAGE.md`** last.

---


---

## Part 4 — Scale Memo (100 million events/day)

### What breaks first

The in-memory store breaks first. Everything is in RAM — if the process restarts, all data is gone. At 100M events/day, RAM alone cannot hold the data, and a single machine cannot handle the write volume.

The deduplication `seen` map breaks next. It grows forever — one entry per unique `event_id` in memory. At 100M events/day that's gigabytes of RAM just for the ID set.

Single-process Go with a mutex means every `POST /events` request waits for the lock. At high concurrency this becomes a bottleneck.

### What I would change, in order

1. **Add persistent storage** — SQLite first (already handles thousands of writes/second), then PostgreSQL when SQLite hits its limit.
2. **Deduplication at DB level** — put a UNIQUE index on `event_id`. Let the database enforce it with an `INSERT OR IGNORE`. No need for an in-memory set.
3. **Introduce a queue (e.g. Kafka or Redis Streams)** — providers POST events → queue → workers consume and write to DB. This decouples ingestion from storage and handles traffic spikes.
4. **Pre-aggregate counters** — instead of counting on every stats request, maintain `sent_count`, `delivered_count` etc. per campaign in a separate table, updated on every insert.
5. **Read replicas** — stats reads go to a replica; writes go to primary.

### Would the API contract change?

`POST /events` and `GET /campaigns/{id}/stats` stay the same from the outside. Internally, stats would be read from a pre-aggregated table instead of a full scan.

### New problems a queue introduces

- Events can be processed out of order (already a known issue, but now more unpredictable).
- Duplicates can now sneak in at the queue level — a provider retries, the first event is in the queue but not yet committed to DB, so the DB unique check doesn't catch it yet. Need idempotency at the consumer level too.
- The queue itself can fail — need dead-letter queues and retry policies.

### How I would keep counters trustworthy

- Use DB transactions: increment counter and insert event in one atomic operation.
- Periodic reconciliation job: recount from raw events table and compare against pre-aggregated counters. Alert on mismatch.

### What I would monitor

- Queue consumer lag (events piling up faster than we process them)
- Duplicate rate per provider (which providers retry the most)
- Counter reconciliation mismatches
- P99 latency on `POST /events` and `GET /stats`

### What I would deliberately NOT solve yet

- Multi-region distribution
- Fine-grained per-hour or per-minute bucketing (per-day is enough for now)
- Authentication (not asked for)

---

## Part 5 — The Angry Marketer

**At 10:00:** Sent=1,000,000 | Delivered=970,000 | Opened=250,000 | Clicked=20,000
**At 10:30:** Sent=1,000,000 | Delivered=975,000 | Opened=248,000 | Clicked=20,000

Delivered went **up**. Opened went **down**. The marketer says: *"Your dashboard is broken."*

### Every plausible explanation before assuming a bug

1. **Late-arriving events were deduplicated.** Between 10:00 and 10:30, some `opened` events arrived that had the same `event_id` as events already counted — so they were correctly rejected as duplicates. The count went down because earlier we had accidentally double-counted some opens that we later caught.

2. **A provider sent a correction / negative event.** Some providers send an event to indicate a previous open was a bot or a tracking pixel auto-load (e.g. Apple Mail Privacy Protection pre-fetches emails). A batch arrived marking some opens as invalid, and they were removed from the count.

3. **Clock skew on the provider side.** Events that appeared before 10:00 with future timestamps were held back and only arrived at 10:30 — but some of those were duplicates of already-counted events, causing a net decrease when deduplication ran.

4. **Out-of-order delivery corrected itself.** An `opened` event arrived before its corresponding `delivered`. When `delivered` arrived later, the total delivered went up. The opened count went down because some of those opens were for contacts that don't have a corresponding `delivered` — and maybe we decided to invalidate them (though we don't in this implementation).

5. **This is expected behaviour, not a bug.** If our deduplication is working correctly and some duplicate opens arrived earlier and were later detected, the count correctly decreased. Delivered going up and opened going down simultaneously is entirely plausible and correct.

### Is delivered going up suspicious?

No. Providers retry delivery webhooks. Events from earlier in the campaign are still arriving. This is normal and expected.

### What I would check first

1. Look at the raw events table — did any `opened` events arrive between 10:00 and 10:30 with `event_id`s that already existed?
2. Check provider logs for bot-detection or invalid-open notifications.
3. Check if deduplication is working correctly by comparing `event_id` uniqueness in the DB.

### Conclusion

I would not assume it's a bug. I would first check the raw data. If duplicates were correctly rejected and the numbers reflect that, this is the system working as intended.

---

## What I completed / what I intentionally skipped

**Completed:**
- Part 1: NOTES.md — problem understanding, assumptions, ambiguities, PM questions, plan
- Part 2: Service — `POST /events` with per-event validation and deduplication, `GET /campaigns/{id}/stats` with unique opens
- Part 3: All 4 bugs found and fixed in `debugging/main.go`, documented in `BUGS.md`
- Part 4: Scale memo
- Part 5: Angry marketer analysis

**Intentionally skipped:**
- Unit tests — ran out of time. I would have tested: (1) deduplication rejects second event with same ID, (2) unique opens counts correctly across contacts and campaigns, (3) invalid events are rejected without affecting others in the batch.
- Optional `GET /campaigns/{id}/events` paginated endpoint — not required, skipped to prioritise correctness over scope.
- Frontend — explicitly stated as not required.

## If I had another day

- Add SQLite persistence so data survives restarts.
- Write the tests I described above.
- Add the paginated events endpoint.
- Test the service with the full `seed/events.json` file and verify all edge cases.
- Add structured logging so every rejected/duplicate event is traceable.
