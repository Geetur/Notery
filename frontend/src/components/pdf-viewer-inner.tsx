// pdf-viewer-inner.tsx — Raw PDF viewer component using react-pdf.
// This file MUST ONLY be imported via next/dynamic with ssr:false.
// Direct import will crash during SSR because react-pdf requires browser APIs.
"use client";

import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { getAccessToken } from "@/lib/api-client";
import { API_V1 } from "@/lib/config";
import {
    ChevronLeft,
    ChevronRight,
    FileText,
    Loader2,
    ZoomIn,
    ZoomOut,
} from "lucide-react";
import {
    Component,
    type ErrorInfo,
    type ReactNode,
    useMemo,
    useState,
} from "react";
import { Document, Page, pdfjs } from "react-pdf";
import "react-pdf/dist/Page/AnnotationLayer.css";
import "react-pdf/dist/Page/TextLayer.css";

// Configure the PDF.js worker from a local copy in public/ for reliability.
if (typeof window !== "undefined") {
    pdfjs.GlobalWorkerOptions.workerSrc = "/pdf.worker.min.mjs";
}

export interface PDFViewerProps {
    /** Note ID to fetch PDF for */
    noteId: number;
    /** "preview" uses the truncated preview endpoint; "full" uses the full content endpoint */
    mode: "preview" | "full";
    /** Max height of the viewer container (default: 600px) */
    maxHeight?: number;
}

/** React error boundary to catch react-pdf rendering crashes gracefully. */
interface EBProps {
    children: ReactNode;
    fallback: ReactNode;
}
interface EBState {
    hasError: boolean;
}
class PDFErrorBoundary extends Component<EBProps, EBState> {
    constructor(props: EBProps) {
        super(props);
        this.state = { hasError: false };
    }
    static getDerivedStateFromError(): EBState {
        return { hasError: true };
    }
    componentDidCatch(error: Error, info: ErrorInfo) {
        console.error("PDFViewer error boundary caught:", error, info);
    }
    render() {
        if (this.state.hasError) return this.props.fallback;
        return this.props.children;
    }
}

/**
 * In-app PDF viewer. Fetches the PDF from the API (with auth) and renders pages
 * using react-pdf. Supports navigation between pages and zoom.
 *
 * - preview mode: GET /notes/:id/preview (truncated, first ~5 pages)
 * - full mode:    GET /notes/:id/content?token=... (full PDF, requires purchase)
 */
export default function PDFViewerInner({ noteId, mode, maxHeight = 600 }: PDFViewerProps) {
    const [numPages, setNumPages] = useState<number>(0);
    const [currentPage, setCurrentPage] = useState(1);
    const [scale, setScale] = useState(1.0);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);

    // Preview page limit: 1 preview page per 5 total pages.
    // Minimum preview threshold: notes with 4 or fewer pages get NO preview
    // (the entire document would be visible, defeating the purpose).
    const MIN_PREVIEW_PAGES = 5; // notes must have at least this many pages for preview
    const previewBlocked = mode === "preview" && numPages > 0 && numPages < MIN_PREVIEW_PAGES;
    const maxPreviewPages =
        mode === "preview" && numPages > 0
            ? Math.max(1, Math.floor(numPages / 5))
            : numPages;
    const displayPages = mode === "preview" ? maxPreviewPages : numPages;

    // Memoize the file URL so react-pdf's Document doesn't treat it as a new
    // document on every render (which causes infinite reload loops).
    const fileUrl = useMemo(() => {
        const token = getAccessToken();
        if (mode === "preview") {
            return token
                ? `${API_V1}/notes/${noteId}/preview?token=${token}`
                : `${API_V1}/notes/${noteId}/preview`;
        }
        return `${API_V1}/notes/${noteId}/content?token=${token}`;
    }, [noteId, mode]);

    const onDocumentLoadSuccess = ({ numPages: total }: { numPages: number }) => {
        setNumPages(total);
        setCurrentPage(1);
        setLoading(false);
        setError(null);
    };

    const onDocumentLoadError = (err: Error) => {
        console.error("PDF load error:", err);
        setLoading(false);
        setError("Failed to load PDF. Please try again.");
    };

    const goToPrevPage = () => setCurrentPage((p) => Math.max(1, p - 1));
    const goToNextPage = () =>
        setCurrentPage((p) => Math.min(displayPages, p + 1));
    const zoomIn = () => setScale((s) => Math.min(2.0, s + 0.2));
    const zoomOut = () => setScale((s) => Math.max(0.5, s - 0.2));

    if (error) {
        return (
            <div className="flex flex-col items-center justify-center py-8 text-center">
                <p className="text-sm text-destructive mb-2">{error}</p>
                <Button
                    variant="outline"
                    size="sm"
                    onClick={() => {
                        setError(null);
                        setLoading(true);
                    }}
                >
                    Retry
                </Button>
            </div>
        );
    }

    // If the note is too short for preview, load the doc silently to detect numPages,
    // then show a "no preview" message instead of the actual pages.
    if (previewBlocked) {
        return (
            <PDFErrorBoundary
                fallback={
                    <div className="flex flex-col items-center justify-center py-8 text-center">
                        <p className="text-sm text-destructive mb-2">
                            PDF viewer encountered an error. Please refresh the page.
                        </p>
                    </div>
                }
            >
                {/* Hidden document mount to detect numPages */}
                <div style={{ display: "none" }}>
                    <Document file={fileUrl} onLoadSuccess={onDocumentLoadSuccess} onLoadError={onDocumentLoadError} />
                </div>
                <div className="flex flex-col items-center justify-center py-12 text-center">
                    <FileText className="h-10 w-10 text-muted-foreground mb-3" />
                    <p className="text-sm font-medium mb-1">Preview not available</p>
                    <p className="text-xs text-muted-foreground">
                        This document is too short for a preview. Purchase to view the full content.
                    </p>
                </div>
            </PDFErrorBoundary>
        );
    }

    return (
        <PDFErrorBoundary
            fallback={
                <div className="flex flex-col items-center justify-center py-8 text-center">
                    <p className="text-sm text-destructive mb-2">
                        PDF viewer encountered an error. Please refresh the page.
                    </p>
                    <Button
                        variant="outline"
                        size="sm"
                        onClick={() => window.location.reload()}
                    >
                        Refresh
                    </Button>
                </div>
            }
        >
            <div className="flex flex-col items-center w-full">
                {/* Toolbar */}
                {!loading && numPages > 0 && (
                    <div className="flex items-center gap-2 mb-2 p-2 bg-muted/50 rounded-md w-full justify-center flex-wrap">
                        <Button
                            variant="ghost"
                            size="sm"
                            onClick={goToPrevPage}
                            disabled={currentPage <= 1}
                        >
                            <ChevronLeft className="h-4 w-4" />
                        </Button>
                        <span className="text-xs text-muted-foreground min-w-[80px] text-center">
                            Page {currentPage} of {displayPages}
                        </span>
                        <Button
                            variant="ghost"
                            size="sm"
                            onClick={goToNextPage}
                            disabled={currentPage >= displayPages}
                        >
                            <ChevronRight className="h-4 w-4" />
                        </Button>
                        <div className="w-px h-5 bg-border mx-1" />
                        <Button
                            variant="ghost"
                            size="sm"
                            onClick={zoomOut}
                            disabled={scale <= 0.5}
                        >
                            <ZoomOut className="h-4 w-4" />
                        </Button>
                        <span className="text-xs text-muted-foreground min-w-[40px] text-center">
                            {Math.round(scale * 100)}%
                        </span>
                        <Button
                            variant="ghost"
                            size="sm"
                            onClick={zoomIn}
                            disabled={scale >= 2.0}
                        >
                            <ZoomIn className="h-4 w-4" />
                        </Button>
                        {mode === "preview" && (
                            <span className="text-xs text-yellow-500 font-medium ml-2">
                                Preview
                            </span>
                        )}
                    </div>
                )}

                {/* PDF Document */}
                <div
                    className="overflow-auto border rounded-md bg-muted/20 w-full"
                    style={{ maxHeight }}
                >
                    {loading && (
                        <div className="flex flex-col items-center justify-center py-12">
                            <Loader2 className="h-8 w-8 animate-spin text-muted-foreground mb-2" />
                            <p className="text-xs text-muted-foreground">
                                Loading PDF...
                            </p>
                        </div>
                    )}
                    <Document
                        file={fileUrl}
                        onLoadSuccess={onDocumentLoadSuccess}
                        onLoadError={onDocumentLoadError}
                        loading={
                            <div className="flex items-center justify-center py-12">
                                <Skeleton className="h-[400px] w-[300px]" />
                            </div>
                        }
                    >
                        <div className="flex justify-center p-4">
                            <Page
                                pageNumber={currentPage}
                                scale={scale}
                                renderTextLayer={true}
                                renderAnnotationLayer={true}
                                loading={
                                    <Skeleton className="h-[400px] w-[300px]" />
                                }
                            />
                        </div>
                    </Document>
                </div>

                {/* Preview watermark */}
                {mode === "preview" && !loading && numPages > 0 && (
                    <p className="text-xs text-muted-foreground mt-2 text-center">
                        Preview: showing {displayPages} of {numPages} page{numPages !== 1 ? "s" : ""}.
                        Purchase to view the full document.
                    </p>
                )}
            </div>
        </PDFErrorBoundary>
    );
}
