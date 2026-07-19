# Noda — Use Case: Video Conferencing with LiveKit

**Version**: 0.4.0

A video conferencing backend with room management, token generation, recording, and webhook-driven event handling. Noda acts as the **control plane** — clients connect to LiveKit directly for media transport (WebRTC). No application code required.

---

## What We're Building

A backend for a video conferencing app (think Google Meet-like). The runnable example ([`examples/video-rooms/`](../../examples/video-rooms)) covers:

- **Room management** — create and list rooms via REST API
- **Token generation** — clients request a token to join a room
- **Participant listing** — see who is in a room

The second half of this page adds **recipes** (not in the example project) for recording to S3, LiveKit webhook handling, and RTMP ingress.

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
| `lk` | `livekit` | LiveKit room management, tokens |

The example intentionally has no database — room state lives in LiveKit itself.

---

## Config Structure

The runnable project is [`examples/video-rooms/`](../../examples/video-rooms):

```
noda.json                         — LiveKit service, CORS, route group
.env                              — LIVEKIT_URL, LIVEKIT_API_KEY, LIVEKIT_API_SECRET
routes/
  create-room.json                — POST /api/rooms
  join-room.json                  — POST /api/rooms/:room/join (mints the client token)
  list-rooms.json                 — GET /api/rooms
  list-participants.json          — GET /api/rooms/:room/participants
workflows/
  create-room.json
  join-room.json
  list-rooms.json
  list-participants.json
```

---

## Environment Variables

Create a `.env` file:

```
LIVEKIT_URL=wss://myapp.livekit.cloud
LIVEKIT_API_KEY=APIxxxxxxxx
LIVEKIT_API_SECRET=xxxxxxxxxxxxxxxxxxxxx
```

(The recipes further down additionally assume `DATABASE_URL` and `JWT_SECRET` once you add a database and auth.)

---

## Root Config

`noda.json`:

```json
{
  "server": {
    "port": 8080
  },
  "services": {
    "lk": {
      "plugin": "livekit",
      "config": {
        "url": "{{ $env('LIVEKIT_URL') }}",
        "api_key": "{{ $env('LIVEKIT_API_KEY') }}",
        "api_secret": "{{ $env('LIVEKIT_API_SECRET') }}"
      }
    }
  },
  "security": {
    "cors": {
      "allow_origins": "*",
      "allow_methods": "GET, POST, OPTIONS, HEAD, PUT, DELETE"
    }
  },
  "middleware_presets": {
    "public": ["security.cors"]
  },
  "route_groups": {
    "/api": {
      "middleware_preset": "public"
    }
  }
}
```

The LiveKit service creates SDK clients for room management, egress, and ingress.

> **The example's `/api` routes are unauthenticated** (CORS-only, for easy local testing). Before deploying anything like this, add JWT auth: a `security.jwt` block, an `"authenticated": ["auth.jwt"]` preset on `/api`, and take the participant identity from `{{ auth.sub }}` instead of the request body.

---

## Key Workflows

### Join Room (token generation)

The most common operation. A client asks to join a room and receives a LiveKit access token.

**Trigger:** `POST /api/rooms/:room/join` → workflow `join-room`

**Route input:** `{ "name": "{{ body.name }}", "room": "{{ params.room }}" }`

**Workflow** (`workflows/join-room.json`):

```json
{
  "id": "join-room",
  "nodes": {
    "gen_identity": {
      "type": "transform.set",
      "config": {
        "fields": { "identity": "{{ $uuid() }}" }
      }
    },
    "create_token": {
      "type": "lk.token",
      "services": { "livekit": "lk" },
      "config": {
        "identity": "{{ nodes.gen_identity.identity }}",
        "name": "{{ input.name }}",
        "room": "{{ input.room }}",
        "grants": {
          "roomJoin": true,
          "canPublish": true,
          "canSubscribe": true,
          "canPublishData": true,
          "canUpdateOwnMetadata": true
        }
      }
    },
    "respond": {
      "type": "response.json",
      "config": {
        "status": 200,
        "body": {
          "identity": "{{ nodes.gen_identity.identity }}",
          "name": "{{ input.name }}",
          "room": "{{ input.room }}",
          "token": "{{ nodes.create_token.token }}",
          "url": "{{ secrets.LIVEKIT_URL }}"
        }
      }
    }
  },
  "edges": [
    { "from": "gen_identity", "to": "create_token" },
    { "from": "create_token", "to": "respond" }
  ]
}
```

The client receives the token and the LiveKit URL, then connects directly to LiveKit using their client SDK (e.g., `livekit-client` for JavaScript). Note the LiveKit URL is read via `secrets.LIVEKIT_URL` — `$env()` doesn't resolve inside workflow configs.

### Create Room

**Trigger:** `POST /api/rooms` → workflow `create-room`

```json
{
  "id": "create-room",
  "nodes": {
    "create": {
      "type": "lk.room_create",
      "services": { "livekit": "lk" },
      "config": {
        "name": "{{ input.name }}",
        "empty_timeout": 600
      }
    },
    "respond": {
      "type": "response.json",
      "config": {
        "status": 201,
        "body": { "room": "{{ nodes.create }}" }
      }
    }
  },
  "edges": [
    { "from": "create", "to": "respond" }
  ]
}
```

`list-rooms` (`lk.room_list` → `response.json`) and `list-participants` (`lk.participant_list` → `response.json`) follow the same two-node shape.

---

## Beyond the Example: Recipes

> **Everything from here to "Architecture Features Validated" is a recipe, not part of `examples/video-rooms`** — the example has no database, recording, webhook receiver, or ingress. The node types (`lk.egress*`, `lk.ingress*`, the `livekit.webhook` middleware) are real; the workflows below sketch how you'd wire them.

### Start Recording

**Trigger:** `POST /api/rooms/:name/recording` → workflow `start-recording`

```json
{
  "id": "start-recording",
  "nodes": {
    "record": {
      "type": "lk.egress_start_room_composite",
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
      }
    },
    "save_egress": {
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
      }
    },
    "respond": {
      "type": "response.json",
      "config": {
        "status": 200,
        "body": {
          "egress_id": "{{ nodes.record.egress_id }}",
          "status": "{{ nodes.record.status }}"
        }
      }
    }
  },
  "edges": [
    { "from": "record", "to": "save_egress" },
    { "from": "save_egress", "to": "respond" }
  ]
}
```

The `egress_id` is saved to the database. Use it later to stop the recording with `lk.egress_stop`.

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
  "nodes": {
    "route": {
      "type": "control.switch",
      "config": {
        "expression": "{{ input.event }}",
        "cases": ["participant_joined", "participant_left", "egress_ended"]
      }
    },
    "log_join": {
      "type": "util.log",
      "config": {
        "level": "info",
        "message": "Participant joined",
        "fields": {
          "room": "{{ input.room.name }}",
          "identity": "{{ input.participant.identity }}"
        }
      }
    },
    "log_leave": {
      "type": "util.log",
      "config": {
        "level": "info",
        "message": "Participant left",
        "fields": {
          "room": "{{ input.room.name }}",
          "identity": "{{ input.participant.identity }}"
        }
      }
    },
    "update_recording": {
      "type": "db.update",
      "services": { "database": "main-db" },
      "config": {
        "table": "recordings",
        "where": { "egress_id": "{{ input.egress_info.egressId }}" },
        "data": { "status": "completed", "ended_at": "{{ now() }}" }
      }
    },
    "log_other": {
      "type": "util.log",
      "config": {
        "level": "debug",
        "message": "LiveKit event",
        "fields": { "event": "{{ input.event }}" }
      }
    },
    "respond_ok": {
      "type": "response.json",
      "config": {
        "status": 200,
        "body": { "received": true }
      }
    }
  },
  "edges": [
    { "from": "route", "output": "participant_joined", "to": "log_join" },
    { "from": "route", "output": "participant_left", "to": "log_leave" },
    { "from": "route", "output": "egress_ended", "to": "update_recording" },
    { "from": "route", "output": "default", "to": "log_other" },
    { "from": "log_join", "to": "respond_ok" },
    { "from": "log_leave", "to": "respond_ok" },
    { "from": "update_recording", "to": "respond_ok" },
    { "from": "log_other", "to": "respond_ok" }
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
  "type": "lk.ingress_create",
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

Validated by the `examples/video-rooms` project itself:

| Feature | How it's used |
|---|---|
| LiveKit service | Room management and token generation (`lk.room_create`, `lk.room_list`, `lk.participant_list`, `lk.token`) |
| Expression engine | `$uuid()` identities, dynamic token grants |
| Secrets | `LIVEKIT_URL` read via `secrets.*` in a workflow; credentials via `$env()` in `noda.json` |
| Middleware presets + route groups | CORS applied to all `/api` routes via the `public` preset |

Exercised only by the recipes above (not in the example): the `livekit.webhook` middleware, `lk.egress*`/`lk.ingress*` nodes, database persistence, and JWT-protected token flows.

---

## What's NOT Needed

No WebSockets in Noda (LiveKit handles media transport), no Redis, no workers, no Wasm, no storage plugin. Noda provides the control plane API; LiveKit provides the data plane.
