// top-nav.tsx — Reddit-style top navigation bar.
// Contains logo, search bar, auth controls, user dropdown, and theme toggle.
"use client";

import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import {
    DropdownMenu,
    DropdownMenuContent,
    DropdownMenuItem,
    DropdownMenuSeparator,
    DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { Sheet, SheetContent, SheetTrigger } from "@/components/ui/sheet";
import { useAuthStore } from "@/stores/auth-store";
import {
    ChevronDown,
    FileText,
    LogIn,
    LogOut,
    Menu,
    Palette,
    Plus,
    Search,
    ShoppingCart,
    User,
} from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useCallback, useState } from "react";
import { LeftSidebar } from "./left-sidebar";
import { NotificationBell } from "./notification-bell";
import { SearchAutocomplete } from "./search-autocomplete";

export function TopNav() {
    const router = useRouter();
    const { user, isAuthenticated, logout } = useAuthStore();
    const [searchQuery, setSearchQuery] = useState("");
    const [showAutocomplete, setShowAutocomplete] = useState(false);

    const handleSearch = (e: React.FormEvent) => {
        e.preventDefault();
        if (searchQuery.trim()) {
            setShowAutocomplete(false);
            router.push(`/search?q=${encodeURIComponent(searchQuery.trim())}`);
        }
    };

    const closeAutocomplete = useCallback(() => setShowAutocomplete(false), []);

    const handleLogout = async () => {
        await logout();
        router.push("/");
    };

    return (
        <header className="sticky top-0 z-50 w-full border-b border-border bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
            <div className="flex h-12 items-center gap-2 px-2 md:px-4">
                {/* Mobile menu trigger */}
                <Sheet>
                    <SheetTrigger asChild>
                        <Button variant="ghost" size="icon" className="md:hidden">
                            <Menu className="h-5 w-5" />
                        </Button>
                    </SheetTrigger>
                    <SheetContent side="left" className="w-64 p-0">
                        <LeftSidebar mobile />
                    </SheetContent>
                </Sheet>

                {/* Logo */}
                <Link
                    href="/"
                    className="flex items-center gap-1.5 font-bold text-lg text-foreground hover:text-primary transition-colors mr-2"
                >
                    <span className="hidden sm:inline">notery</span>
                </Link>

                {/* Search bar */}
                <form
                    onSubmit={handleSearch}
                    className="flex-1 max-w-xl mx-auto"
                >
                    <div className="relative">
                        <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
                        <Input
                            type="search"
                            placeholder="Search Notery"
                            className="pl-8 h-9 bg-muted/50 border-none focus-visible:ring-1 focus-visible:ring-primary rounded-full"
                            value={searchQuery}
                            onChange={(e) => {
                                setSearchQuery(e.target.value);
                                setShowAutocomplete(e.target.value.trim().length > 0);
                            }}
                            onFocus={() => {
                                if (searchQuery.trim()) setShowAutocomplete(true);
                            }}
                        />
                        <SearchAutocomplete
                            query={searchQuery}
                            visible={showAutocomplete}
                            onClose={closeAutocomplete}
                        />
                    </div>
                </form>

                {/* Right-side controls */}
                <div className="flex items-center gap-1">
                    {/* Theme settings */}
                    <Button
                        variant="ghost"
                        size="icon"
                        className="h-9 w-9"
                        onClick={() => router.push("/settings")}
                    >
                        <Palette className="h-4 w-4" />
                    </Button>

                    {isAuthenticated && user ? (
                        <>
                            {/* Create note */}
                            <Button
                                variant="ghost"
                                size="icon"
                                className="h-9 w-9"
                                onClick={() => router.push("/submit")}
                            >
                                <Plus className="h-5 w-5" />
                            </Button>

                            {/* Cart */}
                            <Button
                                variant="ghost"
                                size="icon"
                                className="h-9 w-9"
                                onClick={() => router.push("/cart")}
                            >
                                <ShoppingCart className="h-4 w-4" />
                            </Button>

                            {/* Notifications */}
                            <NotificationBell />

                            {/* User dropdown */}
                            <DropdownMenu>
                                <DropdownMenuTrigger asChild>
                                    <Button
                                        variant="ghost"
                                        className="h-9 gap-1.5 px-2"
                                    >
                                        <Avatar className="h-6 w-6">
                                            <AvatarImage src={user.avatar_url} />
                                            <AvatarFallback className="text-xs">
                                                {user.username[0]?.toUpperCase()}
                                            </AvatarFallback>
                                        </Avatar>
                                        <span className="hidden md:inline text-sm font-medium max-w-[100px] truncate">
                                            {user.username}
                                        </span>
                                        <ChevronDown className="h-3 w-3 text-muted-foreground" />
                                    </Button>
                                </DropdownMenuTrigger>
                                <DropdownMenuContent align="end" className="w-48">
                                    <DropdownMenuItem
                                        onClick={() => router.push("/profile")}
                                    >
                                        <User className="h-4 w-4 mr-2" />
                                        My Profile
                                    </DropdownMenuItem>
                                    <DropdownMenuItem
                                        onClick={() => router.push("/purchases")}
                                    >
                                        <FileText className="h-4 w-4 mr-2" />
                                        My Purchases
                                    </DropdownMenuItem>
                                    <DropdownMenuItem
                                        onClick={() => router.push("/cart")}
                                    >
                                        <ShoppingCart className="h-4 w-4 mr-2" />
                                        Cart
                                    </DropdownMenuItem>
                                    <DropdownMenuSeparator />
                                    <DropdownMenuItem onClick={handleLogout}>
                                        <LogOut className="h-4 w-4 mr-2" />
                                        Log Out
                                    </DropdownMenuItem>
                                </DropdownMenuContent>
                            </DropdownMenu>
                        </>
                    ) : (
                        <div className="flex items-center gap-1.5">
                            <Button
                                variant="outline"
                                size="sm"
                                className="h-8"
                                onClick={() => router.push("/login")}
                            >
                                <LogIn className="h-4 w-4 mr-1.5" />
                                Log In
                            </Button>
                            <Button
                                size="sm"
                                className="h-8"
                                onClick={() => router.push("/signup")}
                            >
                                Sign Up
                            </Button>
                        </div>
                    )}
                </div>
            </div>
        </header>
    );
}
