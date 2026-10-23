# Albums and the Image Plaza

| Method | Path | Description |
|--------|------|-------------|
| GET | `/albums` | Albums of the current account (including image count, owner, and visibility) |
| POST | `/albums` | Create an album (optionally with `permission`: `private`/`public`) |
| GET | `/albums/{id}` | Album details (no login required for public albums) |
| PATCH | `/albums/{id}` | Change the album name/description/visibility (owner only) |
| DELETE | `/albums/{id}` | Delete an album (images are kept; they are only removed from the album) |
| GET | `/albums/{id}/images` | Images in an album (no login required for public albums) |
| GET | `/plaza` | Public image plaza (cross-user; returns images with `permission=public`, supports filtering by uploader with `user_id`) |
| GET | `/plaza/albums` | Public album list (supports `user_id` filtering) |
| GET | `/users/{id}` | Public user profile: username, public image count, and public albums |

An image's `permission` is either `private` (the default, visible only to the owner) or `public` (it may appear in the image plaza, and public listings include `owner_username`). An album's `permission` is likewise `private` (the default) or `public`: the details and images of a public album are open to unauthenticated visitors and appear in `/plaza/albums`. The plaza, public albums, and user profiles are read-only endpoints that can be accessed without signing in.
