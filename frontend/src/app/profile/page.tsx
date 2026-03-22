// page.tsx — Own profile page: Reddit-style layout with Overview, Posts, and Settings tabs.
// Shows profile banner, avatar, username, bio, and tabbed content.
// Settings tab only visible to the profile owner for editing bio, avatar, visibility.
"use client";

import { NoteCard } from "@/components/feed/note-card";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import {
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
} from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import { Skeleton } from "@/components/ui/skeleton";
import { Textarea } from "@/components/ui/textarea";
import { useToast } from "@/hooks/use-toast";
import { avatarUrl, formatDate, timeAgo, userBannerUrl } from "@/lib/format";
import { resendVerification } from "@/services/auth";
import { getMyComments } from "@/services/comments";
import { getMyNotes } from "@/services/notes";
import { getStripeStatus, refreshStripeLink, setupStripeConnect } from "@/services/payouts";
import { deleteAvatar, deleteBanner, updateMyProfile, uploadAvatar, uploadBanner } from "@/services/profile";
import { getPurchaseHistory } from "@/services/purchases";
import { useAuthStore } from "@/stores/auth-store";
import type { MyComment, Note, NoteStatus, ProfileVisibility, PurchaseWithNote, StripeStatusResponse } from "@/types";
import {
    AlertCircle,
    BookOpen,
    Camera,
    CheckCircle,
    CreditCard,
    FileText,
    ImageIcon,
    Loader2,
    Mail,
    MessageSquare,
    Settings,
    Trash2,
} from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useCallback, useEffect, useRef, useState } from "react";

type ProfileTab = "all" | "posts" | "purchased" | "comments" | "settings";

export default function ProfilePage() {
    const router = useRouter();
    const { user, isAuthenticated, loading, setUser } = useAuthStore();
    const { toast } = useToast();
    const [activeTab, setActiveTab] = useState<ProfileTab>("all");

    const [bio, setBio] = useState("");
    const [visibility, setVisibility] = useState<ProfileVisibility>("public");
    const [saving, setSaving] = useState(false);

    const [uploadingAvatar, setUploadingAvatar] = useState(false);
    const [deletingAvatar, setDeletingAvatar] = useState(false);
    const avatarInputRef = useRef<HTMLInputElement>(null);

    const [uploadingBanner, setUploadingBanner] = useState(false);
    const [deletingBanner, setDeletingBanner] = useState(false);
    const bannerInputRef = useRef<HTMLInputElement>(null);

    // My Notes state
    const [myNotes, setMyNotes] = useState<Note[]>([]);
    const [notesTotal, setNotesTotal] = useState(0);
    const [notesPage, setNotesPage] = useState(1);
    const [notesLoading, setNotesLoading] = useState(false);
    const [statusFilter, setStatusFilter] = useState<NoteStatus | "all">("all");

    // My Comments state
    const [myComments, setMyComments] = useState<MyComment[]>([]);
    const [commentsTotal, setCommentsTotal] = useState(0);
    const [commentsPage, setCommentsPage] = useState(1);
    const [commentsLoading, setCommentsLoading] = useState(false);

    // Purchased notes state
    const [purchasedNotes, setPurchasedNotes] = useState<PurchaseWithNote[]>([]);
    const [purchasedTotal, setPurchasedTotal] = useState(0);
    const [purchasedPage, setPurchasedPage] = useState(1);
    const [purchasedLoading, setPurchasedLoading] = useState(false);

    // Stripe Connect state
    const [stripeStatus, setStripeStatus] = useState<StripeStatusResponse | null>(null);
    const [stripeLoading, setStripeLoading] = useState(false);
    const [stripeConnecting, setStripeConnecting] = useState(false);

    const fetchMyNotes = useCallback(async (page: number, status: NoteStatus | "all") => {
        setNotesLoading(true);
        try {
            const params: { page: number; limit: number; status?: NoteStatus } = {
                page,
                limit: 10,
            };
            if (status !== "all") params.status = status;
            const res = await getMyNotes(params);
            setMyNotes(res.notes || []);
            setNotesTotal(res.total);
        } catch {
            toast({ title: "Failed to load your notes", variant: "destructive" });
        } finally {
            setNotesLoading(false);
        }
    }, [toast]);

    const fetchMyComments = useCallback(async (page: number) => {
        setCommentsLoading(true);
        try {
            const res = await getMyComments({ page, limit: 10 });
            setMyComments(res.comments || []);
            setCommentsTotal(res.total);
        } catch {
            toast({ title: "Failed to load your comments", variant: "destructive" });
        } finally {
            setCommentsLoading(false);
        }
    }, [toast]);

    const fetchPurchased = useCallback(async (page: number) => {
        setPurchasedLoading(true);
        try {
            const res = await getPurchaseHistory({ page, limit: 10 });
            setPurchasedNotes(res.purchases || []);
            setPurchasedTotal(res.total);
        } catch {
            toast({ title: "Failed to load purchased notes", variant: "destructive" });
        } finally {
            setPurchasedLoading(false);
        }
    }, [toast]);

    useEffect(() => {
        if (user) {
            setBio(user.bio || "");
            setVisibility(user.profile_visibility || "public");
        }
    }, [user]);

    useEffect(() => {
        if (!isAuthenticated && !loading) {
            router.push("/login");
        }
    }, [isAuthenticated, loading, router]);

    useEffect(() => {
        if (isAuthenticated) {
            fetchMyNotes(notesPage, statusFilter);
        }
    }, [isAuthenticated, notesPage, statusFilter, fetchMyNotes]);

    useEffect(() => {
        if (isAuthenticated) {
            fetchMyComments(commentsPage);
        }
    }, [isAuthenticated, commentsPage, fetchMyComments]);

    useEffect(() => {
        if (isAuthenticated) {
            fetchPurchased(purchasedPage);
        }
    }, [isAuthenticated, purchasedPage, fetchPurchased]);

    // Load Stripe Connect status on settings tab
    useEffect(() => {
        if (isAuthenticated && activeTab === "settings") {
            setStripeLoading(true);
            getStripeStatus()
                .then(setStripeStatus)
                .catch(() => setStripeStatus(null))
                .finally(() => setStripeLoading(false));
        }
    }, [isAuthenticated, activeTab]);

    if (!isAuthenticated || !user) {
        return null;
    }

    const totalPages = Math.ceil(notesTotal / 10);
    const commentsTotalPages = Math.ceil(commentsTotal / 10);
    const purchasedTotalPages = Math.ceil(purchasedTotal / 10);

    const handleSaveProfile = async () => {
        setSaving(true);
        try {
            const updated = await updateMyProfile({
                bio: bio || undefined,
                profile_visibility: visibility,
            });
            setUser(updated);
            toast({ title: "Profile updated" });
        } catch {
            toast({ title: "Failed to update profile", variant: "destructive" });
        } finally {
            setSaving(false);
        }
    };

    const handleResendVerification = async () => {
        try {
            await resendVerification();
            toast({ title: "Verification email sent" });
        } catch {
            toast({ title: "Failed to resend verification", variant: "destructive" });
        }
    };

    const handleAvatarUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
        const file = e.target.files?.[0];
        if (!file) return;

        const allowedTypes = ["image/jpeg", "image/png", "image/webp", "image/gif"];
        if (!allowedTypes.includes(file.type)) {
            toast({
                title: "Invalid file type",
                description: "Only JPEG, PNG, WebP, and GIF images are allowed.",
                variant: "destructive",
            });
            return;
        }
        if (file.size > 5 * 1024 * 1024) {
            toast({
                title: "File too large",
                description: "Maximum avatar size is 5 MB.",
                variant: "destructive",
            });
            return;
        }

        setUploadingAvatar(true);
        try {
            const result = await uploadAvatar(file);
            setUser({ ...user, avatar_url: result.avatar_url });
            toast({ title: "Avatar updated" });
        } catch {
            toast({ title: "Failed to upload avatar", variant: "destructive" });
        } finally {
            setUploadingAvatar(false);
            if (avatarInputRef.current) avatarInputRef.current.value = "";
        }
    };

    const handleAvatarDelete = async () => {
        setDeletingAvatar(true);
        try {
            await deleteAvatar();
            setUser({ ...user, avatar_url: "" });
            toast({ title: "Avatar removed" });
        } catch {
            toast({ title: "Failed to delete avatar", variant: "destructive" });
        } finally {
            setDeletingAvatar(false);
        }
    };

    const handleBannerUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
        const file = e.target.files?.[0];
        if (!file) return;
        const allowedTypes = ["image/jpeg", "image/png", "image/webp", "image/gif"];
        if (!allowedTypes.includes(file.type)) {
            toast({ title: "Invalid file type", description: "Only JPEG, PNG, WebP, and GIF images are allowed.", variant: "destructive" });
            return;
        }
        if (file.size > 5 * 1024 * 1024) {
            toast({ title: "File too large", description: "Maximum banner size is 5 MB.", variant: "destructive" });
            return;
        }
        setUploadingBanner(true);
        try {
            const result = await uploadBanner(file);
            setUser({ ...user, banner_url: result.banner_url });
            toast({ title: "Banner updated" });
        } catch {
            toast({ title: "Failed to upload banner", variant: "destructive" });
        } finally {
            setUploadingBanner(false);
            if (bannerInputRef.current) bannerInputRef.current.value = "";
        }
    };

    const handleBannerDelete = async () => {
        setDeletingBanner(true);
        try {
            await deleteBanner();
            setUser({ ...user, banner_url: "" });
            toast({ title: "Banner removed" });
        } catch {
            toast({ title: "Failed to delete banner", variant: "destructive" });
        } finally {
            setDeletingBanner(false);
        }
    };

    const tabs: { key: ProfileTab; label: string; icon: React.ReactNode }[] = [
        { key: "all", label: "All", icon: null },
        { key: "posts", label: "Posts", icon: <FileText className="h-4 w-4" /> },
        { key: "purchased", label: "My Notes", icon: <BookOpen className="h-4 w-4" /> },
        { key: "comments", label: "Comments", icon: <MessageSquare className="h-4 w-4" /> },
        { key: "settings", label: "Settings", icon: <Settings className="h-4 w-4" /> },
    ];

    return (
        <div className="flex">
            <main className="flex-1 min-w-0 px-4 py-0">
                {/* Profile banner — click to upload/change */}
                <div className="relative h-28 -mx-4 group">
                    {user.banner_url ? (
                        /* eslint-disable-next-line @next/next/no-img-element */
                        <img
                            src={userBannerUrl(user.id, user.banner_url)}
                            alt="Profile banner"
                            className="w-full h-full object-cover"
                        />
                    ) : (
                        <div className="h-full bg-gradient-to-r from-primary/30 via-primary/15 to-primary/5" />
                    )}
                    <button
                        type="button"
                        onClick={() => bannerInputRef.current?.click()}
                        disabled={uploadingBanner}
                        className="absolute inset-0 flex items-center justify-center bg-black/40 opacity-0 group-hover:opacity-100 transition-opacity cursor-pointer"
                    >
                        {uploadingBanner ? (
                            <Loader2 className="h-6 w-6 text-white animate-spin" />
                        ) : (
                            <div className="flex items-center gap-2 text-white text-sm font-medium">
                                <Camera className="h-5 w-5" />
                                {user.banner_url ? "Change Banner" : "Upload Banner"}
                            </div>
                        )}
                    </button>
                    <input ref={bannerInputRef} type="file" accept="image/jpeg,image/png,image/webp,image/gif" onChange={handleBannerUpload} className="hidden" />
                </div>

                {/* Avatar + name row */}
                <div className="max-w-3xl mx-auto">
                    <div className="flex items-end gap-4 -mt-10 mb-2 px-2">
                        <div className="relative group">
                            <Avatar className="h-20 w-20 border-4 border-background">
                                <AvatarImage src={avatarUrl(user.id, user.avatar_url)} />
                                <AvatarFallback className="text-2xl">
                                    {user.username[0]?.toUpperCase()}
                                </AvatarFallback>
                            </Avatar>
                            <button
                                type="button"
                                onClick={() => avatarInputRef.current?.click()}
                                disabled={uploadingAvatar}
                                className="absolute inset-0 flex items-center justify-center bg-black/50 rounded-full opacity-0 group-hover:opacity-100 transition-opacity cursor-pointer"
                            >
                                {uploadingAvatar ? (
                                    <Loader2 className="h-5 w-5 text-white animate-spin" />
                                ) : (
                                    <Camera className="h-5 w-5 text-white" />
                                )}
                            </button>
                            <input
                                ref={avatarInputRef}
                                type="file"
                                accept="image/jpeg,image/png,image/webp,image/gif"
                                onChange={handleAvatarUpload}
                                className="hidden"
                            />
                        </div>
                        <div className="pb-1">
                            <h1 className="text-xl font-bold">{user.username}</h1>
                            <p className="text-sm text-muted-foreground">u/{user.username}</p>
                        </div>
                    </div>

                    {/* Email verification alert */}
                    {!user.email_verified && (
                        <Card className="border-yellow-500/50 bg-yellow-500/5 mb-3 mx-2">
                            <CardContent className="flex items-center gap-3 p-3">
                                <AlertCircle className="h-5 w-5 text-yellow-500 shrink-0" />
                                <div className="flex-1">
                                    <p className="text-sm font-medium">Email not verified</p>
                                    <p className="text-xs text-muted-foreground">
                                        Verify your email to access all features.
                                    </p>
                                </div>
                                <Button size="sm" variant="outline" onClick={handleResendVerification}>
                                    <Mail className="h-4 w-4 mr-1" />
                                    Resend
                                </Button>
                            </CardContent>
                        </Card>
                    )}

                    {/* Tab navigation — Reddit style */}
                    <div className="flex border-b border-border mb-4 px-2">
                        {tabs.map((tab) => (
                            <button
                                key={tab.key}
                                onClick={() => setActiveTab(tab.key)}
                                className={`flex items-center gap-1.5 px-4 py-2.5 text-sm font-medium border-b-2 transition-colors ${activeTab === tab.key
                                    ? "border-primary text-primary"
                                    : "border-transparent text-muted-foreground hover:text-foreground"
                                    }`}
                            >
                                {tab.icon}
                                {tab.label}
                            </button>
                        ))}
                    </div>

                    {/* ─── All Tab ─── */}
                    {activeTab === "all" && (
                        <div className="space-y-4 px-2">
                            {user.bio && (
                                <Card className="border-border p-4">
                                    <p className="text-sm">{user.bio}</p>
                                </Card>
                            )}

                            {/* Recent posts */}
                            <h2 className="text-sm font-semibold text-muted-foreground uppercase tracking-wider">
                                Recent Posts
                            </h2>
                            {notesLoading ? (
                                <div className="space-y-3">
                                    {Array.from({ length: 3 }).map((_, i) => (
                                        <Skeleton key={i} className="h-32 w-full" />
                                    ))}
                                </div>
                            ) : myNotes.length === 0 ? (
                                <Card className="border-border p-6 text-center">
                                    <FileText className="h-8 w-8 text-muted-foreground mx-auto mb-2" />
                                    <p className="text-sm text-muted-foreground">
                                        You haven&apos;t created any notes yet.
                                    </p>
                                    <Button variant="outline" size="sm" className="mt-3" asChild>
                                        <Link href="/submit">Create Note</Link>
                                    </Button>
                                </Card>
                            ) : (
                                <div className="space-y-4">
                                    {myNotes.slice(0, 5).map((note) => (
                                        <NoteCard key={note.id} note={note} />
                                    ))}
                                </div>
                            )}

                            {/* Recent comments */}
                            <h2 className="text-sm font-semibold text-muted-foreground uppercase tracking-wider mt-6">
                                Recent Comments
                            </h2>
                            {commentsLoading ? (
                                <div className="space-y-3">
                                    {Array.from({ length: 3 }).map((_, i) => (
                                        <Skeleton key={i} className="h-20 w-full" />
                                    ))}
                                </div>
                            ) : myComments.length === 0 ? (
                                <Card className="border-border p-6 text-center">
                                    <MessageSquare className="h-8 w-8 text-muted-foreground mx-auto mb-2" />
                                    <p className="text-sm text-muted-foreground">
                                        You haven&apos;t posted any comments yet.
                                    </p>
                                </Card>
                            ) : (
                                <div className="space-y-3">
                                    {myComments.slice(0, 5).map((comment) => (
                                        <Card key={comment.id} className="border-border">
                                            <CardContent className="p-3">
                                                <div className="flex items-center gap-1.5 text-xs text-muted-foreground mb-1">
                                                    <MessageSquare className="h-3 w-3" />
                                                    <span>Comment on</span>
                                                    <Link
                                                        href={`/notes/${comment.note_id}`}
                                                        className="font-semibold text-foreground hover:underline"
                                                    >
                                                        {comment.note_title || `Note #${comment.note_id}`}
                                                    </Link>
                                                    <span>•</span>
                                                    <span>{timeAgo(comment.created_at)}</span>
                                                </div>
                                                <p className="text-sm line-clamp-3 whitespace-pre-wrap">
                                                    {comment.body}
                                                </p>
                                                <div className="flex items-center gap-3 mt-1.5 text-xs text-muted-foreground">
                                                    <span>{comment.upvotes} upvotes</span>
                                                    <span>{comment.downvotes} downvotes</span>
                                                </div>
                                            </CardContent>
                                        </Card>
                                    ))}
                                </div>
                            )}
                        </div>
                    )}

                    {/* ─── Posts Tab ─── */}
                    {activeTab === "posts" && (
                        <div className="space-y-4 px-2">
                            <div className="flex items-center gap-2">
                                <Label className="text-sm whitespace-nowrap">Filter:</Label>
                                <Select
                                    value={statusFilter}
                                    onValueChange={(v) => {
                                        setStatusFilter(v as NoteStatus | "all");
                                        setNotesPage(1);
                                    }}
                                >
                                    <SelectTrigger className="w-[130px]">
                                        <SelectValue />
                                    </SelectTrigger>
                                    <SelectContent>
                                        <SelectItem value="all">All</SelectItem>
                                        <SelectItem value="Pending">Pending</SelectItem>
                                        <SelectItem value="Approved">Approved</SelectItem>
                                        <SelectItem value="Rejected">Rejected</SelectItem>
                                    </SelectContent>
                                </Select>
                            </div>

                            {notesLoading ? (
                                <div className="flex justify-center py-8">
                                    <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
                                </div>
                            ) : myNotes.length === 0 ? (
                                <Card className="border-border p-6 text-center">
                                    <FileText className="h-8 w-8 text-muted-foreground mx-auto mb-2" />
                                    <p className="text-sm text-muted-foreground">
                                        {statusFilter === "all"
                                            ? "You haven't created any notes yet."
                                            : `No ${statusFilter.toLowerCase()} notes found.`}
                                    </p>
                                </Card>
                            ) : (
                                <>
                                    <div className="space-y-4">
                                        {myNotes.map((note) => (
                                            <NoteCard key={note.id} note={note} />
                                        ))}
                                    </div>

                                    {totalPages > 1 && (
                                        <div className="flex items-center justify-center gap-2 pt-2">
                                            <Button
                                                size="sm"
                                                variant="outline"
                                                disabled={notesPage <= 1}
                                                onClick={() => setNotesPage((p) => p - 1)}
                                            >
                                                Previous
                                            </Button>
                                            <span className="text-xs text-muted-foreground">
                                                Page {notesPage} of {totalPages}
                                            </span>
                                            <Button
                                                size="sm"
                                                variant="outline"
                                                disabled={notesPage >= totalPages}
                                                onClick={() => setNotesPage((p) => p + 1)}
                                            >
                                                Next
                                            </Button>
                                        </div>
                                    )}
                                </>
                            )}
                        </div>
                    )}

                    {/* ─── Purchased (My Notes) Tab ─── */}
                    {activeTab === "purchased" && (
                        <div className="space-y-4 px-2">
                            {purchasedLoading ? (
                                <div className="space-y-3">
                                    {Array.from({ length: 3 }).map((_, i) => (
                                        <Skeleton key={i} className="h-16 w-full" />
                                    ))}
                                </div>
                            ) : purchasedNotes.length === 0 ? (
                                <Card className="border-border p-6 text-center">
                                    <BookOpen className="h-8 w-8 text-muted-foreground mx-auto mb-2" />
                                    <p className="text-sm text-muted-foreground">
                                        No purchased notes yet.
                                    </p>
                                    <Button variant="outline" size="sm" className="mt-3" asChild>
                                        <Link href="/">Browse Notes</Link>
                                    </Button>
                                </Card>
                            ) : (
                                <div className="space-y-2">
                                    {purchasedNotes.map((p) => (
                                        <Card key={p.purchase_id} className="border-border p-3">
                                            <div className="flex items-center gap-3">
                                                <div className="h-9 w-9 rounded bg-primary/10 flex items-center justify-center shrink-0">
                                                    <FileText className="h-4 w-4 text-primary" />
                                                </div>
                                                <div className="flex-1 min-w-0">
                                                    <Link
                                                        href={`/notes/${p.note_id}`}
                                                        className="font-medium text-sm hover:text-primary block truncate"
                                                    >
                                                        {p.note_title}
                                                    </Link>
                                                    <p className="text-xs text-muted-foreground">
                                                        by {p.note_author} &bull; {timeAgo(p.purchased_at)}
                                                    </p>
                                                </div>
                                                <span className="text-xs font-medium text-muted-foreground">
                                                    {p.price_paid === 0 ? "Free" : `$${(p.price_paid / 100).toFixed(2)}`}
                                                </span>
                                            </div>
                                        </Card>
                                    ))}
                                </div>
                            )}

                            {/* Pagination */}
                            {purchasedTotalPages > 1 && (
                                <div className="flex items-center justify-center gap-4 mt-4">
                                    <Button
                                        variant="outline"
                                        size="sm"
                                        disabled={purchasedPage <= 1}
                                        onClick={() => setPurchasedPage((p) => Math.max(1, p - 1))}
                                    >
                                        Previous
                                    </Button>
                                    <span className="text-xs text-muted-foreground">
                                        Page {purchasedPage} of {purchasedTotalPages}
                                    </span>
                                    <Button
                                        variant="outline"
                                        size="sm"
                                        disabled={purchasedPage >= purchasedTotalPages}
                                        onClick={() => setPurchasedPage((p) => p + 1)}
                                    >
                                        Next
                                    </Button>
                                </div>
                            )}
                        </div>
                    )}

                    {/* ─── Comments Tab ─── */}
                    {activeTab === "comments" && (
                        <div className="space-y-4 px-2">
                            {commentsLoading ? (
                                <div className="flex justify-center py-8">
                                    <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
                                </div>
                            ) : myComments.length === 0 ? (
                                <Card className="border-border p-6 text-center">
                                    <MessageSquare className="h-8 w-8 text-muted-foreground mx-auto mb-2" />
                                    <p className="text-sm text-muted-foreground">
                                        You haven&apos;t posted any comments yet.
                                    </p>
                                </Card>
                            ) : (
                                <>
                                    <div className="space-y-3">
                                        {myComments.map((comment) => (
                                            <Card key={comment.id} className="border-border">
                                                <CardContent className="p-3">
                                                    <div className="flex items-center gap-1.5 text-xs text-muted-foreground mb-1">
                                                        <MessageSquare className="h-3 w-3" />
                                                        <span>Comment on</span>
                                                        <Link
                                                            href={`/notes/${comment.note_id}`}
                                                            className="font-semibold text-foreground hover:underline"
                                                        >
                                                            {comment.note_title || `Note #${comment.note_id}`}
                                                        </Link>
                                                        <span>•</span>
                                                        <span>{timeAgo(comment.created_at)}</span>
                                                    </div>
                                                    <p className="text-sm line-clamp-3 whitespace-pre-wrap">
                                                        {comment.body}
                                                    </p>
                                                    <div className="flex items-center gap-3 mt-1.5 text-xs text-muted-foreground">
                                                        <span>{comment.upvotes} upvotes</span>
                                                        <span>{comment.downvotes} downvotes</span>
                                                    </div>
                                                </CardContent>
                                            </Card>
                                        ))}
                                    </div>

                                    {commentsTotalPages > 1 && (
                                        <div className="flex items-center justify-center gap-2 pt-2">
                                            <Button
                                                size="sm"
                                                variant="outline"
                                                disabled={commentsPage <= 1}
                                                onClick={() => setCommentsPage((p) => p - 1)}
                                            >
                                                Previous
                                            </Button>
                                            <span className="text-xs text-muted-foreground">
                                                Page {commentsPage} of {commentsTotalPages}
                                            </span>
                                            <Button
                                                size="sm"
                                                variant="outline"
                                                disabled={commentsPage >= commentsTotalPages}
                                                onClick={() => setCommentsPage((p) => p + 1)}
                                            >
                                                Next
                                            </Button>
                                        </div>
                                    )}
                                </>
                            )}
                        </div>
                    )}

                    {/* ─── Settings Tab ─── */}
                    {activeTab === "settings" && (
                        <div className="space-y-4 px-2 max-w-lg">
                            <Card className="border-border">
                                <CardHeader className="py-3 px-4">
                                    <CardTitle className="text-sm">Avatar</CardTitle>
                                </CardHeader>
                                <CardContent className="px-4 pb-4 pt-0">
                                    <div className="flex items-center gap-4">
                                        <Avatar className="h-16 w-16">
                                            <AvatarImage src={avatarUrl(user.id, user.avatar_url)} />
                                            <AvatarFallback className="text-lg">
                                                {user.username[0]?.toUpperCase()}
                                            </AvatarFallback>
                                        </Avatar>
                                        <div className="flex items-center gap-2">
                                            <Button
                                                variant="outline"
                                                size="sm"
                                                className="h-7 text-xs"
                                                onClick={() => avatarInputRef.current?.click()}
                                                disabled={uploadingAvatar}
                                            >
                                                {uploadingAvatar ? (
                                                    <Loader2 className="h-3 w-3 animate-spin mr-1" />
                                                ) : (
                                                    <Camera className="h-3 w-3 mr-1" />
                                                )}
                                                {user.avatar_url ? "Change" : "Upload"}
                                            </Button>
                                            {user.avatar_url && (
                                                <Button
                                                    variant="outline"
                                                    size="sm"
                                                    className="h-7 text-xs text-destructive hover:text-destructive"
                                                    onClick={handleAvatarDelete}
                                                    disabled={deletingAvatar}
                                                >
                                                    {deletingAvatar ? (
                                                        <Loader2 className="h-3 w-3 animate-spin mr-1" />
                                                    ) : (
                                                        <Trash2 className="h-3 w-3 mr-1" />
                                                    )}
                                                    Remove
                                                </Button>
                                            )}
                                        </div>
                                    </div>
                                </CardContent>
                            </Card>

                            <Card className="border-border">
                                <CardHeader className="py-3 px-4">
                                    <CardTitle className="text-sm">Banner</CardTitle>
                                </CardHeader>
                                <CardContent className="px-4 pb-4 pt-0">
                                    <div className="space-y-3">
                                        <div className="w-full h-20 rounded-md overflow-hidden border">
                                            {user.banner_url ? (
                                                /* eslint-disable-next-line @next/next/no-img-element */
                                                <img
                                                    src={userBannerUrl(user.id, user.banner_url)}
                                                    alt="Profile banner"
                                                    className="w-full h-full object-cover"
                                                />
                                            ) : (
                                                <div className="h-full bg-gradient-to-r from-primary/30 via-primary/15 to-primary/5" />
                                            )}
                                        </div>
                                        <div className="flex items-center gap-2">
                                            <Button
                                                variant="outline"
                                                size="sm"
                                                className="h-7 text-xs"
                                                onClick={() => bannerInputRef.current?.click()}
                                                disabled={uploadingBanner}
                                            >
                                                {uploadingBanner ? (
                                                    <Loader2 className="h-3 w-3 animate-spin mr-1" />
                                                ) : (
                                                    <ImageIcon className="h-3 w-3 mr-1" />
                                                )}
                                                {user.banner_url ? "Change" : "Upload"}
                                            </Button>
                                            {user.banner_url && (
                                                <Button
                                                    variant="outline"
                                                    size="sm"
                                                    className="h-7 text-xs text-destructive hover:text-destructive"
                                                    onClick={handleBannerDelete}
                                                    disabled={deletingBanner}
                                                >
                                                    {deletingBanner ? (
                                                        <Loader2 className="h-3 w-3 animate-spin mr-1" />
                                                    ) : (
                                                        <Trash2 className="h-3 w-3 mr-1" />
                                                    )}
                                                    Remove
                                                </Button>
                                            )}
                                        </div>
                                    </div>
                                </CardContent>
                            </Card>

                            <Card className="border-border">
                                <CardHeader className="py-3 px-4">
                                    <CardTitle className="text-sm">Bio</CardTitle>
                                </CardHeader>
                                <CardContent className="px-4 pb-4 pt-0 space-y-2">
                                    <Textarea
                                        id="bio"
                                        value={bio}
                                        onChange={(e) => setBio(e.target.value)}
                                        placeholder="Tell us about yourself"
                                        maxLength={500}
                                        className="resize-none"
                                    />
                                    <p className="text-xs text-muted-foreground">{bio.length}/500</p>
                                </CardContent>
                            </Card>

                            <Card className="border-border">
                                <CardHeader className="py-3 px-4">
                                    <CardTitle className="text-sm">Profile Visibility</CardTitle>
                                </CardHeader>
                                <CardContent className="px-4 pb-4 pt-0">
                                    <Select
                                        value={visibility}
                                        onValueChange={(v) => setVisibility(v as ProfileVisibility)}
                                    >
                                        <SelectTrigger className="w-[180px]">
                                            <SelectValue />
                                        </SelectTrigger>
                                        <SelectContent>
                                            <SelectItem value="public">Public</SelectItem>
                                            <SelectItem value="private">Private</SelectItem>
                                        </SelectContent>
                                    </Select>
                                </CardContent>
                            </Card>

                            <Button onClick={handleSaveProfile} disabled={saving}>
                                {saving && <Loader2 className="h-4 w-4 animate-spin mr-2" />}
                                Save Changes
                            </Button>

                            {/* Stripe Connect — Payout Setup */}
                            <Card className="border-border">
                                <CardHeader className="py-3 px-4">
                                    <CardTitle className="text-sm flex items-center gap-1.5">
                                        <CreditCard className="h-4 w-4" />
                                        Payout Setup
                                    </CardTitle>
                                </CardHeader>
                                <CardContent className="px-4 pb-4 pt-0 space-y-3">
                                    {stripeLoading ? (
                                        <div className="flex items-center gap-2 text-sm text-muted-foreground">
                                            <Loader2 className="h-4 w-4 animate-spin" />
                                            Loading payout status...
                                        </div>
                                    ) : stripeStatus?.payout_enabled ? (
                                        <div className="space-y-2">
                                            <div className="flex items-center gap-2">
                                                <CheckCircle className="h-4 w-4 text-green-500" />
                                                <span className="text-sm font-medium text-green-600">Payouts enabled</span>
                                            </div>
                                            <p className="text-xs text-muted-foreground">
                                                Your Stripe account is connected. Earnings from note sales will be automatically transferred to your account.
                                            </p>
                                            <p className="text-xs text-muted-foreground">
                                                Fee structure: 25¢ flat fee + 15% marketplace fee per sale.
                                            </p>
                                        </div>
                                    ) : stripeStatus?.onboarding_complete ? (
                                        <div className="space-y-2">
                                            <div className="flex items-center gap-2">
                                                <AlertCircle className="h-4 w-4 text-yellow-500" />
                                                <span className="text-sm font-medium">Onboarding complete — awaiting verification</span>
                                            </div>
                                            <p className="text-xs text-muted-foreground">
                                                Stripe is reviewing your account. Payouts will be enabled once verification is complete.
                                            </p>
                                        </div>
                                    ) : (
                                        <div className="space-y-2">
                                            <p className="text-sm text-muted-foreground">
                                                Connect a Stripe account to receive payouts from your note sales. Without a connected account, earnings are retained by Notery.
                                            </p>
                                            <p className="text-xs text-muted-foreground">
                                                Fee structure: 25¢ flat fee + 15% marketplace fee per sale.
                                            </p>
                                            <Button
                                                variant="outline"
                                                size="sm"
                                                disabled={stripeConnecting}
                                                onClick={async () => {
                                                    setStripeConnecting(true);
                                                    try {
                                                        const res = stripeStatus?.has_account
                                                            ? await refreshStripeLink()
                                                            : await setupStripeConnect();
                                                        window.location.href = res.onboarding_url;
                                                    } catch (err: unknown) {
                                                        const msg = err instanceof Error ? err.message : "Failed to start Stripe setup";
                                                        toast({ title: "Error", description: msg, variant: "destructive" });
                                                    } finally {
                                                        setStripeConnecting(false);
                                                    }
                                                }}
                                            >
                                                {stripeConnecting ? (
                                                    <Loader2 className="h-4 w-4 animate-spin mr-1" />
                                                ) : (
                                                    <CreditCard className="h-4 w-4 mr-1" />
                                                )}
                                                {stripeStatus?.has_account ? "Continue Setup" : "Connect Stripe"}
                                            </Button>
                                        </div>
                                    )}
                                </CardContent>
                            </Card>
                        </div>
                    )}
                </div>
            </main>

            {/* Right sidebar — profile info card */}
            <aside className="hidden lg:block w-72 shrink-0 border-l border-border">
                <div className="sticky top-12 h-[calc(100vh-48px)] overflow-y-auto p-4 space-y-4">
                    <Card className="border-border">
                        <CardHeader className="py-3 px-4">
                            <CardTitle className="text-sm font-semibold">
                                About {user.username}
                            </CardTitle>
                        </CardHeader>
                        <CardContent className="px-4 pb-4 pt-0 space-y-3">
                            {user.bio && (
                                <>
                                    <p className="text-xs text-muted-foreground leading-relaxed">
                                        {user.bio}
                                    </p>
                                    <Separator />
                                </>
                            )}
                            <div className="grid grid-cols-2 gap-2 text-xs">
                                <div>
                                    <p className="font-semibold text-foreground">Posts</p>
                                    <p className="text-muted-foreground">{notesTotal}</p>
                                </div>
                                <div>
                                    <p className="font-semibold text-foreground">Joined</p>
                                    <p className="text-muted-foreground">
                                        {formatDate(user.created_at)}
                                    </p>
                                </div>
                            </div>
                            <Separator />
                            <div className="grid grid-cols-2 gap-2 text-xs">
                                <div>
                                    <p className="font-semibold text-foreground">Post Notoriety</p>
                                    <p className="text-muted-foreground">{Math.round(user.post_karma ?? 0)}</p>
                                </div>
                                <div>
                                    <p className="font-semibold text-foreground">Comment Notoriety</p>
                                    <p className="text-muted-foreground">{Math.round(user.comment_karma ?? 0)}</p>
                                </div>
                            </div>
                            {user.email_verified && (
                                <div className="flex items-center gap-1 text-xs text-green-500">
                                    <CheckCircle className="h-3 w-3" />
                                    Verified
                                </div>
                            )}
                        </CardContent>
                    </Card>
                </div>
            </aside>
        </div>
    );
}
