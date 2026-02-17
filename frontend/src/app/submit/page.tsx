// page.tsx — Submit (create) a new note.
"use client";

import { useState, useEffect, useRef, type FormEvent, type ChangeEvent } from "react";
import { useRouter } from "next/navigation";
import { Upload, FileText, X, ArrowLeft, Loader2 } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useToast } from "@/hooks/use-toast";
import { useAuthStore } from "@/stores/auth-store";
import { createNote, uploadNotePDF } from "@/services/notes";

const MAX_PDF_SIZE = 50 * 1024 * 1024; // 50 MB

export default function SubmitPage() {
  const router = useRouter();
  const { isAuthenticated, loading } = useAuthStore();
  const { toast } = useToast();
  const fileInputRef = useRef<HTMLInputElement>(null);

  const [title, setTitle] = useState("");
  const [author, setAuthor] = useState("");
  const [subnoteryName, setSubnoteryName] = useState("");
  const [price, setPrice] = useState("");
  const [pdfFile, setPdfFile] = useState<File | null>(null);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    if (!isAuthenticated && !loading) {
      router.push("/login");
    }
  }, [isAuthenticated, loading, router]);

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

    if (!author.trim()) {
      toast({
        title: "Author required",
        description: "Please enter an author name.",
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
        author: author.trim(),
        subnotery_name: subnoteryName.trim(),
        price: priceInCents,
      });

      const noteId = (result as { id?: number }).id ?? (result as { ID?: number }).ID;

      // Upload PDF if provided
      if (pdfFile && noteId) {
        try {
          await uploadNotePDF(noteId, pdfFile);
        } catch {
          toast({
            title: "Note created, but PDF upload failed",
            description: "You can try uploading the PDF again from the note page.",
            variant: "destructive",
          });
        }
      }

      toast({
        title: "Note submitted!",
        description:
          "Your note has been submitted for review. It will appear once approved.",
      });

      router.push(noteId ? `/notes/${noteId}` : "/");
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
            approval before they appear publicly.
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

            {/* Author */}
            <div className="space-y-1.5">
              <Label htmlFor="author">
                Author <span className="text-destructive">*</span>
              </Label>
              <Input
                id="author"
                placeholder="Author name"
                value={author}
                onChange={(e) => setAuthor(e.target.value)}
                maxLength={100}
                required
              />
            </div>

            {/* Community name */}
            <div className="space-y-1.5">
              <Label htmlFor="subnotery">
                Community (Subnotery) <span className="text-destructive">*</span>
              </Label>
              <Input
                id="subnotery"
                placeholder="e.g. ComputerScience"
                value={subnoteryName}
                onChange={(e) => setSubnoteryName(e.target.value)}
                required
              />
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

            {/* PDF Upload */}
            <div className="space-y-1.5">
              <Label>PDF Attachment (optional)</Label>
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
                    Click to upload a PDF
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

            {/* Submit */}
            <Button type="submit" className="w-full" disabled={submitting}>
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
