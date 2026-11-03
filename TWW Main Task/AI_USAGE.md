# AI_USAGE.md

## Tools used
- **Antigravity (Google DeepMind AI)** — used throughout the assignment for explanations, code writing, bug analysis, and written sections.

## What I used it for
- Understanding the assignment requirements and clarifying ambiguities
- Writing the initial Go service code (`main.go`, `store.go`)
- Identifying and explaining the 4 bugs in `debugging/main.go`
- Drafting Parts 4 and 5 of `NOTES.md` (scale memo, angry marketer)

## One suggestion from AI that I rejected or changed
AI initially suggested returning **both** `opened` (total) and `unique_opens` (distinct contacts) in the stats API response. I rejected this because in real email marketing platforms (Mailchimp, HubSpot, SendGrid), "opened" always means unique contacts — not total opens. Returning two similar metrics would confuse the dashboard. I kept only `opened` = unique contacts, which matches industry standard.

## One thing AI helped me understand
AI helped me understand **why Go needs a mutex but Node.js doesn't** — Go runs each HTTP request in its own goroutine (like a thread), so multiple requests touch the same data at the same time. Node.js is single-threaded, so requests are handled one at a time and there's no clash. This made the concurrency bug (Bug 4) click immediately.

## Anything AI generated that I had to debug
The initial explanation of "real-time vs pre-aggregated" was overcomplicated. AI described it in abstract terms before I pushed back and forced a simpler explanation using a concrete scoreboard analogy. The final understanding came from me asking the right follow-up questions, not from the first AI response.

