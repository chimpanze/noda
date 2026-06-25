# Brief 04 — Auth: protected profile route

You are building a web API with Noda that requires authentication.

**What I want:** A `GET /me` endpoint that requires a logged-in user (JWT / OIDC
bearer token) and returns the caller's own profile. Unauthenticated requests must
be rejected with 401. Add a second endpoint `GET /admin/stats` that only users
with an `admin` role may access; everyone else gets 403.

**Done looks like:** The project validates cleanly; `/me` requires a valid token,
and `/admin/stats` additionally enforces the admin role.
