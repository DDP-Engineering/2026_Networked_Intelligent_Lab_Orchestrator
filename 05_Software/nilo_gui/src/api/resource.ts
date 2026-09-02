import type { BenchCoreFailure } from './client';

/**
 * One resource the interface asked Bench Core for: either the value it
 * returned, or why that particular resource could not be used.
 *
 * Keeping the failure per resource is what lets an unusable integration
 * inventory be told from an unusable device inventory without blanking the rest
 * of the screen.
 */
export type Resource<T> =
  { ok: true; value: T } | { ok: false; failure: BenchCoreFailure; detail: string };
