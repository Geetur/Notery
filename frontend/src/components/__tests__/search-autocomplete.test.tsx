// search-autocomplete.test.tsx — Tests for search autocomplete dropdown.
// Validates: no author name shown, subnotery name (not ID), loading/empty states.

import type { Note, Subnotery } from "@/types";
import { act, render, screen, waitFor } from "@testing-library/react";
import React from "react";

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

const mockSearchNotes = jest.fn();
const mockSearchSubnoteries = jest.fn();

jest.mock("@/services/search", () => ({
    searchNotes: (...args: unknown[]) => mockSearchNotes(...args),
    searchSubnoteries: (...args: unknown[]) => mockSearchSubnoteries(...args),
}));

jest.mock("next/link", () => {
    const MockLink = ({ children, href, onClick, ...props }: { children: React.ReactNode; href: string; onClick?: () => void }) => (
        <a href={href} onClick={onClick} {...props}>{children}</a>
    );
    MockLink.displayName = "MockLink";
    return { __esModule: true, default: MockLink };
});

import { SearchAutocomplete } from "../layout/search-autocomplete";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeNote(overrides: Partial<Note> = {}): Note {
    return {
        id: 1,
        creator_id: 10,
        title: "Calculus Notes",
        author: "mathguru",
        description: "Great notes on calculus",
        status: "Approved",
        subnotery_id: 5,
        subnotery_name: "mathematics",
        subnotery_profile_picture_url: "",
        price: 0,
        has_pdf: true,
        pdf_size: 1024,
        pdf_uploaded_at: null,
        has_thumbnail: false,
        thumbnail_url: "",
        upvotes: 10,
        downvotes: 2,
        hotness: 30,
        user_vote: "",
        comment_count: 0,
        has_full_access: false,
        is_locked: false,
        pdf_pages: 0,
        created_at: "2025-01-01T00:00:00Z",
        updated_at: "2025-01-01T00:00:00Z",
        ...overrides,
    };
}

function makeSub(overrides: Partial<Subnotery> = {}): Subnotery {
    return {
        id: 5,
        name: "mathematics",
        member_count: 42,
        admin_count: 2,
        created_at: "2025-01-01T00:00:00Z",
        updated_at: "2025-01-01T00:00:00Z",
        ...overrides,
    };
}

beforeEach(() => {
    jest.clearAllMocks();
    jest.useFakeTimers();
});

afterEach(() => {
    jest.useRealTimers();
});

describe("SearchAutocomplete", () => {
    it("renders nothing when not visible", () => {
        mockSearchNotes.mockResolvedValue({ results: [], total: 0 });
        mockSearchSubnoteries.mockResolvedValue({ results: [], total: 0 });

        const { container } = render(
            <SearchAutocomplete query="test" visible={false} onClose={jest.fn()} />
        );
        expect(container.innerHTML).toBe("");
    });

    it("renders nothing when query is empty", () => {
        const { container } = render(
            <SearchAutocomplete query="" visible={true} onClose={jest.fn()} />
        );
        expect(container.innerHTML).toBe("");
    });

    it("shows note title without author name", async () => {
        const notes = [makeNote({ title: "Physics 101", author: "profsmith" })];
        mockSearchNotes.mockResolvedValue({ results: notes, total: 1 });
        mockSearchSubnoteries.mockResolvedValue({ results: [], total: 0 });

        render(
            <SearchAutocomplete query="phys" visible={true} onClose={jest.fn()} />
        );

        // Advance past the 250ms debounce
        await act(async () => {
            jest.advanceTimersByTime(300);
        });

        await waitFor(() => {
            expect(screen.getByText("Physics 101")).toBeInTheDocument();
        });

        // Author name should NOT appear
        expect(screen.queryByText(/profsmith/)).not.toBeInTheDocument();
        expect(screen.queryByText(/by profsmith/)).not.toBeInTheDocument();
    });

    it("shows subnotery name, not subnotery ID", async () => {
        const notes = [makeNote({ subnotery_name: "biology", subnotery_id: 99 })];
        mockSearchNotes.mockResolvedValue({ results: notes, total: 1 });
        mockSearchSubnoteries.mockResolvedValue({ results: [], total: 0 });

        render(
            <SearchAutocomplete query="bio" visible={true} onClose={jest.fn()} />
        );

        await act(async () => {
            jest.advanceTimersByTime(300);
        });

        await waitFor(() => {
            expect(screen.getByText("n/biology")).toBeInTheDocument();
        });

        // Should not show the raw ID
        expect(screen.queryByText("n/99")).not.toBeInTheDocument();
    });

    it("falls back to subnotery_id when subnotery_name is empty", async () => {
        const notes = [makeNote({ subnotery_name: "", subnotery_id: 42 })];
        mockSearchNotes.mockResolvedValue({ results: notes, total: 1 });
        mockSearchSubnoteries.mockResolvedValue({ results: [], total: 0 });

        render(
            <SearchAutocomplete query="test" visible={true} onClose={jest.fn()} />
        );

        await act(async () => {
            jest.advanceTimersByTime(300);
        });

        await waitFor(() => {
            expect(screen.getByText("n/42")).toBeInTheDocument();
        });
    });

    it("shows subnotery results with community names", async () => {
        const subs = [makeSub({ name: "physics", member_count: 100 })];
        mockSearchNotes.mockResolvedValue({ results: [], total: 0 });
        mockSearchSubnoteries.mockResolvedValue({ results: subs, total: 1 });

        render(
            <SearchAutocomplete query="phys" visible={true} onClose={jest.fn()} />
        );

        await act(async () => {
            jest.advanceTimersByTime(300);
        });

        await waitFor(() => {
            expect(screen.getByText("n/physics")).toBeInTheDocument();
            expect(screen.getByText("100 members")).toBeInTheDocument();
        });
    });

    it("shows 'No results found' when search returns empty", async () => {
        mockSearchNotes.mockResolvedValue({ results: [], total: 0 });
        mockSearchSubnoteries.mockResolvedValue({ results: [], total: 0 });

        render(
            <SearchAutocomplete query="zzzzz" visible={true} onClose={jest.fn()} />
        );

        await act(async () => {
            jest.advanceTimersByTime(300);
        });

        await waitFor(() => {
            expect(screen.getByText("No results found")).toBeInTheDocument();
        });
    });
});
