// format.test.ts — Tests for formatting utilities.
import { formatFileSize, formatPrice, formatVotes, netScore, timeAgo } from "@/lib/format";

describe("formatPrice", () => {
    it("formats cents to dollar string", () => {
        expect(formatPrice(499)).toBe("$4.99");
        expect(formatPrice(1000)).toBe("$10.00");
        expect(formatPrice(1)).toBe("$0.01");
    });

    it("returns 'Free' for zero", () => {
        expect(formatPrice(0)).toBe("Free");
    });

    it("handles large amounts", () => {
        expect(formatPrice(999999)).toBe("$9999.99");
    });
});

describe("formatVotes", () => {
    it("returns the number for small values", () => {
        expect(formatVotes(0)).toBe("0");
        expect(formatVotes(999)).toBe("999");
        expect(formatVotes(-5)).toBe("-5");
    });

    it("abbreviates thousands", () => {
        expect(formatVotes(1000)).toBe("1.0k");
        expect(formatVotes(1500)).toBe("1.5k");
        expect(formatVotes(10000)).toBe("10.0k");
    });
});

describe("formatFileSize", () => {
    it("formats bytes to human-readable", () => {
        expect(formatFileSize(0)).toBe("0 B");
        expect(formatFileSize(512)).toBe("512 B");
        expect(formatFileSize(1024)).toBe("1.0 KB");
        expect(formatFileSize(1048576)).toBe("1.0 MB");
    });
});

describe("netScore", () => {
    it("calculates upvotes minus downvotes", () => {
        expect(netScore(10, 3)).toBe(7);
        expect(netScore(0, 0)).toBe(0);
        expect(netScore(5, 10)).toBe(-5);
    });
});

describe("timeAgo", () => {
    it("returns a relative time string for recent times", () => {
        const now = new Date().toISOString();
        const result = timeAgo(now);
        expect(result).toContain("ago");
    });

    it("returns relative time for minutes ago", () => {
        const fiveMinAgo = new Date(Date.now() - 5 * 60 * 1000).toISOString();
        const result = timeAgo(fiveMinAgo);
        expect(result).toContain("minute");
        expect(result).toContain("ago");
    });

    it("returns relative time for hours ago", () => {
        const twoHoursAgo = new Date(Date.now() - 2 * 60 * 60 * 1000).toISOString();
        const result = timeAgo(twoHoursAgo);
        expect(result).toContain("hour");
        expect(result).toContain("ago");
    });

    it("returns relative time for days ago", () => {
        const threeDaysAgo = new Date(Date.now() - 3 * 24 * 60 * 60 * 1000).toISOString();
        const result = timeAgo(threeDaysAgo);
        expect(result).toContain("day");
        expect(result).toContain("ago");
    });

    it("returns the original string on invalid input", () => {
        expect(timeAgo("not-a-date")).toBe("not-a-date");
    });
});
