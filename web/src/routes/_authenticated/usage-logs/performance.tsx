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
import { createFileRoute, redirect } from '@tanstack/react-router'
import z from 'zod'

import { PerformanceHealth } from '@/features/performance-health'
import {
  TIME_RANGE_PRESETS,
  STREAM_FILTERS,
} from '@/features/performance-health/lib/search'
import { PERFORMANCE_GRANULARITIES } from '@/features/performance-health/types'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

const performanceSearchSchema = z.object({
  preset: z.enum(TIME_RANGE_PRESETS).optional(),
  startTime: z.number().optional(),
  endTime: z.number().optional(),
  granularity: z.enum(PERFORMANCE_GRANULARITIES).optional().catch('auto'),
  model: z.string().optional().catch(''),
  channel: z.string().optional().catch(''),
  group: z.string().optional().catch(''),
  token: z.string().optional().catch(''),
  username: z.string().optional().catch(''),
  stream: z.enum(STREAM_FILTERS).optional().catch('all'),
})

export const Route = createFileRoute('/_authenticated/usage-logs/performance')({
  beforeLoad: () => {
    const { auth } = useAuthStore.getState()

    if (!auth.user || auth.user.role < ROLE.ADMIN) {
      throw redirect({
        to: '/403',
      })
    }
  },
  validateSearch: performanceSearchSchema,
  component: PerformanceHealth,
})
