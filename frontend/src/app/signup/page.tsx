// page.tsx — Signup page.
"use client";

import OAuthButtons from "@/components/auth/oauth-buttons";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useToast } from "@/hooks/use-toast";
import { ApiRequestError } from "@/lib/api-client";
import { signup } from "@/services/auth";
import { getMyProfile } from "@/services/profile";
import { useAuthStore } from "@/stores/auth-store";
import { FileText, Loader2 } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";

export default function SignupPage() {
    const router = useRouter();
    const { toast } = useToast();
    const setUser = useAuthStore((s) => s.setUser);
    const [username, setUsername] = useState("");
    const [email, setEmail] = useState("");
    const [password, setPassword] = useState("");
    const [loading, setLoading] = useState(false);

    const handleSubmit = async (e: React.FormEvent) => {
        e.preventDefault();
        if (!username.trim() || !email.trim() || !password) return;

        setLoading(true);
        try {
            await signup(email.trim(), password, username.trim());
            const profile = await getMyProfile();
            setUser(profile);
            toast({ title: "Account created!", description: "Please check your email to verify your account." });
            router.push("/");
        } catch (err) {
            const message =
                err instanceof ApiRequestError
                    ? err.body.error
                    : "Signup failed. Please try again.";
            toast({ title: "Signup failed", description: message, variant: "destructive" });
        } finally {
            setLoading(false);
        }
    };

    return (
        <div className="flex items-center justify-center min-h-[calc(100vh-48px)] px-4">
            <Card className="w-full max-w-sm">
                <CardHeader className="text-center">
                    <div className="flex items-center justify-center gap-2 mb-2">
                        <FileText className="h-6 w-6 text-primary" />
                        <span className="text-xl font-bold">notery</span>
                    </div>
                    <CardTitle className="text-lg">Sign Up</CardTitle>
                </CardHeader>
                <CardContent>
                    <OAuthButtons action="Sign up" />
                    <form onSubmit={handleSubmit} className="space-y-4">
                        <div>
                            <Label htmlFor="username">Username</Label>
                            <Input
                                id="username"
                                type="text"
                                placeholder="cooluser123"
                                value={username}
                                onChange={(e) => setUsername(e.target.value)}
                                required
                                autoFocus
                                minLength={3}
                                maxLength={30}
                            />
                            <p className="text-xs text-muted-foreground mt-1">3–30 characters</p>
                        </div>
                        <div>
                            <Label htmlFor="email">Email</Label>
                            <Input
                                id="email"
                                type="email"
                                placeholder="you@example.com"
                                value={email}
                                onChange={(e) => setEmail(e.target.value)}
                                required
                            />
                        </div>
                        <div>
                            <Label htmlFor="password">Password</Label>
                            <Input
                                id="password"
                                type="password"
                                placeholder="Min. 8 characters"
                                value={password}
                                onChange={(e) => setPassword(e.target.value)}
                                required
                                minLength={8}
                            />
                        </div>
                        <Button type="submit" className="w-full" disabled={loading}>
                            {loading && <Loader2 className="h-4 w-4 animate-spin mr-2" />}
                            Sign Up
                        </Button>
                    </form>

                    <div className="mt-4 text-center text-sm text-muted-foreground">
                        Already have an account?{" "}
                        <Link href="/login" className="text-primary hover:underline font-medium">
                            Log In
                        </Link>
                    </div>
                </CardContent>
            </Card>
        </div>
    );
}
