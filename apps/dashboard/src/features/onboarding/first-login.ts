// The handoff from the login form to the shell: the login mutation answers firstLogin exactly once
// per account, and the dashboard mounts long after that response is gone
//
// Deliberately NOT part of AuthState - performRefresh in lib/apollo.ts replaces the whole stored
// auth object with the refresh payload, which would silently wipe a flag parked there 15 minutes in
const KEY = 'skelet-dash:welcome'

export function markFirstLogin(): void {
  try {
    localStorage.setItem(KEY, '1')
  } catch {
    /* private mode - the admin just doesn't get the introduction */
  }
}

// Split from the clear on purpose: peeking is a pure read, safe to do while rendering, and the
// clearing is the side effect that belongs in an effect. Read-and-clear in one call would have to
// happen somewhere - during render it double-fires under StrictMode, in an effect it means setting
// state synchronously, which the React Compiler rejects
export function peekFirstLogin(): boolean {
  try {
    return localStorage.getItem(KEY) === '1'
  } catch {
    return false
  }
}

// Idempotent, so running twice (StrictMode, a remount) costs nothing
export function clearFirstLogin(): void {
  try {
    localStorage.removeItem(KEY)
  } catch {
    /* nothing to clear */
  }
}
