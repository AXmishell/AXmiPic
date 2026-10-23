<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useI18n } from 'vue-i18n'
import type { FormInstance, FormRules } from 'element-plus'
import { Close, Delete, EditPen, Plus, Refresh } from '@element-plus/icons-vue'

import {
  adminListCoupons,
  adminListPlans,
  createCoupon,
  createPlan,
  deleteCoupon,
  deletePlan,
  getTicket,
  listOrdersPaged,
  listTicketsPaged,
  payOrder,
  replyTicket,
  setTicketStatus,
  updateCoupon,
  updatePlan,
} from '@/api/billing'
import { toApiError } from '@/api/client'
import { listRoleGroups } from '@/api/admin'
import type {
  Coupon,
  CouponInput,
  CouponType,
  Order,
  Plan,
  PlanInput,
  RoleGroup,
  Ticket,
} from '@/api/types'
import EmptyState from '@/components/EmptyState.vue'
import ErrorState from '@/components/ErrorState.vue'
import PageHeader from '@/components/PageHeader.vue'
import TableSkeleton from '@/components/TableSkeleton.vue'
import { formatDateTime } from '@/utils/format'
import { useTabQuery } from '@/utils/useTabQuery'

const { t } = useI18n()

type Tab = 'plans' | 'coupons' | 'orders' | 'tickets'
const BILLING_TABS: readonly Tab[] = ['plans', 'coupons', 'orders', 'tickets']
const activeTab = useTabQuery<Tab>('tab', 'plans', BILLING_TABS)

const plans = ref<Plan[]>([])
const coupons = ref<Coupon[]>([])
const orders = ref<Order[]>([])
const tickets = ref<Ticket[]>([])
const roleGroups = ref<RoleGroup[]>([])
const loading = ref(false)
const errorMessage = ref('')

const orderPage = ref(1)
const orderPageSize = ref(20)
const orderTotal = ref(0)
const ordersLoading = ref(false)
const ticketPage = ref(1)
const ticketPageSize = ref(20)
const ticketTotal = ref(0)
const ticketsLoading = ref(false)

const priceLabel = (cents: number): string => (cents === 0 ? t('billing.free') : `¥${(cents / 100).toFixed(2)}`)

const orderDetailOpen = ref(false)
const detailOrder = ref<Order | null>(null)

function openOrderDetail(order: Order): void {
  detailOrder.value = order
  orderDetailOpen.value = true
}

function orderStatusLabel(status: Order['status']): string {
  return status === 'paid'
    ? t('billing.orderStatus.paid')
    : status === 'cancelled'
      ? t('billing.orderStatus.cancelled')
      : t('billing.orderStatus.pending')
}

function ticketStatusLabel(status: Ticket['status']): string {
  return status === 'open'
    ? t('billing.ticketStatus.open')
    : status === 'answered'
      ? t('billing.ticketStatus.answered')
      : t('billing.ticketStatus.closed')
}

async function fetchOrders(): Promise<void> {
  ordersLoading.value = true
  try {
    const data = await listOrdersPaged({ page: orderPage.value, page_size: orderPageSize.value })
    orders.value = data.items ?? []
    orderTotal.value = data.total ?? 0
    if (orders.value.length === 0 && orderPage.value > 1 && orderTotal.value > 0) {
      orderPage.value -= 1
      await fetchOrders()
    }
  } finally {
    ordersLoading.value = false
  }
}

async function fetchTickets(): Promise<void> {
  ticketsLoading.value = true
  try {
    const data = await listTicketsPaged({ page: ticketPage.value, page_size: ticketPageSize.value })
    tickets.value = data.items ?? []
    ticketTotal.value = data.total ?? 0
    if (tickets.value.length === 0 && ticketPage.value > 1 && ticketTotal.value > 0) {
      ticketPage.value -= 1
      await fetchTickets()
    }
  } finally {
    ticketsLoading.value = false
  }
}

function onOrderPageChange(page: number): void {
  orderPage.value = page
  void fetchOrders().catch((error) => ElMessage.error(toApiError(error).message))
}

function onOrderSizeChange(size: number): void {
  orderPageSize.value = size
  orderPage.value = 1
  void fetchOrders().catch((error) => ElMessage.error(toApiError(error).message))
}

function onTicketPageChange(page: number): void {
  ticketPage.value = page
  void fetchTickets().catch((error) => ElMessage.error(toApiError(error).message))
}

function onTicketSizeChange(size: number): void {
  ticketPageSize.value = size
  ticketPage.value = 1
  void fetchTickets().catch((error) => ElMessage.error(toApiError(error).message))
}

async function load(): Promise<void> {
  loading.value = true
  errorMessage.value = ''
  try {
    const [planList, couponList, , , groups] = await Promise.all([
      adminListPlans(),
      adminListCoupons(),
      fetchOrders(),
      fetchTickets(),
      listRoleGroups().catch(() => []),
    ])
    plans.value = planList ?? []
    coupons.value = couponList ?? []
    roleGroups.value = groups ?? []
  } catch (error) {
    errorMessage.value = toApiError(error).message
  } finally {
    loading.value = false
  }
}

// 管理员核销 manual 等人工渠道的待支付订单。
async function confirmOrder(row: Order): Promise<void> {
  try {
    await ElMessageBox.confirm(
      t('billing.confirm.verifyMessage', {
        plan: row.plan_name,
        amount: priceLabel(row.amount_cents),
      }),
      t('billing.confirm.verifyTitle'),
      { type: 'warning' },
    )
  } catch {
    return
  }
  try {
    await payOrder(row.id)
    ElMessage.success(t('billing.messages.orderVerified'))
    if (detailOrder.value?.id === row.id) {
      detailOrder.value = { ...detailOrder.value, status: 'paid', paid_at: new Date().toISOString() }
    }
    await load()
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  }
}

// ---- 套餐 ----
const planOpen = ref(false)
const planSaving = ref(false)
const planFormRef = ref<FormInstance>()
const editingPlanId = ref('')
const planForm = reactive<PlanInput>({
  name: '',
  description: '',
  price_cents: 0,
  duration_days: 0,
  quota_mb: 0,
  role_group_id: '',
  active: true,
  sort_order: 0,
})
const planRules = computed<FormRules>(() => ({
  name: [{ required: true, message: t('billing.validation.planNameRequired'), trigger: 'blur' }],
}))
const planSnapshot = ref('')

function planState(): string {
  return JSON.stringify(planForm)
}

async function closePlanDialog(done: () => void): Promise<void> {
  if (planSaving.value) return
  if (planState() === planSnapshot.value) {
    done()
    return
  }
  try {
    await ElMessageBox.confirm(t('billing.confirm.discardMessage'), t('billing.confirm.discardTitle'), {
      confirmButtonText: t('billing.confirm.discard'),
      cancelButtonText: t('billing.confirm.continueEditing'),
      type: 'warning',
      confirmButtonClass: 'el-button--danger',
    })
    done()
  } catch {
    // 用户选择继续编辑。
  }
}

function openCreatePlan(): void {
  editingPlanId.value = ''
  Object.assign(planForm, {
    name: '',
    description: '',
    price_cents: 0,
    duration_days: 0,
    quota_mb: 0,
    role_group_id: '',
    active: true,
    sort_order: 0,
  })
  planSnapshot.value = planState()
  planOpen.value = true
}

function openEditPlan(plan: Plan): void {
  editingPlanId.value = plan.id
  Object.assign(planForm, {
    name: plan.name,
    description: plan.description,
    price_cents: plan.price_cents,
    duration_days: plan.duration_days,
    quota_mb: plan.quota_mb,
    role_group_id: plan.role_group_id ?? '',
    active: plan.active,
    sort_order: plan.sort_order,
  })
  planSnapshot.value = planState()
  planOpen.value = true
}

async function submitPlan(): Promise<void> {
  const instance = planFormRef.value
  if (!instance || planSaving.value) return
  const valid = await instance.validate().catch(() => false)
  if (!valid) return
  planSaving.value = true
  try {
    const payload: PlanInput = { ...planForm, name: planForm.name.trim(), description: planForm.description.trim() }
    if (editingPlanId.value) await updatePlan(editingPlanId.value, payload)
    else await createPlan(payload)
    ElMessage.success(t('billing.messages.planSaved'))
    planSnapshot.value = planState()
    planOpen.value = false
    await load()
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    planSaving.value = false
  }
}

async function removePlan(plan: Plan): Promise<void> {
  try {
    await ElMessageBox.confirm(t('billing.confirm.deletePlanMessage', { name: plan.name }), t('billing.confirm.deletePlanTitle'), {
      confirmButtonText: t('common.delete'),
      cancelButtonText: t('common.cancel'),
      type: 'warning',
      confirmButtonClass: 'el-button--danger',
    })
  } catch {
    return
  }
  try {
    await deletePlan(plan.id)
    ElMessage.success(t('billing.messages.planDeleted'))
    await load()
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  }
}

// ---- 优惠券 ----
const couponOpen = ref(false)
const couponSaving = ref(false)
const couponFormRef = ref<FormInstance>()
const editingCouponId = ref('')
const couponForm = reactive<{ code: string; type: CouponType; value_yuan: number; min_yuan: number; max_uses: number; per_user_limit: number; expires_at: string; active: boolean }>({
  code: '',
  type: 'fixed',
  value_yuan: 0,
  min_yuan: 0,
  max_uses: 0,
  per_user_limit: 0,
  expires_at: '',
  active: true,
})
const couponRules = computed<FormRules>(() => ({
  code: [{ required: true, message: t('billing.validation.couponCodeRequired'), trigger: 'blur' }],
}))
const couponSnapshot = ref('')

function couponState(): string {
  return JSON.stringify(couponForm)
}

async function closeCouponDialog(done: () => void): Promise<void> {
  if (couponSaving.value) return
  if (couponState() === couponSnapshot.value) {
    done()
    return
  }
  try {
    await ElMessageBox.confirm(t('billing.confirm.discardMessage'), t('billing.confirm.discardTitle'), {
      confirmButtonText: t('billing.confirm.discard'),
      cancelButtonText: t('billing.confirm.continueEditing'),
      type: 'warning',
      confirmButtonClass: 'el-button--danger',
    })
    done()
  } catch {
    // 用户选择继续编辑。
  }
}

function openCreateCoupon(): void {
  editingCouponId.value = ''
  Object.assign(couponForm, { code: '', type: 'fixed', value_yuan: 0, min_yuan: 0, max_uses: 0, per_user_limit: 0, expires_at: '', active: true })
  couponSnapshot.value = couponState()
  couponOpen.value = true
}

function openEditCoupon(coupon: Coupon): void {
  editingCouponId.value = coupon.id
  Object.assign(couponForm, {
    code: coupon.code,
    type: coupon.type,
    value_yuan: coupon.type === 'fixed' ? coupon.value / 100 : coupon.value,
    min_yuan: coupon.min_amount_cents / 100,
    max_uses: coupon.max_uses,
    per_user_limit: coupon.per_user_limit,
    expires_at: coupon.expires_at ?? '',
    active: coupon.active,
  })
  couponSnapshot.value = couponState()
  couponOpen.value = true
}

async function submitCoupon(): Promise<void> {
  const instance = couponFormRef.value
  if (!instance || couponSaving.value) return
  const valid = await instance.validate().catch(() => false)
  if (!valid) return
  couponSaving.value = true
  try {
    const payload: CouponInput = {
      code: couponForm.code.trim(),
      type: couponForm.type,
      value: couponForm.type === 'fixed' ? Math.round(couponForm.value_yuan * 100) : Math.round(couponForm.value_yuan),
      min_amount_cents: Math.round(couponForm.min_yuan * 100),
      max_uses: couponForm.max_uses,
      per_user_limit: couponForm.per_user_limit,
      expires_at: couponForm.expires_at || null,
      active: couponForm.active,
    }
    if (editingCouponId.value) await updateCoupon(editingCouponId.value, payload)
    else await createCoupon(payload)
    ElMessage.success(t('billing.messages.couponSaved'))
    couponSnapshot.value = couponState()
    couponOpen.value = false
    await load()
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    couponSaving.value = false
  }
}

async function removeCoupon(coupon: Coupon): Promise<void> {
  try {
    await ElMessageBox.confirm(t('billing.confirm.deleteCouponMessage', { code: coupon.code }), t('billing.confirm.deleteCouponTitle'), {
      confirmButtonText: t('common.delete'),
      cancelButtonText: t('common.cancel'),
      type: 'warning',
      confirmButtonClass: 'el-button--danger',
    })
  } catch {
    return
  }
  try {
    await deleteCoupon(coupon.id)
    ElMessage.success(t('billing.messages.couponDeleted'))
    await load()
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  }
}

function couponValueLabel(coupon: Coupon): string {
  return coupon.type === 'fixed'
    ? t('billing.coupon.fixedValue', { amount: (coupon.value / 100).toFixed(2) })
    : t('billing.coupon.percentValue', { value: coupon.value })
}

// ---- 工单 ----
const ticketDetailOpen = ref(false)
const detailTicket = ref<Ticket | null>(null)
const ticketDetailLoading = ref(false)
const replyBody = ref('')
const replying = ref(false)

async function openTicketDetail(ticket: Ticket): Promise<void> {
  ticketDetailOpen.value = true
  ticketDetailLoading.value = true
  detailTicket.value = null
  replyBody.value = ''
  try {
    detailTicket.value = await getTicket(ticket.id)
  } catch (error) {
    ElMessage.error(toApiError(error).message)
    ticketDetailOpen.value = false
  } finally {
    ticketDetailLoading.value = false
  }
}

async function submitReply(): Promise<void> {
  const ticket = detailTicket.value
  const body = replyBody.value.trim()
  if (!ticket || !body || replying.value) return
  replying.value = true
  try {
    detailTicket.value = await replyTicket(ticket.id, body)
    replyBody.value = ''
    ElMessage.success(t('billing.messages.replySent'))
    await load()
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    replying.value = false
  }
}

async function closeTicket(ticket: Ticket): Promise<void> {
  try {
    await setTicketStatus(ticket.id, 'closed')
    ElMessage.success(t('billing.messages.ticketClosed'))
    if (detailTicket.value?.id === ticket.id) {
      detailTicket.value = { ...detailTicket.value, status: 'closed' }
    }
    await load()
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  }
}

onMounted(load)
</script>

<template>
  <div class="ax-page">
    <PageHeader :title="t('billing.title')" :description="t('billing.description')">
      <template #actions>
        <el-button :icon="Refresh" :loading="loading" @click="load">{{ t('common.refresh') }}</el-button>
        <el-button v-if="activeTab === 'plans'" type="primary" :icon="Plus" @click="openCreatePlan">{{ t('billing.actions.createPlan') }}</el-button>
        <el-button v-else-if="activeTab === 'coupons'" type="primary" :icon="Plus" @click="openCreateCoupon">{{ t('billing.actions.createCoupon') }}</el-button>
      </template>
    </PageHeader>

    <ErrorState v-if="errorMessage" :message="errorMessage" @retry="load" />

    <TableSkeleton
      v-else-if="loading && plans.length === 0 && coupons.length === 0 && orders.length === 0 && tickets.length === 0"
      :rows="8"
      :columns="5"
      :title="t('billing.loading')"
    />

    <el-tabs v-else v-model="activeTab">
      <el-tab-pane :label="t('billing.tabs.plans', { count: plans.length })" name="plans">
        <EmptyState v-if="plans.length === 0" :title="t('billing.empty.plansTitle')" :description="t('billing.empty.plansDescription')" />
        <div v-else class="ax-card table-card">
          <div class="ax-table-scroll">
            <el-table :data="plans" style="width: 100%">
              <el-table-column prop="name" :label="t('billing.columns.name')" min-width="150" />
              <el-table-column :label="t('billing.columns.price')" width="110">
                <template #default="{ row }">{{ priceLabel(row.price_cents) }}</template>
              </el-table-column>
              <el-table-column :label="t('billing.columns.quota')" width="120">
                <template #default="{ row }">{{ row.quota_mb > 0 ? `${row.quota_mb} MiB` : t('billing.unlimited') }}</template>
              </el-table-column>
              <el-table-column :label="t('billing.columns.duration')" width="100">
                <template #default="{ row }">{{ row.duration_days > 0 ? t('billing.days', { days: row.duration_days }) : t('billing.forever') }}</template>
              </el-table-column>
              <el-table-column :label="t('billing.columns.status')" width="90">
                <template #default="{ row }">
                  <el-tag size="small" :type="row.active ? 'success' : 'info'" effect="plain">
                    {{ row.active ? t('billing.plan.enabled') : t('billing.plan.disabled') }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column :label="t('billing.columns.actions')" fixed="right" width="170" align="right">
                <template #default="{ row }">
                  <div class="ax-row-actions">
                    <el-button link type="primary" :icon="EditPen" @click="openEditPlan(row)">{{ t('common.edit') }}</el-button>
                    <el-button link type="danger" :icon="Delete" @click="removePlan(row)">{{ t('common.delete') }}</el-button>
                  </div>
                </template>
              </el-table-column>
            </el-table>
          </div>
        </div>
      </el-tab-pane>

      <el-tab-pane :label="t('billing.tabs.coupons', { count: coupons.length })" name="coupons">
        <EmptyState v-if="coupons.length === 0" :title="t('billing.empty.couponsTitle')" :description="t('billing.empty.couponsDescription')" />
        <div v-else class="ax-card table-card">
          <div class="ax-table-scroll">
            <el-table :data="coupons" style="width: 100%">
              <el-table-column prop="code" :label="t('billing.columns.code')" min-width="140" />
              <el-table-column :label="t('billing.columns.discount')" width="120">
                <template #default="{ row }">{{ couponValueLabel(row) }}</template>
              </el-table-column>
              <el-table-column :label="t('billing.columns.usage')" width="120">
                <template #default="{ row }">
                  {{ row.used }}{{ row.max_uses > 0 ? ` / ${row.max_uses}` : '' }}
                </template>
              </el-table-column>
              <el-table-column :label="t('billing.columns.status')" width="90">
                <template #default="{ row }">
                  <el-tag size="small" :type="row.active ? 'success' : 'info'" effect="plain">
                    {{ row.active ? t('billing.plan.enabled') : t('billing.plan.disabled') }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column :label="t('billing.columns.actions')" fixed="right" width="170" align="right">
                <template #default="{ row }">
                  <div class="ax-row-actions">
                    <el-button link type="primary" :icon="EditPen" @click="openEditCoupon(row)">{{ t('common.edit') }}</el-button>
                    <el-button link type="danger" :icon="Delete" @click="removeCoupon(row)">{{ t('common.delete') }}</el-button>
                  </div>
                </template>
              </el-table-column>
            </el-table>
          </div>
        </div>
      </el-tab-pane>

      <el-tab-pane :label="t('billing.tabs.orders', { count: orderTotal })" name="orders">
        <EmptyState v-if="orderTotal === 0" :title="t('billing.empty.ordersTitle')" :description="t('billing.empty.ordersDescription')" />
        <div v-else class="ax-card table-card">
          <div class="ax-table-scroll">
            <el-table v-loading="ordersLoading" :data="orders" style="width: 100%">
              <el-table-column prop="plan_name" :label="t('billing.columns.plan')" min-width="150" />
              <el-table-column :label="t('billing.columns.amount')" width="110">
                <template #default="{ row }">{{ priceLabel(row.amount_cents) }}</template>
              </el-table-column>
              <el-table-column :label="t('billing.columns.provider')" width="100">
                <template #default="{ row }">{{ row.provider }}</template>
              </el-table-column>
              <el-table-column :label="t('billing.columns.status')" width="90">
                <template #default="{ row }">
                  <el-tag size="small" :type="row.status === 'paid' ? 'success' : 'info'" effect="plain">
                    {{ row.status === 'paid' ? t('billing.orderStatus.paid') : row.status === 'cancelled' ? t('billing.orderStatus.cancelled') : t('billing.orderStatus.pending') }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column :label="t('billing.columns.createdAt')" min-width="170">
                <template #default="{ row }">{{ formatDateTime(row.created_at) }}</template>
              </el-table-column>
              <el-table-column :label="t('billing.columns.actions')" fixed="right" width="160" align="right">
                <template #default="{ row }">
                  <div class="ax-row-actions">
                    <el-button link type="primary" @click="openOrderDetail(row)">{{ t('billing.actions.detail') }}</el-button>
                    <el-button
                      v-if="row.status === 'pending'"
                      link
                      type="primary"
                      @click="confirmOrder(row)"
                    >
                      {{ t('billing.actions.verify') }}
                    </el-button>
                  </div>
                </template>
              </el-table-column>
            </el-table>
          </div>
        </div>
        <div v-if="orderTotal > 0" class="list-pager">
          <el-pagination
            background
            layout="sizes, prev, pager, next, total"
            :page-sizes="[10, 20, 50, 100]"
            :total="orderTotal"
            :current-page="orderPage"
            :page-size="orderPageSize"
            @current-change="onOrderPageChange"
            @size-change="onOrderSizeChange"
          />
        </div>
      </el-tab-pane>

      <el-tab-pane :label="t('billing.tabs.tickets', { count: ticketTotal })" name="tickets">
        <EmptyState v-if="ticketTotal === 0" :title="t('billing.empty.ticketsTitle')" :description="t('billing.empty.ticketsDescription')" />
        <div v-else class="ax-card table-card">
          <div class="ax-table-scroll">
            <el-table v-loading="ticketsLoading" :data="tickets" style="width: 100%">
              <el-table-column prop="subject" :label="t('billing.columns.subject')" min-width="200" />
              <el-table-column :label="t('billing.columns.user')" width="140">
                <template #default="{ row }">{{ row.username || row.user_id }}</template>
              </el-table-column>
              <el-table-column :label="t('billing.columns.status')" width="100">
                <template #default="{ row }">
                  <el-tag size="small" :type="row.status === 'open' ? 'warning' : row.status === 'answered' ? 'success' : 'info'" effect="plain">
                    {{ row.status === 'open' ? t('billing.ticketStatus.open') : row.status === 'answered' ? t('billing.ticketStatus.answered') : t('billing.ticketStatus.closed') }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column :label="t('billing.columns.updatedAt')" min-width="170">
                <template #default="{ row }">{{ formatDateTime(row.updated_at) }}</template>
              </el-table-column>
              <el-table-column :label="t('billing.columns.actions')" fixed="right" width="170" align="right">
                <template #default="{ row }">
                  <div class="ax-row-actions">
                    <el-button link type="primary" @click="openTicketDetail(row)">{{ t('billing.actions.detail') }}</el-button>
                    <el-button link type="info" :icon="Close" :disabled="row.status === 'closed'" @click="closeTicket(row)">{{ t('common.close') }}</el-button>
                  </div>
                </template>
              </el-table-column>
            </el-table>
          </div>
        </div>
        <div v-if="ticketTotal > 0" class="list-pager">
          <el-pagination
            background
            layout="sizes, prev, pager, next, total"
            :page-sizes="[10, 20, 50, 100]"
            :total="ticketTotal"
            :current-page="ticketPage"
            :page-size="ticketPageSize"
            @current-change="onTicketPageChange"
            @size-change="onTicketSizeChange"
          />
        </div>
      </el-tab-pane>
    </el-tabs>

    <!-- 套餐编辑 -->
    <el-dialog v-model="planOpen" :title="editingPlanId ? t('billing.plan.edit') : t('billing.plan.create')" width="min(520px, 92vw)" append-to-body :before-close="closePlanDialog">
      <el-form ref="planFormRef" :model="planForm" :rules="planRules" label-position="top">
        <el-form-item :label="t('billing.plan.name')" prop="name"><el-input v-model="planForm.name" maxlength="64" /></el-form-item>
        <el-form-item :label="t('billing.plan.description')"><el-input v-model="planForm.description" type="textarea" :rows="2" maxlength="255" /></el-form-item>
        <el-form-item :label="t('billing.plan.price')">
          <el-input-number v-model="planForm.price_cents" :min="0" />
          <span class="form-hint">{{ t('billing.plan.priceHint') }}</span>
        </el-form-item>
        <el-form-item :label="t('billing.plan.duration')"><el-input-number v-model="planForm.duration_days" :min="0" /></el-form-item>
        <el-form-item :label="t('billing.plan.quota')"><el-input-number v-model="planForm.quota_mb" :min="0" /></el-form-item>
        <el-form-item :label="t('billing.plan.roleGroup')">
          <el-select v-model="planForm.role_group_id" clearable :placeholder="t('billing.plan.roleGroupPlaceholder')" style="width: 100%">
            <el-option v-for="g in roleGroups" :key="g.id" :label="g.name" :value="g.id" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('billing.plan.sortOrder')"><el-input-number v-model="planForm.sort_order" :min="0" /></el-form-item>
        <el-form-item :label="t('billing.plan.active')"><el-switch v-model="planForm.active" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="planOpen = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="planSaving" @click="submitPlan">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <!-- 优惠券编辑 -->
    <el-dialog v-model="couponOpen" :title="editingCouponId ? t('billing.coupon.edit') : t('billing.coupon.create')" width="min(520px, 92vw)" append-to-body :before-close="closeCouponDialog">
      <el-form ref="couponFormRef" :model="couponForm" :rules="couponRules" label-position="top">
        <el-form-item :label="t('billing.coupon.code')" prop="code"><el-input v-model="couponForm.code" maxlength="64" /></el-form-item>
        <el-form-item :label="t('billing.coupon.type')">
          <el-radio-group v-model="couponForm.type">
            <el-radio-button value="fixed">{{ t('billing.coupon.fixed') }}</el-radio-button>
            <el-radio-button value="percent">{{ t('billing.coupon.percent') }}</el-radio-button>
          </el-radio-group>
        </el-form-item>
        <el-form-item :label="couponForm.type === 'fixed' ? t('billing.coupon.valueFixed') : t('billing.coupon.valuePercent')">
          <el-input-number v-model="couponForm.value_yuan" :min="0" :max="couponForm.type === 'percent' ? 100 : undefined" />
        </el-form-item>
        <el-form-item :label="t('billing.coupon.minAmount')"><el-input-number v-model="couponForm.min_yuan" :min="0" /></el-form-item>
        <el-form-item :label="t('billing.coupon.maxUses')"><el-input-number v-model="couponForm.max_uses" :min="0" /></el-form-item>
        <el-form-item :label="t('billing.coupon.perUserLimit')"><el-input-number v-model="couponForm.per_user_limit" :min="0" /></el-form-item>
        <el-form-item :label="t('billing.coupon.expiresAt')">
          <el-date-picker v-model="couponForm.expires_at" type="datetime" value-format="YYYY-MM-DDTHH:mm:ssZ" :placeholder="t('billing.coupon.expiresPlaceholder')" />
        </el-form-item>
        <el-form-item :label="t('billing.coupon.active')"><el-switch v-model="couponForm.active" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="couponOpen = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="couponSaving" @click="submitCoupon">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <el-drawer
      v-model="orderDetailOpen"
      :title="t('billing.orderDetail.title')"
      size="min(440px, 92vw)"
      append-to-body
    >
      <template v-if="detailOrder">
        <div class="order-detail__head">
          <div>
            <p class="order-detail__plan">{{ detailOrder.plan_name }}</p>
            <p class="order-detail__hint">{{ detailOrder.id }}</p>
          </div>
          <el-tag
            size="small"
            :type="
              detailOrder.status === 'paid'
                ? 'success'
                : detailOrder.status === 'cancelled'
                  ? 'info'
                  : 'warning'
            "
            effect="plain"
          >
            {{ orderStatusLabel(detailOrder.status) }}
          </el-tag>
        </div>

        <dl class="order-detail__list">
          <div class="order-detail__row">
            <dt>{{ t('billing.orderDetail.amount') }}</dt>
            <dd>{{ priceLabel(detailOrder.amount_cents) }}</dd>
          </div>
          <div class="order-detail__row">
            <dt>{{ t('billing.orderDetail.discount') }}</dt>
            <dd>{{ priceLabel(detailOrder.discount_cents) }}</dd>
          </div>
          <div class="order-detail__row">
            <dt>{{ t('billing.orderDetail.provider') }}</dt>
            <dd>{{ detailOrder.provider || '—' }}</dd>
          </div>
          <div class="order-detail__row">
            <dt>{{ t('billing.orderDetail.userId') }}</dt>
            <dd class="ax-mono">{{ detailOrder.user_id }}</dd>
          </div>
          <div class="order-detail__row">
            <dt>{{ t('billing.orderDetail.tradeNo') }}</dt>
            <dd class="ax-mono">{{ detailOrder.trade_no || '—' }}</dd>
          </div>
          <div class="order-detail__row">
            <dt>{{ t('billing.orderDetail.paidAt') }}</dt>
            <dd>{{ detailOrder.paid_at ? formatDateTime(detailOrder.paid_at) : '—' }}</dd>
          </div>
          <div class="order-detail__row">
            <dt>{{ t('billing.orderDetail.createdAt') }}</dt>
            <dd>{{ formatDateTime(detailOrder.created_at) }}</dd>
          </div>
          <div class="order-detail__row">
            <dt>{{ t('billing.orderDetail.updatedAt') }}</dt>
            <dd>{{ formatDateTime(detailOrder.updated_at) }}</dd>
          </div>
        </dl>

        <div class="order-detail__actions">
          <el-button @click="orderDetailOpen = false">{{ t('common.close') }}</el-button>
          <el-button
            v-if="detailOrder.status === 'pending'"
            type="primary"
            @click="confirmOrder(detailOrder)"
          >
            {{ t('billing.actions.verifyOrder') }}
          </el-button>
        </div>
      </template>
    </el-drawer>

    <el-drawer
      v-model="ticketDetailOpen"
      :title="t('billing.ticketDetail.title')"
      size="min(520px, 92vw)"
      append-to-body
    >
      <el-skeleton v-if="ticketDetailLoading" :rows="5" animated />
      <template v-else-if="detailTicket">
        <div class="ticket-detail__head">
          <div>
            <p class="ticket-detail__subject">{{ detailTicket.subject }}</p>
            <p class="ticket-detail__hint">
              {{ detailTicket.username || detailTicket.user_id }} · {{ detailTicket.category }}
            </p>
          </div>
          <el-tag
            size="small"
            :type="
              detailTicket.status === 'open'
                ? 'warning'
                : detailTicket.status === 'answered'
                  ? 'success'
                  : 'info'
            "
            effect="plain"
          >
            {{ ticketStatusLabel(detailTicket.status) }}
          </el-tag>
        </div>

        <div class="ticket-thread">
          <article
            v-for="message in detailTicket.messages ?? []"
            :key="message.id"
            class="ticket-message"
            :class="{ 'is-admin': message.author_role === 'admin' }"
          >
            <header class="ticket-message__head">
              <span>{{ message.author_role === 'admin' ? t('billing.ticketDetail.admin') : t('billing.ticketDetail.user') }}</span>
              <span class="ticket-message__time">{{ formatDateTime(message.created_at) }}</span>
            </header>
            <p class="ticket-message__body">{{ message.body }}</p>
          </article>
          <p v-if="(detailTicket.messages ?? []).length === 0" class="ax-muted">
            {{ t('billing.ticketDetail.noMessages') }}
          </p>
        </div>

        <template v-if="detailTicket.status !== 'closed'">
          <el-input
            v-model="replyBody"
            type="textarea"
            :rows="3"
            maxlength="5000"
            :placeholder="t('billing.ticketDetail.replyPlaceholder')"
            class="ticket-reply"
          />
          <div class="ticket-detail__actions">
            <el-button :icon="Close" @click="closeTicket(detailTicket)">{{ t('billing.actions.closeTicket') }}</el-button>
            <el-button type="primary" :loading="replying" :disabled="!replyBody.trim()" @click="submitReply">
              {{ t('billing.actions.sendReply') }}
            </el-button>
          </div>
        </template>
        <div v-else class="ticket-detail__actions">
          <el-button @click="ticketDetailOpen = false">{{ t('common.close') }}</el-button>
        </div>
      </template>
    </el-drawer>
  </div>
</template>

<style scoped>
.table-card {
  overflow: hidden;
}

.form-hint {
  margin-left: var(--ax-space-2);
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
}

.list-pager {
  display: flex;
  justify-content: flex-end;
  margin-top: var(--ax-space-3);
}

@media (max-width: 768px) {
  .list-pager {
    justify-content: center;
  }
}

.order-detail__head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--ax-space-4);
  padding-bottom: var(--ax-space-4);
  border-bottom: 1px solid var(--ax-border-subtle);
}

.order-detail__plan {
  color: var(--ax-text);
  font-size: var(--ax-text-md);
  font-weight: var(--ax-weight-medium);
  overflow-wrap: anywhere;
}

.order-detail__hint {
  margin-top: var(--ax-space-1);
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
  overflow-wrap: anywhere;
}

.order-detail__list {
  display: flex;
  flex-direction: column;
  margin: var(--ax-space-2) 0 0;
}

.order-detail__row {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--ax-space-4);
  padding: var(--ax-space-3) 0;
  border-bottom: 1px solid var(--ax-border-subtle);
}

.order-detail__row:last-child {
  border-bottom: none;
}

.order-detail__row dt {
  flex: none;
  color: var(--ax-text-3);
  font-size: var(--ax-text-sm);
}

.order-detail__row dd {
  margin: 0;
  min-width: 0;
  color: var(--ax-text-2);
  font-size: var(--ax-text-sm);
  text-align: right;
  overflow-wrap: anywhere;
}

.order-detail__actions {
  display: flex;
  justify-content: flex-end;
  gap: var(--ax-space-2);
  margin-top: var(--ax-space-5);
}

.ticket-detail__head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--ax-space-4);
  padding-bottom: var(--ax-space-4);
  border-bottom: 1px solid var(--ax-border-subtle);
}

.ticket-detail__subject {
  color: var(--ax-text);
  font-size: var(--ax-text-md);
  font-weight: var(--ax-weight-medium);
  overflow-wrap: anywhere;
}

.ticket-detail__hint {
  margin-top: var(--ax-space-1);
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
}

.ticket-thread {
  display: flex;
  flex-direction: column;
  gap: var(--ax-space-2);
  margin: var(--ax-space-4) 0;
}

.ticket-message {
  padding: var(--ax-space-3);
  background: var(--ax-tint-weak);
  border: 1px solid var(--ax-border-subtle);
  border-radius: var(--ax-radius-md);
}

.ticket-message.is-admin {
  background: var(--ax-accent-soft);
  border-color: var(--ax-accent-ring);
}

.ticket-message__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--ax-space-3);
  color: var(--ax-text-3);
  font-size: var(--ax-text-xs);
  font-weight: var(--ax-weight-medium);
}

.ticket-message__time {
  color: var(--ax-text-4);
  font-weight: var(--ax-weight-regular);
}

.ticket-message__body {
  margin-top: var(--ax-space-2);
  color: var(--ax-text-2);
  font-size: var(--ax-text-sm);
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.ticket-reply {
  margin-top: var(--ax-space-2);
}

.ticket-detail__actions {
  display: flex;
  justify-content: flex-end;
  gap: var(--ax-space-2);
  margin-top: var(--ax-space-4);
}
</style>
