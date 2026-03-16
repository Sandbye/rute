import { z } from 'zod';

export const NotFoundSchema = z.object({
  error: z.string(),
  message: z.string(),
  statusCode: z.number().int(),
});

export const ValidationErrorSchema = z.object({
  error: z.string(),
  issues: z.array(z.object({
    field: z.string(),
    message: z.string(),
  })),
});
