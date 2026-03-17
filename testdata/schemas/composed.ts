import { z } from 'zod';
import { BaseEntitySchema, TimestampSchema } from './base';

// .extend() — adds fields to a base schema
export const ProductSchema = BaseEntitySchema.extend({
  name: z.string().min(1),
  price: z.number().positive(),
  sku: z.string(),
});

// .merge() — merges two schemas
export const AuditedProductSchema = ProductSchema.merge(TimestampSchema);

// .pick() — keep only selected fields
export const ProductSummarySchema = ProductSchema.pick({ id: true, name: true, price: true });

// .omit() — remove specific fields
export const CreateProductSchema = ProductSchema.omit({ id: true, createdAt: true, updatedAt: true });

// .partial() — make all fields optional
export const PatchProductSchema = CreateProductSchema.partial();

// Chained: extend + pick
export const ProductNameOnlySchema = BaseEntitySchema.extend({
  name: z.string(),
}).pick({ name: true });
