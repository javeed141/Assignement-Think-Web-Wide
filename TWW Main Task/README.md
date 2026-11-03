# campaign-events service

A Go HTTP service that receives marketing events from providers and serves campaign statistics.

## Requirements

- Go 1.22 or later

## Run

```bash
cd starter
go run .
```

Server starts on port **8080**.

## Endpoints

### POST /events
Send a batch of events:
```bash
curl -s -X POST localhost:8080/events \
  -H 'Content-Type: application/json' \
  -d '[{"event_id":"evt_1","campaign_id":"cmp_summer","contact_id":"ct_001","type":"sent","timestamp":"2026-08-10T06:00:00Z"}]'
```

### GET /campaigns/{campaign_id}/stats
Get stats for a campaign:
```bash
curl -s localhost:8080/campaigns/cmp_summer/stats
```

### Load seed data
```bash
curl -s -X POST localhost:8080/events \
  -H 'Content-Type: application/json' \
  --data-binary @starter/seed/events.json
```

