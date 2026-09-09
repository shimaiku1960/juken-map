import { createClient, type MicroCMSImage } from "microcms-js-sdk";

export const client = createClient({
  serviceDomain: process.env.MICROCMS_SERVICE_DOMAIN!,
  apiKey: process.env.MICROCMS_API_KEY!,
});

export type Blog = {
  id: string;
  title: string;
  description?: string;
  content: string;
  eyecatch?: MicroCMSImage;
  createdAt: string;
  updatedAt: string;
};
