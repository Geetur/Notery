// page.tsx — Login page.
"use client";

import OAuthButtons from "@/components/auth/oauth-buttons";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useToast } from "@/hooks/use-toast";
import { ApiRequestError } from "@/lib/api-client";
import { login } from "@/services/auth";
import { getMyProfile } from "@/services/profile";
import { useAuthStore } from "@/stores/auth-store";
import { Loader2 } from "lucide-react";
import Image from "next/image";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";

export default function LoginPage() {
    const router = useRouter();
    const { toast } = useToast();
    const setUser = useAuthStore((s) => s.setUser);
    const [email, setEmail] = useState("");
    const [password, setPassword] = useState("");
    const [loading, setLoading] = useState(false);

    const handleSubmit = async (e: React.FormEvent) => {
        e.preventDefault();
        if (!email.trim() || !password) return;

        setLoading(true);
        try {
            await login(email.trim(), password);
            const profile = await getMyProfile();
            setUser(profile);
            toast({ title: "Welcome back!" });
            router.push("/");
        } catch (err) {
            const message =
                err instanceof ApiRequestError
                    ? err.body.error
                    : "Login failed. Please try again.";
            toast({ title: "Login failed", description: message, variant: "destructive" });
        } finally {
            setLoading(false);
        }
    };

    return (
        <div className="flex items-center justify-center min-h-[calc(100vh-48px)] px-4">
            <Card className="w-full max-w-sm">
                <CardHeader className="text-center">
                    <div className="flex items-center justify-center gap-2 mb-2">
                        <Image src="/notery-logo.png" alt="Notery" width={28} height={28} className="h-7 w-7" />
                        <span className="text-xl font-bold">notery</span>
                    </div>
                    <CardTitle className="text-lg">Log In</CardTitle>
                </CardHeader>
                <CardContent>
                    <OAuthButtons action="Log in" />
                    <form onSubmit={handleSubmit} className="space-y-4">
                        <div>
                            <Label htmlFor="email">Email</Label>
                            <Input
                                id="email"
                                type="email"
                                placeholder="you@example.com"
                                value={email}
                                onChange={(e) => setEmail(e.target.value)}
                                required
                                autoFocus
                            />
                        </div>
                        <div>
                            <Label htmlFor="password">Password</Label>
                            <Input
                                id="password"
                                type="password"
                                placeholder="Password"
                                value={password}
                                onChange={(e) => setPassword(e.target.value)}
                                required
                                minLength={8}
                            />
                        </div>
                        <Button type="submit" className="w-full" disabled={loading}>
                            {loading && <Loader2 className="h-4 w-4 animate-spin mr-2" />}
                            Log In
                        </Button>
                    </form>

                    <div className="mt-4 text-center text-sm text-muted-foreground">
                        <Link
                            href="/forgot-password"
                            className="text-primary hover:underline"
                        >
                            Forgot password?
                        </Link>
                    </div>
                    <div className="mt-2 text-center text-sm text-muted-foreground">
                        New to Notery?{" "}
                        <Link href="/signup" className="text-primary hover:underline font-medium">
                            Sign Up
                        </Link>
                    </div>
                </CardContent>
            </Card>
        </div>
    );
}
