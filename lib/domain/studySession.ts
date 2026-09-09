import { z } from "zod";

export const studySessionStorageKey = (userId: string) =>
  `juken-map:active-study-session:${userId}`;

const nullablePositiveInt = z.number().int().positive().nullable();

export const activeStudySessionSchema = z.object({
  version: z.literal(1),
  id: z.string().min(1),
  planId: nullablePositiveInt,
  label: z.string().min(1),
  subject: z.string().nullable(),
  textbookId: nullablePositiveInt,
  rangeStart: nullablePositiveInt,
  rangeEnd: nullablePositiveInt,
  rangeUnit: z.string().nullable(),
  startedAt: z.number().int().nonnegative(),
  accumulatedMs: z.number().int().nonnegative(),
  runningSince: z.number().int().nonnegative().nullable(),
  status: z.enum(["running", "paused", "reviewing"]),
});

export type ActiveStudySession = z.infer<typeof activeStudySessionSchema>;

export type StudySessionTarget = Pick<
  ActiveStudySession,
  | "planId"
  | "label"
  | "subject"
  | "textbookId"
  | "rangeStart"
  | "rangeEnd"
  | "rangeUnit"
>;

export function startStudySession(
  target: StudySessionTarget,
  now = Date.now()
): ActiveStudySession {
  return {
    version: 1,
    id: `${now}-${Math.random().toString(36).slice(2)}`,
    ...target,
    startedAt: now,
    accumulatedMs: 0,
    runningSince: now,
    status: "running",
  };
}

export function elapsedStudyMs(session: ActiveStudySession, now = Date.now()) {
  if (session.runningSince == null) return session.accumulatedMs;
  return session.accumulatedMs + Math.max(0, now - session.runningSince);
}

export function pauseStudySession(
  session: ActiveStudySession,
  now = Date.now()
): ActiveStudySession {
  return {
    ...session,
    accumulatedMs: elapsedStudyMs(session, now),
    runningSince: null,
    status: "paused",
  };
}

export function resumeStudySession(
  session: ActiveStudySession,
  now = Date.now()
): ActiveStudySession {
  return { ...session, runningSince: now, status: "running" };
}

export function reviewStudySession(
  session: ActiveStudySession,
  now = Date.now()
): ActiveStudySession {
  const paused = pauseStudySession(session, now);
  return { ...paused, status: "reviewing" };
}

export function recordedMinutes(session: ActiveStudySession, now = Date.now()) {
  return Math.max(1, Math.ceil(elapsedStudyMs(session, now) / 60_000));
}

export function formatStudyElapsed(milliseconds: number) {
  const totalSeconds = Math.max(0, Math.floor(milliseconds / 1000));
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;
  return [hours, minutes, seconds]
    .map((value) => String(value).padStart(2, "0"))
    .join(":");
}

export function parseStoredStudySession(value: string | null) {
  if (!value) return null;
  try {
    const parsed = activeStudySessionSchema.safeParse(JSON.parse(value));
    return parsed.success ? parsed.data : null;
  } catch {
    return null;
  }
}
