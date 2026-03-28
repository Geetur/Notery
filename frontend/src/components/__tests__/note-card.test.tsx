// note-card.test.tsx — Tests for the unified NoteCard component.
// Validates: unified expanded layout (no compact mode), status badge colours,
// price badges, PDF info, description rendering, thumbnail.

import type { Note } from "@/types";
import { render, screen } from "@testing-library/react";
import React from "react";

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

// Mock next/link to render a plain anchor.
jest.mock("next/link", () => {
    const MockLink = ({ children, href, ...props }: { children: React.ReactNode; href: string }) => (
        <a href={href} {...props}>{children}</a>
    );
    MockLink.displayName = "MockLink";
    return { __esModule: true, default: MockLink };
});

// Mock the auth store.
jest.mock("@/stores/auth-store", () => ({
    useAuthStore: jest.fn(() => ({ isAuthenticated: false })),
}));

// Mock toast hook.
jest.mock("@/hooks/use-toast", () => ({
    useToast: jest.fn(() => ({ toast: jest.fn() })),
}));

// Mock vote buttons.
jest.mock("../feed/vote-buttons", () => ({
    VoteButtons: ({ noteId, upvotes, downvotes }: { noteId: number; upvotes: number; downvotes: number }) => (
        <div data-testid="vote-buttons" data-note-id={noteId}>
            {upvotes} up / {downvotes} down
        </div>
    ),
}));

// Mock bookmark services.
jest.mock("@/services/bookmarks", () => ({
    addBookmark: jest.fn(),
    removeBookmark: jest.fn(),
}));

// Mock SubnoteryAvatar component.
jest.mock("@/components/subnotery-avatar", () => ({
    SubnoteryAvatar: ({ name }: { name?: string }) => (
        <div data-testid="subnotery-avatar">{name}</div>
    ),
}));

// Mock format helpers.
jest.mock("@/lib/format", () => ({
    formatPrice: (cents: number) => (cents === 0 ? "Free" : `$${(cents / 100).toFixed(2)}`),
    formatFileSize: (bytes: number) => `${(bytes / 1024).toFixed(0)} KB`,
    thumbnailUrl: (id: number, url: string) => `/thumb/${id}/${url}`,
    timeAgo: () => "2 hours ago",
}));

import { NoteCard } from "../feed/note-card";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeNote(overrides: Partial<Note> = {}): Note {
    return {
        id: 1,
        creator_id: 10,
        title: "Test Note Title",
        author: "testuser",
        description: "",
        status: "Approved",
        subnotery_id: 5,
        subnotery_name: "science",
        subnotery_profile_picture_url: "",
        price: 0,
        has_pdf: false,
        pdf_size: 0,
        pdf_uploaded_at: null,
        has_thumbnail: false,
        thumbnail_url: "",
        upvotes: 12,
        downvotes: 3,
        hotness: 42,
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

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("NoteCard", () => {
    it("renders the note title", () => {
        render(<NoteCard note={makeNote()} />);
        expect(screen.getByText("Test Note Title")).toBeInTheDocument();
    });

    it("renders the subnotery name, not ID", () => {
        render(<NoteCard note={makeNote({ subnotery_name: "physics" })} />);
        expect(screen.getByText("n/physics")).toBeInTheDocument();
    });

    it("falls back to subnotery_id when name is empty", () => {
        render(<NoteCard note={makeNote({ subnotery_name: "", subnotery_id: 99 })} />);
        expect(screen.getByText("n/99")).toBeInTheDocument();
    });

    it("renders vote buttons with correct counts", () => {
        render(<NoteCard note={makeNote({ upvotes: 10, downvotes: 2 })} />);
        expect(screen.getByTestId("vote-buttons")).toHaveTextContent("10 up / 2 down");
    });

    it("does not render author in feed card meta line", () => {
        render(<NoteCard note={makeNote({ author: "johndoe" })} />);
        expect(screen.queryByText("u/johndoe")).not.toBeInTheDocument();
    });

    it("shows Free badge for free notes", () => {
        render(<NoteCard note={makeNote({ price: 0 })} />);
        expect(screen.getByText("Free")).toBeInTheDocument();
    });

    it("shows price badge for paid notes", () => {
        render(<NoteCard note={makeNote({ price: 999 })} />);
        expect(screen.getByText("$9.99")).toBeInTheDocument();
    });

    it("shows PDF badge with file size when note has PDF", () => {
        render(<NoteCard note={makeNote({ has_pdf: true, pdf_size: 204800 })} />);
        expect(screen.getByText("PDF — 200 KB")).toBeInTheDocument();
    });

    it("does not show PDF badge when note has no PDF", () => {
        render(<NoteCard note={makeNote({ has_pdf: false })} />);
        expect(screen.queryByText(/PDF —/)).not.toBeInTheDocument();
    });

    it("renders description when present", () => {
        render(<NoteCard note={makeNote({ description: "This is a great note about science." })} />);
        expect(screen.getByText("This is a great note about science.")).toBeInTheDocument();
    });

    it("does not render description paragraph when empty", () => {
        const { container } = render(<NoteCard note={makeNote({ description: "" })} />);
        expect(container.querySelector(".line-clamp-3")).not.toBeInTheDocument();
    });

    // Status badge colour tests
    it("does not show Approved status badge", () => {
        render(<NoteCard note={makeNote({ status: "Approved" })} />);
        expect(screen.queryByText("Approved")).not.toBeInTheDocument();
    });

    it("shows yellow Pending status badge", () => {
        render(<NoteCard note={makeNote({ status: "Pending" })} />);
        const badge = screen.getByText("Pending");
        expect(badge).toBeInTheDocument();
        expect(badge.className).toContain("yellow");
    });

    it("shows red Rejected status badge", () => {
        render(<NoteCard note={makeNote({ status: "Rejected" })} />);
        const badge = screen.getByText("Rejected");
        expect(badge).toBeInTheDocument();
        expect(badge.className).toContain("red");
    });

    // Locked/Owned badges were removed from feed cards
    it("does not show Locked badge on feed card", () => {
        render(<NoteCard note={makeNote({ price: 500, status: "Approved" })} />);
        expect(screen.queryByText("Locked")).not.toBeInTheDocument();
    });

    it("does not show Owned badge on feed card", () => {
        render(<NoteCard note={makeNote({ price: 500 })} />);
        expect(screen.queryByText("Owned")).not.toBeInTheDocument();
    });

    // Thumbnail rendering
    it("renders thumbnail when has_thumbnail is true", () => {
        render(
            <NoteCard
                note={makeNote({ has_thumbnail: true, thumbnail_url: "thumb.jpg" })}
            />
        );
        const img = screen.getByAltText("Thumbnail for Test Note Title");
        expect(img).toBeInTheDocument();
        expect(img).toHaveAttribute("src", "/thumb/1/thumb.jpg");
    });

    it("does not render thumbnail when has_thumbnail is false", () => {
        render(<NoteCard note={makeNote({ has_thumbnail: false })} />);
        expect(screen.queryByAltText("Thumbnail for Test Note Title")).not.toBeInTheDocument();
    });

    // Unified layout — viewMode prop is ignored
    it("always renders expanded layout regardless of viewMode prop", () => {
        const { container: c1 } = render(<NoteCard note={makeNote()} />);
        const { container: c2 } = render(<NoteCard note={makeNote()} viewMode="compact" />);
        // Both should produce the same structure (expanded layout)
        expect(c1.querySelectorAll("[data-testid='vote-buttons']").length).toBe(1);
        expect(c2.querySelectorAll("[data-testid='vote-buttons']").length).toBe(1);
    });

    // Action bar — icons only, no text labels
    it("renders comment icon link", () => {
        render(<NoteCard note={makeNote()} />);
        // Comments text removed; icon-only action bar
        expect(screen.queryByText("Comments")).not.toBeInTheDocument();
    });

    it("renders bookmark icon button", () => {
        render(<NoteCard note={makeNote()} />);
        // Save text removed; icon-only action bar
        expect(screen.queryByText("Save")).not.toBeInTheDocument();
    });
});
