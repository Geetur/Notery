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
    useEffect,
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
    /** Max height of the viewer container (default: "70vh"). Accepts px number or CSS string. */
    maxHeight?: number | string;
    /** Total pages in the full PDF (from note.pdf_pages). Used in preview mode to compute how many pages to request from the server. */
    totalPages?: number;
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
 * - preview mode: GET /notes/:id/preview?pages=N (server extracts first N pages)
 * - full mode:    GET /notes/:id/content?token=... (full PDF, requires purchase)
 */
export default function PDFViewerInner({ noteId, mode, maxHeight = "70vh", totalPages: totalPagesProp }: PDFViewerProps) {
    const [numPages, setNumPages] = useState<number>(0);
    const [currentPage, setCurrentPage] = useState(1);
    const [scale, setScale] = useState(1.0);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);
    const [previewData, setPreviewData] = useState<Uint8Array | null>(null);
    const [tooShortForPreview, setTooShortForPreview] = useState(false);
    const [retryCount, setRetryCount] = useState(0);

    // Preview page limit: 1 preview page per 5 total pages.
    // Minimum preview threshold: notes with 4 or fewer pages get NO preview
    // (the entire document would be visible, defeating the purpose).
    const MIN_PREVIEW_PAGES = 5;

    // Block preview for notes with too few pages (when we know the total).
    const previewBlocked =
        mode === "preview" &&
        totalPagesProp !== undefined &&
        totalPagesProp > 0 &&
        totalPagesProp < MIN_PREVIEW_PAGES;

    // How many pages to request from the server in preview mode.
    const previewPagesCount = useMemo(() => {
        if (mode !== "preview") return 0;
        if (totalPagesProp && totalPagesProp >= MIN_PREVIEW_PAGES) {
            return Math.max(1, Math.floor(totalPagesProp / 5));
        }
        // Legacy notes (pdf_pages=0/unknown): request 1 page as safe fallback.
        // The backend will backfill pdf_pages for subsequent requests.
        if (!totalPagesProp || totalPagesProp === 0) return 1;
        return 0; // totalPages < MIN_PREVIEW_PAGES → blocked, no request needed
    }, [mode, totalPagesProp]);

    // In preview mode numPages IS the preview count (server already trimmed).
    // In full mode numPages is the full document length.
    const displayPages = numPages;

    // Memoize the file URL so react-pdf's Document doesn't treat it as a new
    // document on every render (which causes infinite reload loops).
    const fileUrl = useMemo(() => {
        const token = getAccessToken();
        if (mode === "preview") {
            const pages = previewPagesCount || 1;
            const params = new URLSearchParams();
            if (token) params.set("token", token);
            params.set("pages", String(pages));
            return `${API_V1}/notes/${noteId}/preview?${params}`;
        }
        return `${API_V1}/notes/${noteId}/content?token=${token}`;
    }, [noteId, mode, previewPagesCount]);

    // Memoize the file prop for <Document> to avoid unnecessary reloads.
    // In preview mode we wait for pre-fetched bytes; in full mode use the URL.
    const fileProp = useMemo(() => {
        if (mode === "preview" && previewData) {
            return { data: previewData };
        }
        if (mode === "full") {
            return fileUrl;
        }
        return null; // preview mode, data not ready yet
    }, [mode, previewData, fileUrl]);

    // Pre-fetch the preview PDF manually so react-pdf never receives a
    // non-PDF error response (which causes console errors).
    useEffect(() => {
        if (mode !== "preview" || previewBlocked) return;
        let cancelled = false;

        (async () => {
            try {
                const res = await fetch(fileUrl);
                if (cancelled) return;

                if (res.ok) {
                    const buf = await res.arrayBuffer();
                    if (!cancelled) setPreviewData(new Uint8Array(buf));
                } else if (res.status === 422) {
                    const body = await res.json();
                    if (!cancelled) setTooShortForPreview(true);
                    // body.total_pages is available if needed
                    void body;
                } else {
                    let msg = "Failed to load preview";
                    try {
                        const body = await res.json();
                        if (body.error) msg = body.error;
                    } catch { /* non-JSON response */ }
                    if (!cancelled) setError(msg);
                }
            } catch {
                if (!cancelled) setError("Network error loading preview");
            } finally {
                if (!cancelled && !tooShortForPreview) setLoading(false);
            }
        })();

        return () => { cancelled = true; };
    }, [fileUrl, mode, previewBlocked, retryCount]);

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
                        setPreviewData(null);
                        setTooShortForPreview(false);
                        setRetryCount((c) => c + 1);
                    }}
                >
                    Retry
                </Button>
            </div>
        );
    }

    // If the note is too short for preview, load the doc silently to detect numPages,
    // then show a "no preview" message instead of the actual pages.
    if (previewBlocked || tooShortForPreview) {
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
                    {fileProp && (
                        <Document
                            file={fileProp}
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
                    )}
                </div>

                {/* Preview watermark */}
                {mode === "preview" && !loading && numPages > 0 && (
                    <p className="text-xs text-muted-foreground mt-2 text-center">
                        Preview: showing {numPages} of {totalPagesProp && totalPagesProp > 0 ? totalPagesProp : "?"} page{(totalPagesProp || 0) !== 1 ? "s" : ""}.
                        Purchase to view the full document.
                    </p>
                )}
            </div>
        </PDFErrorBoundary>
    );
}
