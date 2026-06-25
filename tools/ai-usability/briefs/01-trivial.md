# Brief 01 — Trivial: a greeting endpoint

You are building a small web API with Noda.

**What I want:** A single HTTP `GET /hello` endpoint that returns JSON like
`{ "message": "Hello, world!" }`. The message should be produced by a workflow,
not hard-coded in the route itself.

**Done looks like:** The project validates cleanly, and hitting `GET /hello`
would return the JSON above.
