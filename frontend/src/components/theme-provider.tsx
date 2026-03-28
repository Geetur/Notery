// theme-provider.tsx — next-themes provider for dark mode + custom color theme support.
"use client";

import { useThemeStore } from "@/stores/theme-store";
import { ThemeProvider as NextThemesProvider, type ThemeProviderProps } from "next-themes";
import { useEffect } from "react";

/** Syncs the Zustand color theme to a CSS class on <html>. */
function ThemeClassSync() {
    const theme = useThemeStore((s) => s.theme);

    useEffect(() => {
        const root = document.documentElement;
        // Remove any existing theme-* classes
        root.classList.forEach((cls) => {
            if (cls.startsWith("theme-")) root.classList.remove(cls);
        });
        // Apply new theme class (skip for default which uses :root vars)
        if (theme !== "default") {
            root.classList.add(`theme-${theme}`);
        }
    }, [theme]);

    return null;
}

export function ThemeProvider({ children, ...props }: ThemeProviderProps) {
    return (
        <NextThemesProvider {...props}>
            <ThemeClassSync />
            {children}
        </NextThemesProvider>
    );
}
