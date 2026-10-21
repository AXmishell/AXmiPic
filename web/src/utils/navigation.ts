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
  label: string
  to: string
  icon: Component
  exact?: boolean
  /** 额外的搜索关键词（别名、英文名等）。 */
  keywords?: string
}

export interface NavGroup {
  label: string
  items: NavItem[]
}

/** 普通用户控制台导航。 */
export const userNavGroups: NavGroup[] = [
  {
    label: '概览',
    items: [{ label: '用户中心', to: '/user', icon: Odometer, exact: true, keywords: 'home dashboard 概览' }],
  },
  {
    label: '资源',
    items: [
      { label: '我的图片', to: '/user/images', icon: Picture, keywords: 'image 图片 上传' },
      { label: '图片处理', to: '/user/processing', icon: MagicStick, keywords: 'process 处理 压缩' },
      { label: '我的相册', to: '/user/albums', icon: Folder, keywords: 'album 相册' },
      { label: '图片广场', to: '/user/plaza', icon: Grid, keywords: 'plaza 广场 公开' },
      { label: '我的分享', to: '/user/shares', icon: Share, keywords: 'share 分享 链接' },
      { label: '访问令牌', to: '/user/tokens', icon: Key, keywords: 'token api 令牌' },
    ],
  },
  {
    label: '账户',
    items: [
      { label: '套餐', to: '/user/pricing', icon: ShoppingCart, keywords: 'plan 套餐 价格' },
      { label: '工单', to: '/user/tickets', icon: Service, keywords: 'ticket 工单 客服' },
    ],
  },
]

/** 管理员控制台导航。 */
export const adminNavGroups: NavGroup[] = [
  {
    label: '概览',
    items: [{ label: '仪表盘', to: '/admin', icon: Odometer, exact: true, keywords: 'dashboard 概览 统计' }],
  },
  {
    label: '内容',
    items: [
      { label: '用户管理', to: '/admin/users', icon: User, keywords: 'user 用户 账号 角色' },
      { label: '图片广场', to: '/admin/plaza', icon: Grid, keywords: 'plaza 广场 图片' },
      { label: '站点内容', to: '/admin/site', icon: Bell, keywords: 'site 公告 举报 页面' },
    ],
  },
  {
    label: '运营',
    items: [
      { label: '角色策略', to: '/admin/policies', icon: Management, keywords: 'policy 角色 权限 配额 限流' },
      { label: '计费管理', to: '/admin/billing', icon: ShoppingCart, keywords: 'billing 订单 套餐 优惠券 工单' },
      { label: '存储配置', to: '/admin/storage', icon: Coin, keywords: 'storage s3 oss cos qiniu 存储' },
    ],
  },
  {
    label: '系统',
    items: [
      { label: '系统设置', to: '/admin/system', icon: Setting, keywords: 'system 设置 通知 支付 集成' },
      { label: '插件市场', to: '/admin/plugins', icon: Goods, keywords: 'plugin 插件 扩展' },
      { label: '邮件日志', to: '/admin/mail-logs', icon: Message, keywords: 'mail sms log 邮件 短信 日志' },
    ],
  },
]

/** 根据控制台类型返回导航。 */
export function navGroupsFor(variant: 'user' | 'admin'): NavGroup[] {
  return variant === 'admin' ? adminNavGroups : userNavGroups
}
