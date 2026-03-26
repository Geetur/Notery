// page.tsx — OAuth callback handler. Receives access token from URL fragment.
// The refresh token is set as an httpOnly cookie by the backend (never in URL).
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

        // Check for error in query params (backend redirects errors to /login?error=...)
        const error = searchParams.get("error");
        if (error) {
            router.replace(`/login?error=${encodeURIComponent(error)}`);
            return;
        }

        // Read access token from URL fragment (not query params — prevents server log leakage)
        const hash = window.location.hash.substring(1);
        const hashParams = new URLSearchParams(hash);
        const accessToken = hashParams.get("access_token");

        if (!accessToken) {
            router.replace("/login?error=missing_tokens");
            return;
        }

        // Store access token (refresh token is in httpOnly cookie set by backend)
        setTokens(accessToken);
        // Clear the fragment from the URL to prevent token leakage in history
        window.history.replaceState(null, "", window.location.pathname);

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
