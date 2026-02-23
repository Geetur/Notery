// page.tsx — Submit (create) a new note. PDF is required; description and thumbnail are optional.
"use client";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { useToast } from "@/hooks/use-toast";
import { createNote, uploadNotePDF, uploadNoteThumbnail } from "@/services/notes";
import { searchSubnoteries } from "@/services/search";
import { useAuthStore } from "@/stores/auth-store";
import type { Subnotery } from "@/types";
import { ArrowLeft, FileText, Hash, ImageIcon, Loader2, Upload, X } from "lucide-react";
import { useRouter } from "next/navigation";
import { useEffect, useRef, useState, type ChangeEvent, type FormEvent } from "react";

const MAX_PDF_SIZE = 50 * 1024 * 1024; // 50 MB
const MAX_THUMBNAIL_SIZE = 5 * 1024 * 1024; // 5 MB
const ALLOWED_IMAGE_TYPES = ["image/jpeg", "image/png", "image/webp", "image/gif"];

export default function SubmitPage() {
    const router = useRouter();
    const { isAuthenticated, loading } = useAuthStore();
    const { toast } = useToast();
    const fileInputRef = useRef<HTMLInputElement>(null);
    const thumbnailInputRef = useRef<HTMLInputElement>(null);

    const [title, setTitle] = useState("");
    const [description, setDescription] = useState("");
    const [subnoteryName, setSubnoteryName] = useState("");
    const [price, setPrice] = useState("");
    const [pdfFile, setPdfFile] = useState<File | null>(null);
    const [thumbnailFile, setThumbnailFile] = useState<File | null>(null);
    const [thumbnailPreview, setThumbnailPreview] = useState<string | null>(null);
    const [submitting, setSubmitting] = useState(false);

    // Subnotery autocomplete state
    const [subnoterySuggestions, setSubnoterySuggestions] = useState<Subnotery[]>([]);
    const [showSuggestions, setShowSuggestions] = useState(false);
    const [suggestionsLoading, setSuggestionsLoading] = useState(false);
    const subnoteryInputRef = useRef<HTMLDivElement>(null);
    const abortRef = useRef<AbortController | null>(null);

    useEffect(() => {
        if (!isAuthenticated && !loading) {
            router.push("/login");
        }
    }, [isAuthenticated, loading, router]);

    // Debounced subnotery search
    useEffect(() => {
        if (!subnoteryName.trim() || subnoteryName.trim().length < 1) {
            setSubnoterySuggestions([]);
            setShowSuggestions(false);
            return;
        }
        const timeout = setTimeout(async () => {
            abortRef.current?.abort();
            const controller = new AbortController();
            abortRef.current = controller;
            setSuggestionsLoading(true);
            try {
                const res = await searchSubnoteries(subnoteryName.trim(), { limit: 6 });
                if (!controller.signal.aborted) {
                    setSubnoterySuggestions(res.results ?? []);
                    setShowSuggestions(true);
                }
            } catch {
                if (!controller.signal.aborted) {
                    setSubnoterySuggestions([]);
                }
            } finally {
                if (!controller.signal.aborted) setSuggestionsLoading(false);
            }
        }, 200);
        return () => {
            clearTimeout(timeout);
            abortRef.current?.abort();
        };
    }, [subnoteryName]);

    // Close suggestions on click outside
    useEffect(() => {
        if (!showSuggestions) return;
        const handleClick = (e: MouseEvent) => {
            if (subnoteryInputRef.current && !subnoteryInputRef.current.contains(e.target as Node)) {
                setShowSuggestions(false);
            }
        };
        document.addEventListener("mousedown", handleClick);
        return () => document.removeEventListener("mousedown", handleClick);
    }, [showSuggestions]);

    if (!isAuthenticated) {
        return null;
    }

    const handleFileChange = (e: ChangeEvent<HTMLInputElement>) => {
        const file = e.target.files?.[0];
        if (!file) return;

        if (file.type !== "application/pdf") {
            toast({
                title: "Invalid file",
                description: "Only PDF files are allowed.",
                variant: "destructive",
            });
            return;
        }

        if (file.size > MAX_PDF_SIZE) {
            toast({
                title: "File too large",
                description: "Maximum PDF size is 50 MB.",
                variant: "destructive",
            });
            return;
        }

        setPdfFile(file);
    };

    const removeFile = () => {
        setPdfFile(null);
        if (fileInputRef.current) {
            fileInputRef.current.value = "";
        }
    };

    const handleThumbnailChange = (e: ChangeEvent<HTMLInputElement>) => {
        const file = e.target.files?.[0];
        if (!file) return;

        if (!ALLOWED_IMAGE_TYPES.includes(file.type)) {
            toast({
                title: "Invalid file",
                description: "Only JPEG, PNG, WebP, and GIF images are allowed.",
                variant: "destructive",
            });
            return;
        }

        if (file.size > MAX_THUMBNAIL_SIZE) {
            toast({
                title: "File too large",
                description: "Maximum thumbnail size is 5 MB.",
                variant: "destructive",
            });
            return;
        }

        setThumbnailFile(file);
        setThumbnailPreview(URL.createObjectURL(file));
    };

    const removeThumbnail = () => {
        setThumbnailFile(null);
        if (thumbnailPreview) {
            URL.revokeObjectURL(thumbnailPreview);
            setThumbnailPreview(null);
        }
        if (thumbnailInputRef.current) {
            thumbnailInputRef.current.value = "";
        }
    };

    const handleSubmit = async (e: FormEvent) => {
        e.preventDefault();

        if (!title.trim()) {
            toast({
                title: "Title required",
                description: "Please enter a title for your note.",
                variant: "destructive",
            });
            return;
        }

        if (!subnoteryName.trim()) {
            toast({
                title: "Community required",
                description: "Please enter a community name.",
                variant: "destructive",
            });
            return;
        }

        if (!pdfFile) {
            toast({
                title: "PDF required",
                description: "Please upload a PDF file for your note.",
                variant: "destructive",
            });
            return;
        }

        const priceInCents = Math.round(parseFloat(price) * 100);
        if (isNaN(priceInCents) || priceInCents < 0) {
            toast({
                title: "Invalid price",
                description: "Please enter a valid price (0 for free).",
                variant: "destructive",
            });
            return;
        }

        setSubmitting(true);

        try {
            const result = await createNote({
                title: title.trim(),
                description: description.trim() || undefined,
                subnotery_name: subnoteryName.trim(),
                price: priceInCents,
            });

            const noteId = (result as { id?: number }).id ?? (result as { ID?: number }).ID;

            if (!noteId) {
                throw new Error("Note was created but no ID was returned.");
            }

            // Upload the required PDF
            await uploadNotePDF(noteId, pdfFile);

            // Upload optional thumbnail
            if (thumbnailFile) {
                try {
                    await uploadNoteThumbnail(noteId, thumbnailFile);
                } catch {
                    // Non-fatal: note is created, just thumbnail failed
                    toast({
                        title: "Thumbnail upload failed",
                        description: "Note was created but thumbnail could not be uploaded.",
                        variant: "destructive",
                    });
                }
            }

            toast({
                title: "Note submitted!",
                description:
                    "Your note has been submitted for review. It will appear once approved.",
            });

            router.push("/");
        } catch (err: unknown) {
            const message =
                err instanceof Error ? err.message : "Failed to create note.";
            toast({ title: "Error", description: message, variant: "destructive" });
        } finally {
            setSubmitting(false);
        }
    };

    return (
        <div className="max-w-2xl mx-auto px-4 py-4">
            <Button
                variant="ghost"
                size="sm"
                className="mb-3 -ml-2 text-muted-foreground"
                onClick={() => router.back()}
            >
                <ArrowLeft className="h-4 w-4 mr-1" /> Back
            </Button>

            <Card className="border-border">
                <CardHeader className="pb-3">
                    <CardTitle className="text-lg">Create a Note</CardTitle>
                    <p className="text-sm text-muted-foreground">
                        Share your knowledge with the community. Notes require admin
                        approval before they appear publicly. A PDF file is required.
                        You can optionally add a description and thumbnail image.
                    </p>
                </CardHeader>
                <CardContent>
                    <form onSubmit={handleSubmit} className="space-y-4">
                        {/* Title */}
                        <div className="space-y-1.5">
                            <Label htmlFor="title">
                                Title <span className="text-destructive">*</span>
                            </Label>
                            <Input
                                id="title"
                                placeholder="An interesting and descriptive title"
                                value={title}
                                onChange={(e) => setTitle(e.target.value)}
                                maxLength={300}
                                required
                            />
                            <p className="text-xs text-muted-foreground text-right">
                                {title.length}/300
                            </p>
                        </div>

                        {/* Description */}
                        <div className="space-y-1.5">
                            <Label htmlFor="description">Description</Label>
                            <Textarea
                                id="description"
                                placeholder="Describe what your note covers, key topics, etc. (optional)"
                                value={description}
                                onChange={(e) => setDescription(e.target.value)}
                                maxLength={2000}
                                rows={4}
                            />
                            <p className="text-xs text-muted-foreground text-right">
                                {description.length}/2000
                            </p>
                        </div>

                        {/* Community name with autocomplete */}
                        <div className="space-y-1.5">
                            <Label htmlFor="subnotery">
                                Community (Subnotery) <span className="text-destructive">*</span>
                            </Label>
                            <div className="relative" ref={subnoteryInputRef}>
                                <Input
                                    id="subnotery"
                                    placeholder="e.g. ComputerScience"
                                    value={subnoteryName}
                                    onChange={(e) => setSubnoteryName(e.target.value)}
                                    onFocus={() => {
                                        if (subnoterySuggestions.length > 0) setShowSuggestions(true);
                                    }}
                                    autoComplete="off"
                                    required
                                />
                                {showSuggestions && (subnoterySuggestions.length > 0 || suggestionsLoading) && (
                                    <div className="absolute top-full left-0 right-0 mt-1 bg-popover border border-border rounded-lg shadow-lg z-50 overflow-hidden max-h-52 overflow-y-auto">
                                        {suggestionsLoading && subnoterySuggestions.length === 0 ? (
                                            <div className="flex items-center justify-center py-3">
                                                <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
                                            </div>
                                        ) : (
                                            subnoterySuggestions.map((sub) => (
                                                <button
                                                    key={sub.id}
                                                    type="button"
                                                    className="flex items-center gap-2.5 px-3 py-2 hover:bg-accent transition-colors w-full text-left"
                                                    onClick={() => {
                                                        setSubnoteryName(sub.name);
                                                        setShowSuggestions(false);
                                                    }}
                                                >
                                                    <Hash className="h-4 w-4 text-muted-foreground shrink-0" />
                                                    <div className="min-w-0 flex-1">
                                                        <p className="text-sm font-medium">n/{sub.name}</p>
                                                        {sub.member_count !== undefined && (
                                                            <p className="text-xs text-muted-foreground">
                                                                {sub.member_count} member{sub.member_count !== 1 ? "s" : ""}
                                                            </p>
                                                        )}
                                                    </div>
                                                </button>
                                            ))
                                        )}
                                    </div>
                                )}
                            </div>
                            <p className="text-xs text-muted-foreground">
                                Enter a community name. A new one will be created if it doesn&apos;t exist.
                            </p>
                        </div>

                        {/* Price */}
                        <div className="space-y-1.5">
                            <Label htmlFor="price">
                                Price (USD) <span className="text-destructive">*</span>
                            </Label>
                            <div className="relative">
                                <span className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground text-sm">
                                    $
                                </span>
                                <Input
                                    id="price"
                                    type="number"
                                    min="0"
                                    step="0.01"
                                    placeholder="0.00"
                                    value={price}
                                    onChange={(e) => setPrice(e.target.value)}
                                    className="pl-7"
                                    required
                                />
                            </div>
                            <p className="text-xs text-muted-foreground">
                                Set to 0 for a free note.
                            </p>
                        </div>

                        {/* PDF Upload (required) */}
                        <div className="space-y-1.5">
                            <Label>
                                PDF Attachment <span className="text-destructive">*</span>
                            </Label>
                            {pdfFile ? (
                                <div className="flex items-center gap-2 p-3 border rounded-md border-border bg-muted/50">
                                    <FileText className="h-5 w-5 text-primary shrink-0" />
                                    <div className="flex-1 min-w-0">
                                        <p className="text-sm font-medium truncate">{pdfFile.name}</p>
                                        <p className="text-xs text-muted-foreground">
                                            {(pdfFile.size / (1024 * 1024)).toFixed(2)} MB
                                        </p>
                                    </div>
                                    <Button
                                        type="button"
                                        variant="ghost"
                                        size="sm"
                                        className="h-7 w-7 p-0"
                                        onClick={removeFile}
                                    >
                                        <X className="h-4 w-4" />
                                    </Button>
                                </div>
                            ) : (
                                <button
                                    type="button"
                                    onClick={() => fileInputRef.current?.click()}
                                    className="w-full flex flex-col items-center gap-2 p-6 border-2 border-dashed rounded-lg border-muted-foreground/25 hover:border-primary/50 transition-colors text-muted-foreground hover:text-primary cursor-pointer"
                                >
                                    <Upload className="h-8 w-8" />
                                    <span className="text-sm font-medium">
                                        Click to upload a PDF (required)
                                    </span>
                                    <span className="text-xs">Max 50 MB</span>
                                </button>
                            )}
                            <input
                                ref={fileInputRef}
                                type="file"
                                accept="application/pdf"
                                onChange={handleFileChange}
                                className="hidden"
                            />
                        </div>

                        {/* Thumbnail Upload (optional) */}
                        <div className="space-y-1.5">
                            <Label>Thumbnail Image</Label>
                            {thumbnailFile ? (
                                <div className="flex items-center gap-3 p-3 border rounded-md border-border bg-muted/50">
                                    {thumbnailPreview && (
                                        /* eslint-disable-next-line @next/next/no-img-element */
                                        <img
                                            src={thumbnailPreview}
                                            alt="Thumbnail preview"
                                            className="rounded object-cover w-20 h-20"
                                        />
                                    )}
                                    <div className="flex-1 min-w-0">
                                        <p className="text-sm font-medium truncate">{thumbnailFile.name}</p>
                                        <p className="text-xs text-muted-foreground">
                                            {(thumbnailFile.size / (1024 * 1024)).toFixed(2)} MB
                                        </p>
                                    </div>
                                    <Button
                                        type="button"
                                        variant="ghost"
                                        size="sm"
                                        className="h-7 w-7 p-0"
                                        onClick={removeThumbnail}
                                    >
                                        <X className="h-4 w-4" />
                                    </Button>
                                </div>
                            ) : (
                                <button
                                    type="button"
                                    onClick={() => thumbnailInputRef.current?.click()}
                                    className="w-full flex flex-col items-center gap-2 p-4 border-2 border-dashed rounded-lg border-muted-foreground/25 hover:border-primary/50 transition-colors text-muted-foreground hover:text-primary cursor-pointer"
                                >
                                    <ImageIcon className="h-6 w-6" />
                                    <span className="text-sm font-medium">
                                        Click to upload a thumbnail (optional)
                                    </span>
                                    <span className="text-xs">JPEG, PNG, WebP, GIF — Max 5 MB</span>
                                </button>
                            )}
                            <input
                                ref={thumbnailInputRef}
                                type="file"
                                accept="image/jpeg,image/png,image/webp,image/gif"
                                onChange={handleThumbnailChange}
                                className="hidden"
                            />
                        </div>

                        {/* Submit */}
                        <Button type="submit" className="w-full" disabled={submitting || !pdfFile}>
                            {submitting ? (
                                <Loader2 className="h-4 w-4 animate-spin mr-2" />
                            ) : null}
                            {submitting ? "Submitting..." : "Submit Note"}
                        </Button>
                    </form>
                </CardContent>
            </Card>
        </div>
    );
}
