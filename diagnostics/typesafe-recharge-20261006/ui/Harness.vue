<script setup lang="ts">
import { computed, ref } from 'vue'
import TierEditor from '@/features/admin-settings/presentation/widgets/RechargeBonusTierEditor.vue'
import AmountInput from '@/features/billing/presentation/widgets/AmountInput.vue'
import UseKeyDialog from '@/features/keys/presentation/widgets/UseKeyDialog.vue'
import { quoteRechargeBonus, type RechargeBonusTierDraft, type RechargeBonusMode } from '@/features/billing/rechargeBonus'
const tiers=ref<RechargeBonusTierDraft[]>([{min_amount:10,bonus_percent:10},{min_amount:100,bonus_percent:20}])
const mode=ref<RechargeBonusMode>('bonus'),notice=ref(''),amount=ref(100),showKey=ref(false)
const cleanTiers=computed(()=>tiers.value.filter(t=>t.min_amount!==null&&t.bonus_percent!==null).map(t=>({min_amount:t.min_amount!,bonus_percent:t.bonus_percent!})))
const quote=computed(()=>quoteRechargeBonus(cleanTiers.value,amount.value,{multiplier:1,mode:mode.value,currencyDigits:2}))
</script>
<template>
 <main class="mx-auto max-w-6xl space-y-5 p-4 sm:p-8">
  <h1 class="text-xl font-semibold">TypeSafe 与充值阶梯 · 组件验收</h1>
  <button class="btn btn-primary" @click="showKey=true">查看 TypeSafe 使用说明</button>
  <section class="grid gap-6 md:grid-cols-2">
   <div class="card p-5"><TierEditor v-model="tiers" v-model:mode="mode" v-model:notice="notice" /></div>
   <div class="card space-y-5 p-5"><h2 class="text-lg font-semibold">用户充值预览</h2><AmountInput v-model="amount" :min="1" :max="1000" :bonus-tiers="cleanTiers" :bonus-mode="mode" :multiplier="1" currency="USD" />
    <p>实付 {{quote.payBase}} USD · 到账 {{quote.credited}} USD · 赠送 {{quote.bonus}} USD</p>
   </div>
  </section>
  <UseKeyDialog :show="showKey" platform="typesafe" api-key="fixture-key" base-url="https://example.com/v1" @close="showKey=false" />
 </main>
</template>
