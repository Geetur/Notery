// theme-store.ts — Zustand store for color theme selection.
// Manages the active color theme (retro, pink, gold, etc.) independently from dark/light mode.
import { create } from "zustand";

export const THEMES = [
    { id: "default", label: "Default", color: "hsl(24, 100%, 50%)" },
    { id: "retro", label: "Retro", color: "hsl(30, 80%, 45%)" },
    { id: "pink", label: "Pink", color: "hsl(340, 82%, 52%)" },
    { id: "gold", label: "Gold", color: "hsl(45, 90%, 42%)" },
    { id: "ocean", label: "Ocean", color: "hsl(210, 90%, 50%)" },
    { id: "forest", label: "Forest", color: "hsl(150, 60%, 36%)" },
    { id: "lavender", label: "Lavender", color: "hsl(270, 70%, 55%)" },
    { id: "sunset", label: "Sunset", color: "hsl(15, 85%, 52%)" },
    { id: "neon", label: "Neon", color: "hsl(175, 100%, 40%)" },
] as const;

export type ThemeId = (typeof THEMES)[number]["id"];

interface ThemeState {
    theme: ThemeId;
    setTheme: (theme: ThemeId) => void;
}

function getStoredTheme(): ThemeId {
    if (typeof window === "undefined") return "default";
    const stored = localStorage.getItem("notery-theme");
    if (stored && THEMES.some((t) => t.id === stored)) return stored as ThemeId;
    return "default";
}

export const useThemeStore = create<ThemeState>((set) => ({
    theme: getStoredTheme(),
    setTheme: (theme) => {
        localStorage.setItem("notery-theme", theme);
        set({ theme });
    },
}));
