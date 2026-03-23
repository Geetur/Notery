// reports.ts — Bug report API service.
import { apiPost } from "@/lib/api-client";

/** POST /reports/bug — Submit a bug report. */
export function submitBugReport(
    description: string,
    page?: string
): Promise<{ message: string }> {
    return apiPost("/reports/bug", { description, page });
}
