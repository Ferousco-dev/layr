// Backend auth error codes mapped to short, non-technical text.
const AUTH_ERRORS = {
  OAUTH_STATE_INVALID: 'That sign-in link expired. Please try again.',
  FIGMA_AUTH_FAILED: 'Figma did not approve the sign-in. Please try again.',
  FIGMA_IDENTITY_FAILED: 'We could not read your Figma profile. Please try again.',
  DEPENDENCY_UNAVAILABLE: 'Layr is temporarily unavailable. Please try again shortly.',
  OAUTH_CALLBACK_FAILED: 'Sign-in did not complete. Please try again.',
}

export const UNREACHABLE = 'We cannot reach Layr right now. Please try again shortly.'
export const LOGOUT_FAILED = 'Could not log out. Please try again.'

export function authErrorMessage(code) {
  if (!code) return ''
  return AUTH_ERRORS[code] || AUTH_ERRORS.OAUTH_CALLBACK_FAILED
}
