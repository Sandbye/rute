import { z } from 'zod';

export const UserParamsSchema = z.object({
  id: z.string().uuid(),
});

export const UserResponseSchema = z.object({
  id: z.string().uuid(),
  email: z.string().email(),
  age: z.number().min(18).max(120).optional(),
  role: z.enum(['admin', 'user', 'guest']),
  createdAt: z.string().describe('ISO 8601 timestamp'),
});

export const CreateUserSchema = z.object({
  email: z.string().email(),
  password: z.string().min(8).max(128),
  role: z.enum(['admin', 'user', 'guest']).default('user'),
  profile: z.object({
    name: z.string().min(1).max(100),
    bio: z.string().max(500).optional(),
  }).optional(),
});
