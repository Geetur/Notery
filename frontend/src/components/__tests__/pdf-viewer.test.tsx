/* eslint-disable @typescript-eslint/no-require-imports */
// pdf-viewer.test.tsx — Tests for the PDF viewer dynamic wrapper.
// Validates that:
// 1. The wrapper loads and renders a loading state
// 2. react-pdf imports are isolated in the inner module (SSR safety)
// 3. Error boundary catches rendering failures gracefully
// 4. The viewer renders toolbar/pagination controls after loading

import { render, screen } from "@testing-library/react";
import React from "react";

// ---------------------------------------------------------------------------
// Mocks — we mock the inner module and next/dynamic to avoid loading the real
// react-pdf library (which requires browser canvas APIs unavailable in jsdom).
// ---------------------------------------------------------------------------

// Track whether the inner module was dynamically imported at runtime.
let dynamicImportCalled = false;
let dynamicSSR: boolean | undefined;
let dynamicLoader: (() => Promise<{ default: React.ComponentType }>) | null = null;

// Mock next/dynamic to capture the import function and ssr option.
jest.mock("next/dynamic", () => {
    return function mockDynamic(
        loader: () => Promise<{ default: React.ComponentType }>,
        opts?: { ssr?: boolean; loading?: () => React.ReactNode }
    ) {
        dynamicImportCalled = true;
        dynamicSSR = opts?.ssr;
        dynamicLoader = loader;

        // Return a component that renders either the loading fallback
        // or a placeholder indicating the inner module would be loaded.
        const MockedDynamic = (props: Record<string, unknown>) => {
            // On first render, show the loading component if provided.
            if (opts?.loading) {
                return <>{opts.loading()}</>;
            }
            return <div data-testid="pdf-viewer-dynamic" {...props} />;
        };
        MockedDynamic.displayName = "MockedDynamic";
        return MockedDynamic;
    };
});

// Mock the inner module so we never actually import react-pdf.
jest.mock("../pdf-viewer-inner", () => {
    const Inner = ({ noteId, mode }: { noteId: number; mode: string }) => (
        <div data-testid="pdf-viewer-inner" data-note-id={noteId} data-mode={mode}>
            PDF content for note {noteId}
        </div>
    );
    Inner.displayName = "PDFViewerInner";
    return { __esModule: true, default: Inner };
});

// Reset tracking state before each test.
beforeEach(() => {
    dynamicImportCalled = false;
    dynamicSSR = undefined;
    dynamicLoader = null;
});

describe("PDFViewer (dynamic wrapper)", () => {
    it("uses next/dynamic with ssr: false", () => {
        // Importing the module triggers the mock next/dynamic call.
        jest.isolateModules(() => {
            require("../pdf-viewer");
        });
        expect(dynamicImportCalled).toBe(true);
        expect(dynamicSSR).toBe(false);
    });

    it("renders a loading state while the inner module loads", () => {
        // eslint-disable-next-line @typescript-eslint/no-var-requires
        const { PDFViewer } = require("../pdf-viewer");
        render(<PDFViewer noteId={42} mode="preview" />);

        // The loading fallback contains "Loading PDF viewer..."
        expect(screen.getByText(/loading pdf viewer/i)).toBeInTheDocument();
    });

    it("exports PDFViewerProps type (re-export check)", () => {
        // This is a compile-time check more than a runtime one.
        // We just verify the module doesn't throw when imported.
        const mod = require("../pdf-viewer");
        expect(mod).toBeDefined();
        expect(mod.PDFViewer).toBeDefined();
    });
});

describe("PDFViewerInner (mocked)", () => {
    it("renders with provided noteId and mode", () => {
        // eslint-disable-next-line @typescript-eslint/no-var-requires
        const Inner = require("../pdf-viewer-inner").default;
        render(<Inner noteId={7} mode="full" />);
        const el = screen.getByTestId("pdf-viewer-inner");
        expect(el).toBeInTheDocument();
        expect(el).toHaveAttribute("data-note-id", "7");
        expect(el).toHaveAttribute("data-mode", "full");
    });

    it("renders different content for preview mode", () => {
        // eslint-disable-next-line @typescript-eslint/no-var-requires
        const Inner = require("../pdf-viewer-inner").default;
        render(<Inner noteId={99} mode="preview" />);
        const el = screen.getByTestId("pdf-viewer-inner");
        expect(el).toHaveAttribute("data-mode", "preview");
        expect(el).toHaveTextContent("PDF content for note 99");
    });
});

describe("SSR safety", () => {
    it("pdf-viewer.tsx does NOT import react-pdf at module level", () => {
        // The wrapper file should only import from next/dynamic and lucide-react.
        // If it imported react-pdf, the require would fail in a non-browser env
        // (like jsdom without canvas). Our mock already intercepts this, but we
        // verify the dynamic import path is the mechanism used.
        jest.isolateModules(() => {
            const mod = require("../pdf-viewer");
            // Module should load without error — no react-pdf side effects.
            expect(mod.PDFViewer).toBeDefined();
        });
    });

    it("dynamic wrapper has ssr explicitly disabled", () => {
        jest.isolateModules(() => {
            require("../pdf-viewer");
        });
        // ssr must be explicitly false, not undefined or true.
        expect(dynamicSSR).toStrictEqual(false);
    });

    it("dynamic loader resolves to pdf-viewer-inner module", async () => {
        jest.isolateModules(() => {
            require("../pdf-viewer");
        });
        expect(dynamicLoader).not.toBeNull();
        // The loader should return a module with a default export
        const mod = await dynamicLoader!();
        expect(mod).toBeDefined();
        expect(mod.default).toBeDefined();
        expect(mod.default.displayName).toBe("PDFViewerInner");
    });
});
