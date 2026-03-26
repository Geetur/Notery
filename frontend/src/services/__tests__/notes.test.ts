// notes.test.ts — Tests for notes service functions.

jest.mock("@/lib/api-client", () => ({
    apiGet: jest.fn(),
    apiPost: jest.fn(),
    apiDelete: jest.fn(),
    getAccessToken: jest.fn(() => "mock-token"),
}));

jest.mock("@/lib/config", () => ({
    API_V1: "http://localhost:8080/api/v1",
    TOKEN_KEY: "notery_access_token",
}));

import { apiDelete, apiGet, apiPost } from "@/lib/api-client";
import {
    createNote,
    deleteNote,
    downvoteNote,
    getApprovedNotes,
    getHotFeed,
    getNoteById,
    getPendingNotes,
    upvoteNote,
} from "@/services/notes";

const mockApiGet = apiGet as jest.MockedFunction<typeof apiGet>;
const mockApiPost = apiPost as jest.MockedFunction<typeof apiPost>;
const mockApiDelete = apiDelete as jest.MockedFunction<typeof apiDelete>;

beforeEach(() => {
    jest.clearAllMocks();
});

describe("getHotFeed", () => {
    it("calls /feed/hot with no params", async () => {
        mockApiGet.mockResolvedValue({ notes: [], total: 0 });
        await getHotFeed();
        expect(mockApiGet).toHaveBeenCalledWith("/feed/hot");
    });

    it("passes pagination params", async () => {
        mockApiGet.mockResolvedValue({ notes: [], total: 0 });
        await getHotFeed({ page: 2, limit: 10 });
        expect(mockApiGet).toHaveBeenCalledWith("/feed/hot?page=2&limit=10");
    });
});

describe("getApprovedNotes", () => {
    it("calls /notes/approved", async () => {
        mockApiGet.mockResolvedValue({ notes: [], total: 0 });
        await getApprovedNotes();
        expect(mockApiGet).toHaveBeenCalledWith("/notes/approved");
    });
});

describe("getNoteById", () => {
    it("calls /notes/:id", async () => {
        const mockNote = { id: 1, title: "Test" };
        mockApiGet.mockResolvedValue(mockNote);
        const result = await getNoteById(1);
        expect(mockApiGet).toHaveBeenCalledWith("/notes/1");
        expect(result).toEqual(mockNote);
    });
});

describe("createNote", () => {
    it("calls POST /notes with note data (no author — auto-derived)", async () => {
        const data = {
            title: "My Note",
            subnotery_name: "Science",
            price: 499,
        };
        mockApiPost.mockResolvedValue({ id: 1, ...data, author: "testuser" });
        await createNote(data);
        expect(mockApiPost).toHaveBeenCalledWith("/notes", data);
    });
});

describe("upvoteNote", () => {
    it("calls POST /notes/:id/upvote", async () => {
        mockApiPost.mockResolvedValue({ vote: 1 });
        await upvoteNote(5);
        expect(mockApiPost).toHaveBeenCalledWith("/notes/5/upvote");
    });
});

describe("downvoteNote", () => {
    it("calls POST /notes/:id/downvote", async () => {
        mockApiPost.mockResolvedValue({ vote: -1 });
        await downvoteNote(5);
        expect(mockApiPost).toHaveBeenCalledWith("/notes/5/downvote");
    });
});

describe("deleteNote", () => {
    it("calls DELETE /notes/:id", async () => {
        mockApiDelete.mockResolvedValue({ message: "deleted" });
        await deleteNote(3);
        expect(mockApiDelete).toHaveBeenCalledWith("/notes/3");
    });
});

describe("getPendingNotes", () => {
    it("calls /notes/pending with pagination", async () => {
        mockApiGet.mockResolvedValue({ notes: [], total: 0 });
        await getPendingNotes({ page: 1, limit: 25 });
        expect(mockApiGet).toHaveBeenCalledWith(
            "/notes/pending?page=1&limit=25"
        );
    });
});
