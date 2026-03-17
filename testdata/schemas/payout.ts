import { z } from 'zod';

export const PayoutSchema = z.object({
  id: z.string().uuid(),
  amount: z.number().positive(),
  currency: z.enum(['USD', 'EUR', 'GBP', 'DKK']),
  status: z.enum(['pending', 'processing', 'completed', 'failed']),
  recipientId: z.string().uuid(),
  createdAt: z.string().describe('ISO 8601 timestamp'),
});

export const CreatePayoutSchema = z.object({
  amount: z.number().positive(),
  currency: z.enum(['USD', 'EUR', 'GBP', 'DKK']),
  recipientId: z.string().uuid().describe('The user ID to pay out to'),
});

export const PayoutListQuerySchema = z.object({
  status: z.enum(['pending', 'processing', 'completed', 'failed']).optional(),
  limit: z.number().int().min(1).max(100).default(20),
  offset: z.number().int().min(0).default(0),
});
