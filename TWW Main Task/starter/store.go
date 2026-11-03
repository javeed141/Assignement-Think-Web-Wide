package main

import "sync"

// store is our in-memory database.
// In JS this would be like:
//
//	const store = {
//	  seenEventIDs: new Set(),
//	  campaigns: {}
//	}
type store struct {
	mu           sync.Mutex
	seenEventIDs map[string]bool        // tracks duplicates
	campaigns    map[string]*campaignData
}

// campaignData holds all stats for one campaign.
// In JS:
//
//	{ sent: 0, delivered: 0, opened: 0, clicked: 0, openers: new Set() }
type campaignData struct {
	Sent      int
	Delivered int
	Opened    int // unique contacts who opened
	Clicked   int
	openers   map[string]bool // contact IDs who already opened — for unique count
}

// global store — one instance for the whole app
var db = &store{
	seenEventIDs: make(map[string]bool),
	campaigns:    make(map[string]*campaignData),
}

// isDuplicate returns true if this event_id was already seen.
// If not seen before, it marks it as seen and returns false.
func (s *store) isDuplicate(eventID string) bool {
	// We lock here because multiple requests can come in at the same time.
	// In JS with Node.js you don't need this because JS is single-threaded.
	// Go can handle multiple requests at the same time (goroutines), so we lock.
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.seenEventIDs[eventID] {
		return true
	}
	s.seenEventIDs[eventID] = true
	return false
}

// addEvent saves the event into the campaign stats.
func (s *store) addEvent(ev Event) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Create campaign entry if it doesn't exist yet.
	// Like JS: if (!store.campaigns[id]) store.campaigns[id] = { sent:0, ... }
	cd, ok := s.campaigns[ev.CampaignID]
	if !ok {
		cd = &campaignData{openers: make(map[string]bool)}
		s.campaigns[ev.CampaignID] = cd
	}

	switch ev.Type {
	case "sent":
		cd.Sent++
	case "delivered":
		cd.Delivered++
	case "opened":
		// Only count if this contact hasn't opened before in this campaign.
		// unique opens — same as: if (!openers.has(contactID)) { opened++; openers.add(contactID) }
		if !cd.openers[ev.ContactID] {
			cd.openers[ev.ContactID] = true
			cd.Opened++
		}
	case "clicked":
		cd.Clicked++
	}
}

// getStats returns a copy of the stats for one campaign.
// Returns nil if campaign not found.
func (s *store) getStats(campaignID string) *campaignData {
	s.mu.Lock()
	defer s.mu.Unlock()

	cd, ok := s.campaigns[campaignID]
	if !ok {
		return nil
	}
	// return a copy so the caller can't accidentally modify our data
	copy := &campaignData{
		Sent:      cd.Sent,
		Delivered: cd.Delivered,
		Opened:    cd.Opened,
		Clicked:   cd.Clicked,
	}
	return copy
}

