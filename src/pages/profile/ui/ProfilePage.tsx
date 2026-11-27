import { ProfileForm } from '@/features/profile-edit';
import { PasswordForm } from '@/features/password-change';

export default function ProfilePage() {
  return (
    <div className="page stack" style={{ maxWidth: 560 }}>
      <ProfileForm />
      <PasswordForm />
    </div>
  );
}
