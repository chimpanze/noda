# Brief 02 — Basic: validated greeting

You are building a small web API with Noda.

**What I want:** A `POST /greet` endpoint that accepts a JSON body
`{ "name": "Ada" }`. Reject the request with a clear validation error if `name`
is missing or empty. On success, return `{ "message": "Hello, Ada!" }`, where the
name is taken from the request and the greeting is assembled in a workflow.

**Done looks like:** The project validates cleanly; a request with a name returns
the greeting, and a request without a name is rejected before the workflow runs.
