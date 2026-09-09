import { z } from "zod";

export const notificationPreferenceSchema = z.object({
  emailMorningEnabled: z.boolean(),
  emailEveningEnabled: z.boolean(),
  lineMorningEnabled: z.boolean(),
  lineEveningEnabled: z.boolean(),
});

export type NotificationPreferenceInput = z.infer<
  typeof notificationPreferenceSchema
>;
