import { AuthCard } from "@/app/components/account/AccountUI";
import { ForgotForm } from "@/app/components/auth/ForgotForm";

export const metadata = { title: "Reset your password" };

export default function ForgotPasswordPage() {
  return (
    <AuthCard
      title="Reset your password"
      description="Enter the email tied to your account and we'll send a link to choose a new one."
    >
      <ForgotForm />
    </AuthCard>
  );
}
