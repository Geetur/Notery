// pdf-viewer.tsx — In-app PDF viewer using react-pdf.
// Renders PDFs inline with page navigation. Supports preview (truncated) and full modes.
// No download functionality — all viewing is in-app only.
"use client";

import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { getAccessToken } from "@/lib/api-client";
import { API_V1 } from "@/lib/config";
import {
    ChevronLeft,
    ChevronRight,
    Loader2,
    ZoomIn,
    ZoomOut,
} from "lucide-react";
import { useCallback, useState } from "react";
import { Document, Page, pdfjs } from "react-pdf";
import "react-pdf/dist/Page/AnnotationLayer.css";
import "react-pdf/dist/Page/TextLayer.css";

// Configure the PDF.js worker from the CDN matching the installed version.
pdfjs.GlobalWorkerOptions.workerSrc = `//unpkg.com/pdfjs-dist@${pdfjs.version}/build/pdf.worker.min.mjs`;

interface PDFViewerProps {
    /** Note ID to fetch PDF for */
    noteId: number;
    /** "preview" uses the truncated preview endpoint; "full" uses the full content endpoint */
    mode: "preview" | "full";
    /** Max height of the viewer container (default: 600px) */
    maxHeight?: number;
}

/**
 * In-app PDF viewer. Fetches the PDF from the API (with auth) and renders pages
 * using react-pdf. Supports navigation between pages and zoom.
 *
 * - preview mode: GET /notes/:id/preview (truncated, first ~5 pages)
 * - full mode:    GET /notes/:id/content?token=... (full PDF, requires purchase)
 */
export function PDFViewer({ noteId, mode, maxHeight = 600 }: PDFViewerProps) {
    const [numPages, setNumPages] = useState<number>(0);
    const [currentPage, setCurrentPage] = useState(1);
    const [scale, setScale] = useState(1.0);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);

    const pdfUrl = useCallback(() => {
        const token = getAccessToken();
        if (mode === "preview") {
            // Preview endpoint is under OptionalAuth — token optional but helpful
            return token
                ? `${API_V1}/notes/${noteId}/preview?token=${token}`
                : `${API_V1}/notes/${noteId}/preview`;
        }
        // Full content requires auth
        return `${API_V1}/notes/${noteId}/content?token=${token}`;
    }, [noteId, mode]);

    const onDocumentLoadSuccess = useCallback(
        ({ numPages: total }: { numPages: number }) => {
            setNumPages(total);
            setCurrentPage(1);
            setLoading(false);
            setError(null);
        },
        []
    );

    const onDocumentLoadError = useCallback((err: Error) => {
        console.error("PDF load error:", err);
        setLoading(false);
        setError("Failed to load PDF. Please try again.");
    }, []);

    const goToPrevPage = () => setCurrentPage((p) => Math.max(1, p - 1));
    const goToNextPage = () => setCurrentPage((p) => Math.min(numPages, p + 1));
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

    return (
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
                        Page {currentPage} of {numPages}
                    </span>
                    <Button
                        variant="ghost"
                        size="sm"
                        onClick={goToNextPage}
                        disabled={currentPage >= numPages}
                    >
                        <ChevronRight className="h-4 w-4" />
                    </Button>
                    <div className="w-px h-5 bg-border mx-1" />
                    <Button variant="ghost" size="sm" onClick={zoomOut} disabled={scale <= 0.5}>
                        <ZoomOut className="h-4 w-4" />
                    </Button>
                    <span className="text-xs text-muted-foreground min-w-[40px] text-center">
                        {Math.round(scale * 100)}%
                    </span>
                    <Button variant="ghost" size="sm" onClick={zoomIn} disabled={scale >= 2.0}>
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
                        <p className="text-xs text-muted-foreground">Loading PDF...</p>
                    </div>
                )}
                <Document
                    file={pdfUrl()}
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
                    This is a preview of the first few pages. Purchase to view the full document.
                </p>
            )}
        </div>
    );
}
