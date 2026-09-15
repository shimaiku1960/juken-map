import { logger } from "@/api/observability/logger";

export async function measured<T>(
  name: string,
  fn: () => Promise<T>
): Promise<T> {
  const startedAt = performance.now();
  let success = false;

  try {
    const result = await fn();
    success = true;
    return result;
  } finally {
    logger.info({
      operation: name,
      duration_ms: Number((performance.now() - startedAt).toFixed(2)),
      success,
    });
  }
}
