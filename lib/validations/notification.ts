import { z } from "zod";

export const notificationPreferenceSchema = z.object({
  morningEnabled: z.boolean(),
  eveningEnabled: z.boolean(),
});

export type NotificationPreferenceInput = z.infer<
  typeof notificationPreferenceSchema
>;
