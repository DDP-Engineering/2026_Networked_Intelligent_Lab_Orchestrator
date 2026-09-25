import type { BenchCoreClient, Credential } from '../api/client';
import type { BenchCoreSession } from '../api/contract';
import { ApiTokensPage } from './authentication/ApiTokensPage';
import { UsersPage } from './authentication/UsersPage';

export type AuthenticationView = 'users' | 'tokens';

export interface AuthenticationPageProps {
  client: BenchCoreClient;
  credential: Credential;
  session: BenchCoreSession;
  view: AuthenticationView;
  canManageUsers?: boolean;
  canManageTokens?: boolean;
  onSessionEnded: () => void;
  onOpenActivity?: () => void;
}

/** Fetch-free dispatcher for the two focused Authentication workspaces. */
export function AuthenticationPage({
  client,
  credential,
  session,
  view,
  canManageUsers = true,
  canManageTokens = true,
  onSessionEnded,
  onOpenActivity,
}: AuthenticationPageProps) {
  return view === 'users' ? (
    <UsersPage
      client={client}
      credential={credential}
      session={session}
      canManage={canManageUsers}
      onSessionEnded={onSessionEnded}
      onOpenActivity={onOpenActivity}
    />
  ) : (
    <ApiTokensPage
      client={client}
      credential={credential}
      session={session}
      canManage={canManageTokens}
      onSessionEnded={onSessionEnded}
      onOpenActivity={onOpenActivity}
    />
  );
}
