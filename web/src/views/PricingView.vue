<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { Refresh } from '@element-plus/icons-vue'

import { createOrder, listOrders, listPaymentGateways, listPlans, payOrder, validateCoupon } from '@/api/billing'
import { toApiError } from '@/api/client'
import type { Order, Plan } from '@/api/types'
import EmptyState from '@/components/EmptyState.vue'
import ErrorState from '@/components/ErrorState.vue'
import PageHeader from '@/components/PageHeader.vue'
import { formatBytes, formatDateTime } from '@/utils/format'

const { t } = useI18n()

const plans = ref<Plan[]>([])
const orders = ref<Order[]>([])
const loading = ref(false)
const errorMessage = ref('')

const couponCode = ref('')
const couponPlanId = ref('')
const couponDiscount = ref(0)
const couponChecking = ref(false)
const ordering = ref('')
const gateways = ref<string[]>([])
const provider = ref('manual')

const providerLabels = computed<Record<string, string>>(() => ({
  manual: t('pricing.providers.manual'),
  mock: t('pricing.providers.mock'),
  alipay: t('pricing.providers.alipay'),
  wechat: t('pricing.providers.wechat'),
  epay: t('pricing.providers.epay'),
}))

const priceLabel = (cents: number): string => (cents === 0 ? t('pricing.free') : `¥${(cents / 100).toFixed(2)}`)

const selectedPlan = computed(() => plans.value.find((p) => p.id === couponPlanId.value) ?? null)
const payableLabel = computed(() => {
  if (!selectedPlan.value) return ''
  const payable = Math.max(0, selectedPlan.value.price_cents - couponDiscount.value)
  return priceLabel(payable)
})

async function load(): Promise<void> {
  loading.value = true
  errorMessage.value = ''
  try {
    const [planList, orderList, gatewayList] = await Promise.all([
      listPlans(),
      listOrders(),
      listPaymentGateways().catch(() => ['manual']),
    ])
    plans.value = planList ?? []
    orders.value = orderList ?? []
    gateways.value = gatewayList ?? []
    if (gateways.value.length > 0 && !gateways.value.includes(provider.value)) {
      provider.value = gateways.value[0]!
    }
  } catch (error) {
    errorMessage.value = toApiError(error).message
  } finally {
    loading.value = false
  }
}

async function checkCoupon(plan: Plan): Promise<void> {
  if (!couponCode.value.trim()) {
    ElMessage.warning(t('pricing.messages.couponRequired'))
    return
  }
  couponChecking.value = true
  try {
    const result = await validateCoupon(couponCode.value.trim(), plan.price_cents)
    couponPlanId.value = plan.id
    couponDiscount.value = result.discount_cents
    ElMessage.success(t('pricing.messages.couponApplied', { amount: priceLabel(result.discount_cents) }))
  } catch (error) {
    couponPlanId.value = ''
    couponDiscount.value = 0
    ElMessage.error(toApiError(error).message)
  } finally {
    couponChecking.value = false
  }
}

async function buy(plan: Plan): Promise<void> {
  ordering.value = plan.id
  try {
    const coupon = couponPlanId.value === plan.id ? couponCode.value.trim() : ''
    const order = await createOrder(plan.id, coupon, provider.value)
    if (order.status === 'paid') {
      ElMessage.success(t('pricing.messages.planActivated'))
    } else if (order.status === 'pending' && order.pay_url && order.provider === 'mock') {
      // 仅模拟渠道可自助完成支付；真实渠道与人工核销需等待支付/管理员确认。
      await payOrder(order.id)
      ElMessage.success(t('pricing.messages.paidActivated'))
    } else {
      ElMessage.success(t('pricing.messages.orderCreated'))
    }
    couponDiscount.value = 0
    couponPlanId.value = ''
    await load()
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    ordering.value = ''
  }
}

function statusLabel(status: string): string {
  switch (status) {
    case 'paid':
      return t('pricing.status.paid')
    case 'cancelled':
      return t('pricing.status.cancelled')
    default:
      return t('pricing.status.pending')
  }
}

onMounted(load)
</script>

<template>
  <div class="ax-page">
    <PageHeader :title="t('pricing.title')" :description="t('pricing.description')">
      <template #actions>
        <el-button :icon="Refresh" :loading="loading" @click="load">{{ t('common.refresh') }}</el-button>
      </template>
    </PageHeader>

    <ErrorState v-if="errorMessage" :message="errorMessage" @retry="load" />

    <div v-else-if="loading && plans.length === 0" class="ax-grid" aria-busy="true">
      <div v-for="i in 3" :key="i" class="ax-card"><div class="ax-card__body"><el-skeleton :rows="4" animated /></div></div>
    </div>

    <EmptyState
      v-else-if="plans.length === 0"
      :title="t('pricing.empty.title')"
      :description="t('pricing.empty.description')"
    />

    <template v-else>
      <section class="ax-grid plan-grid">
        <article v-for="plan in plans" :key="plan.id" class="plan-card ax-card">
          <div class="ax-card__body">
            <h2 class="plan-card__name">{{ plan.name }}</h2>
            <p class="plan-card__price">
              {{ priceLabel(plan.price_cents) }}
              <span v-if="plan.duration_days" class="plan-card__period">{{ t('pricing.perDays', { days: plan.duration_days }) }}</span>
            </p>
            <p v-if="plan.description" class="plan-card__desc">{{ plan.description }}</p>
            <ul class="plan-card__features">
              <li>{{ t('pricing.storageQuota', { size: plan.quota_mb > 0 ? formatBytes(plan.quota_mb * 1024 * 1024) : t('pricing.unlimited') }) }}</li>
              <li v-if="plan.role_group_id">{{ t('pricing.roleGroupFeature') }}</li>
              <li>{{ plan.duration_days > 0 ? t('pricing.validityDays', { days: plan.duration_days }) : t('pricing.validityForever') }}</li>
            </ul>
            <div class="plan-card__actions">
              <el-button type="primary" :loading="ordering === plan.id" @click="buy(plan)">
                {{ plan.price_cents === 0 ? t('pricing.getFree') : t('pricing.buy') }}
              </el-button>
            </div>
          </div>
        </article>
      </section>

      <section class="ax-card coupon-card">
        <div class="ax-card__body">
          <h2 class="ax-card__title">{{ t('pricing.coupon.title') }}</h2>
          <div class="coupon-row">
            <el-input v-model="couponCode" :placeholder="t('pricing.coupon.codePlaceholder')" class="coupon-input" />
            <el-select v-model="couponPlanId" :placeholder="t('pricing.coupon.planPlaceholder')" class="coupon-select">
              <el-option v-for="plan in plans" :key="plan.id" :label="plan.name" :value="plan.id" />
            </el-select>
            <el-button
              :loading="couponChecking"
              :disabled="!selectedPlan"
              @click="selectedPlan && checkCoupon(selectedPlan)"
            >
              {{ t('pricing.coupon.check') }}
            </el-button>
            <el-select v-model="provider" class="coupon-select" :aria-label="t('pricing.coupon.providerAria')">
              <el-option
                v-for="g in gateways"
                :key="g"
                :label="providerLabels[g] ?? g"
                :value="g"
              />
            </el-select>
          </div>
          <p v-if="couponDiscount > 0" class="coupon-hint">
            {{ t('pricing.coupon.applied', { discount: priceLabel(couponDiscount), payable: payableLabel }) }}
          </p>
        </div>
      </section>

      <section class="ax-card table-card">
        <header class="ax-card__head"><h2 class="ax-card__title">{{ t('pricing.orders.title') }}</h2></header>
        <div v-if="orders.length === 0" class="ax-card__body ax-muted">{{ t('pricing.orders.empty') }}</div>
        <div v-else class="ax-table-scroll">
          <el-table :data="orders" style="width: 100%">
            <el-table-column prop="plan_name" :label="t('pricing.orders.plan')" min-width="160" />
            <el-table-column :label="t('pricing.orders.amount')" width="120">
              <template #default="{ row }">{{ priceLabel(row.amount_cents) }}</template>
            </el-table-column>
            <el-table-column :label="t('pricing.orders.status')" width="100">
              <template #default="{ row }">
                <el-tag size="small" :type="row.status === 'paid' ? 'success' : 'info'" effect="plain">
                  {{ statusLabel(row.status) }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column :label="t('pricing.orders.createdAt')" min-width="170">
              <template #default="{ row }">{{ formatDateTime(row.created_at) }}</template>
            </el-table-column>
          </el-table>
        </div>
      </section>
    </template>
  </div>
</template>

<style scoped>
.plan-grid {
  margin-bottom: var(--ax-space-4);
}

.plan-card__name {
  margin: 0 0 var(--ax-space-2);
  color: var(--ax-text);
  font-size: var(--ax-text-lg);
}

.plan-card__price {
  margin: 0 0 var(--ax-space-3);
  color: var(--ax-accent-hover);
  font-size: var(--ax-text-2xl);
  font-weight: var(--ax-weight-semibold);
}

.plan-card__period {
  color: var(--ax-text-4);
  font-size: var(--ax-text-sm);
  font-weight: normal;
}

.plan-card__desc {
  margin: 0 0 var(--ax-space-3);
  color: var(--ax-text-3);
  font-size: var(--ax-text-sm);
}

.plan-card__features {
  margin: 0 0 var(--ax-space-4);
  padding-left: var(--ax-space-4);
  color: var(--ax-text-3);
  font-size: var(--ax-text-sm);
}

.plan-card__actions {
  display: flex;
}

.plan-card__actions :deep(.el-button) {
  flex: 1;
}

.coupon-card {
  margin-bottom: var(--ax-space-4);
}

.coupon-row {
  display: flex;
  flex-wrap: wrap;
  gap: var(--ax-space-2);
}

.coupon-input {
  width: 220px;
}

.coupon-select {
  width: 200px;
}

.coupon-hint {
  margin: var(--ax-space-2) 0 0;
  color: var(--ax-success);
  font-size: var(--ax-text-sm);
}

.table-card {
  overflow: hidden;
}
</style>
