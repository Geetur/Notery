// page.tsx — Terms of Service page.
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

export default function TermsPage() {
    return (
        <div className="max-w-3xl mx-auto px-6 py-8">
            <Card>
                <CardHeader>
                    <CardTitle className="text-2xl">Terms of Service</CardTitle>
                    <p className="text-sm text-muted-foreground">Effective Date: March 21, 2026</p>
                </CardHeader>
                <CardContent className="prose prose-sm dark:prose-invert max-w-none space-y-6">
                    <p>
                        By creating an account on Notery (&quot;the Service&quot;), you agree to the
                        following terms. If you do not agree, do not use the Service.
                    </p>

                    <section>
                        <h3 className="text-lg font-semibold">1. Accounts</h3>
                        <p>
                            You are responsible for maintaining the security of your account credentials.
                            You must not share your login details or allow others to access your account.
                            You must be at least 13 years old to create an account.
                        </p>
                    </section>

                    <section>
                        <h3 className="text-lg font-semibold">2. Content Ownership &amp; License</h3>
                        <p>
                            You retain ownership of any content (notes, comments, PDFs) you upload to
                            Notery. By uploading content, you grant Notery a non-exclusive, worldwide,
                            royalty-free license to display, distribute, and store your content as
                            necessary to operate the Service.
                        </p>
                    </section>

                    <section>
                        <h3 className="text-lg font-semibold">3. Prohibited Conduct</h3>
                        <p>You agree not to:</p>
                        <ul className="list-disc pl-6 space-y-1">
                            <li>Upload content you do not have the right to share</li>
                            <li>Post spam, malware, or misleading content</li>
                            <li>Harass, threaten, or impersonate other users</li>
                            <li>Scrape, crawl, or reverse-engineer the Service</li>
                            <li>Attempt to bypass rate limits, access controls, or security measures</li>
                            <li>Use the Service for any unlawful purpose</li>
                        </ul>
                    </section>

                    <section>
                        <h3 className="text-lg font-semibold">4. Payments &amp; Refunds</h3>
                        <p>
                            All purchases of digital notes are final. Payments are processed by Stripe.
                            Notery does not store your payment card details. Creators receive payouts
                            minus applicable platform fees. Notery reserves the right to withhold payouts
                            for content that violates these terms.
                        </p>
                    </section>

                    <section>
                        <h3 className="text-lg font-semibold">5. Content Removal &amp; DMCA</h3>
                        <p>
                            If you believe content on Notery infringes your intellectual property,
                            contact us with a description of the copyrighted work, the infringing
                            material&apos;s location, and your contact information. We will review and
                            remove infringing content in accordance with applicable law.
                        </p>
                    </section>

                    <section>
                        <h3 className="text-lg font-semibold">6. Limitation of Liability</h3>
                        <p>
                            The Service is provided &quot;as is&quot; without warranties of any kind,
                            express or implied. Notery is not liable for any indirect, incidental, or
                            consequential damages arising from your use of the Service. Our total
                            liability to you for any claim is limited to the amount you paid to Notery
                            in the 12 months preceding the claim.
                        </p>
                    </section>

                    <section>
                        <h3 className="text-lg font-semibold">7. Account Termination</h3>
                        <p>
                            You may delete your account at any time. We may suspend or terminate accounts
                            that violate these terms, at our sole discretion, with or without notice.
                            Upon termination, your right to use the Service ceases immediately.
                        </p>
                    </section>

                    <section>
                        <h3 className="text-lg font-semibold">8. Changes to Terms</h3>
                        <p>
                            We may update these terms from time to time. Continued use of the Service
                            after changes constitutes acceptance. Material changes will be communicated
                            via the Service or email.
                        </p>
                    </section>

                    <section>
                        <h3 className="text-lg font-semibold">9. Contact</h3>
                        <p>
                            For questions about these terms, contact us at:{" "}
                            <strong>support@notery.app</strong>
                        </p>
                    </section>
                </CardContent>
            </Card>
        </div>
    );
}
