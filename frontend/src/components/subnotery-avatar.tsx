// subnotery-avatar.tsx — Circular avatar for subnotery profile pictures.
// Falls back to the Notery logo when no profile picture is set.
"use client";

import { API_BASE_URL } from "@/lib/config";
import Image from "next/image";

const sizes = {
    sm: 20,
    md: 40,
    lg: 64,
} as const;

interface SubnoteryAvatarProps {
    subnoteryId: number;
    profilePictureUrl?: string;
    name?: string;
    size?: keyof typeof sizes;
    className?: string;
}

export function SubnoteryAvatar({
    subnoteryId,
    profilePictureUrl,
    size = "sm",
    className = "",
}: SubnoteryAvatarProps) {
    const px = sizes[size];

    if (profilePictureUrl) {
        return (
            // eslint-disable-next-line @next/next/no-img-element
            <img
                src={`${API_BASE_URL}/api/v1/subnoteries/${subnoteryId}/profile-picture?v=${encodeURIComponent(profilePictureUrl)}`}
                alt="Community avatar"
                width={px}
                height={px}
                className={`rounded-full object-cover shrink-0 ${className}`}
                style={{ width: px, height: px }}
            />
        );
    }

    // Fallback: Notery logo in a neutral circle
    return (
        <div
            className={`rounded-full bg-muted flex items-center justify-center shrink-0 ${className}`}
            style={{ width: px, height: px }}
        >
            <Image
                src="/notery-logo.png"
                alt="Community"
                width={Math.round(px * 0.6)}
                height={Math.round(px * 0.6)}
                className="opacity-60"
            />
        </div>
    );
}
