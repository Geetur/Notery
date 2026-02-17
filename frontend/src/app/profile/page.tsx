// page.tsx — Own profile settings page.
"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import { Loader2, CheckCircle, AlertCircle, Mail } from "lucide-react";
import { useAuthStore } from "@/stores/auth-store";
import { useToast } from "@/hooks/use-toast";
import { updateMyProfile } from "@/services/profile";
import { resendVerification, changePassword } from "@/services/auth";
import type { ProfileVisibility } from "@/types";

export default function ProfilePage() {
  const router = useRouter();
  const { user, isAuthenticated, loading, setUser } = useAuthStore();
  const { toast } = useToast();

  const [displayName, setDisplayName] = useState("");
  const [bio, setBio] = useState("");
  const [visibility, setVisibility] = useState<ProfileVisibility>("public");
  const [saving, setSaving] = useState(false);

  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [changingPassword, setChangingPassword] = useState(false);

  useEffect(() => {
    if (user) {
      setDisplayName(user.display_name || "");
      setBio(user.bio || "");
      setVisibility(user.profile_visibility || "public");
    }
  }, [user]);

  useEffect(() => {
    if (!isAuthenticated && !loading) {
      router.push("/login");
    }
  }, [isAuthenticated, loading, router]);

  if (!isAuthenticated || !user) {
    return null;
  }

  const handleSaveProfile = async () => {
    setSaving(true);
    try {
      const updated = await updateMyProfile({
        display_name: displayName || undefined,
        bio: bio || undefined,
        profile_visibility: visibility,
      });
      setUser(updated);
      toast({ title: "Profile updated" });
    } catch {
      toast({ title: "Failed to update profile", variant: "destructive" });
    } finally {
      setSaving(false);
    }
  };

  const handleChangePassword = async () => {
    if (!currentPassword || !newPassword) return;
    setChangingPassword(true);
    try {
      await changePassword(currentPassword, newPassword);
      setCurrentPassword("");
      setNewPassword("");
      toast({
        title: "Password changed",
        description: "All sessions have been revoked. Please log in again.",
      });
      router.push("/login");
    } catch {
      toast({ title: "Failed to change password", variant: "destructive" });
    } finally {
      setChangingPassword(false);
    }
  };

  const handleResendVerification = async () => {
    try {
      await resendVerification();
      toast({ title: "Verification email sent" });
    } catch {
      toast({ title: "Failed to resend verification", variant: "destructive" });
    }
  };

  return (
    <div className="max-w-2xl mx-auto px-4 py-4 space-y-4">
      <h1 className="text-xl font-bold">Profile Settings</h1>

      {/* Email verification alert */}
      {!user.email_verified && (
        <Card className="border-yellow-500/50 bg-yellow-500/5">
          <CardContent className="flex items-center gap-3 p-3">
            <AlertCircle className="h-5 w-5 text-yellow-500 shrink-0" />
            <div className="flex-1">
              <p className="text-sm font-medium">Email not verified</p>
              <p className="text-xs text-muted-foreground">
                Please verify your email to access all features.
              </p>
            </div>
            <Button size="sm" variant="outline" onClick={handleResendVerification}>
              <Mail className="h-4 w-4 mr-1" />
              Resend
            </Button>
          </CardContent>
        </Card>
      )}

      {/* Profile info */}
      <Card className="border-border">
        <CardHeader>
          <CardTitle className="text-sm">Profile Information</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          {/* Avatar */}
          <div className="flex items-center gap-4">
            <Avatar className="h-16 w-16">
              <AvatarImage src={user.avatar_url} />
              <AvatarFallback className="text-lg">
                {user.username[0]?.toUpperCase()}
              </AvatarFallback>
            </Avatar>
            <div>
              <p className="font-medium">{user.username}</p>
              <p className="text-xs text-muted-foreground">{user.email}</p>
              {user.email_verified && (
                <span className="flex items-center gap-1 text-xs text-green-500">
                  <CheckCircle className="h-3 w-3" /> Verified
                </span>
              )}
            </div>
          </div>

          <Separator />

          {/* Display name */}
          <div>
            <Label htmlFor="displayName">Display Name</Label>
            <Input
              id="displayName"
              value={displayName}
              onChange={(e) => setDisplayName(e.target.value)}
              placeholder="Your display name"
              maxLength={50}
            />
            <p className="text-xs text-muted-foreground mt-1">2-50 characters</p>
          </div>

          {/* Bio */}
          <div>
            <Label htmlFor="bio">Bio</Label>
            <Textarea
              id="bio"
              value={bio}
              onChange={(e) => setBio(e.target.value)}
              placeholder="Tell us about yourself"
              maxLength={500}
              className="resize-none"
            />
            <p className="text-xs text-muted-foreground mt-1">{bio.length}/500</p>
          </div>

          {/* Visibility */}
          <div>
            <Label>Profile Visibility</Label>
            <Select
              value={visibility}
              onValueChange={(v) => setVisibility(v as ProfileVisibility)}
            >
              <SelectTrigger className="w-[180px]">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="public">Public</SelectItem>
                <SelectItem value="private">Private</SelectItem>
              </SelectContent>
            </Select>
          </div>

          <Button onClick={handleSaveProfile} disabled={saving}>
            {saving && <Loader2 className="h-4 w-4 animate-spin mr-2" />}
            Save Changes
          </Button>
        </CardContent>
      </Card>

      {/* Change password */}
      <Card className="border-border">
        <CardHeader>
          <CardTitle className="text-sm">Change Password</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div>
            <Label htmlFor="currentPassword">Current Password</Label>
            <Input
              id="currentPassword"
              type="password"
              value={currentPassword}
              onChange={(e) => setCurrentPassword(e.target.value)}
            />
          </div>
          <div>
            <Label htmlFor="newPassword">New Password</Label>
            <Input
              id="newPassword"
              type="password"
              value={newPassword}
              onChange={(e) => setNewPassword(e.target.value)}
              minLength={8}
            />
            <p className="text-xs text-muted-foreground mt-1">Minimum 8 characters</p>
          </div>
          <Button
            onClick={handleChangePassword}
            disabled={changingPassword || !currentPassword || !newPassword}
            variant="outline"
          >
            {changingPassword && <Loader2 className="h-4 w-4 animate-spin mr-2" />}
            Change Password
          </Button>
        </CardContent>
      </Card>
    </div>
  );
}
