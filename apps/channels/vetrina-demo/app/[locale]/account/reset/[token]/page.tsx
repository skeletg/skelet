import { AuthCard } from "@/app/components/account/AccountUI";
import { ResetForm } from "./ResetForm";

export const metadata = {
  title: "Choose a new password",
  // The token-bearing URL must never end up in a search index
  robots: { index: false, follow: false },
};

export default async function ResetPasswordPage(
  props: PageProps<"/[locale]/account/reset/[token]">,
) {
  const { token } = await props.params;
  return (
    // lens-block keeps the whole reset surface out of session replay + DOM snapshots
    <div className="lens-block">
      <AuthCard
        title="Choose a new password"
        description="At least 8 characters. You'll sign in with the new one afterwards."
      >
        <ResetForm token={token} />
      </AuthCard>
    </div>
  );
}
