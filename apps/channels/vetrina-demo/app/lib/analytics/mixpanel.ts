// Mixpanel session replay and heatmaps in the browser - optional, off until the shop sets a project token, and only for
// a visitor who allowed analytics, since its cookie and recordings are analytics storage. It sends no events: shop
// events reach Mixpanel server to server, and a browser copy would count everything twice. Without analytics consent
// nothing here loads or sends anything; a withdrawal stops the recording and deletes Mixpanel's cookie

import type { Config, Mixpanel } from "mixpanel-browser";

export type MixpanelReplayConfig = { token: string; region: string; percent: number };

export type ReplayLib = Pick<Mixpanel, "init" | "identify" | "reset" | "opt_out_tracking" | "stop_session_recording">;

// The browser library's ingestion host per data residency - it must match the project's, as the server destination's does
const API_HOSTS: Record<string, string> = {
  us: "https://api-js.mixpanel.com",
  eu: "https://api-eu.mixpanel.com",
  in: "https://api-in.mixpanel.com",
};
export const MIXPANEL_API_HOSTS = Object.values(API_HOSTS);

// Mixpanel's own key for its opt-out flag. Our consent cookie is the record of the choice, so a grant clears it
const OPT_OUT_KEY = "__mp_opt_in_out_";

const TOKEN = /^[0-9a-f]{32}$/;

// Its persistence cookie, mp_<token>_mixpanel - deleted by the proxy for any visitor without analytics consent
export const isMixpanelCookie = (name: string) => /^mp_[0-9a-f]{32}_mixpanel$/.test(name);

export function validToken(token: string): boolean {
  return TOKEN.test(token);
}

export function initOptions(cfg: MixpanelReplayConfig): Partial<Config> {
  return {
    api_host: API_HOSTS[cfg.region] ?? API_HOSTS.us,
    autocapture: false, // no click or page events - heatmaps come from the recordings
    track_pageview: false,
    stop_utm_persistence: true, // no UTM parameters kept in its cookie
    save_referrer: false,
    store_google: false,
    persistence: "cookie",
    cross_subdomain_cookie: false, // host-only, so the proxy can delete it when analytics is withdrawn on another page
    record_sessions_percent: Math.min(100, Math.max(0, Number.isFinite(cfg.percent) ? cfg.percent : 0)),
    record_heatmap_data: true,
    record_mask_all_inputs: true, // typed text is never recorded - checkout fields included
  };
}

type State = { lib: Promise<ReplayLib> | null; identified: string | null; on: boolean };
const page: State = { lib: null, identified: null, on: false };

const loadLibrary = async (): Promise<ReplayLib> => (await import("mixpanel-browser")).default;

// Brings the page in line with the visitor's choice and who is signed in. Returns whether Mixpanel is running
export async function syncMixpanel(
  cfg: MixpanelReplayConfig,
  analytics: boolean,
  customerId: string | null,
  storage: Pick<Storage, "removeItem"> | null,
  load: () => Promise<ReplayLib> = loadLibrary,
  state: State = page,
): Promise<boolean> {
  if (!analytics || !validToken(cfg.token)) {
    if (state.lib && state.on) {
      const lib = await state.lib;
      lib.stop_session_recording();
      // Clears its cookie and stops sending. delete_user off: the profile is the server destination's, not this page's
      lib.opt_out_tracking({ delete_user: false });
      state.on = false;
      state.identified = null;
    }
    return false;
  }
  if (state.on) {
    const lib = await state.lib!;
    if (customerId && customerId !== state.identified) lib.identify(customerId);
    else if (!customerId && state.identified) lib.reset(); // signed out: a new device id, so the next person isn't joined to them
    state.identified = customerId;
    return true;
  }
  if (state.lib) return false; // withdrawn earlier on this page - recording comes back with the next page load
  storage?.removeItem(OPT_OUT_KEY + cfg.token);
  state.lib = load();
  const lib = await state.lib;
  lib.init(cfg.token, initOptions(cfg));
  state.on = true;
  if (customerId) lib.identify(customerId); // joins the recording to the customer the server destination sends as $user_id
  state.identified = customerId;
  return true;
}
