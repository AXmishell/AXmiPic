# Introduction

A lightweight, reliable self-hosted image hosting service. It provides image uploads, on-the-fly processing, multiple storage backends, and an admin console. The backend is a single Go binary, and the frontend is an embedded single-page app.

## Features

- **Multiple upload methods**: admin UI upload, batch upload, paste upload, drag-and-drop upload, `multipart` API upload, and presigned direct upload to object storage
- **One-click embed code**: copy an image's URL, HTML, BBCode, or Markdown (controlled by role feature toggles)
- **Image and album sharing**: generate share links with an optional access password, expiry, and maximum visit count; the public share page is `/s/{token}`
- **Announcements and standalone pages**: administrators can publish announcements (with pinning and severity levels) shown on the dashboard, and maintain standalone pages publicly accessible at `/p/{slug}`
- **Report management**: users report images, and administrators handle or dismiss the reports in the admin console
- **Plans and billing**: plans (price, validity period, quota, role group) and coupons (fixed/percentage, threshold, usage limit, validity), plus checkout, order management, and pluggable payment channels
- **First-party payment integrations**: Alipay Face-to-Face Payment (RSA2 signing/verification) and WeChat Pay v3 (SHA256-RSA signing, AES-GCM callback decryption), along with manual/mock channels
- **Image safety**: an upload content scanner (pluggable) with a built-in whitelist and dangerous magic-number detection, rejecting executable content disguised as images
- **Notification system**: pluggable SMS (generic HTTP gateway) and email (SMTP/STARTTLS) channels, with test sending from the admin console
- **Ticket system**: users submit tickets and reach support, while administrators reply and close them
- **Random storage keys and deduplication**: stored files use date-partitioned, random, unguessable key names; the content `sha256` is recorded separately, so the same owner uploading identical content is deduplicated automatically, while different owners each hold an independent object
- **Original filenames preserved**: the storage layer uses randomly named files, while the database separately records the original filename, stored filename, and hash
- **Multiple image processing drivers**: pure Go (default), libvips (`-tags libvips`), and ImageMagick (the `magick` command) processors, selected by configuration at runtime with fallback when unavailable
- **On-the-fly image processing**: real-time resize, crop, rotate, flip, grayscale, blur, sharpen, text watermark, and transcode via URL query parameters, with ETag caching
- **Multiple storage backends**: local filesystem, S3-compatible object storage (AWS S3 / MinIO / Cloudflare R2), Alibaba Cloud OSS, Tencent Cloud COS, and Qiniu Cloud Kodo
- **Hot storage switching at runtime**: the default storage can be switched at any time from the admin console; existing images are automatically routed back to their original storage for reads based on their records, with no restart required
- **Optional database**: SQLite (default, works out of the box) or PostgreSQL
- **Albums and the image plaza**: organize images into albums; both images and albums can be made public, appearing in the cross-user image plaza and public album listings, with public user profile pages
- **Role groups and multiple policies**: role groups control a user's storage quota, upload limits, rate limits, image processing capabilities, and feature toggles; policies are reusable and overridable by type, and newly registered users automatically join the default role group
- **Account system**: administrators and regular users are managed in separate tables; JWT sessions + long-lived API tokens
- **Separated interfaces and setup mode**: a public landing page + user center (`/user`) separated from the admin console (`/admin`); a built-in low-privilege Guest visitor and a setup mode entered manually at `/install`
- **Quotas and rate limiting**: per-user storage quotas and per-user/guest/IP rate limits
- **Security**: secrets encrypted at rest with AES-256-GCM, bcrypt passwords, and defense-in-depth response headers

## Tech stack

| Layer | Technology |
|-------|------------|
| Backend | Go 1.26, chi router, GORM |
| Database | SQLite (pure Go driver) / PostgreSQL |
| Image processing | Pure Go (`disintegration/imaging`), optional libvips (`-tags libvips`) |
| Object storage | Local / S3-compatible (minio-go) / Alibaba Cloud OSS SDK / Tencent Cloud COS SDK / Qiniu SDK |
| Payments | Official WeChat Pay SDK (`wechatpay-apiv3/wechatpay-go`), Alipay RSA2 (native), YiPay (native) |
| Frontend | Vue 3, Pinia, Vue Router, Element Plus, TypeScript, Vite |
