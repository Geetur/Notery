// pdf-viewer.tsx — Dynamic wrapper for the PDF viewer component.
// This file ONLY re-exports the viewer via next/dynamic with ssr:false.
// The actual react-pdf imports live in pdf-viewer-inner.tsx and are NEVER
// loaded on the server, preventing the "Object.defineProperty called on
// non-object" SSR crash from pdfjs-dist.
"use client";

import { Loader2 } from "lucide-react";
import dynamic from "next/dynamic";

export type { PDFViewerProps } from "./pdf-viewer-inner";

/**
 * Dynamically imported PDFViewer — prevents SSR crashes from react-pdf's
 * browser-only dependencies (canvas, pdfjs-dist).
 *
 * Usage: `import { PDFViewer } from "@/components/pdf-viewer";`
 */
export const PDFViewer = dynamic(
    () => import("./pdf-viewer-inner"),
    {
        ssr: false,
        loading: () => (
            <div className="flex flex-col items-center justify-center py-12">
                <Loader2 className="h-8 w-8 animate-spin text-muted-foreground mb-2" />
                <p className="text-xs text-muted-foreground">
                    Loading PDF viewer...
                </p>
            </div>
        ),
    }
);
