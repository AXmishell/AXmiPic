---
layout: home

hero:
  name: AXmiPic
  text: 轻量、可靠的自托管图床服务
  tagline: 单个 Go 二进制 + 内嵌单页应用。支持多存储后端、即时图片处理、相册与图片广场、套餐计费与后台管理。
  image:
    src: /favicon.svg
    alt: AXmiPic
  actions:
    - theme: brand
      text: 快速开始
      link: /guide/getting-started
    - theme: alt
      text: API 参考
      link: /api/
    - theme: alt
      text: GitHub
      link: https://github.com/AXmishell/AXmiPic

features:
  - title: 多种上传方式
    details: 后台界面上传、批量上传、粘贴上传、拖拽上传、multipart 接口与对象存储预签名直传。
  - title: 即时图片处理
    details: 通过 URL 查询参数实时缩放、裁剪、旋转、翻转、转灰度、模糊、锐化、文字水印与转码，带 ETag 缓存。
  - title: 多存储后端
    details: 本地文件系统、S3 兼容对象存储（AWS / MinIO / R2 / OSS / COS）与七牛云 Kodo，后台可运行中热切换。
  - title: 账户与权限
    details: JWT 会话 + 长期 API 令牌，角色组按策略控制配额、限流、处理能力与功能开关，内置 Guest 访客。
  - title: 站内运营
    details: 相册与图片广场、分享链接、公告与独立页面、举报处理、工单系统与 AI 图片审查。
  - title: 套餐与支付
    details: 套餐、优惠券、订单与可插拔支付渠道（支付宝当面付、微信支付 v3、易支付、人工与模拟）。
---
