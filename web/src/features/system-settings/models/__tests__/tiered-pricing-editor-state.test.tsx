/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import assert from 'node:assert/strict'
import { after, describe, test } from 'node:test'

import { Window } from 'happy-dom'

const domWindow = new Window()
const domGlobals = [
  'window',
  'document',
  'navigator',
  'HTMLElement',
  'HTMLInputElement',
  'SVGElement',
  'Node',
  'Element',
  'Event',
  'CustomEvent',
  'MutationObserver',
  'ResizeObserver',
  'requestAnimationFrame',
  'cancelAnimationFrame',
  'getComputedStyle',
] as const

for (const key of domGlobals) {
  Object.defineProperty(globalThis, key, {
    configurable: true,
    value: domWindow[key],
  })
}

const { act, useState } = await import('react')
const { createRoot } = await import('react-dom/client')
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { TieredPricingEditor } = await import('../tiered-pricing-editor')

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { translation: {} } },
})

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

describe('tiered pricing editor state', () => {
  after(() => {
    domWindow.close()
  })

  test('preserves a saved request-aware expression when the editor remounts', async () => {
    const expression =
      'param("resolution") == "720p" ? tier("720p", number(param("duration")) * 670000) : tier("fallback", 3350000)'
    const observedExpressions: string[] = []
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)

    function Harness() {
      const [billingExpr, setBillingExpr] = useState(expression)
      return (
        <I18nextProvider i18n={i18n}>
          <TieredPricingEditor
            modelName='yu-video-2-pro'
            billingExpr={billingExpr}
            requestRuleExpr=''
            onBillingExprChange={(next) => {
              observedExpressions.push(next)
              setBillingExpr(next)
            }}
            onRequestRuleExprChange={() => {}}
          />
        </I18nextProvider>
      )
    }

    await act(async () => {
      root.render(<Harness />)
    })

    const expressionEditor = container.querySelector<HTMLTextAreaElement>(
      'textarea[spellcheck="false"]'
    )
    assert.ok(expressionEditor)
    assert.equal(expressionEditor.value, expression)
    assert.deepEqual(observedExpressions, [])

    await act(async () => root.unmount())
    container.remove()
  })
})
