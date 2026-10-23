import process from 'node:process'
import { defineConfig } from 'vitepress'

export default defineConfig({
  // English is the default documentation language; Chinese lives under /zh/.
  lang: 'en-US',
  title: 'AXmiPic',
  description: 'Lightweight, reliable self-hosted image hosting',
  // The site is served from the root of the custom domain axmipic.gpcn.cc, so base is '/'.
  // GitHub Pages redirects the default axmishell.github.io/AXmiPic/ address to that domain.
  // For deployments without the custom domain, override with DOCS_BASE=/AXmiPic/.
  base: process.env.DOCS_BASE ?? '/',
  lastUpdated: true,
  head: [['link', { rel: 'icon', type: 'image/svg+xml', href: '/favicon.svg' }]],
  themeConfig: {
    nav: [
      { text: 'Guide', link: '/guide/', activeMatch: '/guide/' },
      { text: 'API', link: '/api/', activeMatch: '/api/' },
      { text: 'Development', link: '/dev/', activeMatch: '/dev/' },
      { text: 'GitHub', link: 'https://github.com/AXmishell/AXmiPic' },
    ],
    sidebar: {
      '/guide/': [
        {
          text: 'Guide',
          items: [
            { text: 'Introduction', link: '/guide/' },
            { text: 'Getting started', link: '/guide/getting-started' },
            { text: 'Configuration', link: '/guide/configuration' },
            { text: 'Image processing', link: '/guide/image-processing' },
            { text: 'UI and routes', link: '/guide/ui-and-routes' },
            { text: 'Docker deployment', link: '/guide/deployment' },
          ],
        },
      ],
      '/api/': [
        {
          text: 'API',
          items: [
            { text: 'Overview', link: '/api/' },
            { text: 'Authentication', link: '/api/auth' },
            { text: 'Upload', link: '/api/upload' },
            { text: 'Images and tokens', link: '/api/images' },
            { text: 'Albums and the Image Plaza', link: '/api/albums' },
            { text: 'Shares', link: '/api/shares' },
            { text: 'Site content', link: '/api/content' },
            { text: 'Plans and orders', link: '/api/billing' },
            { text: 'Payment channels', link: '/api/payment' },
            { text: 'Image Plaza AI moderation', link: '/api/moderation' },
            { text: 'Image security and notifications', link: '/api/security' },
            { text: 'Role groups and policies', link: '/api/policies' },
            { text: 'Admin endpoints', link: '/api/admin' },
          ],
        },
      ],
      '/dev/': [
        {
          text: 'Development',
          items: [
            { text: 'Development guide', link: '/dev/' },
            { text: 'SDK', link: '/dev/sdk' },
            { text: 'Plugin system', link: '/dev/plugins' },
            { text: 'Continuous integration', link: '/dev/ci' },
            { text: 'Directory structure', link: '/dev/structure' },
            { text: 'Frontend i18n', link: '/dev/i18n' },
          ],
        },
      ],
    },
    outline: { level: [2, 3], label: 'On this page' },
    search: { provider: 'local' },
    editLink: {
      pattern: 'https://github.com/AXmishell/AXmiPic/edit/main/docs/:path',
      text: 'Edit this page on GitHub',
    },
    docFooter: { prev: 'Previous page', next: 'Next page' },
    lastUpdatedText: 'Last updated',
    returnToTopLabel: 'Back to top',
    sidebarMenuLabel: 'Menu',
    darkModeSwitchLabel: 'Theme',
    socialLinks: [{ icon: 'github', link: 'https://github.com/AXmishell/AXmiPic' }],
    footer: {
      message: 'Released under the MIT License',
      copyright: 'Copyright © AXmiPic',
    },
  },
  locales: {
    root: {
      label: 'English',
      lang: 'en-US',
    },
    zh: {
      label: '简体中文',
      lang: 'zh-CN',
      link: '/zh/',
      themeConfig: {
        nav: [
          { text: '指南', link: '/zh/guide/', activeMatch: '/zh/guide/' },
          { text: 'API', link: '/zh/api/', activeMatch: '/zh/api/' },
          { text: '开发', link: '/zh/dev/', activeMatch: '/zh/dev/' },
          { text: 'GitHub', link: 'https://github.com/AXmishell/AXmiPic' },
        ],
        sidebar: {
          '/zh/guide/': [
            {
              text: '指南',
              items: [
                { text: '项目简介', link: '/zh/guide/' },
                { text: '快速开始', link: '/zh/guide/getting-started' },
                { text: '配置说明', link: '/zh/guide/configuration' },
                { text: '图片处理', link: '/zh/guide/image-processing' },
                { text: '界面与路由', link: '/zh/guide/ui-and-routes' },
                { text: 'Docker 部署', link: '/zh/guide/deployment' },
              ],
            },
          ],
          '/zh/api/': [
            {
              text: 'API',
              items: [
                { text: '概览', link: '/zh/api/' },
                { text: '认证', link: '/zh/api/auth' },
                { text: '上传', link: '/zh/api/upload' },
                { text: '图片与令牌', link: '/zh/api/images' },
                { text: '相册与图片广场', link: '/zh/api/albums' },
                { text: '分享', link: '/zh/api/shares' },
                { text: '站点内容', link: '/zh/api/content' },
                { text: '套餐与订单', link: '/zh/api/billing' },
                { text: '支付渠道', link: '/zh/api/payment' },
                { text: '图片广场 AI 审查', link: '/zh/api/moderation' },
                { text: '图片安全与通知', link: '/zh/api/security' },
                { text: '角色组与策略', link: '/zh/api/policies' },
                { text: '管理接口', link: '/zh/api/admin' },
              ],
            },
          ],
          '/zh/dev/': [
            {
              text: '开发',
              items: [
                { text: '开发指南', link: '/zh/dev/' },
                { text: 'SDK', link: '/zh/dev/sdk' },
                { text: '插件系统', link: '/zh/dev/plugins' },
                { text: '持续集成', link: '/zh/dev/ci' },
                { text: '目录结构', link: '/zh/dev/structure' },
                { text: '前端国际化', link: '/zh/dev/i18n' },
              ],
            },
          ],
        },
        outline: { level: [2, 3], label: '本页目录' },
        editLink: {
          pattern: 'https://github.com/AXmishell/AXmiPic/edit/main/docs/:path',
          text: '在 GitHub 上编辑此页',
        },
        docFooter: { prev: '上一页', next: '下一页' },
        lastUpdatedText: '最后更新',
        returnToTopLabel: '回到顶部',
        sidebarMenuLabel: '菜单',
        darkModeSwitchLabel: '主题',
        footer: {
          message: '基于 MIT 许可证发布',
          copyright: 'Copyright © AXmiPic',
        },
      },
    },
  },
})
