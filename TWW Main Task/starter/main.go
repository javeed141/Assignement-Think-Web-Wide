package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /events", handlePostEvents)
	mux.HandleFunc("GET /campaigns/{campaignID}/stats", handleGetStats)

	addr := ":8080"
	fmt.Printf("listening on %s\n", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

// handlePostEvents receives a JSON array of events from providers.
//
// In JS this would be like:
//
//	app.post('/events', (req, res) => {
//	  const events = req.body  // array
//	  ...
//	})
func handlePostEvents(w http.ResponseWriter, r *http.Request) {
	// Decode each line as raw JSON first, so one bad event doesn't kill the whole batch.
	// This is like doing: events.forEach(e => { try { process(e) } catch(err) { errors.push(err) } })
	var rawItems []json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&rawItems); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "request body must be a JSON array",
		})
		return
	}

	type result struct {
		EventID string `json:"event_id,omitempty"`
		Status  string `json:"status"`
		Reason  string `json:"reason,omitempty"`
	}

	results := make([]result, 0, len(rawItems))

	for _, raw := range rawItems {
		var ev Event
		if err := json.Unmarshal(raw, &ev); err != nil {
			// This one event is malformed — skip it, continue with others
			results = append(results, result{Status: "rejected", Reason: "invalid JSON structure"})
			continue
		}

		// Validate required fields
		if ev.EventID == "" || ev.CampaignID == "" || ev.ContactID == "" || ev.Type == "" {
			results = append(results, result{
				EventID: ev.EventID,
				Status:  "rejected",
				Reason:  "missing required field (event_id, campaign_id, contact_id, or type)",
			})
			continue
		}

		// Validate event type — only 4 allowed types
		validTypes := map[string]bool{"sent": true, "delivered": true, "opened": true, "clicked": true}
		if !validTypes[ev.Type] {
			results = append(results, result{
				EventID: ev.EventID,
				Status:  "rejected",
				Reason:  fmt.Sprintf("unknown event type: %q", ev.Type),
			})
			continue
		}

		// Check for duplicate — same event_id already received before
		if db.isDuplicate(ev.EventID) {
			results = append(results, result{
				EventID: ev.EventID,
				Status:  "duplicate", // not an error, just ignored
			})
			continue
		}

		// All good — save to our store
		db.addEvent(ev)
		results = append(results, result{
			EventID: ev.EventID,
			Status:  "accepted",
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"processed": len(rawItems),
		"results":   results,
	})
}

// handleGetStats returns the campaign stats for a given campaign ID.
//
// In JS:
//
//	app.get('/campaigns/:campaignID/stats', (req, res) => {
//	  const { campaignID } = req.params
//	  ...
//	})
func handleGetStats(w http.ResponseWriter, r *http.Request) {
	campaignID := r.PathValue("campaignID")

	stats := db.getStats(campaignID)
	if stats == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": fmt.Sprintf("campaign %q not found", campaignID),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"campaign_id": campaignID,
		"sent":        stats.Sent,
		"delivered":   stats.Delivered,
		"opened":      stats.Opened, // unique contacts who opened
		"clicked":     stats.Clicked,
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("writeJSON: %v", err)
	}
}
