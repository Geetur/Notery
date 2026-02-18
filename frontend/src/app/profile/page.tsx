// page.tsx — Own profile page with settings and created notes tabs.
"use client";

import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
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
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import { useToast } from "@/hooks/use-toast";
import { avatarUrl } from "@/lib/format";
import { resendVerification } from "@/services/auth";
import { getMyNotes } from "@/services/notes";
import { deleteAvatar, updateMyProfile, uploadAvatar } from "@/services/profile";
import { useAuthStore } from "@/stores/auth-store";
import type { Note, NoteStatus, ProfileVisibility } from "@/types";
import {
    AlertCircle,
    Camera,
    CheckCircle,
    Clock,
    FileText,
    Loader2,
    Mail,
    Trash2,
    XCircle,
} from "lucide-react";
import { useRouter } from "next/navigation";
import { useCallback, useEffect, useRef, useState } from "react";

export default function ProfilePage() {
    const router = useRouter();
    const { user, isAuthenticated, loading, setUser } = useAuthStore();
    const { toast } = useToast();

    const [bio, setBio] = useState("");
    const [visibility, setVisibility] = useState<ProfileVisibility>("public");
    const [saving, setSaving] = useState(false);

    const [uploadingAvatar, setUploadingAvatar] = useState(false);
    const [deletingAvatar, setDeletingAvatar] = useState(false);
    const avatarInputRef = useRef<HTMLInputElement>(null);

    // My Notes state
    const [myNotes, setMyNotes] = useState<Note[]>([]);
    const [notesTotal, setNotesTotal] = useState(0);
    const [notesPage, setNotesPage] = useState(1);
    const [notesLoading, setNotesLoading] = useState(false);
    const [statusFilter, setStatusFilter] = useState<NoteStatus | "all">("all");

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

    // Fetch notes when page or filter changes
    useEffect(() => {
        if (isAuthenticated) {
            fetchMyNotes(notesPage, statusFilter);
        }
    }, [isAuthenticated, notesPage, statusFilter, fetchMyNotes]);

    if (!isAuthenticated || !user) {
        return null;
    }

    const statusBadge = (status: NoteStatus) => {
        switch (status) {
            case "Approved":
                return (
                    <Badge className="bg-green-500/15 text-green-500 border-green-500/30 hover:bg-green-500/20">
                        <CheckCircle className="h-3 w-3 mr-1" />
                        Approved
                    </Badge>
                );
            case "Pending":
                return (
                    <Badge className="bg-yellow-500/15 text-yellow-500 border-yellow-500/30 hover:bg-yellow-500/20">
                        <Clock className="h-3 w-3 mr-1" />
                        Pending
                    </Badge>
                );
            case "Rejected":
                return (
                    <Badge variant="destructive" className="bg-red-500/15 text-red-500 border-red-500/30 hover:bg-red-500/20">
                        <XCircle className="h-3 w-3 mr-1" />
                        Rejected
                    </Badge>
                );
        }
    };

    const totalPages = Math.ceil(notesTotal / 10);

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
            // Re-fetch profile to get updated avatar_url
            setUser({ ...user, avatar_url: result.avatar_url });
            toast({ title: "Avatar updated" });
        } catch {
            toast({ title: "Failed to upload avatar", variant: "destructive" });
        } finally {
            setUploadingAvatar(false);
            if (avatarInputRef.current) {
                avatarInputRef.current.value = "";
            }
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

    return (
        <div className="max-w-2xl mx-auto px-4 py-4 space-y-4">
            <h1 className="text-xl font-bold">Profile</h1>

            {/* Email verification alert */}
            {!user.email_verified && (
                <Card className="border-yellow-500/50 bg-yellow-500/5">
                    <CardContent className="flex items-center gap-3 p-3">
                        <AlertCircle className="h-5 w-5 text-yellow-500 shrink-0" />
                        <div className="flex-1">
                            <p className="text-sm font-medium">Email not verified</p>
                            <p className="text-xs text-muted-foreground">
                                Please verify your email to access all features.
                            </p>
                        </div>
                        <Button size="sm" variant="outline" onClick={handleResendVerification}>
                            <Mail className="h-4 w-4 mr-1" />
                            Resend
                        </Button>
                    </CardContent>
                </Card>
            )}

            <Tabs defaultValue="settings">
                <TabsList className="w-full">
                    <TabsTrigger value="settings" className="flex-1">Settings</TabsTrigger>
                    <TabsTrigger value="my-notes" className="flex-1">
                        <FileText className="h-4 w-4 mr-1" />
                        My Notes
                        {notesTotal > 0 && (
                            <span className="ml-1.5 text-xs bg-muted-foreground/20 rounded-full px-1.5">
                                {notesTotal}
                            </span>
                        )}
                    </TabsTrigger>
                </TabsList>

                {/* ─── Settings Tab ─── */}
                <TabsContent value="settings" className="space-y-4">
                    {/* Profile info */}
                    <Card className="border-border">
                        <CardHeader>
                            <CardTitle className="text-sm">Profile Information</CardTitle>
                        </CardHeader>
                        <CardContent className="space-y-4">
                            {/* Avatar with upload/delete controls */}
                            <div className="flex items-center gap-4">
                                <div className="relative group">
                                    <Avatar className="h-16 w-16">
                                        <AvatarImage src={avatarUrl(user.id, user.avatar_url)} />
                                        <AvatarFallback className="text-lg">
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
                                <div className="flex-1">
                                    <p className="font-medium">{user.username}</p>
                                    <p className="text-xs text-muted-foreground">{user.email}</p>
                                    {user.email_verified && (
                                        <span className="flex items-center gap-1 text-xs text-green-500">
                                            <CheckCircle className="h-3 w-3" /> Verified
                                        </span>
                                    )}
                                    <div className="flex items-center gap-2 mt-1.5">
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
                                            {user.avatar_url ? "Change Avatar" : "Upload Avatar"}
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
                            </div>

                            <Separator />

                            {/* Bio */}
                            <div>
                                <Label htmlFor="bio">Bio</Label>
                                <Textarea
                                    id="bio"
                                    value={bio}
                                    onChange={(e) => setBio(e.target.value)}
                                    placeholder="Tell us about yourself"
                                    maxLength={500}
                                    className="resize-none"
                                />
                                <p className="text-xs text-muted-foreground mt-1">{bio.length}/500</p>
                            </div>

                            {/* Visibility */}
                            <div>
                                <Label>Profile Visibility</Label>
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
                            </div>

                            <Button onClick={handleSaveProfile} disabled={saving}>
                                {saving && <Loader2 className="h-4 w-4 animate-spin mr-2" />}
                                Save Changes
                            </Button>
                        </CardContent>
                    </Card>
                </TabsContent>

                {/* ─── My Notes Tab ─── */}
                <TabsContent value="my-notes" className="space-y-4">
                    {/* Status filter */}
                    <div className="flex items-center gap-2">
                        <Label className="text-sm whitespace-nowrap">Filter by status:</Label>
                        <Select
                            value={statusFilter}
                            onValueChange={(v) => {
                                setStatusFilter(v as NoteStatus | "all");
                                setNotesPage(1);
                            }}
                        >
                            <SelectTrigger className="w-[150px]">
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
                        <Card className="border-border">
                            <CardContent className="flex flex-col items-center justify-center py-8 text-center">
                                <FileText className="h-10 w-10 text-muted-foreground mb-2" />
                                <p className="text-sm text-muted-foreground">
                                    {statusFilter === "all"
                                        ? "You haven't created any notes yet."
                                        : `No ${statusFilter.toLowerCase()} notes found.`}
                                </p>
                            </CardContent>
                        </Card>
                    ) : (
                        <>
                            <div className="space-y-2">
                                {myNotes.map((note) => (
                                    <Card key={note.id} className="border-border">
                                        <CardContent className="flex items-center justify-between p-3">
                                            <div className="flex-1 min-w-0">
                                                <p className="font-medium text-sm truncate">
                                                    {note.title}
                                                </p>
                                                <p className="text-xs text-muted-foreground">
                                                    by {note.author} &middot;{" "}
                                                    {new Date(note.created_at).toLocaleDateString()}
                                                    {note.price > 0 && (
                                                        <> &middot; ${(note.price / 100).toFixed(2)}</>
                                                    )}
                                                </p>
                                            </div>
                                            <div className="flex items-center gap-2 ml-3 shrink-0">
                                                {statusBadge(note.status)}
                                                {note.has_pdf && (
                                                    <span className="text-xs text-muted-foreground">
                                                        PDF
                                                    </span>
                                                )}
                                            </div>
                                        </CardContent>
                                    </Card>
                                ))}
                            </div>

                            {/* Pagination */}
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
                </TabsContent>
            </Tabs>
        </div>
    );
}
