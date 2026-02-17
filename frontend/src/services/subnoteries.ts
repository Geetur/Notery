// subnoteries.ts — Subnotery API service.
import { apiPost } from "@/lib/api-client";

/** POST /subnoteries/:id/join — Join a subnotery. */
export function joinSubnotery(
  subnoteryId: number
): Promise<{ message: string }> {
  return apiPost(`/subnoteries/${subnoteryId}/join`);
}

/** POST /subnoteries/:id/admins — Add admin to subnotery (admin only). */
export function addAdminToSubnotery(
  subnoteryId: number,
  userId: number
): Promise<{ message: string }> {
  return apiPost(`/subnoteries/${subnoteryId}/admins`, { user_id: userId });
}
