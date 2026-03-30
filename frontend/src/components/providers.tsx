// providers.tsx — App-level providers: React Query, Auth initialization, Toast.
"use client";

import { ThemeProvider } from "@/components/theme-provider";
import { Toaster } from "@/components/ui/toaster";
import { useAuthStore } from "@/stores/auth-store";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useEffect, useRef, type ReactNode } from "react";

const queryClient = new QueryClient({
    defaultOptions: {
        queries: {
            staleTime: 30 * 1000, // 30 seconds
            retry: 1,
            refetchOnWindowFocus: false,
        },
    },
});

/** Initializes auth state on mount by checking for stored tokens. */
function AuthInitializer({ children }: { children: ReactNode }) {
    const initialize = useAuthStore((s) => s.initialize);
    const initialized = useRef(false);

    useEffect(() => {
        if (!initialized.current) {
            initialized.current = true;
            initialize();
        }
    }, [initialize]);

    return <>{children}</>;
}

/** Root providers wrapper. Wraps the entire application. */
export function Providers({ children }: { children: ReactNode }) {
    return (
        <QueryClientProvider client={queryClient}>
            <ThemeProvider
                attribute="class"
                defaultTheme="light"
                disableTransitionOnChange
            >
                <AuthInitializer>
                    {children}
                    <Toaster />
                </AuthInitializer>
            </ThemeProvider>
        </QueryClientProvider>
    );
}
