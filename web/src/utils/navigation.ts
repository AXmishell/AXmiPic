import type { Component } from 'vue'
import {
  Bell,
  Coin,
  Folder,
  Goods,
  Grid,
  Key,
  MagicStick,
  Management,
  Message,
  Odometer,
  Picture,
  Service,
  Setting,
  Share,
  ShoppingCart,
  User,
} from '@element-plus/icons-vue'

export interface NavItem {
  /** i18n 文案键，组件中通过 t(labelKey) 渲染。 */
  labelKey: string
  to: string
  icon: Component
  exact?: boolean
  /** 额外的搜索关键词（别名、英文名等）。 */
  keywords?: string
}

export interface NavGroup {
  labelKey: string
  items: NavItem[]
}

/** 普通用户控制台导航。 */
export const userNavGroups: NavGroup[] = [
  {
    labelKey: 'nav.groups.overview',
    items: [
      {
        labelKey: 'nav.items.userCenter',
        to: '/user',
        icon: Odometer,
        exact: true,
        keywords: 'home dashboard 概览 用户中心',
      },
    ],
  },
  {
    labelKey: 'nav.groups.resources',
    items: [
      { labelKey: 'nav.items.images', to: '/user/images', icon: Picture, keywords: 'image 图片 上传' },
      { labelKey: 'nav.items.processing', to: '/user/processing', icon: MagicStick, keywords: 'process 处理 压缩' },
      { labelKey: 'nav.items.albums', to: '/user/albums', icon: Folder, keywords: 'album 相册' },
      { labelKey: 'nav.items.plaza', to: '/user/plaza', icon: Grid, keywords: 'plaza 广场 公开' },
      { labelKey: 'nav.items.shares', to: '/user/shares', icon: Share, keywords: 'share 分享 链接' },
      { labelKey: 'nav.items.tokens', to: '/user/tokens', icon: Key, keywords: 'token api 令牌' },
    ],
  },
  {
    labelKey: 'nav.groups.account',
    items: [
      { labelKey: 'nav.items.pricing', to: '/user/pricing', icon: ShoppingCart, keywords: 'plan 套餐 价格' },
      { labelKey: 'nav.items.tickets', to: '/user/tickets', icon: Service, keywords: 'ticket 工单 客服' },
    ],
  },
]

/** 管理员控制台导航。 */
export const adminNavGroups: NavGroup[] = [
  {
    labelKey: 'nav.groups.overview',
    items: [
      {
        labelKey: 'nav.items.dashboard',
        to: '/admin',
        icon: Odometer,
        exact: true,
        keywords: 'dashboard 概览 统计',
      },
    ],
  },
  {
    labelKey: 'nav.groups.content',
    items: [
      { labelKey: 'nav.items.users', to: '/admin/users', icon: User, keywords: 'user 用户 账号 角色' },
      { labelKey: 'nav.items.plaza', to: '/admin/plaza', icon: Grid, keywords: 'plaza 广场 图片' },
      { labelKey: 'nav.items.site', to: '/admin/site', icon: Bell, keywords: 'site 公告 举报 页面' },
    ],
  },
  {
    labelKey: 'nav.groups.operations',
    items: [
      { labelKey: 'nav.items.policies', to: '/admin/policies', icon: Management, keywords: 'policy 角色 权限 配额 限流' },
      { labelKey: 'nav.items.billing', to: '/admin/billing', icon: ShoppingCart, keywords: 'billing 订单 套餐 优惠券 工单' },
      { labelKey: 'nav.items.storage', to: '/admin/storage', icon: Coin, keywords: 'storage s3 oss cos qiniu 存储' },
    ],
  },
  {
    labelKey: 'nav.groups.system',
    items: [
      { labelKey: 'nav.items.system', to: '/admin/system', icon: Setting, keywords: 'system 设置 通知 支付 集成' },
      { labelKey: 'nav.items.plugins', to: '/admin/plugins', icon: Goods, keywords: 'plugin 插件 扩展' },
      { labelKey: 'nav.items.mailLogs', to: '/admin/mail-logs', icon: Message, keywords: 'mail sms log 邮件 短信 日志' },
    ],
  },
]

/** 根据控制台类型返回导航。 */
export function navGroupsFor(variant: 'user' | 'admin'): NavGroup[] {
  return variant === 'admin' ? adminNavGroups : userNavGroups
}
