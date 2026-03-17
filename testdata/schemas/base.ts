import { z } from 'zod';

export const BaseEntitySchema = z.object({
  id: z.string().uuid(),
  createdAt: z.string(),
  updatedAt: z.string(),
});

export const TimestampSchema = z.object({
  timestamp: z.string(),
  timezone: z.string().optional(),
});
