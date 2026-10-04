import { request } from './client'
import type {
  Coupon,
  CouponInput,
  Order,
  Plan,
  PlanInput,
  Ticket,
  TicketInput,
  TicketStatus,
} from './types'

/** 启用的套餐（无需登录）。 */
export function listPlans(): Promise<Plan[]> {
  return request<Plan[]>({ method: 'GET', url: '/plans' })
}

/** 已注册的支付渠道。 */
export function listPaymentGateways(): Promise<string[]> {
  return request<string[]>({ method: 'GET', url: '/payment-gateways' })
}

/** 校验优惠券并返回折扣（分）。 */
export function validateCoupon(code: string, amountCents: number): Promise<{ coupon: Coupon; discount_cents: number }> {
  return request({ method: 'POST', url: '/coupons/validate', data: { code, amount_cents: amountCents } })
}

/** 下单。 */
export function createOrder(planId: string, couponCode = '', provider = 'manual'): Promise<Order> {
  return request<Order>({
    method: 'POST',
    url: '/orders',
    data: { plan_id: planId, coupon_code: couponCode, provider },
  })
}

export function listOrders(): Promise<Order[]> {
  return request<Order[]>({ method: 'GET', url: '/orders' })
}

/** 完成手动/模拟订单的支付。 */
export function payOrder(id: string): Promise<Order> {
  return request<Order>({ method: 'POST', url: `/orders/${id}/pay` })
}

/** 工单。 */
export function listTickets(status?: TicketStatus): Promise<Ticket[]> {
  return request<Ticket[]>({ method: 'GET', url: '/tickets', params: status ? { status } : undefined })
}

export function createTicket(input: TicketInput): Promise<Ticket> {
  return request<Ticket>({ method: 'POST', url: '/tickets', data: input })
}

export function getTicket(id: string): Promise<Ticket> {
  return request<Ticket>({ method: 'GET', url: `/tickets/${id}` })
}

export function replyTicket(id: string, body: string): Promise<Ticket> {
  return request<Ticket>({ method: 'POST', url: `/tickets/${id}/reply`, data: { body } })
}

/** 管理端：套餐。 */
export function adminListPlans(): Promise<Plan[]> {
  return request<Plan[]>({ method: 'GET', url: '/admin/plans' })
}

export function createPlan(input: PlanInput): Promise<Plan> {
  return request<Plan>({ method: 'POST', url: '/admin/plans', data: input })
}

export function updatePlan(id: string, input: PlanInput): Promise<Plan> {
  return request<Plan>({ method: 'PUT', url: `/admin/plans/${id}`, data: input })
}

export function deletePlan(id: string): Promise<void> {
  return request<void>({ method: 'DELETE', url: `/admin/plans/${id}` })
}

/** 管理端：优惠券。 */
export function adminListCoupons(): Promise<Coupon[]> {
  return request<Coupon[]>({ method: 'GET', url: '/admin/coupons' })
}

export function createCoupon(input: CouponInput): Promise<Coupon> {
  return request<Coupon>({ method: 'POST', url: '/admin/coupons', data: input })
}

export function updateCoupon(id: string, input: CouponInput): Promise<Coupon> {
  return request<Coupon>({ method: 'PUT', url: `/admin/coupons/${id}`, data: input })
}

export function deleteCoupon(id: string): Promise<void> {
  return request<void>({ method: 'DELETE', url: `/admin/coupons/${id}` })
}

/** 管理端：工单状态。 */
export function setTicketStatus(id: string, status: TicketStatus): Promise<Ticket> {
  return request<Ticket>({ method: 'PATCH', url: `/admin/tickets/${id}`, data: { status } })
}

/** 管理端：通知渠道与测试。 */
export function getNotifyChannels(): Promise<{ sms: string; email: string }> {
  return request<{ sms: string; email: string }>({ method: 'GET', url: '/admin/notify/channels' })
}

export function sendTestNotify(payload: {
  channel: 'sms' | 'email'
  to: string
  subject?: string
  body: string
}): Promise<{ status: string }> {
  return request<{ status: string }>({ method: 'POST', url: '/admin/notify/test', data: payload })
}

/** 管理端：安全扫描器信息。 */
export function getSecurityInfo(): Promise<{ scanner?: string }> {
  return request<{ scanner?: string }>({ method: 'GET', url: '/admin/security' })
}
