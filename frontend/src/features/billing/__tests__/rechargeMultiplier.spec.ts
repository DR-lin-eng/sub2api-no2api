import { describe, expect, it } from 'vitest'
import { formatBalanceRechargeMultiplier, quoteRechargeBonus } from '../rechargeBonus'

 describe('six decimal recharge multiplier', () => {
  it.each([[0.140845, '0.140845'], [0.5, '0.50'], [1, '1.00'], [0.000001, '0.000001']])('shows %s without truncation', (value, text) => {
    expect(formatBalanceRechargeMultiplier(Number(value))).toBe(text)
  })
  it('keeps the credited balance at currency precision', () => {
    expect(quoteRechargeBonus([], 100, { multiplier: 0.140845 }).credited).toBe(14.08)
  })
 })
