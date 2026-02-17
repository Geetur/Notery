// page.tsx — Forgot password page.
"use client";

import { useState, type FormEvent } from "react";
import Link from "next/link";
import { ArrowLeft, Mail, Loader2, CheckCircle } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { forgotPassword } from "@/services/auth";

export default function ForgotPasswordPage() {
  const [email, setEmail] = useState("");
  const [loading, setLoading] = useState(false);
  const [sent, setSent] = useState(false);

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    if (!email.trim()) return;
    setLoading(true);
    try {
      await forgotPassword(email.trim());
      setSent(true);
    } catch {
      // Anti-enumeration: the API always returns 200, so we still show success
      setSent(true);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="max-w-sm mx-auto px-4 py-8">
      <Button variant="ghost" size="sm" className="mb-3 -ml-2 text-muted-foreground" asChild>
        <Link href="/login">
          <ArrowLeft className="h-4 w-4 mr-1" /> Back to Login
        </Link>
      </Button>

      <Card className="border-border">
        <CardHeader className="pb-3 text-center">
          <Mail className="h-10 w-10 text-primary mx-auto mb-2" />
          <CardTitle className="text-lg">Reset your password</CardTitle>
          <p className="text-sm text-muted-foreground mt-1">
            Enter your email and we&apos;ll send a password reset link.
          </p>
        </CardHeader>
        <CardContent>
          {sent ? (
            <div className="text-center py-4 space-y-3">
              <CheckCircle className="h-10 w-10 text-green-500 mx-auto" />
              <p className="text-sm">
                If an account exists with <strong>{email}</strong>, you&apos;ll
                receive a password reset email shortly.
              </p>
              <p className="text-xs text-muted-foreground">
                Check your spam folder if you don&apos;t see it.
              </p>
              <Button variant="outline" size="sm" asChild className="mt-2">
                <Link href="/login">Return to Login</Link>
              </Button>
            </div>
          ) : (
            <form onSubmit={handleSubmit} className="space-y-4">
              <div className="space-y-1.5">
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
              <Button type="submit" className="w-full" disabled={loading}>
                {loading && <Loader2 className="h-4 w-4 animate-spin mr-2" />}
                {loading ? "Sending..." : "Send Reset Link"}
              </Button>
            </form>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
