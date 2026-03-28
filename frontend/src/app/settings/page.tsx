// page.tsx — Settings page with theme picker and dark/light mode toggle.
"use client";

import { RightSidebar } from "@/components/layout/right-sidebar";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { cn } from "@/lib/utils";
import { THEMES, useThemeStore, type ThemeId } from "@/stores/theme-store";
import { Moon, Sun } from "lucide-react";
import { useTheme } from "next-themes";

export default function SettingsPage() {
    const { theme: colorTheme, setTheme: setColorTheme } = useThemeStore();
    const { theme: mode, setTheme: setMode } = useTheme();

    return (
        <div className="flex flex-1 overflow-hidden">
            <main className="flex-1 min-w-0 px-6 py-4 overflow-y-auto">
                <div className="max-w-3xl mx-auto space-y-6">
                    <h1 className="text-2xl font-bold">Settings</h1>

                    {/* Dark / Light mode */}
                    <Card className="p-6">
                        <div className="flex items-center justify-between">
                            <div className="flex items-center gap-3">
                                {mode === "dark" ? (
                                    <Moon className="h-5 w-5 text-muted-foreground" />
                                ) : (
                                    <Sun className="h-5 w-5 text-muted-foreground" />
                                )}
                                <div>
                                    <p className="text-base font-medium">Dark Mode</p>
                                    <p className="text-sm text-muted-foreground">
                                        Toggle between light and dark appearance
                                    </p>
                                </div>
                            </div>
                            <Button
                                variant="outline"
                                size="sm"
                                onClick={() =>
                                    setMode(mode === "dark" ? "light" : "dark")
                                }
                            >
                                {mode === "dark" ? (
                                    <><Sun className="h-4 w-4 mr-1" /> Light</>
                                ) : (
                                    <><Moon className="h-4 w-4 mr-1" /> Dark</>
                                )}
                            </Button>
                        </div>
                    </Card>

                    {/* Color theme picker */}
                    <Card className="p-6 space-y-4">
                        <div>
                            <h2 className="text-lg font-semibold">Color Theme</h2>
                            <p className="text-sm text-muted-foreground">
                                Choose a color palette for the interface
                            </p>
                        </div>
                        <div className="grid grid-cols-3 sm:grid-cols-4 md:grid-cols-5 gap-3">
                            {THEMES.map((t) => (
                                <button
                                    key={t.id}
                                    onClick={() => setColorTheme(t.id as ThemeId)}
                                    className={cn(
                                        "flex flex-col items-center gap-2 p-3 rounded-lg border-2 transition-all hover:scale-105",
                                        colorTheme === t.id
                                            ? "border-primary bg-primary/5 shadow-sm"
                                            : "border-border hover:border-primary/30"
                                    )}
                                >
                                    <div
                                        className="w-10 h-10 rounded-full border border-border shadow-sm"
                                        style={{ backgroundColor: t.color }}
                                    />
                                    <span className="text-xs font-medium">{t.label}</span>
                                </button>
                            ))}
                        </div>
                    </Card>
                </div>
            </main>
            <RightSidebar />
        </div>
    );
}
