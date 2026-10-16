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
import { describe, expect, test } from 'vitest'

import {
  CHANNEL_PROVIDER_PRESENTATION,
  CHANNEL_TYPE_CODEBUDDY,
  CHANNEL_TYPE_OPTIONS,
  CHANNEL_TYPES,
  MODEL_FETCHABLE_TYPES,
} from '../../constants'
import { CHANNEL_TYPE_DEFAULTS } from '../channel-type-defaults'
import { getChannelTypeIcon } from '../channel-utils'

describe('CodeBuddy channel', () => {
  test('registers the channel type, ordering, model discovery, and icon metadata', () => {
    expect(CHANNEL_TYPE_CODEBUDDY).toBe(93)
    expect(CHANNEL_TYPES[CHANNEL_TYPE_CODEBUDDY]).toBe('CodeBuddy')
    expect(
      CHANNEL_TYPE_OPTIONS.find(
        (item) => item.value === CHANNEL_TYPE_CODEBUDDY
      )
    ).toStrictEqual({ value: CHANNEL_TYPE_CODEBUDDY, label: 'CodeBuddy' })
    expect(MODEL_FETCHABLE_TYPES.has(CHANNEL_TYPE_CODEBUDDY)).toBe(true)
    expect(getChannelTypeIcon(CHANNEL_TYPE_CODEBUDDY)).toBe('CodeBuddy')
    expect(
      CHANNEL_PROVIDER_PRESENTATION[CHANNEL_TYPE_CODEBUDDY]?.descriptionKey
    ).toBe('Connect to CodeBuddy model services')
  })

  test('declares no default base URL', () => {
    expect(
      CHANNEL_TYPE_DEFAULTS[CHANNEL_TYPE_CODEBUDDY]?.base_url
    ).toBeUndefined()
  })
})
