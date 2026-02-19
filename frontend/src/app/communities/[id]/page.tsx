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
import { getSubnotery, getSubnoteryNotes, joinSubnotery, leaveSubnotery, updateSubnoterySettings } from "@/services/subnoteries";
import { useAuthStore } from "@/stores/auth-store";
import { useFeedStore } from "@/stores/feed-store";
import type { Note, SubnoteryDetail } from "@/types";
import {
    CheckCircle,
    ChevronLeft,
    ChevronRight,
    Eye,
    FileText,
    Settings,
    Shield,
    Users,
    XCircle,
} from "lucide-react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { useCallback, useEffect, useState } from "react";

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
    const [savingSettings, setSavingSettings] = useState(false);

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
                min_post_notoriety: settingsMinPostNotoriety,
                min_comment_notoriety: settingsMinCommentNotoriety,
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
            <main className="flex-1 min-w-0 px-6 py-4">
                <div className="max-w-3xl mx-auto">
                    {/* Community header */}
                    <Card className="p-6 mb-4">
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
                                {user && !isAdmin && (
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

                                    <Button
                                        onClick={handleSaveSettings}
                                        disabled={savingSettings}
                                    >
                                        {savingSettings ? "Saving..." : "Save Settings"}
                                    </Button>
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
                    created_at: community.created_at,
                } : undefined}
            />
        </div>
    );
}
