import { logger } from "@/api/observability/logger";
import { currentReqId, currentSim } from "@/api/observability/requestContext";

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
      // どのリクエストの処理かを示す。同時に複数が流れると、これが無い行は
      // 他のリクエストの行と混ざって読めなくなる。
      reqId: currentReqId(),
      sim: currentSim(),
      operation: name,
      duration_ms: Number((performance.now() - startedAt).toFixed(2)),
      success,
    });
  }
}
