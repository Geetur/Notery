// page.tsx — Reset password page (reached via email link).
"use client";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useToast } from "@/hooks/use-toast";
import { resetPassword } from "@/services/auth";
import { ArrowLeft, CheckCircle, KeyRound, Loader2 } from "lucide-react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { Suspense, useState, type FormEvent } from "react";

function ResetPasswordForm() {
    const searchParams = useSearchParams();
    const token = searchParams.get("token") ?? "";
    const { toast } = useToast();
    const [password, setPassword] = useState("");
    const [confirm, setConfirm] = useState("");
    const [loading, setLoading] = useState(false);
    const [success, setSuccess] = useState(false);

    const handleSubmit = async (e: FormEvent) => {
        e.preventDefault();
        if (password.length < 8) {
            toast({ title: "Password too short", description: "Must be at least 8 characters.", variant: "destructive" });
            return;
        }
        if (password !== confirm) {
            toast({ title: "Passwords don't match", variant: "destructive" });
            return;
        }
        if (!token) {
            toast({ title: "Invalid reset link", description: "Missing reset token.", variant: "destructive" });
            return;
        }
        setLoading(true);
        try {
            await resetPassword(token, password);
            setSuccess(true);
        } catch (err: unknown) {
            const msg = err instanceof Error ? err.message : "Failed to reset password";
            toast({ title: "Error", description: msg, variant: "destructive" });
        } finally {
            setLoading(false);
        }
    };

    if (!token) {
        return (
            <div className="max-w-sm mx-auto px-4 py-8 text-center">
                <p className="text-destructive mb-4">Invalid or missing reset token.</p>
                <Button variant="outline" size="sm" asChild>
                    <Link href="/forgot-password">Request a new link</Link>
                </Button>
            </div>
        );
    }

    return (
        <div className="max-w-sm mx-auto px-4 py-8">
            <Button variant="ghost" size="sm" className="mb-3 -ml-2 text-muted-foreground" asChild>
                <Link href="/login">
                    <ArrowLeft className="h-4 w-4 mr-1" /> Back to Login
                </Link>
            </Button>

            <Card className="border-border">
                <CardHeader className="pb-3 text-center">
                    <KeyRound className="h-10 w-10 text-primary mx-auto mb-2" />
                    <CardTitle className="text-lg">Set a new password</CardTitle>
                    <p className="text-sm text-muted-foreground mt-1">
                        Enter your new password below.
                    </p>
                </CardHeader>
                <CardContent>
                    {success ? (
                        <div className="text-center py-4 space-y-3">
                            <CheckCircle className="h-10 w-10 text-green-500 mx-auto" />
                            <p className="text-sm">Your password has been reset successfully.</p>
                            <Button variant="outline" size="sm" asChild className="mt-2">
                                <Link href="/login">Log In</Link>
                            </Button>
                        </div>
                    ) : (
                        <form onSubmit={handleSubmit} className="space-y-4">
                            <div className="space-y-1.5">
                                <Label htmlFor="password">New Password</Label>
                                <Input
                                    id="password"
                                    type="password"
                                    placeholder="At least 8 characters"
                                    value={password}
                                    onChange={(e) => setPassword(e.target.value)}
                                    required
                                    minLength={8}
                                    autoFocus
                                />
                            </div>
                            <div className="space-y-1.5">
                                <Label htmlFor="confirm">Confirm Password</Label>
                                <Input
                                    id="confirm"
                                    type="password"
                                    placeholder="Re-enter password"
                                    value={confirm}
                                    onChange={(e) => setConfirm(e.target.value)}
                                    required
                                    minLength={8}
                                />
                            </div>
                            <Button type="submit" className="w-full" disabled={loading}>
                                {loading && <Loader2 className="h-4 w-4 animate-spin mr-2" />}
                                {loading ? "Resetting..." : "Reset Password"}
                            </Button>
                        </form>
                    )}
                </CardContent>
            </Card>
        </div>
    );
}

export default function ResetPasswordPage() {
    return (
        <Suspense fallback={
            <div className="max-w-sm mx-auto px-4 py-8 text-center">
                <Loader2 className="h-8 w-8 animate-spin mx-auto text-muted-foreground" />
            </div>
        }>
            <ResetPasswordForm />
        </Suspense>
    );
}
