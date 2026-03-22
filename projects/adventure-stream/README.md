# Adventure Stream

Live streaming platform for outdoor adventures. Stream from a DJI Osmo Action 4 camera via RTMP to LiveKit, and let friends/family watch in real-time through a web viewer.

## Architecture

```
DJI Osmo Action 4 → DJI Mimo App → RTMP → LiveKit Ingress → LiveKit Room
                                                                    ↓
                                              Viewers (WebRTC via LiveKit JS SDK)
```

The API is built entirely with **Noda** — no application code, just JSON config files that define routes, workflows, auth, and services.

## Prerequisites

- [Noda](https://github.com/chimpanze/noda) CLI
- [LiveKit Cloud](https://livekit.io/) account (or self-hosted LiveKit server)
- Redis (for stream state caching)
- Node.js 20+ (for the frontend)
- DJI Osmo Action 4 + DJI Mimo app (or any RTMP-capable camera/app)

## Setup

### 1. Environment Variables

```bash
cp .env.example .env
```

Edit `.env` with your values:

| Variable | Description |
|---|---|
| `LIVEKIT_URL` | Your LiveKit server WebSocket URL (e.g. `wss://your-app.livekit.cloud`) |
| `LIVEKIT_API_KEY` | LiveKit API key |
| `LIVEKIT_API_SECRET` | LiveKit API secret |
| `JWT_SECRET` | Secret for admin JWT tokens (min 32 chars) |
| `ADMIN_PASSWORD` | Password for the admin panel |
| `REDIS_URL` | Redis connection URL |

### 2. Start the API

```bash
cd projects/adventure-stream
noda run
```

The API starts on `http://localhost:8080`.

### 3. Start the Frontend

```bash
cd frontend
npm install
npm run dev
```

Frontend runs on `http://localhost:3000` with API proxy to `:8080`.

## Usage

### Admin Flow

1. Go to `http://localhost:3000/admin`
2. Login with your `ADMIN_PASSWORD`
3. **Create Ingress** — generates an RTMP URL + Stream Key
4. Enter the RTMP URL and Stream Key in DJI Mimo (Settings → Live → RTMP)
5. **Start Stream** — creates the LiveKit room with a title/description
6. Start streaming from DJI Mimo
7. Share `http://localhost:3000` with friends/family

### Viewer Flow

1. Go to `http://localhost:3000`
2. When the stream is live, enter your name and click "Watch Stream"
3. The video appears via WebRTC (low latency)

### DJI Mimo RTMP Setup

1. Open DJI Mimo app
2. Connect to your DJI Osmo Action 4
3. Go to Live settings
4. Select RTMP as the platform
5. Enter the **RTMP URL** from the admin panel
6. Enter the **Stream Key** from the admin panel
7. Start the live stream

## API Endpoints

### Public

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/stream/status` | Get stream live/offline status |
| `GET` | `/api/stream/token?name=X` | Get viewer token for the stream |
| `POST` | `/api/auth/login` | Admin login (returns JWT) |

### Admin (JWT required)

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/admin/ingress` | Create RTMP ingress endpoint |
| `GET` | `/api/admin/ingress` | List ingress endpoints |
| `DELETE` | `/api/admin/ingress/:id` | Delete ingress endpoint |
| `POST` | `/api/admin/stream/start` | Start stream (title + description) |
| `POST` | `/api/admin/stream/stop` | Stop stream |
| `GET` | `/api/admin/stream/status` | Detailed stream status |
| `GET` | `/api/admin/participants` | List viewers |
| `DELETE` | `/api/admin/participants/:identity` | Kick a viewer |

## Enhancement Ideas

- **Live chat** — `lk.sendData` with topic `"chat"`, no extra infra
- **Live reactions** — Floating emoji via data messages
- **Stream schedule** — Store upcoming adventures in Redis cache
- **Multi-camera** — Multiple ingress endpoints with different identities
- **Guest audio** — Issue tokens with `canPublish: true` for audio
- **GPS map overlay** — Parse DJI RTMP metadata via Wasm module
