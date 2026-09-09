import { describe, expect, it } from 'vitest'
import { renderPaymentHelp } from '../presentation/paymentHelpHtml'

describe('payment help Markdown', () => {
  it('formats help links and blocks executable or interactive markup', () => {
    const html = renderPaymentHelp('**Help** [Guide](https://example.com/help)\n\n<script>alert(1)</script><form><input><button>Send</button></form><img src=x onerror=alert(2)>')
    expect(html).toContain('<strong>Help</strong>')
    expect(html).toContain('href="https://example.com/help"')
    expect(html).not.toMatch(/<script|onerror|<form|<input|<button/i)
    expect(renderPaymentHelp('[click](javascript:alert(1))')).not.toContain('href="javascript:')
  })
})
