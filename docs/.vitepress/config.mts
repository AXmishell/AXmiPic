import process from 'node:process'
import { defineConfig } from 'vitepress'

export default defineConfig({
  lang: 'zh-CN',
  title: 'AXmiPic',
  description: '轻量、可靠的自托管图床服务',
  // 站点通过自定义域名 axmipic.gpcn.cc 在根路径提供，故 base 为 '/'。
  // GitHub Pages 会把默认地址 axmishell.github.io/AXmiPic/ 301 跳转到该域名根路径。
  // 若改回不带自定义域名的部署，可设 DOCS_BASE=/AXmiPic/ 覆盖。
  base: process.env.DOCS_BASE ?? '/',
  lastUpdated: true,
  head: [['link', { rel: 'icon', type: 'image/svg+xml', href: '/favicon.svg' }]],
  themeConfig: {
    nav: [
      { text: '指南', link: '/guide/', activeMatch: '/guide/' },
      { text: 'API', link: '/api/', activeMatch: '/api/' },
      { text: '开发', link: '/dev/', activeMatch: '/dev/' },
      { text: 'GitHub', link: 'https://github.com/AXmishell/AXmiPic' },
    ],
    sidebar: {
      '/guide/': [
        {
          text: '指南',
          items: [
            { text: '项目简介', link: '/guide/' },
            { text: '快速开始', link: '/guide/getting-started' },
            { text: '配置说明', link: '/guide/configuration' },
            { text: '图片处理', link: '/guide/image-processing' },
            { text: '界面与路由', link: '/guide/ui-and-routes' },
            { text: 'Docker 部署', link: '/guide/deployment' },
          ],
        },
      ],
      '/api/': [
        {
          text: 'API',
          items: [
            { text: '概览', link: '/api/' },
            { text: '认证', link: '/api/auth' },
            { text: '上传', link: '/api/upload' },
            { text: '图片与令牌', link: '/api/images' },
            { text: '相册与图片广场', link: '/api/albums' },
            { text: '分享', link: '/api/shares' },
            { text: '站点内容', link: '/api/content' },
            { text: '套餐与订单', link: '/api/billing' },
            { text: '支付渠道', link: '/api/payment' },
            { text: '图片广场 AI 审查', link: '/api/moderation' },
            { text: '图片安全与通知', link: '/api/security' },
            { text: '角色组与策略', link: '/api/policies' },
            { text: '管理接口', link: '/api/admin' },
          ],
        },
      ],
      '/dev/': [
        {
          text: '开发',
          items: [
            { text: '开发指南', link: '/dev/' },
            { text: 'SDK', link: '/dev/sdk' },
            { text: '持续集成', link: '/dev/ci' },
            { text: '目录结构', link: '/dev/structure' },
          ],
        },
      ],
    },
    outline: { level: [2, 3], label: '本页目录' },
    search: { provider: 'local' },
    editLink: {
      pattern: 'https://github.com/AXmishell/AXmiPic/edit/main/docs/:path',
      text: '在 GitHub 上编辑此页',
    },
    docFooter: { prev: '上一页', next: '下一页' },
    lastUpdatedText: '最后更新',
    returnToTopLabel: '回到顶部',
    sidebarMenuLabel: '菜单',
    darkModeSwitchLabel: '主题',
    socialLinks: [{ icon: 'github', link: 'https://github.com/AXmishell/AXmiPic' }],
    footer: {
      message: '基于 MIT 许可证发布',
      copyright: 'Copyright © AXmiPic',
    },
  },
})
