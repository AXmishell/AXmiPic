---
layout: home

hero:
  name: AXmiPic
  text: A lightweight, reliable self-hosted image hosting service
  tagline: A single Go binary + embedded single-page app. Supports multiple storage backends, on-the-fly image processing, albums and the image plaza, plan billing, and an admin console.
  image:
    src: /favicon.svg
    alt: AXmiPic
  actions:
    - theme: brand
      text: Get Started
      link: /guide/getting-started
    - theme: alt
      text: API Reference
      link: /api/
    - theme: alt
      text: GitHub
      link: https://github.com/AXmishell/AXmiPic

features:
  - title: Multiple upload methods
    details: Admin UI upload, batch upload, paste upload, drag-and-drop upload, the multipart API, and presigned direct upload to object storage.
  - title: On-the-fly image processing
    details: Real-time resize, crop, rotate, flip, grayscale, blur, sharpen, text watermark, and transcode via URL query parameters, with ETag caching.
  - title: Multiple storage backends
    details: Local filesystem, S3-compatible object storage (AWS / MinIO / R2 / OSS / COS), and Qiniu Cloud Kodo, hot-switchable at runtime from the admin console.
  - title: Accounts and permissions
    details: JWT sessions + long-lived API tokens. Role groups control quotas, rate limits, processing capabilities, and feature toggles through policies, with a built-in Guest role.
  - title: Site operations
    details: Albums and the image plaza, share links, announcements and standalone pages, report handling, a ticket system, and AI image moderation.
  - title: Plans and payments
    details: Plans, coupons, orders, and pluggable payment channels (Alipay Face-to-Face Payment, WeChat Pay v3, YiPay, manual, and mock).
---
