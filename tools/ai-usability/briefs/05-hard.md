# Brief 05 — Hard: realtime board with auth + DB

You are building a web API with Noda combining authentication, a database, and a
realtime channel.

**What I want:** A small "board" app. Authenticated users (JWT / OIDC) can post
messages via `POST /messages`, which are stored in PostgreSQL. Connected clients
receive new messages in realtime over a WebSocket so the board updates live.
Only authenticated users may post or subscribe.

**Done looks like:** The project validates cleanly; posting a message persists it
and pushes it to subscribed WebSocket clients, and all of it is behind auth.
