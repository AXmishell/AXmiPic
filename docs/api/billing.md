# Plans, Coupons, Orders, and Tickets

| Method | Path | Description |
|--------|------|-------------|
| GET | `/plans` | Enabled plans (no login required) |
| GET | `/admin/plans` | All plans |
| POST/PUT/DELETE | `/admin/plans[/{id}]` | Create / update / delete a plan |
| GET | `/admin/coupons` | Coupon list |
| POST/PUT/DELETE | `/admin/coupons[/{id}]` | Create / update / delete a coupon |
| POST | `/coupons/validate` | Validate a coupon (`{"code":"…","amount_cents":1000}`) |
| GET/POST | `/orders` | My orders / place an order (administrators see all orders) |
| POST | `/orders/{id}/pay` | Complete payment for a manual/mock order |
| GET/POST | `/tickets` | My tickets / create a ticket (administrators see all) |
| GET | `/tickets/{id}` | Ticket details (including messages) |
| POST | `/tickets/{id}/reply` | Reply to a ticket |
| PATCH | `/admin/tickets/{id}` | Update ticket status |

Plan fields: `price_cents` (price in cents), `duration_days` (0 for permanent), `quota_mb` (0 for unlimited), `role_group_id` (the role group applied after purchase; may be empty), `active`, and `sort_order`. A coupon's `type` is `fixed` (amount in cents) or `percent` (percentage), and supports a `min_amount_cents` threshold, a total `max_uses`, a per-user `per_user_limit`, and `expires_at`. The order request body is `{"plan_id":"…","coupon_code":"…","provider":"manual"}`; after successful payment (including free orders), the plan's quota and role group are applied automatically.

## Pagination

`GET /orders` and `GET /tickets` support server-side pagination:

| Parameter | Description |
|-----------|-------------|
| `page` | Page number, starting at 1; default 1 |
| `page_size` | Items per page; default 20, maximum 200 |
| `status` | Orders: `pending` / `paid` / `cancelled`; tickets: `open` / `answered` / `closed` |

Passing `page` or `page_size` returns a paginated object; **without pagination parameters it still returns the legacy array** for compatibility with existing SDKs:

```json
{ "items": [], "total": 0, "page": 1, "page_size": 20 }
```

When administrators call these two endpoints they receive site-wide data; regular users can only see their own orders/tickets.

When `duration_days` is greater than 0, a purchase extends the validity period from the current expiry time (if not yet expired) or from now. Users can check it via `plan_expires_at` (returned by `GET /auth/me`). After expiry, the service automatically (within an hour at most) reverts the user to the default role group and default quota. `duration_days` of 0 means permanent and clears the expiry time.
