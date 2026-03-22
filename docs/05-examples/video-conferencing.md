# Noda — Use Case: Video Conferencing with LiveKit

**Version**: 0.4.0

A video conferencing backend with room management, token generation, recording, and webhook-driven event handling. Noda acts as the **control plane** — clients connect to LiveKit directly for media transport (WebRTC). No application code required.

---

## What We're Building

A backend for a video conferencing app (think Google Meet-like):

- **Room management** — create, list, and delete rooms via REST API
- **Token generation** — authenticated users request a token to join a room
- **Participant control** — mute, remove, and update participants
- **Recording** — start/stop room recording to S3
- **Webhook handling** — react to LiveKit events (participant joined, room ended, etc.)
- **Ingress** — allow RTMP streaming into a room

---

## Prerequisites

A running LiveKit server. Options:

1. **LiveKit Cloud** — [cloud.livekit.io](https://cloud.livekit.io) (managed, free tier available)
2. **Self-hosted** — `docker run --rm -p 7880:7880 -p 7881:7881 -p 7882:7882/udp livekit/livekit-server --dev`

You need three values from LiveKit:
- `LIVEKIT_URL` — e.g., `wss://myapp.livekit.cloud` or `ws://localhost:7880`
- `LIVEKIT_API_KEY` — e.g., `APIxxxxxxxx`
- `LIVEKIT_API_SECRET` — e.g., `xxxxxxxxxxxxxxxxxxxxx`

---

## Services Required

| Instance | Plugin | Purpose |
|---|---|---|
| `lk` | `lk` | LiveKit room management, tokens, recording |
| `main-db` | `db` | User accounts, room metadata, recording history |

---

## Config Structure

```
noda.json                         — services, JWT, LiveKit credentials
.env                              — LIVEKIT_URL, LIVEKIT_API_KEY, LIVEKIT_API_SECRET
routes/
  rooms.json                      — room CRUD endpoints
  tokens.json                     — token generation endpoint
  participants.json               — participant management endpoints
  recording.json                  — recording start/stop endpoints
  livekit-webhook.json            — webhook receiver
workflows/
  create-room.json
  list-rooms.json
  delete-room.json
  generate-token.json
  list-participants.json
  remove-participant.json
  mute-participant.json
  start-recording.json
  stop-recording.json
  on-livekit-event.json
```

---

## Environment Variables

Create a `.env` file:

```
LIVEKIT_URL=wss://myapp.livekit.cloud
LIVEKIT_API_KEY=APIxxxxxxxx
LIVEKIT_API_SECRET=xxxxxxxxxxxxxxxxxxxxx
DATABASE_URL=postgres://noda:noda@localhost:5432/noda?sslmode=disable
JWT_SECRET=your-jwt-secret-at-least-32-bytes-long
```

---

## Root Config

`noda.json`:

```json
{
  "server": {
    "port": 3000
  },
  "services": {
    "lk": {
      "plugin": "lk",
      "config": {
        "url": "{{ $env('LIVEKIT_URL') }}",
        "api_key": "{{ $env('LIVEKIT_API_KEY') }}",
        "api_secret": "{{ $env('LIVEKIT_API_SECRET') }}"
      }
    },
    "main-db": {
      "plugin": "db",
      "config": {
        "url": "{{ $env('DATABASE_URL') }}"
      }
    }
  },
  "security": {
    "jwt": {
      "secret": "{{ $env('JWT_SECRET') }}"
    }
  },
  "middleware_presets": {
    "authenticated": ["auth.jwt"],
    "webhook": ["livekit.webhook"]
  },
  "route_groups": {
    "/api": {
      "middleware_preset": "authenticated"
    }
  }
}
```

The LiveKit service creates SDK clients for room management, egress, and ingress. The `livekit.webhook` middleware automatically picks up credentials from the `lk` service config — no separate configuration needed.

---

## Key Workflows

### Generate Token

The most common operation. A logged-in user requests a token to join a specific room.

**Trigger:** `POST /api/tokens` → workflow `generate-token`

**Route:**

```json
{
  "path": "/api/tokens",
  "method": "POST",
  "middleware": ["auth.jwt"],
  "trigger": {
    "workflow": "generate-token",
    "input": {
      "room_name": "{{ body.room_name }}",
      "user_id": "{{ auth.user_id }}",
      "user_name": "{{ auth.claims.name }}"
    }
  }
}
```

**Workflow:**

```json
{
  "id": "generate-token",
  "trigger": "http",
  "nodes": [
    {
      "id": "token",
      "type": "lk.token",
      "services": { "livekit": "lk" },
      "config": {
        "identity": "{{ input.user_id }}",
        "room": "{{ input.room_name }}",
        "name": "{{ input.user_name }}",
        "ttl": "2h",
        "grants": {
          "canPublish": true,
          "canSubscribe": true,
          "canPublishData": true
        }
      },
      "outputs": {
        "success": "respond"
      }
    },
    {
      "id": "respond",
      "type": "response.json",
      "config": {
        "status": 200,
        "body": {
          "token": "{{ nodes.token.token }}",
          "url": "{{ $env('LIVEKIT_URL') }}"
        }
      }
    }
  ]
}
```

The client receives a JWT token and the LiveKit URL, then connects directly to LiveKit using their client SDK (e.g., `livekit-client` for JavaScript).

### Create Room

**Trigger:** `POST /api/rooms` → workflow `create-room`

```json
{
  "id": "create-room",
  "trigger": "http",
  "nodes": [
    {
      "id": "create",
      "type": "lk.roomCreate",
      "services": { "livekit": "lk" },
      "config": {
        "name": "{{ input.room_name }}",
        "empty_timeout": 300,
        "max_participants": "{{ input.max_participants ?? 20 }}",
        "metadata": "{{ toJSON({ 'created_by': input.user_id }) }}"
      },
      "outputs": {
        "success": "save-to-db"
      }
    },
    {
      "id": "save-to-db",
      "type": "db.create",
      "services": { "database": "main-db" },
      "config": {
        "table": "rooms",
        "data": {
          "livekit_sid": "{{ nodes.create.sid }}",
          "name": "{{ nodes.create.name }}",
          "created_by": "{{ input.user_id }}"
        }
      },
      "outputs": {
        "success": "respond"
      }
    },
    {
      "id": "respond",
      "type": "response.json",
      "config": {
        "status": 201,
        "body": "{{ nodes.create }}"
      }
    }
  ]
}
```

The room is created on LiveKit first, then recorded in the local database for tracking.

### Start Recording

**Trigger:** `POST /api/rooms/:name/recording` → workflow `start-recording`

```json
{
  "id": "start-recording",
  "trigger": "http",
  "nodes": [
    {
      "id": "record",
      "type": "lk.egressStartRoomComposite",
      "services": { "livekit": "lk" },
      "config": {
        "room": "{{ input.room_name }}",
        "layout": "speaker-dark",
        "output": {
          "type": "s3",
          "bucket": "my-recordings",
          "region": "us-east-1",
          "filepath": "recordings/{{ input.room_name }}/{{ $timestamp() }}.mp4"
        }
      },
      "outputs": {
        "success": "save-egress"
      }
    },
    {
      "id": "save-egress",
      "type": "db.create",
      "services": { "database": "main-db" },
      "config": {
        "table": "recordings",
        "data": {
          "egress_id": "{{ nodes.record.egress_id }}",
          "room_name": "{{ input.room_name }}",
          "started_by": "{{ input.user_id }}",
          "status": "recording"
        }
      },
      "outputs": {
        "success": "respond"
      }
    },
    {
      "id": "respond",
      "type": "response.json",
      "config": {
        "status": 200,
        "body": {
          "egress_id": "{{ nodes.record.egress_id }}",
          "status": "{{ nodes.record.status }}"
        }
      }
    }
  ]
}
```

The `egress_id` is saved to the database. Use it later to stop the recording with `lk.egressStop`.

### Handle LiveKit Webhooks

LiveKit sends events when rooms start, participants join/leave, recordings finish, etc. The webhook middleware verifies the signature, and the workflow routes events using `control.switch`.

**Route:**

```json
{
  "path": "/webhooks/livekit",
  "method": "POST",
  "middleware": ["livekit.webhook"],
  "trigger": {
    "raw_body": true,
    "workflow": "on-livekit-event",
    "input": {
      "event": "{{ body.event }}",
      "room": "{{ body.room }}",
      "participant": "{{ body.participant }}",
      "egress_info": "{{ body.egressInfo }}",
      "ingress_info": "{{ body.ingressInfo }}"
    }
  }
}
```

**Workflow:**

```json
{
  "id": "on-livekit-event",
  "trigger": "http",
  "nodes": [
    {
      "id": "route",
      "type": "control.switch",
      "config": {
        "value": "{{ input.event }}"
      },
      "outputs": {
        "participant_joined": "log-join",
        "participant_left": "log-leave",
        "egress_ended": "update-recording",
        "default": "log-other"
      }
    },
    {
      "id": "log-join",
      "type": "util.log",
      "config": {
        "level": "info",
        "message": "Participant joined",
        "fields": {
          "room": "{{ input.room.name }}",
          "identity": "{{ input.participant.identity }}"
        }
      },
      "outputs": { "success": "respond-ok" }
    },
    {
      "id": "log-leave",
      "type": "util.log",
      "config": {
        "level": "info",
        "message": "Participant left",
        "fields": {
          "room": "{{ input.room.name }}",
          "identity": "{{ input.participant.identity }}"
        }
      },
      "outputs": { "success": "respond-ok" }
    },
    {
      "id": "update-recording",
      "type": "db.update",
      "services": { "database": "main-db" },
      "config": {
        "table": "recordings",
        "where": {
          "egress_id": "{{ input.egress_info.egressId }}"
        },
        "data": {
          "status": "completed",
          "ended_at": "{{ now() }}"
        }
      },
      "outputs": { "success": "respond-ok" }
    },
    {
      "id": "log-other",
      "type": "util.log",
      "config": {
        "level": "debug",
        "message": "LiveKit event",
        "fields": { "event": "{{ input.event }}" }
      },
      "outputs": { "success": "respond-ok" }
    },
    {
      "id": "respond-ok",
      "type": "response.json",
      "config": {
        "status": 200,
        "body": { "received": true }
      }
    }
  ]
}
```

This handles participant join/leave logging and updates the recordings table when an egress finishes. Add more branches as needed (e.g., `room_started`, `track_published`).

---

## LiveKit Webhook Events

| Event | When it fires | Key fields |
|---|---|---|
| `room_started` | Room created (first participant joins) | `room` |
| `room_finished` | Room closed (last participant left + timeout) | `room` |
| `participant_joined` | Participant connected | `room`, `participant` |
| `participant_left` | Participant disconnected | `room`, `participant` |
| `track_published` | Participant published a track | `room`, `participant`, `track` |
| `track_unpublished` | Participant unpublished a track | `room`, `participant`, `track` |
| `egress_started` | Recording/egress started | `egressInfo` |
| `egress_ended` | Recording/egress completed | `egressInfo` |
| `ingress_started` | Ingress stream started | `ingressInfo` |
| `ingress_ended` | Ingress stream ended | `ingressInfo` |

Configure the webhook URL in LiveKit: `https://your-noda-server.com/webhooks/livekit`.

---

## Client Integration

Noda generates tokens — the client connects to LiveKit directly. Example using `livekit-client` (JavaScript):

```javascript
import { Room } from 'livekit-client';

// 1. Get token from your Noda API
const resp = await fetch('/api/tokens', {
  method: 'POST',
  headers: {
    'Authorization': `Bearer ${userJwt}`,
    'Content-Type': 'application/json'
  },
  body: JSON.stringify({ room_name: 'my-room' })
});
const { token, url } = await resp.json();

// 2. Connect to LiveKit directly
const room = new Room();
await room.connect(url, token);

// 3. Publish camera/microphone
await room.localParticipant.enableCameraAndMicrophone();
```

Noda never touches the media. It handles the control plane: who can join, with what permissions, when to record, and how to react to events.

---

## RTMP Ingress (Live Streaming)

Allow users to stream from OBS or similar tools into a LiveKit room:

```json
{
  "id": "create-ingress",
  "type": "lk.ingressCreate",
  "services": { "livekit": "lk" },
  "config": {
    "input_type": "rtmp",
    "room": "{{ input.room_name }}",
    "participant_identity": "{{ input.streamer_id }}",
    "participant_name": "{{ input.streamer_name }}"
  }
}
```

The response includes an RTMP URL and stream key. The streamer configures OBS with these values and starts streaming — the video appears as a participant in the LiveKit room.

---

## Restricted Tokens

Not all participants need the same permissions. Generate viewer-only tokens by restricting grants:

```json
{
  "type": "lk.token",
  "services": { "livekit": "lk" },
  "config": {
    "identity": "{{ input.user_id }}",
    "room": "{{ input.room_name }}",
    "grants": {
      "canPublish": false,
      "canSubscribe": true,
      "canPublishData": false
    }
  }
}
```

Or recorder tokens for server-side recording bots:

```json
{
  "grants": {
    "canPublish": false,
    "canSubscribe": true,
    "hidden": true,
    "recorder": true
  }
}
```

---

## Architecture Features Validated

| Feature | How it's used |
|---|---|
| LiveKit service | Room management, token generation, egress/ingress |
| Webhook middleware | Signature verification for LiveKit events |
| `control.switch` | Route different webhook event types |
| JWT authentication | Protect API endpoints, identity flows to token |
| Database persistence | Track rooms, recordings, and events |
| Expression engine | Dynamic token grants, room metadata, S3 file paths |
| Environment variables | All secrets in `.env`, referenced via `$env()` |
| Middleware presets | Reusable `authenticated` and `webhook` presets |
| Route groups | All `/api` routes require JWT |

---

## What's NOT Needed

No WebSockets in Noda (LiveKit handles media transport), no Redis, no workers, no Wasm, no storage plugin. Noda provides the control plane API; LiveKit provides the data plane.
