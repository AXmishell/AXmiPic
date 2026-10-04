<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import type { FormInstance, FormRules } from 'element-plus'
import { Plus, Refresh } from '@element-plus/icons-vue'

import {
  adminListCoupons,
  adminListPlans,
  createCoupon,
  createPlan,
  deleteCoupon,
  deletePlan,
  listOrders,
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
import { listTickets } from '@/api/billing'
import EmptyState from '@/components/EmptyState.vue'
import ErrorState from '@/components/ErrorState.vue'
import PageHeader from '@/components/PageHeader.vue'
import { formatDateTime } from '@/utils/format'

type Tab = 'plans' | 'coupons' | 'orders' | 'tickets'
const activeTab = ref<Tab>('plans')

const plans = ref<Plan[]>([])
const coupons = ref<Coupon[]>([])
const orders = ref<Order[]>([])
const tickets = ref<Ticket[]>([])
const roleGroups = ref<RoleGroup[]>([])
const loading = ref(false)
const errorMessage = ref('')

const priceLabel = (cents: number): string => (cents === 0 ? '免费' : `¥${(cents / 100).toFixed(2)}`)

async function load(): Promise<void> {
  loading.value = true
  errorMessage.value = ''
  try {
    const [planList, couponList, orderList, ticketList, groups] = await Promise.all([
      adminListPlans(),
      adminListCoupons(),
      listOrders(),
      listTickets(),
      listRoleGroups().catch(() => []),
    ])
    plans.value = planList ?? []
    coupons.value = couponList ?? []
    orders.value = orderList ?? []
    tickets.value = ticketList ?? []
    roleGroups.value = groups ?? []
  } catch (error) {
    errorMessage.value = toApiError(error).message
  } finally {
    loading.value = false
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
const planRules: FormRules = { name: [{ required: true, message: '请输入套餐名称', trigger: 'blur' }] }

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
    ElMessage.success('套餐已保存')
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
    await ElMessageBox.confirm(`将删除套餐「${plan.name}」。`, '删除套餐', {
      confirmButtonText: '删除',
      cancelButtonText: '取消',
      type: 'warning',
      confirmButtonClass: 'el-button--danger',
    })
  } catch {
    return
  }
  try {
    await deletePlan(plan.id)
    ElMessage.success('套餐已删除')
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
const couponRules: FormRules = { code: [{ required: true, message: '请输入优惠券代码', trigger: 'blur' }] }

function openCreateCoupon(): void {
  editingCouponId.value = ''
  Object.assign(couponForm, { code: '', type: 'fixed', value_yuan: 0, min_yuan: 0, max_uses: 0, per_user_limit: 0, expires_at: '', active: true })
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
    ElMessage.success('优惠券已保存')
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
    await ElMessageBox.confirm(`将删除优惠券「${coupon.code}」。`, '删除优惠券', {
      confirmButtonText: '删除',
      cancelButtonText: '取消',
      type: 'warning',
      confirmButtonClass: 'el-button--danger',
    })
  } catch {
    return
  }
  try {
    await deleteCoupon(coupon.id)
    ElMessage.success('优惠券已删除')
    await load()
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  }
}

function couponValueLabel(coupon: Coupon): string {
  return coupon.type === 'fixed' ? `减 ¥${(coupon.value / 100).toFixed(2)}` : `减 ${coupon.value}%`
}

// ---- 工单 ----
async function closeTicket(ticket: Ticket): Promise<void> {
  try {
    await setTicketStatus(ticket.id, 'closed')
    ElMessage.success('工单已关闭')
    await load()
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  }
}

onMounted(load)
</script>

<template>
  <div class="ax-page">
    <PageHeader title="计费管理" description="管理套餐、优惠券、订单与工单。">
      <template #actions>
        <el-button :icon="Refresh" :loading="loading" @click="load">刷新</el-button>
        <el-button v-if="activeTab === 'plans'" type="primary" :icon="Plus" @click="openCreatePlan">新建套餐</el-button>
        <el-button v-else-if="activeTab === 'coupons'" type="primary" :icon="Plus" @click="openCreateCoupon">新建优惠券</el-button>
      </template>
    </PageHeader>

    <ErrorState v-if="errorMessage" :message="errorMessage" @retry="load" />

    <el-tabs v-else v-model="activeTab">
      <el-tab-pane :label="`套餐 (${plans.length})`" name="plans">
        <EmptyState v-if="plans.length === 0" title="暂无套餐" description="点击「新建套餐」创建套餐。" />
        <div v-else class="ax-card table-card">
          <div class="ax-table-scroll">
            <el-table :data="plans" style="width: 100%">
              <el-table-column prop="name" label="名称" min-width="150" />
              <el-table-column label="价格" width="110">
                <template #default="{ row }">{{ priceLabel(row.price_cents) }}</template>
              </el-table-column>
              <el-table-column label="配额" width="120">
                <template #default="{ row }">{{ row.quota_mb > 0 ? `${row.quota_mb} MiB` : '不限' }}</template>
              </el-table-column>
              <el-table-column label="有效期" width="100">
                <template #default="{ row }">{{ row.duration_days > 0 ? `${row.duration_days} 天` : '永久' }}</template>
              </el-table-column>
              <el-table-column label="状态" width="90">
                <template #default="{ row }">
                  <el-tag size="small" :type="row.active ? 'success' : 'info'" effect="plain">
                    {{ row.active ? '启用' : '停用' }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column label="操作" width="150" align="right">
                <template #default="{ row }">
                  <el-button link type="primary" @click="openEditPlan(row)">编辑</el-button>
                  <el-button link type="danger" @click="removePlan(row)">删除</el-button>
                </template>
              </el-table-column>
            </el-table>
          </div>
        </div>
      </el-tab-pane>

      <el-tab-pane :label="`优惠券 (${coupons.length})`" name="coupons">
        <EmptyState v-if="coupons.length === 0" title="暂无优惠券" description="创建优惠券后用户可在下单时使用。" />
        <div v-else class="ax-card table-card">
          <div class="ax-table-scroll">
            <el-table :data="coupons" style="width: 100%">
              <el-table-column prop="code" label="代码" min-width="140" />
              <el-table-column label="优惠" width="120">
                <template #default="{ row }">{{ couponValueLabel(row) }}</template>
              </el-table-column>
              <el-table-column label="使用情况" width="120">
                <template #default="{ row }">
                  {{ row.used }}{{ row.max_uses > 0 ? ` / ${row.max_uses}` : '' }}
                </template>
              </el-table-column>
              <el-table-column label="状态" width="90">
                <template #default="{ row }">
                  <el-tag size="small" :type="row.active ? 'success' : 'info'" effect="plain">
                    {{ row.active ? '启用' : '停用' }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column label="操作" width="150" align="right">
                <template #default="{ row }">
                  <el-button link type="primary" @click="openEditCoupon(row)">编辑</el-button>
                  <el-button link type="danger" @click="removeCoupon(row)">删除</el-button>
                </template>
              </el-table-column>
            </el-table>
          </div>
        </div>
      </el-tab-pane>

      <el-tab-pane :label="`订单 (${orders.length})`" name="orders">
        <EmptyState v-if="orders.length === 0" title="暂无订单" description="用户下单后会显示在这里。" />
        <div v-else class="ax-card table-card">
          <div class="ax-table-scroll">
            <el-table :data="orders" style="width: 100%">
              <el-table-column prop="plan_name" label="套餐" min-width="150" />
              <el-table-column label="金额" width="110">
                <template #default="{ row }">{{ priceLabel(row.amount_cents) }}</template>
              </el-table-column>
              <el-table-column label="渠道" width="100">
                <template #default="{ row }">{{ row.provider }}</template>
              </el-table-column>
              <el-table-column label="状态" width="90">
                <template #default="{ row }">
                  <el-tag size="small" :type="row.status === 'paid' ? 'success' : 'info'" effect="plain">
                    {{ row.status === 'paid' ? '已支付' : row.status === 'cancelled' ? '已取消' : '待支付' }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column label="创建时间" min-width="170">
                <template #default="{ row }">{{ formatDateTime(row.created_at) }}</template>
              </el-table-column>
            </el-table>
          </div>
        </div>
      </el-tab-pane>

      <el-tab-pane :label="`工单 (${tickets.length})`" name="tickets">
        <EmptyState v-if="tickets.length === 0" title="暂无工单" description="用户提交工单后会显示在这里。" />
        <div v-else class="ax-card table-card">
          <div class="ax-table-scroll">
            <el-table :data="tickets" style="width: 100%">
              <el-table-column prop="subject" label="标题" min-width="200" />
              <el-table-column label="用户" width="140">
                <template #default="{ row }">{{ row.username || row.user_id }}</template>
              </el-table-column>
              <el-table-column label="状态" width="100">
                <template #default="{ row }">
                  <el-tag size="small" :type="row.status === 'open' ? 'warning' : row.status === 'answered' ? 'success' : 'info'" effect="plain">
                    {{ row.status === 'open' ? '待处理' : row.status === 'answered' ? '已回复' : '已关闭' }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column label="更新时间" min-width="170">
                <template #default="{ row }">{{ formatDateTime(row.updated_at) }}</template>
              </el-table-column>
              <el-table-column label="操作" width="100" align="right">
                <template #default="{ row }">
                  <el-button link type="info" :disabled="row.status === 'closed'" @click="closeTicket(row)">关闭</el-button>
                </template>
              </el-table-column>
            </el-table>
          </div>
        </div>
      </el-tab-pane>
    </el-tabs>

    <!-- 套餐编辑 -->
    <el-dialog v-model="planOpen" :title="editingPlanId ? '编辑套餐' : '新建套餐'" width="min(520px, 92vw)" append-to-body>
      <el-form ref="planFormRef" :model="planForm" :rules="planRules" label-position="top">
        <el-form-item label="名称" prop="name"><el-input v-model="planForm.name" maxlength="64" /></el-form-item>
        <el-form-item label="简介"><el-input v-model="planForm.description" type="textarea" :rows="2" maxlength="255" /></el-form-item>
        <el-form-item label="价格（元）">
          <el-input-number v-model="planForm.price_cents" :min="0" />
          <span class="form-hint">此处直接填写分，例如 1000 表示 ¥10.00</span>
        </el-form-item>
        <el-form-item label="有效期（天，0 为永久）"><el-input-number v-model="planForm.duration_days" :min="0" /></el-form-item>
        <el-form-item label="存储配额（MiB，0 为不限）"><el-input-number v-model="planForm.quota_mb" :min="0" /></el-form-item>
        <el-form-item label="应用角色组">
          <el-select v-model="planForm.role_group_id" clearable placeholder="不改变角色组" style="width: 100%">
            <el-option v-for="g in roleGroups" :key="g.id" :label="g.name" :value="g.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="排序"><el-input-number v-model="planForm.sort_order" :min="0" /></el-form-item>
        <el-form-item label="启用"><el-switch v-model="planForm.active" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="planOpen = false">取消</el-button>
        <el-button type="primary" :loading="planSaving" @click="submitPlan">保存</el-button>
      </template>
    </el-dialog>

    <!-- 优惠券编辑 -->
    <el-dialog v-model="couponOpen" :title="editingCouponId ? '编辑优惠券' : '新建优惠券'" width="min(520px, 92vw)" append-to-body>
      <el-form ref="couponFormRef" :model="couponForm" :rules="couponRules" label-position="top">
        <el-form-item label="代码" prop="code"><el-input v-model="couponForm.code" maxlength="64" /></el-form-item>
        <el-form-item label="类型">
          <el-radio-group v-model="couponForm.type">
            <el-radio-button value="fixed">固定金额</el-radio-button>
            <el-radio-button value="percent">百分比</el-radio-button>
          </el-radio-group>
        </el-form-item>
        <el-form-item :label="couponForm.type === 'fixed' ? '减免（元）' : '减免百分比（%）'">
          <el-input-number v-model="couponForm.value_yuan" :min="0" :max="couponForm.type === 'percent' ? 100 : undefined" />
        </el-form-item>
        <el-form-item label="使用门槛（元）"><el-input-number v-model="couponForm.min_yuan" :min="0" /></el-form-item>
        <el-form-item label="最大使用次数（0 为不限）"><el-input-number v-model="couponForm.max_uses" :min="0" /></el-form-item>
        <el-form-item label="每用户限用次数（0 为不限）"><el-input-number v-model="couponForm.per_user_limit" :min="0" /></el-form-item>
        <el-form-item label="过期时间（可选）">
          <el-date-picker v-model="couponForm.expires_at" type="datetime" value-format="YYYY-MM-DDTHH:mm:ssZ" placeholder="不填表示不过期" />
        </el-form-item>
        <el-form-item label="启用"><el-switch v-model="couponForm.active" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="couponOpen = false">取消</el-button>
        <el-button type="primary" :loading="couponSaving" @click="submitCoupon">保存</el-button>
      </template>
    </el-dialog>
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
</style>
