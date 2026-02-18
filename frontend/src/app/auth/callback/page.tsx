// page.tsx — OAuth callback handler. Receives tokens from the OAuth redirect.
"use client";

import { setTokens } from "@/lib/api-client";
import { getMyProfile } from "@/services/profile";
import { useAuthStore } from "@/stores/auth-store";
import { Loader2 } from "lucide-react";
import { useRouter, useSearchParams } from "next/navigation";
import { Suspense, useEffect, useRef } from "react";

function CallbackHandler() {
    const router = useRouter();
    const searchParams = useSearchParams();
    const setUser = useAuthStore((s) => s.setUser);
    const handled = useRef(false);

    useEffect(() => {
        if (handled.current) return;
        handled.current = true;

        const accessToken = searchParams.get("access_token");
        const refreshToken = searchParams.get("refresh_token");
        const error = searchParams.get("error");

        if (error) {
            router.replace(`/login?error=${encodeURIComponent(error)}`);
            return;
        }

        if (!accessToken || !refreshToken) {
            router.replace("/login?error=missing_tokens");
            return;
        }

        // Store tokens and fetch profile
        setTokens(accessToken, refreshToken);
        getMyProfile()
            .then((profile) => {
                setUser(profile);
                router.replace("/");
            })
            .catch(() => {
                router.replace("/login?error=profile_failed");
            });
    }, [searchParams, router, setUser]);

    return (
        <div className="flex items-center justify-center min-h-[calc(100vh-48px)]">
            <div className="flex flex-col items-center gap-3">
                <Loader2 className="h-8 w-8 animate-spin text-primary" />
                <p className="text-muted-foreground">Signing you in...</p>
            </div>
        </div>
    );
}

export default function AuthCallbackPage() {
    return (
        <Suspense
            fallback={
                <div className="flex items-center justify-center min-h-[calc(100vh-48px)]">
                    <Loader2 className="h-8 w-8 animate-spin text-primary" />
                </div>
            }
        >
            <CallbackHandler />
        </Suspense>
    );
}
