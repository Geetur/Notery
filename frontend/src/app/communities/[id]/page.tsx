// page.tsx — Community (subnotery) detail page.
// Shows community info, approved notes, and admin controls (pending notes + settings).
"use client";

import { NoteCard } from "@/components/feed";
import { SortTabs } from "@/components/feed/sort-tabs";
import { RightSidebar } from "@/components/layout/right-sidebar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useToast } from "@/hooks/use-toast";
import { timeAgo } from "@/lib/format";
import { approveNote, deleteNote, getPendingNotes, rejectNote } from "@/services/notes";
import { inviteAdmin } from "@/services/notifications";
import { banUser, deleteSubnoteryBanner, getSubnotery, getSubnoteryMembers, getSubnoteryNotes, joinSubnotery, leaveSubnotery, listBans, removeAdminFromSubnotery, removeMemberFromSubnotery, unbanUser, updateSubnoterySettings, uploadSubnoteryBanner } from "@/services/subnoteries";
import { useAuthStore } from "@/stores/auth-store";
import { useFeedStore } from "@/stores/feed-store";
import type { Ban, BanDuration, Note, SubnoteryDetail, SubnoteryMember } from "@/types";
import {
    CheckCircle,
    ChevronLeft,
    ChevronRight,
    Eye,
    FileText,
    Ban as BanIcon,
    ImageIcon,
    Paintbrush,
    Send,
    Settings,
    Shield,
    Trash2,
    Upload,
    UserMinus,
    Users,
    XCircle,
} from "lucide-react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { useCallback, useEffect, useRef, useState } from "react";

const PAGE_SIZE = 20;

export default function CommunityDetailPage() {
    const params = useParams();
    const communityId = Number(params.id);
    const { user } = useAuthStore();
    const { sort, timeFilter } = useFeedStore();
    const { toast } = useToast();

    const [community, setCommunity] = useState<SubnoteryDetail | null>(null);
    const [notes, setNotes] = useState<Note[]>([]);
    const [notesTotal, setNotesTotal] = useState(0);
    const [notesPage, setNotesPage] = useState(1);
    const [pendingNotes, setPendingNotes] = useState<Note[]>([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);
    const [joining, setJoining] = useState(false);
    const [leaving, setLeaving] = useState(false);
    const [activeTab, setActiveTab] = useState("notes");

    // Settings form state (admin only)
    const [settingsDescription, setSettingsDescription] = useState("");
    const [settingsContentType, setSettingsContentType] = useState("");
    const [settingsRules, setSettingsRules] = useState("");
    const [settingsMinPostNotoriety, setSettingsMinPostNotoriety] = useState(0);
    const [settingsMinCommentNotoriety, setSettingsMinCommentNotoriety] = useState(0);
    const [settingsBackgroundColor, setSettingsBackgroundColor] = useState("");
    const [settingsAutoApprove, setSettingsAutoApprove] = useState(false);
    const [savingSettings, setSavingSettings] = useState(false);
    const [uploadingBanner, setUploadingBanner] = useState(false);
    const [removingAdmin, setRemovingAdmin] = useState<number | null>(null);
    const bannerInputRef = useRef<HTMLInputElement>(null);

    // Admin invite state
    const [inviteUsername, setInviteUsername] = useState("");
    const [inviting, setInviting] = useState(false);

    // Members state
    const [members, setMembers] = useState<SubnoteryMember[]>([]);
    const [membersTotal, setMembersTotal] = useState(0);
    const [membersPage, setMembersPage] = useState(1);
    const [membersLoading, setMembersLoading] = useState(false);
    const [removingMember, setRemovingMember] = useState<number | null>(null);

    // Ban state
    const [bans, setBans] = useState<Ban[]>([]);
    const [bansTotal, setBansTotal] = useState(0);
    const [bansPage, setBansPage] = useState(1);
    const [bansLoading, setBansLoading] = useState(false);
    const [banningMember, setBanningMember] = useState<number | null>(null);
    const [banDuration, setBanDuration] = useState<BanDuration>("7d");
    const [banReason, setBanReason] = useState("");
    const [showBanModal, setShowBanModal] = useState(false);
    const [banTargetId, setBanTargetId] = useState<number | null>(null);
    const [banTargetName, setBanTargetName] = useState("");

    const isAdmin =
        user &&
        community?.admins?.some((a) => a.id === user.id);

    // Load community detail
    useEffect(() => {
        if (!communityId || isNaN(communityId)) {
            setError("Invalid community ID");
            setLoading(false);
            return;
        }
        setLoading(true);
        getSubnotery(communityId)
            .then((data) => {
                setCommunity(data);
                setSettingsDescription(data.description || "");
                setSettingsContentType(data.content_type || "");
                setSettingsRules(data.rules || "");
                setSettingsMinPostNotoriety(data.min_post_notoriety ?? 0);
                setSettingsMinCommentNotoriety(data.min_comment_notoriety ?? 0);
                setSettingsBackgroundColor(data.background_color || "");
                setSettingsAutoApprove(data.auto_approve_free_notes || false);
            })
            .catch((err) => setError(err.message || "Failed to load community"))
            .finally(() => setLoading(false));
    }, [communityId]);

    // Load approved notes
    useEffect(() => {
        if (!communityId || isNaN(communityId)) return;
        getSubnoteryNotes(communityId, {
            page: notesPage,
            limit: PAGE_SIZE,
            sort: sort,
            time: (sort === "top" || sort === "controversial") ? timeFilter : undefined,
        })
            .then((res) => {
                setNotes(res.notes ?? []);
                setNotesTotal(res.total);
            })
            .catch(() => { });
    }, [communityId, notesPage, sort, timeFilter]);

    // Load pending notes (admin only, scoped to this subnotery)
    const loadPending = useCallback(() => {
        if (!isAdmin) return;
        getPendingNotes({ page: 1, limit: PAGE_SIZE, subnotery_id: communityId })
            .then((res) => {
                setPendingNotes(res.notes ?? []);
            })
            .catch(() => { });
    }, [isAdmin, communityId]);

    useEffect(() => {
        loadPending();
    }, [loadPending]);

    const handleJoin = async () => {
        setJoining(true);
        try {
            await joinSubnotery(communityId);
            toast({ title: "Joined!", description: "You are now a member." });
            // Refresh community data
            const updated = await getSubnotery(communityId);
            setCommunity(updated);
        } catch (err: unknown) {
            const msg = err instanceof Error ? err.message : "Failed to join";
            toast({ title: "Error", description: msg, variant: "destructive" });
        } finally {
            setJoining(false);
        }
    };

    const handleLeave = async () => {
        setLeaving(true);
        try {
            await leaveSubnotery(communityId);
            toast({ title: "Left", description: "You are no longer a member." });
            const updated = await getSubnotery(communityId);
            setCommunity(updated);
        } catch (err: unknown) {
            const msg = err instanceof Error ? err.message : "Failed to leave";
            toast({ title: "Error", description: msg, variant: "destructive" });
        } finally {
            setLeaving(false);
        }
    };

    const handleApprove = async (noteId: number) => {
        try {
            await approveNote(noteId);
            toast({ title: "Approved", description: "Note has been approved." });
            loadPending();
            // Refresh approved notes
            getSubnoteryNotes(communityId, {
                page: notesPage,
                limit: PAGE_SIZE,
                sort: sort,
                time: (sort === "top" || sort === "controversial") ? timeFilter : undefined,
            })
                .then((res) => {
                    setNotes(res.notes ?? []);
                    setNotesTotal(res.total);
                })
                .catch(() => { });
        } catch (err: unknown) {
            const msg = err instanceof Error ? err.message : "Failed to approve";
            toast({ title: "Error", description: msg, variant: "destructive" });
        }
    };

    const handleReject = async (noteId: number) => {
        try {
            await rejectNote(noteId);
            toast({ title: "Rejected", description: "Note has been rejected." });
            loadPending();
        } catch (err: unknown) {
            const msg = err instanceof Error ? err.message : "Failed to reject";
            toast({ title: "Error", description: msg, variant: "destructive" });
        }
    };

    const handleDeleteNote = async (noteId: number) => {
        if (!confirm("Are you sure you want to delete this note? This cannot be undone.")) return;
        try {
            await deleteNote(noteId);
            toast({ title: "Deleted", description: "Note has been deleted." });
            // Refresh approved notes
            getSubnoteryNotes(communityId, {
                page: notesPage,
                limit: PAGE_SIZE,
                sort: sort,
                time: (sort === "top" || sort === "controversial") ? timeFilter : undefined,
            })
                .then((res) => {
                    setNotes(res.notes ?? []);
                    setNotesTotal(res.total);
                })
                .catch(() => { });
        } catch (err: unknown) {
            const msg = err instanceof Error ? err.message : "Failed to delete";
            toast({ title: "Error", description: msg, variant: "destructive" });
        }
    };

    const handleSaveSettings = async () => {
        setSavingSettings(true);
        try {
            await updateSubnoterySettings(communityId, {
                description: settingsDescription,
                content_type: settingsContentType,
                rules: settingsRules,
                background_color: settingsBackgroundColor,
                min_post_notoriety: settingsMinPostNotoriety,
                min_comment_notoriety: settingsMinCommentNotoriety,
                auto_approve_free_notes: settingsAutoApprove,
            });
            toast({ title: "Saved", description: "Community settings updated." });
            // Refresh community data
            const updated = await getSubnotery(communityId);
            setCommunity(updated);
        } catch (err: unknown) {
            const msg = err instanceof Error ? err.message : "Failed to save";
            toast({ title: "Error", description: msg, variant: "destructive" });
        } finally {
            setSavingSettings(false);
        }
    };

    const notesTotalPages = Math.ceil(notesTotal / PAGE_SIZE);

    const handleUploadBanner = async (e: React.ChangeEvent<HTMLInputElement>) => {
        const file = e.target.files?.[0];
        if (!file) return;
        setUploadingBanner(true);
        try {
            await uploadSubnoteryBanner(communityId, file);
            toast({ title: "Banner uploaded" });
            const updated = await getSubnotery(communityId);
            setCommunity(updated);
        } catch (err: unknown) {
            const msg = err instanceof Error ? err.message : "Failed to upload banner";
            toast({ title: "Error", description: msg, variant: "destructive" });
        } finally {
            setUploadingBanner(false);
            if (bannerInputRef.current) bannerInputRef.current.value = "";
        }
    };

    const handleDeleteBanner = async () => {
        try {
            await deleteSubnoteryBanner(communityId);
            toast({ title: "Banner removed" });
            const updated = await getSubnotery(communityId);
            setCommunity(updated);
        } catch (err: unknown) {
            const msg = err instanceof Error ? err.message : "Failed to delete banner";
            toast({ title: "Error", description: msg, variant: "destructive" });
        }
    };

    const handleRemoveAdmin = async (adminId: number) => {
        if (!confirm("Remove this user's admin permissions?")) return;
        setRemovingAdmin(adminId);
        try {
            await removeAdminFromSubnotery(communityId, adminId);
            toast({ title: "Admin removed" });
            const updated = await getSubnotery(communityId);
            setCommunity(updated);
        } catch (err: unknown) {
            const msg = err instanceof Error ? err.message : "Failed to remove admin";
            toast({ title: "Error", description: msg, variant: "destructive" });
        } finally {
            setRemovingAdmin(null);
        }
    };

    const handleInviteAdmin = async () => {
        const username = inviteUsername.trim();
        if (!username) return;
        setInviting(true);
        try {
            await inviteAdmin(communityId, username);
            toast({ title: "Invite sent", description: `Admin invite sent to ${username}.` });
            setInviteUsername("");
        } catch (err: unknown) {
            const msg = err instanceof Error ? err.message : "Failed to send invite";
            toast({ title: "Error", description: msg, variant: "destructive" });
        } finally {
            setInviting(false);
        }
    };

    // Load members when Members tab is active
    const loadMembers = useCallback(async () => {
        setMembersLoading(true);
        try {
            const res = await getSubnoteryMembers(communityId, { page: membersPage, limit: PAGE_SIZE });
            setMembers(res.members ?? []);
            setMembersTotal(res.total);
        } catch {
            // ignore
        } finally {
            setMembersLoading(false);
        }
    }, [communityId, membersPage]);

    useEffect(() => {
        if (activeTab === "members") {
            loadMembers();
        }
    }, [activeTab, loadMembers]);

    const membersTotalPages = Math.ceil(membersTotal / PAGE_SIZE);

    const handleRemoveMember = async (memberId: number) => {
        if (!confirm("Remove this user from the community?")) return;
        setRemovingMember(memberId);
        try {
            await removeMemberFromSubnotery(communityId, memberId);
            toast({ title: "Member removed" });
            loadMembers();
            const updated = await getSubnotery(communityId);
            setCommunity(updated);
        } catch (err: unknown) {
            const msg = err instanceof Error ? err.message : "Failed to remove member";
            toast({ title: "Error", description: msg, variant: "destructive" });
        } finally {
            setRemovingMember(null);
        }
    };

    // Open the ban modal for a member
    const openBanModal = (memberId: number, memberName: string) => {
        setBanTargetId(memberId);
        setBanTargetName(memberName);
        setBanDuration("7d");
        setBanReason("");
        setShowBanModal(true);
    };

    const handleBanMember = async () => {
        if (!banTargetId) return;
        setBanningMember(banTargetId);
        try {
            await banUser(communityId, {
                user_id: banTargetId,
                duration: banDuration,
                reason: banReason,
            });
            toast({ title: "User banned", description: `${banTargetName} has been banned.` });
            setShowBanModal(false);
            loadMembers();
            loadBans();
            const updated = await getSubnotery(communityId);
            setCommunity(updated);
        } catch (err: unknown) {
            const msg = err instanceof Error ? err.message : "Failed to ban user";
            toast({ title: "Error", description: msg, variant: "destructive" });
        } finally {
            setBanningMember(null);
        }
    };

    const handleUnban = async (userId: number) => {
        try {
            await unbanUser(communityId, userId);
            toast({ title: "User unbanned" });
            loadBans();
        } catch (err: unknown) {
            const msg = err instanceof Error ? err.message : "Failed to unban";
            toast({ title: "Error", description: msg, variant: "destructive" });
        }
    };

    const loadBans = useCallback(async () => {
        setBansLoading(true);
        try {
            const res = await listBans(communityId, { page: bansPage, limit: PAGE_SIZE });
            setBans(res.bans ?? []);
            setBansTotal(res.total);
        } catch {
            // ignore
        } finally {
            setBansLoading(false);
        }
    }, [communityId, bansPage]);

    useEffect(() => {
        if (activeTab === "bans" && isAdmin) {
            loadBans();
        }
    }, [activeTab, isAdmin, loadBans]);

    if (loading) {
        return (
            <div className="flex">
                <main className="flex-1 min-w-0 px-6 py-4 space-y-4">
                    <Skeleton className="h-32 w-full rounded-lg" />
                    <Skeleton className="h-64 w-full rounded-lg" />
                </main>
            </div>
        );
    }

    if (error || !community) {
        return (
            <div className="flex">
                <main className="flex-1 min-w-0 px-6 py-4">
                    <Card className="p-6 text-center text-destructive">
                        {error || "Community not found"}
                    </Card>
                </main>
            </div>
        );
    }

    return (
        <div className="flex">
            <main
                className="flex-1 min-w-0 px-6 py-4"
                style={community.background_color ? { backgroundColor: community.background_color } : undefined}
            >
                <div className="max-w-3xl mx-auto">
                    {/* Community header */}
                    <Card className="mb-4 overflow-hidden">
                        {/* Community banner */}
                        {community.banner_url && (
                            <div className="relative w-full h-32 bg-muted">
                                {/* eslint-disable-next-line @next/next/no-img-element */}
                                <img
                                    src={`${process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080"}/api/v1/subnoteries/${communityId}/banner?v=${Date.now()}`}
                                    alt={`Banner for n/${community.name}`}
                                    className="w-full h-full object-cover"
                                />
                            </div>
                        )}
                        <div className="p-6">
                            <div className="flex items-start justify-between">
                                <div>
                                    <h1 className="text-2xl font-bold">n/{community.name}</h1>
                                    <div className="flex items-center gap-4 mt-2 text-sm text-muted-foreground">
                                        <span className="flex items-center gap-1">
                                            <Users className="h-4 w-4" />
                                            {community.member_count}{" "}
                                            {community.member_count === 1 ? "member" : "members"}
                                        </span>
                                        <span>
                                            Created {timeAgo(community.created_at)}
                                        </span>
                                    </div>
                                    <div className="flex items-center gap-2 mt-2">
                                        <Shield className="h-4 w-4 text-muted-foreground" />
                                        <span className="text-sm text-muted-foreground">Admins:</span>
                                        {community.admins.map((admin) => (
                                            <Badge key={admin.id} variant="secondary">
                                                {admin.username}
                                            </Badge>
                                        ))}
                                    </div>
                                </div>
                                <div className="flex gap-2">
                                    {user && (
                                        community?.is_member ? (
                                            <Button
                                                variant="outline"
                                                onClick={handleLeave}
                                                disabled={leaving}
                                            >
                                                {leaving ? "Leaving..." : "Leave"}
                                            </Button>
                                        ) : (
                                            <Button
                                                variant="outline"
                                                onClick={handleJoin}
                                                disabled={joining}
                                            >
                                                {joining ? "Joining..." : "Join"}
                                            </Button>
                                        )
                                    )}
                                    {isAdmin && (
                                        <Badge variant="default" className="h-8 flex items-center">
                                            <Shield className="h-3 w-3 mr-1" />
                                            Admin
                                        </Badge>
                                    )}
                                </div>
                            </div>
                        </div>
                    </Card>

                    {/* Tabs: Notes + Admin (if applicable) */}
                    <Tabs value={activeTab} onValueChange={setActiveTab}>
                        <TabsList>
                            <TabsTrigger value="notes">
                                Notes ({notesTotal})
                            </TabsTrigger>
                            {isAdmin && (
                                <TabsTrigger value="pending">
                                    Pending ({pendingNotes.length})
                                </TabsTrigger>
                            )}
                            {isAdmin && (
                                <TabsTrigger value="members">
                                    <Users className="h-3.5 w-3.5 mr-1" />
                                    Members
                                </TabsTrigger>
                            )}
                            {isAdmin && (
                                <TabsTrigger value="bans">
                                    <BanIcon className="h-3.5 w-3.5 mr-1" />
                                    Bans
                                </TabsTrigger>
                            )}
                            {isAdmin && (
                                <TabsTrigger value="settings">
                                    <Settings className="h-3.5 w-3.5 mr-1" />
                                    Settings
                                </TabsTrigger>
                            )}
                        </TabsList>

                        {/* Approved notes tab */}
                        <TabsContent value="notes" className="mt-4">
                            <SortTabs />
                            {notes.length === 0 ? (
                                <Card className="p-6 text-center text-muted-foreground">
                                    No approved notes in this community yet.
                                </Card>
                            ) : (
                                <div className="space-y-4">
                                    {notes.map((note) => (
                                        <NoteCard
                                            key={note.id}
                                            note={note}
                                            isAdmin={!!isAdmin}
                                            onDelete={handleDeleteNote}
                                        />
                                    ))}
                                </div>
                            )}

                            {notesTotalPages > 1 && (
                                <div className="flex items-center justify-center gap-4 mt-6">
                                    <Button
                                        variant="outline"
                                        size="sm"
                                        disabled={notesPage <= 1}
                                        onClick={() =>
                                            setNotesPage((p) => Math.max(1, p - 1))
                                        }
                                    >
                                        <ChevronLeft className="h-4 w-4 mr-1" />
                                        Previous
                                    </Button>
                                    <span className="text-sm text-muted-foreground">
                                        Page {notesPage} of {notesTotalPages}
                                    </span>
                                    <Button
                                        variant="outline"
                                        size="sm"
                                        disabled={notesPage >= notesTotalPages}
                                        onClick={() => setNotesPage((p) => p + 1)}
                                    >
                                        Next
                                        <ChevronRight className="h-4 w-4 ml-1" />
                                    </Button>
                                </div>
                            )}
                        </TabsContent>

                        {/* Pending notes tab (admin only) */}
                        {isAdmin && (
                            <TabsContent value="pending" className="mt-4">
                                {pendingNotes.length === 0 ? (
                                    <Card className="p-6 text-center text-muted-foreground">
                                        No pending notes to review.
                                    </Card>
                                ) : (
                                    <div className="space-y-3">
                                        {pendingNotes.map((note) => (
                                            <Card key={note.id} className="p-4">
                                                <div className="flex items-center justify-between">
                                                    <div className="flex-1 min-w-0">
                                                        <h3 className="font-medium">
                                                            {note.title}
                                                        </h3>
                                                        <div className="flex items-center gap-3 mt-1 text-sm text-muted-foreground">
                                                            <span>by {note.author}</span>
                                                            <span>
                                                                {timeAgo(note.created_at)}
                                                            </span>
                                                            {note.has_pdf ? (
                                                                <span className="flex items-center gap-1 text-green-500">
                                                                    <FileText className="h-3 w-3" />
                                                                    PDF uploaded
                                                                </span>
                                                            ) : (
                                                                <span className="flex items-center gap-1 text-yellow-500">
                                                                    <FileText className="h-3 w-3" />
                                                                    No PDF
                                                                </span>
                                                            )}
                                                            <Badge variant="outline">
                                                                {note.price === 0
                                                                    ? "Free"
                                                                    : `$${(note.price / 100).toFixed(2)}`}
                                                            </Badge>
                                                        </div>
                                                    </div>
                                                    <div className="flex gap-2 shrink-0 ml-4">
                                                        <Button
                                                            size="sm"
                                                            variant="outline"
                                                            asChild
                                                        >
                                                            <Link
                                                                href={`/notes/${note.id}`}
                                                                title="View note details"
                                                            >
                                                                <Eye className="h-4 w-4 mr-1" />
                                                                View
                                                            </Link>
                                                        </Button>
                                                        <Button
                                                            size="sm"
                                                            variant="default"
                                                            onClick={() =>
                                                                handleApprove(note.id)
                                                            }
                                                            disabled={!note.has_pdf}
                                                            title={
                                                                !note.has_pdf
                                                                    ? "PDF required before approval"
                                                                    : "Approve this note"
                                                            }
                                                        >
                                                            <CheckCircle className="h-4 w-4 mr-1" />
                                                            Approve
                                                        </Button>
                                                        <Button
                                                            size="sm"
                                                            variant="destructive"
                                                            onClick={() =>
                                                                handleReject(note.id)
                                                            }
                                                        >
                                                            <XCircle className="h-4 w-4 mr-1" />
                                                            Reject
                                                        </Button>
                                                    </div>
                                                </div>
                                            </Card>
                                        ))}
                                    </div>
                                )}
                            </TabsContent>
                        )}

                        {/* Members tab (admin only) */}
                        {isAdmin && (
                            <TabsContent value="members" className="mt-4">
                                <Card className="p-6 space-y-4">
                                    <h3 className="text-lg font-semibold flex items-center gap-2">
                                        <Users className="h-5 w-5" /> Community Members
                                    </h3>
                                    {membersLoading ? (
                                        <div className="space-y-2">
                                            <Skeleton className="h-10 w-full" />
                                            <Skeleton className="h-10 w-full" />
                                            <Skeleton className="h-10 w-full" />
                                        </div>
                                    ) : members.length === 0 ? (
                                        <p className="text-sm text-muted-foreground">No members yet.</p>
                                    ) : (
                                        <div className="space-y-1">
                                            {members.map((member) => (
                                                <div
                                                    key={member.id}
                                                    className="flex items-center justify-between py-2 px-3 rounded hover:bg-accent/50"
                                                >
                                                    <div className="flex items-center gap-2">
                                                        <Link
                                                            href={`/user/${member.id}`}
                                                            className="text-sm font-medium hover:text-primary"
                                                        >
                                                            u/{member.username}
                                                        </Link>
                                                        {member.is_admin && (
                                                            <Badge variant="secondary" className="text-xs">
                                                                <Shield className="h-3 w-3 mr-0.5" />
                                                                Admin
                                                            </Badge>
                                                        )}
                                                    </div>
                                                    {member.id !== user?.id && !member.is_admin && (
                                                        <div className="flex items-center gap-1">
                                                            <Button
                                                                variant="ghost"
                                                                size="sm"
                                                                className="h-7 text-xs text-orange-600 hover:text-orange-600"
                                                                onClick={() => openBanModal(member.id, member.username)}
                                                                disabled={banningMember === member.id}
                                                            >
                                                                <BanIcon className="h-3 w-3 mr-1" />
                                                                Ban
                                                            </Button>
                                                            <Button
                                                                variant="ghost"
                                                                size="sm"
                                                                className="h-7 text-xs text-destructive hover:text-destructive"
                                                                onClick={() => handleRemoveMember(member.id)}
                                                                disabled={removingMember === member.id}
                                                            >
                                                                <UserMinus className="h-3 w-3 mr-1" />
                                                                {removingMember === member.id ? "..." : "Remove"}
                                                            </Button>
                                                        </div>
                                                    )}
                                                </div>
                                            ))}
                                        </div>
                                    )}

                                    {membersTotalPages > 1 && (
                                        <div className="flex items-center justify-center gap-4 mt-4">
                                            <Button
                                                variant="outline"
                                                size="sm"
                                                disabled={membersPage <= 1}
                                                onClick={() => setMembersPage((p) => Math.max(1, p - 1))}
                                            >
                                                <ChevronLeft className="h-4 w-4 mr-1" />
                                                Previous
                                            </Button>
                                            <span className="text-sm text-muted-foreground">
                                                Page {membersPage} of {membersTotalPages}
                                            </span>
                                            <Button
                                                variant="outline"
                                                size="sm"
                                                disabled={membersPage >= membersTotalPages}
                                                onClick={() => setMembersPage((p) => p + 1)}
                                            >
                                                Next
                                                <ChevronRight className="h-4 w-4 ml-1" />
                                            </Button>
                                        </div>
                                    )}
                                </Card>
                            </TabsContent>
                        )}

                        {/* Bans tab (admin only) */}
                        {isAdmin && (
                            <TabsContent value="bans" className="mt-4">
                                <Card className="p-6 space-y-4">
                                    <h3 className="text-lg font-semibold flex items-center gap-2">
                                        <BanIcon className="h-5 w-5" /> Banned Users
                                    </h3>
                                    {bansLoading ? (
                                        <div className="space-y-2">
                                            <Skeleton className="h-10 w-full" />
                                            <Skeleton className="h-10 w-full" />
                                        </div>
                                    ) : bans.length === 0 ? (
                                        <p className="text-sm text-muted-foreground">No active bans.</p>
                                    ) : (
                                        <div className="space-y-1">
                                            {bans.map((ban) => (
                                                <div
                                                    key={ban.id}
                                                    className="flex items-center justify-between py-2 px-3 rounded hover:bg-accent/50"
                                                >
                                                    <div className="flex-1 min-w-0">
                                                        <div className="flex items-center gap-2">
                                                            <Link
                                                                href={`/user/${ban.user_id}`}
                                                                className="text-sm font-medium hover:text-primary"
                                                            >
                                                                u/{ban.username}
                                                            </Link>
                                                            <Badge variant="outline" className="text-xs">
                                                                {ban.duration}
                                                            </Badge>
                                                        </div>
                                                        {ban.reason && (
                                                            <p className="text-xs text-muted-foreground mt-0.5">
                                                                Reason: {ban.reason}
                                                            </p>
                                                        )}
                                                        <p className="text-xs text-muted-foreground">
                                                            {ban.expires_at
                                                                ? `Expires ${timeAgo(ban.expires_at)}`
                                                                : "Permanent"}
                                                        </p>
                                                    </div>
                                                    <Button
                                                        variant="outline"
                                                        size="sm"
                                                        className="h-7 text-xs"
                                                        onClick={() => handleUnban(ban.user_id)}
                                                    >
                                                        Unban
                                                    </Button>
                                                </div>
                                            ))}
                                        </div>
                                    )}
                                    {Math.ceil(bansTotal / PAGE_SIZE) > 1 && (
                                        <div className="flex items-center justify-center gap-4 mt-4">
                                            <Button
                                                variant="outline"
                                                size="sm"
                                                disabled={bansPage <= 1}
                                                onClick={() => setBansPage((p) => Math.max(1, p - 1))}
                                            >
                                                <ChevronLeft className="h-4 w-4 mr-1" />
                                                Previous
                                            </Button>
                                            <span className="text-sm text-muted-foreground">
                                                Page {bansPage} of {Math.ceil(bansTotal / PAGE_SIZE)}
                                            </span>
                                            <Button
                                                variant="outline"
                                                size="sm"
                                                disabled={bansPage >= Math.ceil(bansTotal / PAGE_SIZE)}
                                                onClick={() => setBansPage((p) => p + 1)}
                                            >
                                                Next
                                                <ChevronRight className="h-4 w-4 ml-1" />
                                            </Button>
                                        </div>
                                    )}
                                </Card>
                            </TabsContent>
                        )}

                        {/* Ban modal */}
                        {showBanModal && (
                            <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
                                <Card className="w-full max-w-md p-6 space-y-4">
                                    <h3 className="text-lg font-semibold">
                                        Ban u/{banTargetName}
                                    </h3>
                                    <div>
                                        <label className="block text-sm font-medium mb-1">Duration</label>
                                        <select
                                            className="w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
                                            value={banDuration}
                                            onChange={(e) => setBanDuration(e.target.value as BanDuration)}
                                        >
                                            <option value="1d">1 Day</option>
                                            <option value="7d">7 Days</option>
                                            <option value="30d">30 Days</option>
                                            <option value="1y">1 Year</option>
                                            <option value="permanent">Permanent</option>
                                        </select>
                                    </div>
                                    <div>
                                        <label className="block text-sm font-medium mb-1">Reason (optional)</label>
                                        <textarea
                                            className="w-full rounded-md border border-border bg-background px-3 py-2 text-sm min-h-[60px] resize-y"
                                            placeholder="Reason for ban..."
                                            value={banReason}
                                            onChange={(e) => setBanReason(e.target.value)}
                                            maxLength={500}
                                        />
                                    </div>
                                    <div className="flex justify-end gap-2">
                                        <Button
                                            variant="outline"
                                            onClick={() => setShowBanModal(false)}
                                        >
                                            Cancel
                                        </Button>
                                        <Button
                                            variant="destructive"
                                            onClick={handleBanMember}
                                            disabled={banningMember === banTargetId}
                                        >
                                            {banningMember === banTargetId ? "Banning..." : "Ban User"}
                                        </Button>
                                    </div>
                                </Card>
                            </div>
                        )}

                        {/* Settings tab (admin only) */}
                        {isAdmin && (
                            <TabsContent value="settings" className="mt-4">
                                <Card className="p-6 space-y-4">
                                    <h3 className="text-lg font-semibold">Community Settings</h3>

                                    <div>
                                        <label className="block text-sm font-medium mb-1">Description</label>
                                        <textarea
                                            className="w-full rounded-md border border-border bg-background px-3 py-2 text-sm min-h-[80px] resize-y"
                                            placeholder="Describe this community..."
                                            value={settingsDescription}
                                            onChange={(e) => setSettingsDescription(e.target.value)}
                                        />
                                    </div>

                                    <div>
                                        <label className="block text-sm font-medium mb-1">Content Type</label>
                                        <input
                                            type="text"
                                            className="w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
                                            placeholder="e.g. PDF Notes, Lecture Summaries"
                                            value={settingsContentType}
                                            onChange={(e) => setSettingsContentType(e.target.value)}
                                        />
                                    </div>

                                    <div>
                                        <label className="block text-sm font-medium mb-1">Community Rules</label>
                                        <textarea
                                            className="w-full rounded-md border border-border bg-background px-3 py-2 text-sm min-h-[120px] resize-y"
                                            placeholder="Enter community rules (one per line)..."
                                            value={settingsRules}
                                            onChange={(e) => setSettingsRules(e.target.value)}
                                        />
                                    </div>

                                    <div className="grid grid-cols-2 gap-4">
                                        <div>
                                            <label className="block text-sm font-medium mb-1">Min Post Notoriety</label>
                                            <input
                                                type="number"
                                                step="0.1"
                                                min="0"
                                                className="w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
                                                placeholder="0 = no restriction"
                                                value={settingsMinPostNotoriety}
                                                onChange={(e) => setSettingsMinPostNotoriety(parseFloat(e.target.value) || 0)}
                                            />
                                            <p className="text-xs text-muted-foreground mt-1">Minimum post notoriety to create notes. 0 = no restriction.</p>
                                        </div>
                                        <div>
                                            <label className="block text-sm font-medium mb-1">Min Comment Notoriety</label>
                                            <input
                                                type="number"
                                                step="0.1"
                                                min="0"
                                                className="w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
                                                placeholder="0 = no restriction"
                                                value={settingsMinCommentNotoriety}
                                                onChange={(e) => setSettingsMinCommentNotoriety(parseFloat(e.target.value) || 0)}
                                            />
                                            <p className="text-xs text-muted-foreground mt-1">Minimum comment notoriety to post comments. 0 = no restriction.</p>
                                        </div>
                                    </div>

                                    <div>
                                        <label className="block text-sm font-medium mb-1 flex items-center gap-1">
                                            <Paintbrush className="h-3.5 w-3.5" /> Background Color
                                        </label>
                                        <div className="flex items-center gap-3">
                                            <input
                                                type="color"
                                                value={settingsBackgroundColor || "#000000"}
                                                onChange={(e) => setSettingsBackgroundColor(e.target.value)}
                                                className="h-9 w-14 rounded border border-border cursor-pointer"
                                            />
                                            <input
                                                type="text"
                                                className="w-28 rounded-md border border-border bg-background px-3 py-2 text-sm font-mono"
                                                placeholder="#000000"
                                                maxLength={7}
                                                value={settingsBackgroundColor}
                                                onChange={(e) => setSettingsBackgroundColor(e.target.value)}
                                            />
                                            {settingsBackgroundColor && (
                                                <Button
                                                    variant="ghost"
                                                    size="sm"
                                                    className="h-8 text-xs text-muted-foreground"
                                                    onClick={() => setSettingsBackgroundColor("")}
                                                >
                                                    Reset to default
                                                </Button>
                                            )}
                                        </div>
                                        <p className="text-xs text-muted-foreground mt-1">Custom background colour for the content area. Leave empty for the default theme.</p>
                                    </div>

                                    <div className="flex items-center gap-3">
                                        <input
                                            type="checkbox"
                                            id="auto-approve-free"
                                            checked={settingsAutoApprove}
                                            onChange={(e) => setSettingsAutoApprove(e.target.checked)}
                                            className="h-4 w-4 rounded border-border"
                                        />
                                        <div>
                                            <label htmlFor="auto-approve-free" className="text-sm font-medium cursor-pointer">
                                                Auto-approve free notes
                                            </label>
                                            <p className="text-xs text-muted-foreground">
                                                When enabled, notes with a price of $0 are automatically approved without admin review.
                                            </p>
                                        </div>
                                    </div>

                                    <Button
                                        onClick={handleSaveSettings}
                                        disabled={savingSettings}
                                    >
                                        {savingSettings ? "Saving..." : "Save Settings"}
                                    </Button>
                                </Card>

                                {/* Banner management */}
                                <Card className="p-6 space-y-4 mt-4">
                                    <h3 className="text-lg font-semibold flex items-center gap-2">
                                        <ImageIcon className="h-5 w-5" /> Community Banner
                                    </h3>
                                    {community?.banner_url ? (
                                        <div className="space-y-3">
                                            <div className="relative w-full h-32 rounded-md overflow-hidden border border-border">
                                                {/* eslint-disable-next-line @next/next/no-img-element */}
                                                <img
                                                    src={`${process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080"}/api/v1/subnoteries/${communityId}/banner?v=${Date.now()}`}
                                                    alt="Current banner"
                                                    className="w-full h-full object-cover"
                                                />
                                            </div>
                                            <div className="flex gap-2">
                                                <Button
                                                    variant="outline"
                                                    size="sm"
                                                    onClick={() => bannerInputRef.current?.click()}
                                                    disabled={uploadingBanner}
                                                >
                                                    <Upload className="h-4 w-4 mr-1" />
                                                    {uploadingBanner ? "Uploading..." : "Replace Banner"}
                                                </Button>
                                                <Button
                                                    variant="destructive"
                                                    size="sm"
                                                    onClick={handleDeleteBanner}
                                                >
                                                    <Trash2 className="h-4 w-4 mr-1" />
                                                    Remove
                                                </Button>
                                            </div>
                                        </div>
                                    ) : (
                                        <div className="space-y-2">
                                            <p className="text-sm text-muted-foreground">No banner set.</p>
                                            <Button
                                                variant="outline"
                                                size="sm"
                                                onClick={() => bannerInputRef.current?.click()}
                                                disabled={uploadingBanner}
                                            >
                                                <Upload className="h-4 w-4 mr-1" />
                                                {uploadingBanner ? "Uploading..." : "Upload Banner"}
                                            </Button>
                                        </div>
                                    )}
                                    <input
                                        ref={bannerInputRef}
                                        type="file"
                                        accept="image/jpeg,image/png,image/webp,image/gif"
                                        className="hidden"
                                        onChange={handleUploadBanner}
                                    />
                                    <p className="text-xs text-muted-foreground">Max 5 MB. JPEG, PNG, WebP, or GIF.</p>
                                </Card>

                                {/* Admin management */}
                                <Card className="p-6 space-y-4 mt-4">
                                    <h3 className="text-lg font-semibold flex items-center gap-2">
                                        <Shield className="h-5 w-5" /> Manage Admins
                                    </h3>
                                    <div className="space-y-2">
                                        {community?.admins.map((admin) => {
                                            // Only show Remove for admins added AFTER the current user
                                            const myAdminSince = community?.admins.find((a) => a.id === user?.id)?.admin_since;
                                            const canRemove =
                                                admin.id !== user?.id &&
                                                myAdminSince &&
                                                admin.admin_since &&
                                                new Date(myAdminSince) < new Date(admin.admin_since);
                                            return (
                                                <div key={admin.id} className="flex items-center justify-between py-1.5 px-2 rounded hover:bg-accent/50">
                                                    <Link href={`/user/${admin.id}`} className="text-sm hover:text-primary">
                                                        u/{admin.username}
                                                    </Link>
                                                    {canRemove && (
                                                        <Button
                                                            variant="ghost"
                                                            size="sm"
                                                            className="h-7 text-xs text-destructive hover:text-destructive"
                                                            onClick={() => handleRemoveAdmin(admin.id)}
                                                            disabled={removingAdmin === admin.id}
                                                        >
                                                            {removingAdmin === admin.id ? "..." : "Remove"}
                                                        </Button>
                                                    )}
                                                </div>
                                            );
                                        })}
                                    </div>
                                    <p className="text-xs text-muted-foreground">
                                        You can only remove admins who were added after you.
                                    </p>

                                    {/* Invite admin */}
                                    <div className="pt-4 border-t border-border">
                                        <h4 className="text-sm font-medium mb-2">Invite Admin</h4>
                                        <div className="flex gap-2">
                                            <input
                                                type="text"
                                                className="flex-1 rounded-md border border-border bg-background px-3 py-2 text-sm"
                                                placeholder="Enter username..."
                                                value={inviteUsername}
                                                onChange={(e) => setInviteUsername(e.target.value)}
                                                onKeyDown={(e) => {
                                                    if (e.key === "Enter") handleInviteAdmin();
                                                }}
                                            />
                                            <Button
                                                size="sm"
                                                onClick={handleInviteAdmin}
                                                disabled={inviting || !inviteUsername.trim()}
                                            >
                                                <Send className="h-4 w-4 mr-1" />
                                                {inviting ? "Sending..." : "Send Invite"}
                                            </Button>
                                        </div>
                                        <p className="text-xs text-muted-foreground mt-1">
                                            The user will receive a notification they can accept or decline.
                                        </p>
                                    </div>
                                </Card>
                            </TabsContent>
                        )}
                    </Tabs>
                </div>
            </main>
            <RightSidebar
                subnotery={community ? {
                    name: community.name,
                    description: community.description,
                    content_type: community.content_type,
                    rules: community.rules,
                    member_count: community.member_count,
                    admins: community.admins,
                    created_at: community.created_at,
                } : undefined}
            />
        </div>
    );
}
